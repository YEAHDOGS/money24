package cloudguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	// Google's free tier: one non-preemptible e2-micro in these regions, 30 GB of standard persistent disk.
	FREE_MACHINE_TYPE = "e2-micro"
	FREE_DISK_TYPE    = "pd-standard"
	FREE_DISK_GB      = 30
	MAX_DISKS         = 1
	METADATA_URL      = "http://169.254.169.254/computeMetadata/v1/"
	COMPUTE_URL       = "https://compute.googleapis.com/compute/v1/"
	SELF_LINK_URL     = "https://www.googleapis.com/compute/v1/" // how the API writes disk links
	METADATA_HEADER   = "Metadata-Flavor"
	METADATA_FLAVOR   = "Google"
	REQUEST_TIMEOUT   = 10 * time.Second
	MAX_RESPONSE      = 1 << 20
	PERSISTENT_DISK   = "PERSISTENT"
	TRUE_VALUE        = "TRUE"
	MACHINE_PATH      = "instance/machine-type"
	ZONE_PATH         = "instance/zone"
	NAME_PATH         = "instance/name"
	PROJECT_PATH      = "project/project-id"
	PREEMPTIBLE_PATH  = "instance/scheduling/preemptible"
	NICS_PATH         = "instance/network-interfaces/?recursive=true"
	DISKS_PATH        = "instance/disks/?recursive=true"
	TOKEN_PATH        = "instance/service-accounts/default/token"
)

// FREE_REGIONS are the only regions where Google's free tier covers an e2-micro.
var FREE_REGIONS = []string{"us-west1", "us-central1", "us-east1"}

// Facts describe the running instance, as far as billing is concerned.
type Facts struct {
	MachineType  string   // e2-micro
	Zone         string   // us-west1-b
	Preemptible  bool     // Spot and preemptible machines are not in the free tier
	ExternalIPv4 []string // every one-to-one NAT address; Google bills each by the hour
	DiskKinds    []string // attached disks as the metadata server lists them
	DiskChecked  bool     // the fields below came from the Compute API on this run
	DiskTypes    []string // pd-standard
	DiskSizesGB  []int64  // 30
}

// Inspector reads instance facts from the metadata server and, for disk types, the Compute API.
type Inspector struct {
	MetadataURL string
	ComputeURL  string
	Client      *http.Client
}

// NewInspector talks to the real metadata server and Compute API.
func NewInspector() Inspector {
	return Inspector{MetadataURL: METADATA_URL, ComputeURL: COMPUTE_URL, Client: &http.Client{Timeout: REQUEST_TIMEOUT}}
}

func (in Inspector) fetch(target string, header http.Header) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header = header
	response, err := in.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, MAX_RESPONSE))
}

func (in Inspector) metadata(name string) (string, error) {
	header := http.Header{METADATA_HEADER: {METADATA_FLAVOR}}
	raw, err := in.fetch(in.MetadataURL+name, header)
	if err != nil {
		return "", fmt.Errorf("metadata %s: %w", name, err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// Instance reads everything the metadata server knows. It needs no credentials.
func (in Inspector) Instance() (Facts, error) {
	var facts Facts
	values := map[string]*string{MACHINE_PATH: &facts.MachineType, ZONE_PATH: &facts.Zone}
	for name, target := range values {
		value, err := in.metadata(name)
		if err != nil {
			return facts, err
		}
		*target = path.Base(value)
	}
	preemptible, err := in.metadata(PREEMPTIBLE_PATH)
	if err != nil {
		return facts, err
	}
	facts.Preemptible = strings.EqualFold(preemptible, TRUE_VALUE)
	rawNICs, err := in.metadata(NICS_PATH)
	if err != nil {
		return facts, err
	}
	var nics []struct {
		AccessConfigs []struct {
			ExternalIP string `json:"externalIp"`
		} `json:"accessConfigs"`
	}
	if err := json.Unmarshal([]byte(rawNICs), &nics); err != nil {
		return facts, errors.New("metadata network interfaces are not JSON")
	}
	for _, nic := range nics {
		for _, config := range nic.AccessConfigs {
			if config.ExternalIP != "" {
				facts.ExternalIPv4 = append(facts.ExternalIPv4, config.ExternalIP)
			}
		}
	}
	rawDisks, err := in.metadata(DISKS_PATH)
	if err != nil {
		return facts, err
	}
	var disks []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(rawDisks), &disks); err != nil {
		return facts, errors.New("metadata disks are not JSON")
	}
	for _, disk := range disks {
		facts.DiskKinds = append(facts.DiskKinds, disk.Type)
	}
	return facts, nil
}

// Disks asks the Compute API for each attached disk's type and size, using the
// instance's own read-only service account. The metadata server does not know them.
func (in Inspector) Disks(facts *Facts) error {
	project, err := in.metadata(PROJECT_PATH)
	if err != nil {
		return err
	}
	name, err := in.metadata(NAME_PATH)
	if err != nil {
		return err
	}
	rawToken, err := in.metadata(TOKEN_PATH)
	if err != nil {
		return fmt.Errorf("the instance needs its read-only service account: %w", err)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(rawToken), &token); err != nil || token.AccessToken == "" {
		return errors.New("metadata returned no access token")
	}
	header := http.Header{"Authorization": {"Bearer " + token.AccessToken}}
	instanceURL := in.ComputeURL + "projects/" + url.PathEscape(project) + "/zones/" +
		url.PathEscape(facts.Zone) + "/instances/" + url.PathEscape(name)
	raw, err := in.fetch(instanceURL, header)
	if err != nil {
		return fmt.Errorf("compute instance: %w", err)
	}
	var instance struct {
		Disks []struct {
			Source string `json:"source"`
		} `json:"disks"`
	}
	if err := json.Unmarshal(raw, &instance); err != nil {
		return errors.New("compute instance is not JSON")
	}
	facts.DiskTypes, facts.DiskSizesGB = nil, nil
	for _, attached := range instance.Disks {
		// The token goes only to the configured API host, never to a link the response chose.
		rest, ok := strings.CutPrefix(attached.Source, SELF_LINK_URL)
		if !ok {
			rest, ok = strings.CutPrefix(attached.Source, in.ComputeURL)
		}
		if !ok {
			return errors.New("compute disk source is outside the Compute API")
		}
		raw, err := in.fetch(in.ComputeURL+rest, header)
		if err != nil {
			return fmt.Errorf("compute disk: %w", err)
		}
		var disk struct {
			Type   string      `json:"type"`
			SizeGB json.Number `json:"sizeGb"`
		}
		if err := json.Unmarshal(raw, &disk); err != nil {
			return errors.New("compute disk is not JSON")
		}
		size, err := disk.SizeGB.Int64()
		if err != nil {
			return errors.New("compute disk has no size")
		}
		facts.DiskTypes = append(facts.DiskTypes, path.Base(disk.Type))
		facts.DiskSizesGB = append(facts.DiskSizesGB, size)
	}
	facts.DiskChecked = true
	return nil
}

// FreeRegion reports whether a zone such as us-west1-b lies in a free-tier region.
func FreeRegion(zone string) bool {
	cut := strings.LastIndex(zone, "-")
	if cut <= 0 {
		return false
	}
	for _, region := range FREE_REGIONS {
		if zone[:cut] == region {
			return true
		}
	}
	return false
}

// Violations lists every way the machine would be billed. Empty means free.
func Violations(facts Facts) []string {
	return append(machineViolations(facts), diskViolations(facts)...)
}

// machineViolations checks what the metadata server reports on every run.
func machineViolations(facts Facts) []string {
	var found []string
	if facts.MachineType != FREE_MACHINE_TYPE {
		found = append(found, fmt.Sprintf("machine type %s is not %s", facts.MachineType, FREE_MACHINE_TYPE))
	}
	if !FreeRegion(facts.Zone) {
		found = append(found, fmt.Sprintf("zone %s is outside %s", facts.Zone, strings.Join(FREE_REGIONS, ", ")))
	}
	if facts.Preemptible {
		found = append(found, "Spot or preemptible machines are not in the free tier")
	}
	if len(facts.ExternalIPv4) > 0 {
		found = append(found, fmt.Sprintf("%d external IPv4 address(es) attached; each is billed hourly", len(facts.ExternalIPv4)))
	}
	if len(facts.DiskKinds) > MAX_DISKS {
		found = append(found, fmt.Sprintf("%d disks attached; only the boot disk is free", len(facts.DiskKinds)))
	}
	for _, kind := range facts.DiskKinds {
		if kind != PERSISTENT_DISK {
			found = append(found, fmt.Sprintf("a %s disk is attached; local SSD is billed", kind))
		}
	}
	return found
}

// diskViolations checks the disk types and sizes the Compute API reported, when it was asked.
func diskViolations(facts Facts) []string {
	var found []string
	if !facts.DiskChecked {
		return found
	}
	var total int64
	for i, kind := range facts.DiskTypes {
		total += facts.DiskSizesGB[i]
		if kind != FREE_DISK_TYPE {
			found = append(found, fmt.Sprintf("disk type %s is not %s", kind, FREE_DISK_TYPE))
		}
	}
	if total > FREE_DISK_GB {
		found = append(found, fmt.Sprintf("%d GB of disk is over the free %d GB", total, FREE_DISK_GB))
	}
	return found
}

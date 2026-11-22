package cloudguard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	TEST_BOOT     = "boot-one"
	TEST_REBOOT   = "boot-two"
	TEST_NIC      = "ens4"
	TEST_LIMIT    = 800 * MEGABYTE
	TEST_TOKEN    = "test-token"
	TEST_PROJECT  = "money24-test"
	TEST_INSTANCE = "money24"
)

func at(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func reading(boot string, counter int64, when time.Time) Sample {
	return Sample{BootID: boot, Counters: map[string]int64{TEST_NIC: counter}, At: when}
}

func freeFacts() Facts {
	return Facts{MachineType: FREE_MACHINE_TYPE, Zone: "us-west1-b", DiskKinds: []string{PERSISTENT_DISK},
		DiskChecked: true, DiskTypes: []string{FREE_DISK_TYPE}, DiskSizesGB: []int64{FREE_DISK_GB}}
}

func TestAdvanceCountsAcrossRebootsAndMonths(t *testing.T) {
	noon := at(t, "2026-11-20T20:00:00Z")
	state, err := Advance(State{}, reading(TEST_BOOT, 1000, noon))
	if err != nil || state.TxBytes != 1000 || state.Month != "2026-11" {
		t.Fatalf("first sample counts everything since boot: %+v %v", state, err)
	}
	state, _ = Advance(state, reading(TEST_BOOT, 1500, noon.Add(5*time.Minute)))
	if state.TxBytes != 1500 {
		t.Fatalf("same boot adds the difference, got %d", state.TxBytes)
	}
	state, _ = Advance(state, reading(TEST_REBOOT, 200, noon.Add(time.Hour)))
	if state.TxBytes != 1700 {
		t.Fatalf("a reboot restarts the kernel counter; the ledger keeps the total, got %d", state.TxBytes)
	}
	state, _ = Advance(state, reading(TEST_REBOOT, 50, noon.Add(2*time.Hour)))
	if state.TxBytes != 1750 {
		t.Fatalf("a counter that went backwards is a reset, got %d", state.TxBytes)
	}
	state.Counters = map[string]int64{TEST_NIC: 50}
	both := Sample{BootID: TEST_REBOOT, Counters: map[string]int64{TEST_NIC: 60, "ens5": 7}, At: noon.Add(3 * time.Hour)}
	state, _ = Advance(state, both)
	if state.TxBytes != 1767 {
		t.Fatalf("a new interface counts in full, got %d", state.TxBytes)
	}
	// Midnight UTC on December 1 is still November in Pacific time, where Google bills.
	state, _ = Advance(state, Sample{BootID: TEST_REBOOT, Counters: both.Counters, At: at(t, "2026-12-01T07:30:00Z")})
	if state.Month != "2026-11" || state.TxBytes != 1767 {
		t.Fatalf("UTC midnight is not the billing month turn: %+v", state)
	}
	state, _ = Advance(state, Sample{BootID: TEST_REBOOT, Counters: map[string]int64{TEST_NIC: 90, "ens5": 7}, At: at(t, "2026-12-01T08:30:00Z")})
	if state.Month != "2026-12" || state.TxBytes != 30 {
		t.Fatalf("the Pacific month turn starts a new total: %+v", state)
	}
}

func TestReadSampleMetersPhysicalInterfacesOnly(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(BOOT_ID_FILE, TEST_BOOT+"\n")
	write(SYS_NET+"/ens4/"+TX_COUNTER, "4096\n")
	write(SYS_NET+"/ens4/"+DEVICE_LINK+"/vendor", "0x1af4")
	write(SYS_NET+"/lo/"+TX_COUNTER, "999999")
	write(SYS_NET+"/CloudflareWARP/"+TX_COUNTER, "888888") // the WARP tunnel rides on ens4 and must not count twice
	sample, err := ReadSample(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if sample.BootID != TEST_BOOT || len(sample.Counters) != 1 || sample.Counters[TEST_NIC] != 4096 {
		t.Fatalf("unexpected sample %+v", sample)
	}
	write(SYS_NET+"/ens4/"+TX_COUNTER, "garbage")
	if _, err := ReadSample(root, time.Now()); err == nil {
		t.Fatal("a corrupt counter must fail, not count as zero")
	}
}

func TestLedgerSurvivesRestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "guard", "state.json")
	empty, err := LoadState(file)
	if err != nil || empty.TxBytes != 0 {
		t.Fatalf("a missing ledger is empty: %+v %v", empty, err)
	}
	state, _ := Advance(State{}, reading(TEST_BOOT, 12345, at(t, "2026-11-20T20:00:00Z")))
	if err := SaveState(file, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadState(file)
	if err != nil || loaded.TxBytes != 12345 || loaded.BootID != TEST_BOOT || loaded.Counters[TEST_NIC] != 12345 {
		t.Fatalf("ledger did not round-trip: %+v %v", loaded, err)
	}
	if err := os.WriteFile(file, []byte("{"), STATE_MODE); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(file); err == nil {
		t.Fatal("a corrupt ledger must fail closed")
	}
}

func TestStepStopsAtTheLimitAndResumesNextMonth(t *testing.T) {
	when := at(t, "2026-11-20T20:00:00Z")
	in := Input{Sample: reading(TEST_BOOT, TEST_LIMIT-1, when), LimitBytes: TEST_LIMIT, Facts: freeFacts()}
	state, verdict, err := Step(State{}, in)
	if err != nil || verdict.Action != RUN || verdict.Resume {
		t.Fatalf("under the limit runs: %+v %v", verdict, err)
	}
	in.Sample = reading(TEST_BOOT, TEST_LIMIT, when.Add(5*time.Minute))
	state, verdict, _ = Step(state, in)
	if verdict.Action != STOP || !state.Blocked || verdict.PowerOff || !strings.Contains(verdict.Reason, "800 MB") {
		t.Fatalf("the limit stops the services without powering off: %+v %+v", verdict, state)
	}
	in.Starting = true
	in.Sample = reading(TEST_REBOOT, 10, when.Add(time.Hour))
	state, verdict, _ = Step(state, in)
	if verdict.Action != STOP {
		t.Fatalf("a reboot does not reset the month: %+v", verdict)
	}
	in.Starting = false
	in.Sample = reading(TEST_REBOOT, 20, at(t, "2026-12-01T09:00:00Z"))
	state, verdict, _ = Step(state, in)
	if verdict.Action != RUN || !verdict.Resume || state.Blocked {
		t.Fatalf("a new billing month resumes the services: %+v %+v", verdict, state)
	}
	_, verdict, _ = Step(state, in)
	if verdict.Resume {
		t.Fatal("resume happens once")
	}
}

func TestStepRefusesBillableMachines(t *testing.T) {
	when := at(t, "2026-11-20T20:00:00Z")
	billable := freeFacts()
	billable.MachineType = "e2-small"
	in := Input{Sample: reading(TEST_BOOT, 1, when), LimitBytes: TEST_LIMIT, Facts: billable}
	state, verdict, _ := Step(State{}, in)
	if verdict.Action != STOP || verdict.PowerOff || state.Strikes != 1 {
		t.Fatalf("one sighting stops the services only: %+v", verdict)
	}
	in.Sample = reading(TEST_BOOT, 2, when.Add(5*time.Minute))
	state, verdict, _ = Step(state, in)
	if !verdict.PowerOff {
		t.Fatalf("a second sighting powers the machine off: %+v", verdict)
	}
	in.Facts = freeFacts()
	in.Sample = reading(TEST_BOOT, 3, when.Add(10*time.Minute))
	state, verdict, _ = Step(state, in)
	if verdict.Action != RUN || !verdict.Resume || state.Strikes != 0 {
		t.Fatalf("a fixed machine runs again: %+v %+v", verdict, state)
	}
}

func TestStepHoldsWhenFactsAreUnknown(t *testing.T) {
	when := at(t, "2026-11-20T20:00:00Z")
	in := Input{Sample: reading(TEST_BOOT, 1, when), LimitBytes: TEST_LIMIT, FactsErr: errors.New("metadata down")}
	state, verdict, _ := Step(State{}, in)
	if verdict.Action != HOLD || state.Blocked {
		t.Fatalf("an unreadable metadata server leaves running services alone: %+v", verdict)
	}
	in.Starting = true
	state, verdict, _ = Step(state, in)
	if verdict.Action != HOLD || !state.Blocked {
		t.Fatalf("but nothing starts without proof: %+v %+v", verdict, state)
	}
	in = Input{Sample: reading(TEST_BOOT, 2, when.Add(time.Minute)), LimitBytes: TEST_LIMIT, Facts: freeFacts(), Starting: true}
	in.Facts.DiskChecked = false
	_, verdict, _ = Step(State{}, in)
	if verdict.Action != HOLD {
		t.Fatalf("starting needs a disk check on this boot: %+v", verdict)
	}
	in.Facts.DiskChecked = true
	state, verdict, _ = Step(state, in)
	if verdict.Action != RUN || !verdict.Resume || state.DiskBootID != TEST_BOOT {
		t.Fatalf("proof arrives, services may run: %+v %+v", verdict, state)
	}
	if DiskCheckDue(state, reading(TEST_BOOT, 3, when.Add(30*time.Minute))) {
		t.Fatal("the disk check is cached within the hour")
	}
	if !DiskCheckDue(state, reading(TEST_BOOT, 3, when.Add(2*time.Hour))) || !DiskCheckDue(state, reading(TEST_REBOOT, 3, when)) {
		t.Fatal("the disk check repeats hourly and on every boot")
	}
}

func TestViolations(t *testing.T) {
	if found := Violations(freeFacts()); len(found) != 0 {
		t.Fatalf("free machine flagged: %v", found)
	}
	cases := map[string]func(*Facts){
		"machine type": func(f *Facts) { f.MachineType = "e2-medium" },
		"zone":         func(f *Facts) { f.Zone = "us-west2-a" },
		"Spot":         func(f *Facts) { f.Preemptible = true },
		"IPv4":         func(f *Facts) { f.ExternalIPv4 = []string{"203.0.113.9"} },
		"disks":        func(f *Facts) { f.DiskKinds = append(f.DiskKinds, PERSISTENT_DISK) },
		"local SSD":    func(f *Facts) { f.DiskKinds = []string{"SCRATCH"} },
		"disk type":    func(f *Facts) { f.DiskTypes = []string{"pd-balanced"} },
		"GB of disk":   func(f *Facts) { f.DiskSizesGB = []int64{31} },
	}
	for want, change := range cases {
		facts := freeFacts()
		change(&facts)
		if found := strings.Join(Violations(facts), "; "); !strings.Contains(found, want) {
			t.Errorf("%s not flagged: %q", want, found)
		}
	}
	for zone, free := range map[string]bool{"us-central1-a": true, "us-east1-d": true, "us-east4-a": false, "us-east1": false, "": false} {
		if FreeRegion(zone) != free {
			t.Errorf("FreeRegion(%q) != %v", zone, free)
		}
	}
}

func TestBackupRoom(t *testing.T) {
	now := at(t, "2026-11-20T20:00:00Z")
	if _, err := BackupRoom(State{}, now); err == nil {
		t.Fatal("no ledger, no backup traffic")
	}
	state := State{Month: "2026-11", TxBytes: 500 * MEGABYTE, LimitBytes: TEST_LIMIT}
	if room, _ := BackupRoom(state, now); room != 100*MEGABYTE {
		t.Fatalf("room = %d", room)
	}
	state.TxBytes = 700 * MEGABYTE
	if room, _ := BackupRoom(state, now); room != 0 {
		t.Fatalf("room past the share = %d", room)
	}
	if room, _ := BackupRoom(state, at(t, "2026-12-02T20:00:00Z")); room != 600*MEGABYTE {
		t.Fatalf("a stale month is empty, room = %d", room)
	}
}

func TestInspectorReadsMetadataAndComputeAPI(t *testing.T) {
	var compute *httptest.Server
	compute = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+TEST_TOKEN {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/projects/" + TEST_PROJECT + "/zones/us-east1-b/instances/" + TEST_INSTANCE:
			w.Write([]byte(`{"disks":[{"source":"` + SELF_LINK_URL + `projects/` + TEST_PROJECT + `/zones/us-east1-b/disks/boot"}]}`))
		case "/projects/" + TEST_PROJECT + "/zones/us-east1-b/disks/boot":
			w.Write([]byte(`{"type":"` + SELF_LINK_URL + `projects/p/zones/us-east1-b/diskTypes/pd-standard","sizeGb":"30"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer compute.Close()
	answers := map[string]string{
		MACHINE_PATH:     "projects/1/machineTypes/e2-micro",
		ZONE_PATH:        "projects/1/zones/us-east1-b",
		PREEMPTIBLE_PATH: "FALSE",
		NICS_PATH:        `[{"accessConfigs":[],"ipv6s":["2600:1900::1"]}]`,
		DISKS_PATH:       `[{"deviceName":"persistent-disk-0","type":"PERSISTENT"}]`,
		PROJECT_PATH:     TEST_PROJECT,
		NAME_PATH:        TEST_INSTANCE,
		TOKEN_PATH:       `{"access_token":"` + TEST_TOKEN + `"}`,
	}
	metadata := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answer, ok := answers[strings.TrimPrefix(r.URL.RequestURI(), "/")]
		if r.Header.Get(METADATA_HEADER) != METADATA_FLAVOR || !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(answer))
	}))
	defer metadata.Close()
	in := Inspector{MetadataURL: metadata.URL + "/", ComputeURL: compute.URL + "/", Client: http.DefaultClient}
	facts, err := in.Instance()
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Disks(&facts); err != nil {
		t.Fatal(err)
	}
	if found := Violations(facts); len(found) != 0 || !facts.DiskChecked {
		t.Fatalf("free machine flagged: %v %+v", found, facts)
	}
	answers[NICS_PATH] = `[{"accessConfigs":[{"externalIp":"203.0.113.9","type":"ONE_TO_ONE_NAT"}]}]`
	facts, _ = in.Instance()
	if len(facts.ExternalIPv4) != 1 {
		t.Fatalf("external IPv4 not seen: %+v", facts)
	}
	answers[DISKS_PATH] = `not json`
	if _, err := in.Instance(); err == nil {
		t.Fatal("bad metadata must fail")
	}
	delete(answers, TOKEN_PATH)
	facts = Facts{Zone: "us-east1-b"}
	if err := in.Disks(&facts); err == nil || facts.DiskChecked {
		t.Fatal("no service account, no disk proof")
	}
}

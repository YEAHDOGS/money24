package calendar

import (
	_ "embed"
	"encoding/json"
)

//go:embed strings.json
var stringsJSON []byte

// tagText is one tag's wording. Label is the header chip when the tag leads the day; only
// structural tags have one.
type tagText struct {
	Name  string `json:"name"`
	Note  string `json:"note"`
	Label string `json:"label"`
}

// wording is strings.json: every word the calendar shows, kept out of the logic so it can be
// edited and translated on its own.
type wording struct {
	NormalLabel string                       `json:"normal_label"`
	Holidays    map[string]string            `json:"holidays"`
	Tags        map[string]tagText           `json:"tags"`
	Markets     map[string]map[string]string `json:"markets"`  // market, then the kind of day
	Playbook    map[string][]string          `json:"playbook"` // tag id, "normal" or "events"
}

// WORDING is loaded once. The file is embedded, so a bad one is a build mistake, not a runtime state.
var WORDING = func() wording {
	var w wording
	if err := json.Unmarshal(stringsJSON, &w); err != nil {
		panic("calendar: strings.json: " + err.Error())
	}
	return w
}()

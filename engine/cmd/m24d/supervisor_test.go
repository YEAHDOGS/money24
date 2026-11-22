package main

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	TEST_STATE_FILE = "forever.json"
	TEST_NOW_MS     = int64(1_000_000_000)
	TEST_STATE      = `{"pid":7,"started_at":999000000,"beat_at":999990000,"url":"http://127.0.0.1:8025/desk/","services":[
 {"name":"newsd","port":8024,"pid":11,"started_at":999100000,"restarts":0,"misses":0,"health":{"ok":true,"at":999990000},"last_exit":null},
 {"name":"m24d","port":8025,"pid":0,"started_at":999100000,"restarts":2,"misses":0,"health":null,"last_exit":{"at":999995000,"how":"exit code 1"}}]}`
)

func TestSupervisorUnknownWithoutFile(t *testing.T) {
	for _, path := range []string{"", filepath.Join(t.TempDir(), TEST_STATE_FILE)} {
		got := readSupervisor(path, TEST_NOW_MS)
		if got.State != SUPERVISOR_UNKNOWN || len(got.Services) != 0 {
			t.Fatalf("path %q: got %+v, want unknown with no services", path, got)
		}
	}
}

func TestSupervisorReadsStateWithoutPids(t *testing.T) {
	path := filepath.Join(t.TempDir(), TEST_STATE_FILE)
	if err := os.WriteFile(path, []byte(TEST_STATE), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readSupervisor(path, TEST_NOW_MS)
	if got.State != SUPERVISOR_RUNNING || len(got.Services) != 2 {
		t.Fatalf("got %+v, want running with two services", got)
	}
	news, runner := got.Services[0], got.Services[1]
	if !news.Up || news.HealthOK == nil || !*news.HealthOK || news.StartedAt != 999100000 {
		t.Fatalf("newsd: got %+v, want up and healthy", news)
	}
	if runner.Up || runner.StartedAt != 0 || runner.Restarts != 2 || runner.LastExit != "exit code 1" || runner.HealthOK != nil {
		t.Fatalf("m24d: got %+v, want down after its second restart", runner)
	}
	if stale := readSupervisor(path, TEST_NOW_MS+SUPERVISOR_STALE_MS*2); stale.State != SUPERVISOR_STALE {
		t.Fatalf("got %q long after the last beat, want stale", stale.State)
	}
}

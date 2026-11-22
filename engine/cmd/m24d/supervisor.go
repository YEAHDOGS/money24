package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"
)

// The supervisor (scripts/forever.js) rewrites a state file with each service's state about once a
// minute and passes its path in ENV_SUPERVISOR_STATE. GET /api/supervisor serves it, read-only, so the
// desktop app's pocket window can show every service without reading files of its own. Pids are left
// out: a tunnel viewer has no use for them.
const (
	ROUTE_SUPERVISOR     = "GET /api/supervisor"
	ENV_SUPERVISOR_STATE = "M24D_SUPERVISOR_STATE"
	SUPERVISOR_UNKNOWN   = "unknown" // no supervisor started this runner, or its file cannot be read
	SUPERVISOR_RUNNING   = "running"
	SUPERVISOR_STALE     = "stale"       // the file is there, but the supervisor has not written it lately
	SUPERVISOR_STALE_MS  = 3 * 60 * 1000 // three of its one-minute beats missed
	SUPERVISOR_FILE_MAX  = 64 * 1024     // a state file is a few hundred bytes
)

// supervisorFile is the state file as forever.js writes it; times are epoch milliseconds.
type supervisorFile struct {
	StartedAt int64              `json:"started_at"`
	BeatAt    int64              `json:"beat_at"`
	URL       string             `json:"url"`
	Services  []supervisedFields `json:"services"`
}

type supervisedFields struct {
	Name      string `json:"name"`
	Port      int    `json:"port"`
	PID       int    `json:"pid"`
	StartedAt int64  `json:"started_at"`
	Restarts  int    `json:"restarts"`
	Misses    int    `json:"misses"`
	Health    *struct {
		OK bool  `json:"ok"`
		At int64 `json:"at"`
	} `json:"health"`
	LastExit *struct {
		At  int64  `json:"at"`
		How string `json:"how"`
	} `json:"last_exit"`
}

// SupervisedService is one service as the route shows it. Up: the supervisor has it running now.
type SupervisedService struct {
	Name      string `json:"name"`
	Port      int    `json:"port"`
	Up        bool   `json:"up"`
	StartedAt int64  `json:"started_at,omitempty"`
	Restarts  int    `json:"restarts"`
	Misses    int    `json:"misses"`
	HealthOK  *bool  `json:"health_ok,omitempty"` // nil until its first health check
	HealthAt  int64  `json:"health_at,omitempty"`
	LastExit  string `json:"last_exit,omitempty"`
	ExitAt    int64  `json:"exit_at,omitempty"`
}

// SupervisorStatus is the route's answer. State is running, stale or unknown.
type SupervisorStatus struct {
	State     string              `json:"state"`
	Now       int64               `json:"now"`
	StartedAt int64               `json:"started_at,omitempty"`
	BeatAt    int64               `json:"beat_at,omitempty"`
	URL       string              `json:"url,omitempty"`
	Services  []SupervisedService `json:"services"`
}

// readSupervisor reads the state file at path, as of now (epoch ms).
func readSupervisor(path string, now int64) SupervisorStatus {
	out := SupervisorStatus{State: SUPERVISOR_UNKNOWN, Now: now, Services: []SupervisedService{}}
	if path == "" {
		return out
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	var file supervisorFile
	if err := json.NewDecoder(io.LimitReader(f, SUPERVISOR_FILE_MAX)).Decode(&file); err != nil {
		return out
	}
	out.State = SUPERVISOR_RUNNING
	if now-file.BeatAt > SUPERVISOR_STALE_MS {
		out.State = SUPERVISOR_STALE
	}
	out.StartedAt, out.BeatAt, out.URL = file.StartedAt, file.BeatAt, file.URL
	for _, s := range file.Services {
		svc := SupervisedService{Name: s.Name, Port: s.Port, Up: s.PID > 0, Restarts: s.Restarts, Misses: s.Misses}
		if svc.Up {
			svc.StartedAt = s.StartedAt
		}
		if s.Health != nil {
			ok := s.Health.OK
			svc.HealthOK, svc.HealthAt = &ok, s.Health.At
		}
		if s.LastExit != nil {
			svc.LastExit, svc.ExitAt = s.LastExit.How, s.LastExit.At
		}
		out.Services = append(out.Services, svc)
	}
	return out
}

// supervisorRoute serves readSupervisor for the file named in the environment when the runner started.
func supervisorRoute() http.Handler {
	path := os.Getenv(ENV_SUPERVISOR_STATE)
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(readSupervisor(path, time.Now().UnixMilli()))
	})
}

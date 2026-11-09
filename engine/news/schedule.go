package news

import (
	"fmt"
	"sync"
	"time"
)

const (
	MIN_POLL     = time.Second
	MAX_POLL     = 15 * time.Minute
	DEFAULT_POLL = time.Minute
)

// Schedule is the one global poll interval. Changing it wakes every waiting source at once.
type Schedule struct {
	mu      sync.Mutex
	every   time.Duration
	changed chan struct{}
}

// NewSchedule starts at every, clamped into the allowed range.
func NewSchedule(every time.Duration) *Schedule {
	return &Schedule{every: min(max(every, MIN_POLL), MAX_POLL), changed: make(chan struct{})}
}

// Every returns the interval and a channel that closes the next time it changes.
func (s *Schedule) Every() (time.Duration, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.every, s.changed
}

// Set changes the interval for every source, effective immediately.
func (s *Schedule) Set(every time.Duration) error {
	if every < MIN_POLL || every > MAX_POLL {
		return fmt.Errorf("poll interval must be between %s and %s", MIN_POLL, MAX_POLL)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.every = every
	close(s.changed)
	s.changed = make(chan struct{})
	return nil
}

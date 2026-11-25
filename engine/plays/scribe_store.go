package plays

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Scribes reads the bounded local research history under the play store's lock.
func (s *Store) Scribes() ([]Scribe, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readScribes()
}

func (s *Store) readScribes() ([]Scribe, error) {
	rows := []Scribe{}
	data, err := os.ReadFile(filepath.Join(s.dir, SCRIBE_FILE))
	if errors.Is(err, os.ErrNotExist) {
		return rows, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// SaveScribe atomically persists a research proposal without changing the executable play book.
func (s *Store) SaveScribe(row Scribe) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.readScribes()
	if err != nil {
		return err
	}
	found := false
	for i := range rows {
		if rows[i].ID == row.ID {
			rows[i], found = row, true
			break
		}
	}
	if !found {
		rows = append([]Scribe{row}, rows...)
	}
	rows = rows[:min(len(rows), SCRIBE_LIMIT)]
	data, err := json.MarshalIndent(rows, "", " ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, SCRIBE_FILE)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, SCRIBE_PRIVATE_PERMS); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

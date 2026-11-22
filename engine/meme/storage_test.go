package meme

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const (
	TEST_SHARD_ROWS = 3
	TEST_TORN       = `{"at":`
)

var testDay = time.Date(2026, time.November, 22, 1, 0, 0, 0, time.UTC)

// writeShard records rows through the live writer and returns their positions
// with the shard's plain path, still open unless sealed.
func writeShard(t *testing.T, w *recordWriter) ([]Position, string) {
	t.Helper()
	day := testDay.Format(DAY_LAYOUT)
	var positions []Position
	for i := range TEST_SHARD_ROWS {
		row := Record{At: testDay.Add(time.Duration(i) * time.Second), Kind: KIND_LAUNCH, Mint: FIXTURE_MINT}
		pos, err := w.write(row, testDay, day)
		if err != nil {
			t.Fatal(err)
		}
		positions = append(positions, pos)
	}
	return positions, w.file.Name()
}

func readShard(t *testing.T, dir, name string) ([]string, []Position) {
	t.Helper()
	reader, err := OpenRecords(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var lines []string
	var positions []Position
	for {
		line, pos, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return lines, positions
		}
		if err != nil {
			t.Fatal(err)
		}
		lines, positions = append(lines, string(line)), append(positions, pos)
	}
}

func TestSealedShardKeepsBytesAndPositions(t *testing.T) {
	dir := t.TempDir()
	w := &recordWriter{dir: dir}
	written, plain := writeShard(t, w)
	before, err := os.ReadFile(plain)
	if err != nil {
		t.Fatal(err)
	}
	name := written[0].File
	plainLines, plainPositions := readShard(t, dir, name)
	if err := w.seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plain); !os.IsNotExist(err) {
		t.Fatalf("plain shard should be gone once sealed: %v", err)
	}
	lines, positions := readShard(t, dir, name)
	if !reflect.DeepEqual(positions, written) || !reflect.DeepEqual(positions, plainPositions) || !reflect.DeepEqual(lines, plainLines) {
		t.Fatalf("compression moved rows: %v vs %v", positions, written)
	}
	var joined string
	for _, line := range lines {
		joined += line
	}
	if joined != string(before) {
		t.Fatal("compressed shard does not hold the original bytes")
	}
}

// Every state an interrupted compression can leave reads as one shard and
// compacts to the archive alone.
func TestInterruptedCompressionRecovers(t *testing.T) {
	cases := map[string]func(t *testing.T, plain string){
		"temp archive left": func(t *testing.T, plain string) {
			if err := os.WriteFile(plain+GZIP_SUFFIX+TEMP_SUFFIX, []byte(TEST_TORN), RECORD_MODE); err != nil {
				t.Fatal(err)
			}
		},
		"archive published, plain not removed": func(t *testing.T, plain string) {
			data, err := os.ReadFile(plain)
			if err != nil {
				t.Fatal(err)
			}
			if err := compressRecords(plain); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(plain, data, RECORD_MODE); err != nil {
				t.Fatal(err)
			}
		},
		"crash mid-line": func(t *testing.T, plain string) {
			file, err := os.OpenFile(plain, os.O_APPEND|os.O_WRONLY, RECORD_MODE)
			if err != nil {
				t.Fatal(err)
			}
			_, err = file.WriteString(TEST_TORN)
			if err := errors.Join(err, file.Close()); err != nil {
				t.Fatal(err)
			}
		},
	}
	for label, interrupt := range cases {
		t.Run(label, func(t *testing.T) {
			dir := t.TempDir()
			w := &recordWriter{dir: dir}
			written, plain := writeShard(t, w)
			if err := w.file.Close(); err != nil {
				t.Fatal(err)
			}
			interrupt(t, plain)
			day := testDay.Format(DAY_LAYOUT)
			names, err := RecordFiles(dir, day)
			if err != nil || len(names) != 1 || names[0] != written[0].File {
				t.Fatalf("want the one shard, got %v %v", names, err)
			}
			if err := Compact(dir); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(filepath.Join(dir, day))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != filepath.Base(plain)+GZIP_SUFFIX {
				t.Fatalf("compaction should leave only the archive: %v", entries)
			}
			reader, err := OpenRecords(dir, written[0].File)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			for _, want := range written {
				if _, pos, err := reader.Next(); err != nil || pos != want {
					t.Fatalf("row at %v: got %v, %v", want, pos, err)
				}
			}
			if _, _, err := reader.Next(); !errors.Is(err, io.EOF) && !errors.Is(err, ErrIncompleteRecord) {
				t.Fatalf("after the rows: %v", err)
			}
		})
	}
}

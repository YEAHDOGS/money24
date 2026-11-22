package meme

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DATA_SUBDIR     = "meme"
	GZIP_SUFFIX     = ".gz"
	TEMP_SUFFIX     = ".tmp"
	RESTORE_BUFFER  = 8 << 10
	RECORD_ROTATION = 5 * time.Minute
	LINE_END        = '\n'
	RECORD_MODE     = 0o600
)

var ErrIncompleteRecord = errors.New("incomplete final recorder line")

// Position identifies a complete line by its offsets in the uncompressed
// file. File is relative to the recorder directory and uses slash separators.
type Position struct {
	File   string `json:"file"`
	Offset int64  `json:"offset,omitempty"`
	End    int64  `json:"-"`
}

type recordWriter struct {
	dir     string
	name    string
	day     string
	file    *os.File
	offset  int64
	expires time.Time
}

func (w *recordWriter) sealExpired(now time.Time, day string) error {
	if w.file != nil && (w.day != day || !now.Before(w.expires)) {
		return w.seal()
	}
	return nil
}

func (w *recordWriter) write(row Record, now time.Time, day string) (Position, error) {
	data, err := json.Marshal(row)
	if err != nil {
		return Position{}, err
	}
	if w.file == nil {
		dir := filepath.Join(w.dir, day)
		if err := os.MkdirAll(dir, DIRECTORY_MODE); err != nil {
			return Position{}, err
		}
		file, err := os.CreateTemp(dir, FILE_PREFIX+now.Format(FILE_CLOCK_LAYOUT)+"-*"+FILE_SUFFIX)
		if err != nil {
			return Position{}, err
		}
		w.file, w.day, w.expires, w.offset = file, day, now.Add(RECORD_ROTATION), 0
		w.name = day + "/" + filepath.Base(file.Name())
	}
	n, err := w.file.Write(append(data, LINE_END))
	if err != nil {
		return Position{}, errors.Join(err, w.seal())
	}
	pos := Position{File: w.name, Offset: w.offset, End: w.offset + int64(n)}
	w.offset = pos.End
	return pos, nil
}

func (w *recordWriter) seal() error {
	if w.file == nil {
		return nil
	}
	path := w.file.Name()
	err := w.file.Close()
	w.file = nil
	if err != nil {
		return err
	}
	return compressRecords(path)
}

// Compact compresses closed recorder files in place. Call it before the
// recorder opens its active shard or on an offline copy.
func Compact(dir string) error {
	days, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var compactErr error
	for _, day := range days {
		if _, err := time.Parse(DAY_LAYOUT, day.Name()); err != nil || !day.IsDir() {
			continue
		}
		dayDir := filepath.Join(dir, day.Name())
		files, err := os.ReadDir(dayDir)
		if err != nil {
			compactErr = errors.Join(compactErr, err)
			continue
		}
		for _, file := range files {
			if file.IsDir() || !isPlainRecord(file.Name()) {
				continue
			}
			if err := compressRecords(filepath.Join(dayDir, file.Name())); err != nil {
				compactErr = errors.Join(compactErr, err)
			}
		}
	}
	return compactErr
}

func compressRecords(path string) error {
	raw, err := os.Open(path)
	if err != nil {
		return err
	}
	defer raw.Close()
	archive := path + GZIP_SUFFIX
	if _, err := os.Stat(archive); err == nil {
		// The archive was published before the previous attempt removed its source.
		raw.Close()
		return os.Remove(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp := archive + TEMP_SUFFIX
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, RECORD_MODE)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	zip := gzip.NewWriter(out)
	_, copyErr := io.Copy(zip, raw)
	if err := errors.Join(copyErr, zip.Close(), raw.Close(), out.Sync(), out.Close()); err != nil {
		return err
	}
	if err := os.Rename(tmp, archive); err != nil {
		return err
	}
	return os.Remove(path)
}

func isPlainRecord(name string) bool {
	return strings.HasPrefix(name, FILE_PREFIX) && strings.HasSuffix(name, FILE_SUFFIX)
}

// RecordFiles lists logical file names in rotation order, even when an
// interrupted compaction left both a plain file and its published archive.
func RecordFiles(dir, day string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(dir, day))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), GZIP_SUFFIX)
		if entry.IsDir() || !isPlainRecord(name) {
			continue
		}
		name = day + "/" + name
		if len(files) == 0 || files[len(files)-1] != name {
			files = append(files, name)
		}
	}
	return files, nil
}

// RecordReader streams original JSONL bytes with their stable file positions.
type RecordReader struct {
	file    *os.File
	zip     *gzip.Reader
	scanner *bufio.Scanner
	name    string
	offset  int64
}

func OpenRecords(dir, name string) (*RecordReader, error) {
	day, base, ok := strings.Cut(name, "/")
	if _, err := time.Parse(DAY_LAYOUT, day); !ok || err != nil || !filepath.IsLocal(base) ||
		strings.ContainsAny(base, "/\\:") || !isPlainRecord(base) {
		return nil, os.ErrInvalid
	}
	path := filepath.Join(dir, name)
	file, err := os.Open(path + GZIP_SUFFIX)
	var zip *gzip.Reader
	if err == nil {
		zip, err = gzip.NewReader(file)
		if err != nil {
			file.Close()
			return nil, err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		file, err = os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			// The plain file may have been replaced after the first open.
			if _, statErr := os.Stat(path + GZIP_SUFFIX); statErr == nil {
				return OpenRecords(dir, name)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	var source io.Reader = file
	if zip != nil {
		source = zip
	}
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, RESTORE_BUFFER), MAX_LOOKUP_BYTES)
	scanner.Split(completeLine)
	return &RecordReader{file: file, zip: zip, scanner: scanner, name: name}, nil
}

func completeLine(data []byte, atEOF bool) (int, []byte, error) {
	if end := bytes.IndexByte(data, LINE_END) + 1; end > 0 {
		return end, data[:end], nil
	}
	if atEOF && len(data) != 0 {
		return 0, nil, ErrIncompleteRecord
	}
	return 0, nil, nil
}

func (r *RecordReader) Next() ([]byte, Position, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return nil, Position{}, err
		}
		return nil, Position{}, io.EOF
	}
	line := r.scanner.Bytes()
	pos := Position{File: r.name, Offset: r.offset, End: r.offset + int64(len(line))}
	r.offset = pos.End
	return line, pos, nil
}

func (r *RecordReader) Close() error {
	if r.zip != nil {
		return errors.Join(r.zip.Close(), r.file.Close())
	}
	return r.file.Close()
}

func dayBytes(dir, day string) int64 {
	entries, _ := os.ReadDir(filepath.Join(dir, day))
	var size int64
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), GZIP_SUFFIX)
		if !entry.IsDir() && isPlainRecord(name) {
			if info, err := entry.Info(); err == nil {
				size += info.Size()
			}
		}
	}
	return size
}

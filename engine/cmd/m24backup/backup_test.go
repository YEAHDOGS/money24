package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	TEST_AWS_ACCESS = "AKIAIOSFODNN7EXAMPLE" // Public AWS documentation fixture.
	TEST_AWS_SECRET = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	TEST_AWS_HOST   = "https://examplebucket.s3.amazonaws.com"
	TEST_DAY1       = "2026-11-18"
	TEST_DAY2       = "2026-11-19"
	TEST_DAY3       = "2026-11-20"
	TEST_DAY4       = "2026-11-21"
	TEST_BYTES      = 20
)

func TestSignatureAWS(t *testing.T) {
	// https://docs.aws.amazon.com/AmazonS3/latest/developerguide/sig-v4-header-based-auth.html
	at, err := time.Parse(SIGN_TIME, "20130524T000000Z")
	if err != nil {
		t.Fatal(err)
	}
	for _, vector := range []struct{ path, query, byteRange, signature string }{
		{"/test.txt", "", "bytes=0-9", "f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"},
		{"/", "lifecycle", "", "fea454ca298b7da1c68078a5d1bdbfbbe0d65c699e0f91ac7a200a0136783543"},
		{"/", "prefix=J&max-keys=2", "", "34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"},
	} {
		req, err := http.NewRequest(http.MethodGet, TEST_AWS_HOST+vector.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		query, err := url.ParseQuery(vector.query)
		if err != nil {
			t.Fatal(err)
		}
		req.URL.RawQuery = canonicalQuery(query)
		if vector.byteRange != "" {
			req.Header.Set("Range", vector.byteRange)
		}
		signRequest(req, at, "us-east-1", TEST_AWS_ACCESS, TEST_AWS_SECRET, EMPTY_HASH)
		if got := req.Header.Get("Authorization"); !strings.HasSuffix(got, "Signature="+vector.signature) {
			t.Errorf("%s?%s signature differs from AWS example", vector.path, vector.query)
		}
	}
	if got := canonicalQuery(url.Values{"token": {"a b+/", "b"}}); got != "token=a%20b%2B%2F&token=b" {
		t.Errorf("canonical query = %s", got)
	}
}

type memoryStore struct {
	files map[string][]byte
}

func (s memoryStore) list() ([]object, error) {
	var listed []object
	for key, contents := range s.files {
		listed = append(listed, object{key, int64(len(contents))})
	}
	return listed, nil
}

func (s memoryStore) put(key string, data io.Reader, size int64, hash string) error {
	if _, exists := s.files[key]; exists {
		return errors.New("duplicate upload")
	}
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	if int64(len(b)) != size || len(hash) != len(EMPTY_HASH) {
		return errors.New("invalid archive metadata")
	}
	s.files[key] = b
	return nil
}

func (s memoryStore) get(key string) (io.ReadCloser, error) {
	content, ok := s.files[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (s memoryStore) delete(key string) error {
	delete(s.files, key)
	return nil
}

func TestBackupPackingAndRestore(t *testing.T) {
	root := t.TempDir()
	present := map[string]string{
		"trading/paper-journal.jsonl":                  "paper journal",
		"trading/telegram.session":                     "session fixture",
		"trading/meme/" + TEST_DAY1 + "/records.jsonl": "meme",
		"trading/tape/" + TEST_DAY2 + "/TEST.jsonl.gz": "tape",
		"trading/future/unknown-rulebook.json":         "rulebook",
	}
	excluded := "trading/meme/" + TEST_DAY3 + "/records.jsonl"
	for name, body := range present {
		writeFixture(t, root, name, body)
	}
	writeFixture(t, root, excluded, "day still recording")
	store := memoryStore{make(map[string][]byte)}
	if err := backup(store, root, TEST_DAY3, TEST_DAY3, DEFAULT_CAP); err != nil {
		t.Fatal(err)
	}
	if len(store.files) != 3 {
		t.Fatalf("got %d archives", len(store.files))
	}
	present["trading/paper-journal.jsonl"] = "new journal"
	writeFixture(t, root, "trading/paper-journal.jsonl", "new journal")
	if err := backup(store, root, TEST_DAY3, TEST_DAY3, DEFAULT_CAP); err != nil {
		t.Fatal(err) // No same-day duplicate uploads.
	}
	if err := backup(store, root, TEST_DAY4, TEST_DAY3, DEFAULT_CAP); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "fresh")
	if err := restore(store, target); err != nil {
		t.Fatal(err)
	}
	for name, want := range present {
		data, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
		if err != nil || string(data) != want {
			t.Errorf("%s was not restored: %q, %v", name, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(target, filepath.FromSlash(excluded))); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial day was restored")
	}
	if err := restore(store, target); err == nil {
		t.Fatal("restore accepted an existing target")
	}
}

func writeFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), DIR_MODE); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(contents), FILE_MODE); err != nil {
		t.Fatal(err)
	}
}

func TestRetention(t *testing.T) {
	oldDay := object{OBJECT_PREFIX + DAILY_KIND + "/" + TEST_DAY1 + ARCHIVE_SUFFIX, TEST_BYTES}
	newDay := object{OBJECT_PREFIX + DAILY_KIND + "/" + TEST_DAY2 + ARCHIVE_SUFFIX, TEST_BYTES}
	state := object{OBJECT_PREFIX + STATE_GROUP + "/" + TEST_DAY3 + ARCHIVE_SUFFIX, TEST_BYTES}
	next := object{OBJECT_PREFIX + STATE_GROUP + "/" + TEST_DAY4 + ARCHIVE_SUFFIX, TEST_BYTES}
	inventory := []object{state, newDay, oldDay}
	removed, err := retention(inventory, next, 3*TEST_BYTES)
	if err != nil || !reflect.DeepEqual(removed, []object{oldDay}) {
		t.Fatalf("expected oldest pruning, got %v, %v", removed, err)
	}
	if _, err := retention(inventory, next, TEST_BYTES); !errors.Is(err, ERR_NO_ROOM) {
		t.Fatal("retention would discard the last state before its replacement")
	}
	if _, err := retention([]object{state, newDay}, oldDay, 2*TEST_BYTES); !errors.Is(err, ERR_NO_ROOM) {
		t.Fatal("an old daily archive would displace newer history")
	}
	removed, err = retention(inventory, object{}, TEST_BYTES)
	if err != nil || !reflect.DeepEqual(removed, []object{oldDay, newDay}) {
		t.Fatalf("lower cap should retain current state only, got %v, %v", removed, err)
	}
}

func TestRestoreRejectsUnsafeArchive(t *testing.T) {
	var output bytes.Buffer
	compressed := gzip.NewWriter(&output)
	archive := tar.NewWriter(compressed)
	if err := archive.WriteHeader(&tar.Header{Name: "../outside", Mode: FILE_MODE, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	store := memoryStore{map[string][]byte{OBJECT_PREFIX + STATE_GROUP + "/" + TEST_DAY3 + ARCHIVE_SUFFIX: output.Bytes()}}
	target := filepath.Join(t.TempDir(), "fresh")
	if err := restore(store, target); err == nil {
		t.Fatal("restore accepted a traversal")
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed restore created its destination")
	}
}

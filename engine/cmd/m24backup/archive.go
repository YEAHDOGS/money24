package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	DAY_FORMAT  = "2006-01-02"
	DAY_LENGTH  = len(DAY_FORMAT)
	STATE_GROUP = "state"
	NEWS_DB     = "news/news.db"
	SQLITE_COPY = "news.db"
	FILE_MODE   = 0600
	DIR_MODE    = 0700
)

// archiveGroups separates immutable date partitions from changing, undated state.
func archiveGroups(root, cutoff string) (map[string][]string, error) {
	groups := make(map[string][]string)
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, file)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		switch name {
		case NEWS_DB + "-wal", NEWS_DB + "-shm", NEWS_DB + "-journal":
			return nil // SQLite's online backup includes committed WAL changes.
		}
		group := STATE_GROUP
		for _, part := range strings.Split(name, "/") {
			if len(part) < DAY_LENGTH || (len(part) > DAY_LENGTH && part[DAY_LENGTH] != '.') {
				continue
			}
			day := part[:DAY_LENGTH]
			if _, err := time.Parse(DAY_FORMAT, day); err != nil {
				continue
			}
			if day >= cutoff {
				return nil
			}
			group = day
			break
		}
		groups[group] = append(groups[group], name)
		return nil
	})
	return groups, err
}

func writeEntry(archive *tar.Writer, file, name string) error {
	source, err := os.Open(file)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("archive source is no longer a regular file")
	}
	header := &tar.Header{Name: name, Mode: FILE_MODE, Size: info.Size(), ModTime: info.ModTime()}
	if err := archive.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.CopyN(archive, source, header.Size)
	return err
}

// pack streams files into tar.gz, taking a consistent SQLite snapshot for newsd.
func pack(dst io.Writer, root string, files []string, scratch string) error {
	compressed, err := gzip.NewWriterLevel(dst, gzip.BestSpeed)
	if err != nil {
		return err
	}
	archive := tar.NewWriter(compressed)
	for _, name := range files {
		file := filepath.Join(root, filepath.FromSlash(name))
		if name == NEWS_DB {
			cmd := exec.Command("sqlite3", "-batch", "-bail", "-readonly", "-cmd", ".timeout 10000", file, ".backup "+SQLITE_COPY)
			cmd.Dir = scratch
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("snapshot news SQLite database: %w", err)
			}
			file = filepath.Join(scratch, SQLITE_COPY)
		}
		if err := writeEntry(archive, file, name); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	return compressed.Close()
}

// unpack restores regular files only and rejects traversal, links and duplicate entries.
func unpack(src io.Reader, root string) error {
	compressed, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	defer compressed.Close()
	archive := tar.NewReader(compressed)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			_, err := io.Copy(io.Discard, compressed) // verify gzip's checksum
			return err
		}
		if err != nil {
			return err
		}
		name, err := filepath.Localize(header.Name)
		if err != nil || header.Name == "." || header.Name != path.Clean(header.Name) ||
			strings.ContainsAny(header.Name, "\\:") || header.Typeflag != tar.TypeReg {
			return fmt.Errorf("unsafe or unsupported archive entry")
		}
		target := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(target), DIR_MODE); err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, FILE_MODE)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, archive)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if err := os.Chtimes(target, header.ModTime, header.ModTime); err != nil {
			return err
		}
	}
}

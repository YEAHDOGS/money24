package diskusage

import "os"

// Files totals the byte lengths of the files currently on disk.
func Files(paths ...string) int64 {
	var size int64
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			size += info.Size()
		}
	}
	return size
}

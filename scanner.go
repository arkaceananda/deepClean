package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type FileInfo struct {
	Path     string
	Size     int64
	ModTime  time.Time
	Atime    time.Time
	Category string
	Risk     string
}

type ScanResult struct {
	Files     []FileInfo
	TotalSize int64
	Skipped   int
	Errors    []string
}

var skipDirs = map[string]bool{
	"/proc": true, "/sys": true, "/dev": true, "/run": true,
	"/snap": true, "/lost+found": true,
}

func isSystemPath(p string) bool {
	for _, root := range []string{"/usr", "/etc", "/boot", "/lib", "/lib64", "/bin", "/sbin", "/opt"} {
		if p == root || strings.HasPrefix(p, root+"/") {
			return true
		}
	}
	return false
}

func categorize(p string) string {
	l := strings.ToLower(p)
	switch {
	case strings.Contains(l, "/.cache/") || strings.Contains(l, "/cache/"):
		return "cache"
	case strings.HasSuffix(l, ".log") || strings.Contains(l, "/log/") || strings.Contains(l, "/logs/"):
		return "log"
	case strings.HasPrefix(l, "/tmp/") || strings.HasPrefix(l, "/var/tmp/"):
		return "temp"
	default:
		return "other"
	}
}

// riskLevel gives a coarse risk hint for deletion review.
func riskLevel(p string) string {
	if isSystemPath(p) {
		return "risky"
	}
	l := strings.ToLower(p)
	switch {
	case strings.Contains(l, "/cache/"), strings.Contains(l, "/.cache/"),
		strings.HasPrefix(l, "/tmp/"), strings.HasPrefix(l, "/var/tmp/"),
		strings.HasSuffix(l, ".log"), strings.Contains(l, "/log/"), strings.Contains(l, "/logs/"):
		return "safe"
	case strings.Contains(l, "/build/"), strings.Contains(l, "/target/"),
		strings.Contains(l, "node_modules/"), strings.Contains(l, "/.gradle/"):
		return "caution"
	default:
		return "caution"
	}
}

func scan(root string, minSize int64, olderThan time.Duration, maxFiles int) (ScanResult, error) {
	res := ScanResult{}
	cutoff := time.Now().Add(-olderThan)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", path, err))
			return nil // continue
		}
		// skip special dirs
		if d.IsDir() {
			if skipDirs[path] {
				return filepath.SkipDir
			}
			// don't descend into mount-ish huge dirs handled above
			return nil
		}
		info, err := d.Info()
		if err != nil {
			res.Skipped++
			return nil
		}
		if info.Size() < minSize {
			return nil
		}
		// get atime via syscall (Linux)
		var atime time.Time
		var st syscall.Stat_t
		if err := syscall.Stat(path, &st); err == nil {
			atime = time.Unix(st.Atim.Sec, st.Atim.Nsec)
		} else {
			atime = info.ModTime()
		}
		// last used = max(atime, mtime) to be safe under relatime
		lastUsed := info.ModTime()
		if atime.After(lastUsed) {
			lastUsed = atime
		}
		if lastUsed.After(cutoff) {
			return nil
		}
		res.Files = append(res.Files, FileInfo{
			Path:     path,
			Size:     info.Size(),
			ModTime:  info.ModTime(),
			Atime:    atime,
			Category: categorize(path),
			Risk:     riskLevel(path),
		})
		res.TotalSize += info.Size()
		return nil
	})
	if err != nil {
		return res, err
	}

	// sort by size desc
	sort.Slice(res.Files, func(i, j int) bool { return res.Files[i].Size > res.Files[j].Size })

	if maxFiles > 0 && len(res.Files) > maxFiles {
		res.Files = res.Files[:maxFiles]
	}
	return res, nil
}

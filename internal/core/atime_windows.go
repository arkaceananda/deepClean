//go:build windows

package core

import (
	"os"
	"time"
)

func getFileAtime(path string, info os.FileInfo) time.Time {
	// Windows atime is less reliable; fall back to mtime.
	return info.ModTime()
}

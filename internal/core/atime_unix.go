//go:build !windows

package core

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func getFileAtime(path string, info os.FileInfo) time.Time {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err == nil {
		return time.Unix(st.Atim.Sec, int64(st.Atim.Nsec))
	}
	return info.ModTime()
}

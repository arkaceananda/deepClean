package main

import "fmt"

const (
	mb = 1e6
	gb = 1e9
)

// FormatSize renders bytes as decimal MB/GB (1 MB = 1,000,000 bytes).
func FormatSize(b int64) string {
	if b >= gb {
		return fmt.Sprintf("%.2f GB", float64(b)/gb)
	}
	return fmt.Sprintf("%.2f MB", float64(b)/mb)
}

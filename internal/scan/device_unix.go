//go:build unix

package scan

import (
	"fmt"
	"os"
	"syscall"
)

func deviceID(info os.FileInfo) (uint64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("file info does not contain Unix stat data")
	}
	return uint64(stat.Dev), nil
}

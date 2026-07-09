//go:build darwin

package scan

import "syscall"

func detectPseudoFilesystem(path string) (bool, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return false, err
	}

	switch statfsTypeName(stat.Fstypename) {
	case "autofs", "devfs", "fdesc", "procfs":
		return true, nil
	default:
		return false, nil
	}
}

func statfsTypeName(raw [16]int8) string {
	buf := make([]byte, 0, len(raw))
	for _, c := range raw {
		if c == 0 {
			break
		}
		buf = append(buf, byte(c))
	}
	return string(buf)
}

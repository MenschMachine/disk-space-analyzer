//go:build linux

package scan

import "syscall"

const (
	autofsSuperMagic  int64 = 0x0187
	binfmtfsMagic     int64 = 0x42494e4d
	cgroupSuperMagic  int64 = 0x27e0eb
	cgroup2SuperMagic int64 = 0x63677270
	configfsMagic     int64 = 0x62656570
	debugfsMagic      int64 = 0x64626720
	devptsSuperMagic  int64 = 0x1cd1
	efivarfsMagic     int64 = 0xde5e81e4
	fusectlSuperMagic int64 = 0x65735543
	hugetlbfsMagic    int64 = 0x958458f6
	mqueueMagic       int64 = 0x19800202
	nsfsMagic         int64 = 0x6e736673
	procSuperMagic    int64 = 0x9fa0
	pstorefsMagic     int64 = 0x6165676c
	securityfsMagic   int64 = 0x73636673
	selinuxMagic      int64 = 0xf97cff8c
	sockfsMagic       int64 = 0x534f434b
	sysfsMagic        int64 = 0x62656572
	tmpfsMagic        int64 = 0x01021994
	tracefsMagic      int64 = 0x74726163
)

func detectPseudoFilesystem(path string) (bool, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return false, err
	}

	switch int64(stat.Type) {
	case autofsSuperMagic,
		binfmtfsMagic,
		cgroupSuperMagic,
		cgroup2SuperMagic,
		configfsMagic,
		debugfsMagic,
		devptsSuperMagic,
		efivarfsMagic,
		fusectlSuperMagic,
		hugetlbfsMagic,
		mqueueMagic,
		nsfsMagic,
		procSuperMagic,
		pstorefsMagic,
		securityfsMagic,
		selinuxMagic,
		sockfsMagic,
		sysfsMagic,
		tmpfsMagic,
		tracefsMagic:
		return true, nil
	default:
		return false, nil
	}
}

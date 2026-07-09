//go:build !linux && !darwin

package scan

func detectPseudoFilesystem(string) (bool, error) {
	return false, nil
}

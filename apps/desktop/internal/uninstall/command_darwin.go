package uninstall

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const Script = "/Library/Application Support/OpenSurge/share/uninstall-gui.sh"

func ValidateInstalledScript() error {
	// Refuse symlinks and writable components before asking for elevation.
	for path := Script; path != "/"; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil {
			return ErrUnavailable
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
			return ErrUnavailable
		}
		if path == Script {
			if !info.Mode().IsRegular() || info.Mode().Perm()&0100 == 0 {
				return ErrUnavailable
			}
		} else if !info.IsDir() {
			return ErrUnavailable
		}
	}
	return nil
}
func command(mode Mode) (string, []string, error) {
	if mode != KeepData && mode != RemoveAll {
		return "", nil, ErrUnavailable
	}
	// Both the script and its two arguments are native constants.
	return "/usr/bin/osascript", []string{"-e", `do shell script "exec '` + Script + `' --` + string(mode) + `" with administrator privileges`}, nil
}
func RunInstalled(mode Mode) error {
	if err := ValidateInstalledScript(); err != nil {
		return err
	}
	executable, args, err := command(mode)
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd := exec.Command(executable, args...)
	cmd.Stderr = &stderr
	if cmd.Run() == nil {
		return nil
	}
	if strings.Contains(stderr.String(), "(-128)") {
		return ErrCancelled
	}
	return ErrFailed // Never expose command output or local paths to JavaScript.
}

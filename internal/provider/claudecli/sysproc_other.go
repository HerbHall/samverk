//go:build !linux

package claudecli

import "os/exec"

// setProcessGroup is a no-op on non-Linux platforms.
// Setpgid subprocess isolation only applies to the Linux deployment target (CT 202).
func setProcessGroup(_ *exec.Cmd) {}

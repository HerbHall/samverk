//go:build linux

package claudecli

import (
	"os/exec"
	"syscall"
)

// setProcessGroup places cmd in its own process group (Setpgid=true).
// This prevents signals sent to the parent's process group from propagating
// to the claude-cli subprocess — belt-and-suspenders alongside KillMode=process
// in the systemd service unit.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

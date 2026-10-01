//go:build unix

package ansible

import (
	"os/exec"
	"syscall"
)

// setProcessGroup runs cmd in its own process group so that cancelling it
// reaches Ansible's forked workers too. Cancellation sends SIGINT, which
// Ansible handles gracefully; os/exec sends SIGKILL after cmd.WaitDelay.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
}

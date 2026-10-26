package liveness

import (
	"syscall"
)

// Alive returns true if the process with pid is running.
// Uses kill(pid, 0) which works on both Linux and macOS.
func Alive(pid int) bool {
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}

//go:build !windows

package sysutil

import "syscall"

// ProcessoVivo dice se esiste ancora un processo con quel PID.
// Il segnale 0 non viene consegnato: serve solo a far fare al kernel
// il controllo di esistenza e permessi.
func ProcessoVivo(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

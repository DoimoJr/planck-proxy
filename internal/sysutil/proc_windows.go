//go:build windows

package sysutil

import "golang.org/x/sys/windows"

// ProcessoVivo dice se esiste ancora un processo con quel PID.
//
// Usa PROCESS_QUERY_LIMITED_INFORMATION, il diritto minimo che basta a
// interrogare lo stato: funziona anche su processi di cui non siamo
// proprietari, al contrario di PROCESS_QUERY_INFORMATION.
//
// Un processo terminato ma non ancora "reaped" resta aperto con exit
// code diverso da STILL_ACTIVE: per questo non basta che l'handle si
// apra, va letto anche il codice di uscita.
func ProcessoVivo(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	var codice uint32
	if err := windows.GetExitCodeProcess(h, &codice); err != nil {
		return false
	}
	const stillActive = 259 // STILL_ACTIVE
	return codice == stillActive
}

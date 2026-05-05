//go:build windows

// Package sysutil contiene helper Windows-specific per nascondere le
// console window dei sub-process e per attendere che una porta TCP
// sia pronta ad accettare connessioni.
package sysutil

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW e' il flag Win32 che impedisce a un sub-process console
// di creare una finestra console visibile. Senza, ogni `exec.Command()` di
// un programma console (es. `veyon-cli.exe`, `ping.exe`) lanciato da un
// binario subsystem GUI fa lampeggiare una cmd window per qualche frame.
//
// Riferimento: https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags
const createNoWindow = 0x08000000

// HideConsoleWindow configura cmd in modo che il sub-process non crei
// finestre console visibili. Idempotente: se SysProcAttr e' nil lo
// inizializza, altrimenti aggiunge i flag.
func HideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

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

// HideOwnConsole nasconde la finestra console allocata al PROPRIO
// processo. Usato quando il binario e' compilato col subsystem console
// (default Go) ma vogliamo comportamento "GUI app" senza il flag
// -H=windowsgui che triggera Windows Defender (pattern malware).
//
// Trade-off: la finestra cmd viene allocata da Windows al lancio e poi
// nascosta entro pochi ms — l'utente puo' vedere un flash breve, ma il
// PE header resta "console subsystem" → niente piu' false positive AV.
//
// No-op se il processo non ha una console attached (es. lanciato da
// servizio).
func HideOwnConsole() {
	user32 := syscall.NewLazyDLL("user32.dll")
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")

	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd == 0 {
		return // nessuna console (es. servizio Windows)
	}
	const SW_HIDE = 0
	_, _, _ = showWindow.Call(hwnd, SW_HIDE)
}

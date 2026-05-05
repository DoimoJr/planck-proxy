//go:build !windows

package sysutil

import "os/exec"

// HideConsoleWindow e' no-op su Linux/macOS: i sub-process console
// non creano finestre proprie (girano in fg/bg del terminale chiamante).
func HideConsoleWindow(cmd *exec.Cmd) {}

// HideOwnConsole e' no-op fuori da Windows.
func HideOwnConsole() {}

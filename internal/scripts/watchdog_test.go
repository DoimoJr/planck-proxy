package scripts

import (
	"strings"
	"testing"
)

// Gli script watchdog sono PowerShell generato: girano su 26 macchine e non
// sono eseguibili dalla toolchain Go, quindi questi test coprono cio' che si
// puo' verificare staticamente — sostituzione dei segnaposto, presenza delle
// funzioni chiave, e bilanciamento di parentesi e graffe, che e' il modo
// tipico in cui una modifica al template rompe lo script silenziosamente.
// Uno script rotto non e' rumoroso: i plugin semplicemente non riportano
// piu' nulla.

func TestWatchdogScriptsSostituisconoISegnaposto(t *testing.T) {
	casi := map[string]string{
		"process": WatchdogProcessScript("10.0.0.1", 9999, []string{"cmd.exe", "firefox.exe"}),
		"usb":     WatchdogUsbScript("10.0.0.1", 9999, []string{"HIDClass"}, []string{}),
	}
	for nome, s := range casi {
		if strings.Contains(s, "__") {
			for _, seg := range []string{"__IP_DOCENTE__", "__PORTA_WEB__", "__DENY_LIST__", "__IGNORED_CLASSES__", "__ALLOW_VID_PID__"} {
				if strings.Contains(s, seg) {
					t.Errorf("script %s: segnaposto %s non sostituito", nome, seg)
				}
			}
		}
		if !strings.Contains(s, "10.0.0.1") || !strings.Contains(s, "9999") {
			t.Errorf("script %s: IP o porta non iniettati", nome)
		}
		if err := bilanciato(s); err != "" {
			t.Errorf("script %s: %s", nome, err)
		}
	}
}

// Il plugin process conta per NOME e non per PID: aprire Firefox, che e'
// multi-processo, deve produrre un evento solo. Vedi il commento nel
// template — sul campo i PID producevano 988 eventi su 1000.
func TestWatchdogProcessContaPerNome(t *testing.T) {
	s := WatchdogProcessScript("10.0.0.1", 9999, []string{"firefox.exe"})
	for _, atteso := range []string{"Get-SospettiPerNome", "$baseline.ContainsKey($k)", "istanze"} {
		if !strings.Contains(s, atteso) {
			t.Errorf("il template process non contiene %q: la deduplica per nome e' saltata", atteso)
		}
	}
	if strings.Contains(s, "$baseline[$p.Id]") {
		t.Errorf("il template process traccia ancora i PID")
	}
}

// Una chiavetta USB su Windows ha classe DiskDrive, la stessa del disco
// interno che ignoriamo di proposito: senza l'eccezione su USBSTOR il
// plugin non potrebbe rilevarne nessuna.
func TestWatchdogUsbRilevaComunqueUSBSTOR(t *testing.T) {
	s := WatchdogUsbScript("10.0.0.1", 9999, []string{"DiskDrive"}, []string{})
	if !strings.Contains(s, "USBSTOR") {
		t.Errorf("il template usb non forza USBSTOR: le chiavette resterebbero invisibili")
	}
	if !strings.Contains(s, "Test-Forzato") {
		t.Errorf("il template usb non applica l'eccezione ai prefissi forzati")
	}
}

// bilanciato controlla graffe e parentesi tonde. Ritorna "" se tutto torna.
func bilanciato(s string) string {
	var graffe, tonde int
	for _, r := range s {
		switch r {
		case '{':
			graffe++
		case '}':
			graffe--
		case '(':
			tonde++
		case ')':
			tonde--
		}
		if graffe < 0 {
			return "graffa chiusa in eccesso"
		}
		if tonde < 0 {
			return "parentesi chiusa in eccesso"
		}
	}
	if graffe != 0 {
		return "graffe non bilanciate"
	}
	if tonde != 0 {
		return "parentesi non bilanciate"
	}
	return ""
}

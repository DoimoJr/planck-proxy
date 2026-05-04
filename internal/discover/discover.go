// Package discover implementa il discovery degli IP attivi nella LAN
// del docente via ping sweep concorrente del /24 derivato dal suo IP.
//
// Approccio: per ogni IP del /24 (.1 → .254) lancia un ping con timeout
// breve. Gli IP che rispondono entro il timeout sono considerati "vivi"
// e diventano candidati a popolare la mappa studenti.
//
// Implementazione: shell out a `ping.exe` (Windows) o `ping` (Linux/macOS)
// — niente raw ICMP perche' richiederebbe privilegi admin. La concorrenza
// e' limitata da un semaforo (default 32) per non saturare la NIC.
package discover

import (
	"context"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout e' il tempo massimo di attesa per la risposta ICMP.
// 250ms e' un compromesso tra falsi negativi (PC lento) e velocita'
// dello sweep totale (~2s per /24 con concorrenza 32).
const DefaultTimeout = 250 * time.Millisecond

// DefaultConcurrency e' il numero massimo di ping concorrenti. 32 e'
// abbondante per una LAN /24 senza affaticare la macchina.
const DefaultConcurrency = 32

// Sweep esegue il ping di tutti gli IP nel /24 derivato dal `lanIP`
// fornito (es. lanIP=192.168.5.100 → testa 192.168.5.1 ... 192.168.5.254).
// Ritorna la lista degli IP che hanno risposto, ordine non garantito.
//
// L'IP del docente stesso e' escluso. Se `lanIP` non e' un IPv4 valido
// ritorna nil.
func Sweep(lanIP string, timeout time.Duration) []string {
	base := subnetBase(lanIP)
	if base == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	var (
		found []string
		mu    sync.Mutex
		wg    sync.WaitGroup
	)
	sem := make(chan struct{}, DefaultConcurrency)

	for i := 1; i <= 254; i++ {
		ip := base + strconv.Itoa(i)
		if ip == lanIP {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(target string) {
			defer wg.Done()
			defer func() { <-sem }()
			if pingOnce(target, timeout) {
				mu.Lock()
				found = append(found, target)
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()
	return found
}

// subnetBase ritorna il prefisso /24 di `ip` (es. "192.168.5." per
// "192.168.5.100"). Stringa vuota se l'input non e' un IPv4 valido.
func subnetBase(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		return ""
	}
	parts := strings.Split(parsed.To4().String(), ".")
	if len(parts) != 4 {
		return ""
	}
	return parts[0] + "." + parts[1] + "." + parts[2] + "."
}

// pingOnce lancia un singolo ICMP echo verso `ip` con timeout. Ritorna
// true se ping.exe (Windows) o ping (Unix) esce con codice 0.
//
// Su Windows: `ping -n 1 -w <ms> <ip>`
// Su Unix:    `ping -c 1 -W <s> <ip>` (-W secondi su Linux; macOS usa -t)
//
// Il context con timeout aggiuntivo protegge da hang del processo
// (ping.exe puo' impallarsi se il driver di rete e' in stato strano).
func pingOnce(ip string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout+500*time.Millisecond)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		ms := strconv.Itoa(int(timeout / time.Millisecond))
		cmd = exec.CommandContext(ctx, "ping", "-n", "1", "-w", ms, ip)
	case "darwin":
		// macOS: -t e' total timeout in secondi, minimo 1
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-t", "1", ip)
	default: // linux e altri Unix
		// -W in secondi, minimo 1
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", ip)
	}
	return cmd.Run() == nil
}

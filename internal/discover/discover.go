// Package discover genera la lista di IP del laboratorio scolastico
// a partire dal LAN IP del docente.
//
// Convenzione: PC studenti negli IP `.1` … `.30` del /24 del docente
// (es. docente=192.168.5.100 → studenti=192.168.5.1 … 192.168.5.30).
//
// Approccio "range fisso" senza ping/probe: e' una mappatura statica
// che permette di vedere la grid Live popolata anche prima che gli
// studenti accendano il PC. Quando un PC studente si accende e
// installa il proxy, comincia ad apparire traffico sulla card che
// gia' esiste, senza bisogno di refresh.
//
// Il range e' hardcoded per coerenza col layout standard dei
// laboratori scolastici (30 postazioni). Per range diversi l'utente
// modifica/elimina manualmente le voci dalla UI Impostazioni → Mappa
// studenti.
package discover

import (
	"net"
	"strconv"
	"strings"
)

// DefaultFirst e DefaultLast sono gli estremi (inclusivi) del range
// di IP studente generato dal LAN IP del docente.
const (
	DefaultFirst = 1
	DefaultLast  = 30
)

// DefaultRange ritorna gli IP `.DefaultFirst` … `.DefaultLast` del /24
// derivato da `lanIP`, escludendo l'IP del docente stesso.
//
// Es. lanIP=192.168.5.100 → ["192.168.5.1", "192.168.5.2", ..., "192.168.5.30"].
//
// Ritorna nil se `lanIP` non e' un IPv4 valido.
func DefaultRange(lanIP string) []string {
	base := subnetBase(lanIP)
	if base == "" {
		return nil
	}
	out := make([]string, 0, DefaultLast-DefaultFirst+1)
	for i := DefaultFirst; i <= DefaultLast; i++ {
		ip := base + strconv.Itoa(i)
		if ip == lanIP {
			continue
		}
		out = append(out, ip)
	}
	return out
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

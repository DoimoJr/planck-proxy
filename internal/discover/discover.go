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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
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

// LocalSubnet ritorna l'IPNet (IP + maschera) dell'interfaccia di rete
// che ha esattamente `lanIP`. Cosi' lo scan adatta automaticamente al
// /24, /23, /22, ecc. del PC docente — niente piu' assunzione hardcoded.
// Ritorna nil se non trova match.
func LocalSubnet(lanIP string) *net.IPNet {
	target := net.ParseIP(lanIP)
	if target == nil {
		return nil
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if ipnet.IP.To4() == nil {
				continue
			}
			if ipnet.IP.Equal(target) {
				// Normalizza a IPv4 e ritorna copia con network base.
				_, m := ipnet.Mask.Size()
				if m != 32 {
					continue
				}
				return &net.IPNet{IP: ipnet.IP.To4().Mask(ipnet.Mask), Mask: ipnet.Mask}
			}
		}
	}
	return nil
}

// enumerateSubnet ritorna tutti gli host IP della subnet `ipnet` (esclusi
// network e broadcast). Cap di sicurezza `maxHosts`: se la subnet e' piu'
// grande, ritorna nil (il caller cadra' su un /24 fallback). Senza il cap,
// uno scan /16 farebbe 65k probe TCP — troppo aggressivo per una LAN.
func enumerateSubnet(ipnet *net.IPNet, maxHosts int) []string {
	ones, bits := ipnet.Mask.Size()
	if bits != 32 {
		return nil
	}
	hostBits := bits - ones
	if hostBits > 30 {
		return nil
	}
	total := (1 << hostBits) - 2
	if total <= 0 {
		return nil
	}
	if maxHosts > 0 && total > maxHosts {
		return nil
	}
	network := ipnet.IP.Mask(ipnet.Mask).To4()
	if network == nil {
		return nil
	}
	netInt := uint32(network[0])<<24 | uint32(network[1])<<16 | uint32(network[2])<<8 | uint32(network[3])
	broadcastInt := netInt | uint32((1<<hostBits)-1)
	ips := make([]string, 0, total)
	for v := netInt + 1; v < broadcastInt; v++ {
		ip := net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
		ips = append(ips, ip.String())
	}
	return ips
}

// ScanSubnet esegue un probe TCP parallelo dell'intera subnet
// dell'interfaccia con IP `lanIP` (rilevata via LocalSubnet). Per subnet
// > /22 (1024 host) ricade su un /24 attorno a `lanIP` per evitare scan
// massivi. Worker pool a 128 connect concorrenti.
//
// Quando `veyonOnly = true` considera vivi SOLO i PC che rispondono su
// :11100 (servizio Veyon installato e attivo) — utile per filtrare i PC
// studente "preparati" rispetto a tutti i Windows in LAN. Quando false
// (default), accetta anche :445 (SMB) e :135 (RPC).
func ScanSubnet(lanIP string, perPortTimeout time.Duration, veyonOnly bool) []string {
	ipnet := LocalSubnet(lanIP)
	var hosts []string
	if ipnet != nil {
		hosts = enumerateSubnet(ipnet, 1024)
	}
	if len(hosts) == 0 {
		// Fallback: /24 implicito attorno al lanIP.
		base := subnetBase(lanIP)
		if base == "" {
			return nil
		}
		hosts = make([]string, 0, 254)
		for i := 1; i <= 254; i++ {
			hosts = append(hosts, base+strconv.Itoa(i))
		}
	}
	return scanHosts(hosts, lanIP, perPortTimeout, veyonOnly)
}

// scanHosts e' il worker comune di ScanLAN/ScanSubnet: riceve una lista
// di IP candidati e ritorna quelli che rispondono a uno dei probe TCP.
func scanHosts(hosts []string, lanIP string, perPortTimeout time.Duration, veyonOnly bool) []string {
	if perPortTimeout <= 0 {
		perPortTimeout = 300 * time.Millisecond
	}
	var probePorts []string
	if veyonOnly {
		probePorts = []string{":11100"}
	} else {
		probePorts = []string{":11100", ":445", ":135"}
	}

	const maxConcurrent = 128
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	out := []string{}
	for _, ip := range hosts {
		if ip == lanIP {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			for _, port := range probePorts {
				conn, err := net.DialTimeout("tcp", ip+port, perPortTimeout)
				if err == nil {
					_ = conn.Close()
					mu.Lock()
					out = append(out, ip)
					mu.Unlock()
					return
				}
			}
		}(ip)
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool {
		return ipLess(out[i], out[j])
	})
	return out
}

// ipLess ordina IPv4 per ottetto (es. ".10" dopo ".2"). Senza, sort
// lessicografico mette "192.168.1.10" prima di "192.168.1.2".
func ipLess(a, b string) bool {
	ai := net.ParseIP(a).To4()
	bi := net.ParseIP(b).To4()
	if ai == nil || bi == nil {
		return a < b
	}
	for i := 0; i < 4; i++ {
		if ai[i] != bi[i] {
			return ai[i] < bi[i]
		}
	}
	return false
}

// ScanLAN esegue un probe TCP parallelo sull'intera /24 di `lanIP` per
// scoprire i PC vivi. Per ogni IP del range .1-.254 (escluso `lanIP`)
// prova in sequenza le porte note dei servizi Windows piu' comuni:
//
//   - 11100 (Veyon Service): aperta su tutti gli studenti con Veyon installato
//   - 445   (SMB):          aperta di default su Windows in LAN domestica/lab
//   - 135   (RPC Endpoint): fallback ulteriore
//
// Se almeno una porta risponde entro `perPortTimeout`, l'IP e' considerato
// vivo. La scansione di tutti i 253 IP avviene in parallelo (una goroutine
// per IP), tipicamente completata in 1-2 secondi col timeout default 300ms.
//
// Vantaggio rispetto a ICMP/ping: non richiede privilegi raw socket
// (Windows blocca ICMP raw senza CAP_NET_RAW). Svantaggio: PC con tutte
// le porte chiuse (firewall stretto) non vengono trovati.
//
// Ritorna lista ordinata per ultimo ottetto. Lista vuota se `lanIP` non
// e' valido. Pensata per essere chiamata in goroutine al boot e
// periodicamente da main.
func ScanLAN(lanIP string, perPortTimeout time.Duration, veyonOnly bool) []string {
	base := subnetBase(lanIP)
	if base == "" {
		return nil
	}
	hosts := make([]string, 0, 254)
	for i := 1; i <= 254; i++ {
		hosts = append(hosts, base+strconv.Itoa(i))
	}
	return scanHosts(hosts, lanIP, perPortTimeout, veyonOnly)
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

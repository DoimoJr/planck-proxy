package sysutil

import (
	"net"
	"net/http"
	"time"
)

// WaitForPort dial-loops `addr` (es. "127.0.0.1:9999") finche' accetta
// connessioni o `timeout` scade. Ritorna true se ha ottenuto almeno una
// connessione TCP, false in caso di timeout.
//
// Usato a boot per non lanciare il browser prima che il server HTTP
// abbia completato il bind: senza questa attesa, Edge in app-mode
// puo' fare GET prima del listen e mostrare "impossibile raggiungere
// la pagina" al primo avvio.
func WaitForPort(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// WaitForHTTP polla `url` con HTTP GET finche' ottiene un response (anche
// 4xx/5xx — basta che il server risponda) o `timeout` scade. Piu' robusto
// di WaitForPort: il TCP puo' essere in LISTEN ma `http.Serve()` non
// ancora attivo nel suo loop di Accept, generando il "pagina non
// raggiungibile" su Edge anche con bind gia' completato.
func WaitForHTTP(url string, timeout time.Duration) bool {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return true
		}
		time.Sleep(80 * time.Millisecond)
	}
	return false
}

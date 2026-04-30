package veyon

import (
	"crypto/rsa"
	"fmt"
	"net"
	"time"
)

// DefaultPort e' la porta TCP di default su cui veyon-server ascolta.
const DefaultPort = 11100

// DialTimeout e' il timeout di default per Dial. Il caller puo'
// sovrascriverlo passando un *Config.
const DefaultDialTimeout = 10 * time.Second

// Config raccoglie i parametri di connessione/auth.
type Config struct {
	// Addr e' "host:port". Se ":port" manca, viene usato DefaultPort.
	Addr string

	// KeyName e' il nome assegnato alla chiave master in `veyon-cli authkeys create`.
	// Es. "teacher". Senza questo, il server non sa quale public key usare per
	// verificare la firma.
	KeyName string

	// Username e' la stringa loggata dal server (audit). Vuoto e' OK.
	Username string

	// PrivateKey e' la chiave privata RSA del docente. Vedi LoadPrivateKeyPEM.
	PrivateKey *rsa.PrivateKey

	// DialTimeout e' il timeout per la chiamata net.Dial. 0 = DefaultDialTimeout.
	DialTimeout time.Duration
}

// Conn e' una connessione autenticata al veyon-server. Permette
// di inviare comandi feature dopo Dial.
//
// Non e' concurrent-safe: una sola goroutine alla volta sui metodi.
type Conn struct {
	c          net.Conn
	cfg        Config
	serverInit ServerInit
}

// ServerInit ritorna i metadati ricevuti dal server al ClientInit.
// Width, Height, e Name sono popolati. Per Veyon non sono significativi
// (e' un control plane, non una sessione VNC vera) ma sono esposti
// per debug.
func (c *Conn) ServerInit() ServerInit { return c.serverInit }

// Dial apre una connessione a veyon-server, fa l'handshake RFB,
// negozia il security type Veyon, e completa l'auth KeyFile.
//
// In caso di successo ritorna un *Conn pronto per inviare comandi
// feature. In caso di fallimento la connessione TCP sottostante e'
// gia' stata chiusa.
func Dial(cfg Config) (*Conn, error) {
	if cfg.PrivateKey == nil {
		return nil, fmt.Errorf("veyon: PrivateKey richiesta")
	}
	if cfg.KeyName == "" {
		return nil, fmt.Errorf("veyon: KeyName richiesto")
	}
	if cfg.Addr == "" {
		return nil, fmt.Errorf("veyon: Addr richiesto")
	}
	timeout := cfg.DialTimeout
	if timeout == 0 {
		timeout = DefaultDialTimeout
	}

	// 0. TCP connect.
	tcp, err := net.DialTimeout("tcp", cfg.Addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("veyon: dial %s: %w", cfg.Addr, err)
	}

	// 1. RFB version handshake.
	if err := rfbHandshakeVersion(tcp); err != nil {
		tcp.Close()
		return nil, err
	}

	// 2. Security type negotiation.
	if err := rfbSelectSecurityType(tcp, SecTypeVeyon); err != nil {
		tcp.Close()
		return nil, err
	}

	// 3. Veyon KeyFile auth flow.
	if err := performKeyFileAuth(tcp, cfg.KeyName, cfg.Username, cfg.PrivateKey); err != nil {
		tcp.Close()
		return nil, err
	}

	// 4. RFB v3.8 ClientInit + ServerInit (RFC 6143 §7.3). Veyon richiede
	//    questo step anche se il control plane non usa il framebuffer.
	if err := rfbSendClientInit(tcp, true); err != nil {
		tcp.Close()
		return nil, err
	}
	si, err := rfbReadServerInit(tcp)
	if err != nil {
		tcp.Close()
		return nil, err
	}

	return &Conn{c: tcp, cfg: cfg, serverInit: si}, nil
}

// Close chiude la connessione TCP sottostante.
func (c *Conn) Close() error { return c.c.Close() }

// SetDeadline imposta deadline I/O (utile per timeout su comandi feature).
func (c *Conn) SetDeadline(t time.Time) error { return c.c.SetDeadline(t) }

package web

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/DoimoJr/planck-proxy/internal/state"
	"golang.org/x/crypto/bcrypt"
)

// AuthRealm e' il realm presentato dal server all'header WWW-Authenticate.
const AuthRealm = "Planck Proxy"

// RequireAuth e' un middleware che verifica HTTP Basic credentials se
// l'auth e' abilitata in state. Wraps un handler.
//
// Comportamento:
//   - Auth disabilitata in state → next() chiamato senza check
//   - Auth abilitata ma password hash vuoto → 401 (l'utente ha abilitato
//     auth dimenticando di settare la password — fail-closed)
//   - Header Authorization mancante o malformato → 401 + WWW-Authenticate
//   - Credenziali errate (user diverso o bcrypt mismatch) → 401
//   - OK → next()
func RequireAuth(s *state.State, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enabled, expectedUser, expectedHash := s.AuthInfo()
		if !enabled {
			next(w, r)
			return
		}
		if expectedHash == "" {
			authChallenge(w, "Auth abilitata ma password non impostata")
			return
		}

		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Basic ") {
			authChallenge(w, "Autenticazione richiesta")
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(header[len("Basic "):])
		if err != nil {
			authChallenge(w, "Header Authorization malformato")
			return
		}
		idx := strings.IndexByte(string(decoded), ':')
		if idx < 0 {
			authChallenge(w, "Header Authorization malformato")
			return
		}
		user := string(decoded[:idx])
		pass := string(decoded[idx+1:])

		// Confronto user constant-time per principio (non protegge davvero
		// per stringhe di lunghezza diversa — ma e' meglio di == diretto).
		if user != expectedUser {
			authChallenge(w, "Credenziali errate")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(expectedHash), []byte(pass)); err != nil {
			authChallenge(w, "Credenziali errate")
			return
		}

		next(w, r)
	}
}

// authChallenge scrive una risposta 401 con WWW-Authenticate.
func authChallenge(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="`+AuthRealm+`"`)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusUnauthorized)
	// JSON body coerente con writeError (il middleware non importa api.go).
	_, _ = w.Write([]byte(`{"ok":false,"error":"` + msg + `","code":"AUTH_REQUIRED"}`))
}

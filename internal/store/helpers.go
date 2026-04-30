package store

import "fmt"

// ErrNomeInvalido viene ritornato quando un nome (preset, classe, lab)
// non passa la sanitizzazione.
var ErrNomeInvalido = fmt.Errorf("nome invalido (caratteri ammessi: lettere, cifre, _, -)")

// sanitizeName valida nomi usati come identificatori in DB: accetta solo
// alfanumerici, underscore, trattino. Restituisce stringa vuota se invalida.
//
// Stessa logica del package internal/persist (file-based) per garantire
// retro-compatibilita' degli identificatori dopo la migrazione.
func sanitizeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			out = append(out, r)
		}
	}
	return string(out)
}

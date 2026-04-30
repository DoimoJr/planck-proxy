package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// publicFS contiene i file statici del frontend (HTML, CSS, JS)
// embeddati nel binario al momento della compilazione.
//
//go:embed all:public
var publicFS embed.FS

// StaticHandler ritorna un http.Handler che serve i file statici dal
// FS embeddato, con `public/` come root virtuale ("/" → public/index.html).
//
// Eventuali path con `..` sono respinti automaticamente da http.FileServer.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(publicFS, "public")
	if err != nil {
		// Impossibile in produzione: l'embed e' validato a compile time.
		// Fallback "directory non trovata" se mai dovesse succedere.
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}

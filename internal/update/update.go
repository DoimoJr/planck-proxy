// Package update implementa l'auto-aggiornamento del binario.
//
// Meccanica (identica su tutte le piattaforme, ma in produzione solo
// Windows ha un asset pubblicato): un eseguibile in esecuzione non si
// puo' sovrascrivere, ma si puo' RINOMINARE. Quindi:
//
//	planck.exe      -> planck.old.exe   (il processo vivo continua da qui)
//	planck.new.exe  -> planck.exe       (la nuova versione prende il posto)
//
// Poi il chiamante rilancia il binario e esce. Al boot successivo
// PuliziaResidui() cancella il .old.
//
// La verifica del download NON esegue il binario scaricato: su Windows
// il build usa -H=windowsgui (nessuna console), quindi una probe
// `--version` sarebbe cieca e inaffidabile. Verifichiamo invece la
// dimensione dichiarata dall'API GitHub e i magic bytes del formato
// eseguibile, che e' sufficiente a scartare download troncati o pagine
// di errore HTML servite al posto del binario.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// APIDefault e' l'endpoint GitHub per l'ultima release pubblicata.
// Override via PLANCK_UPDATE_API (usato dai test e per puntare a un
// server finto in sviluppo).
const APIDefault = "https://api.github.com/repos/DoimoJr/planck-proxy/releases/latest"

// timeoutHTTP vale sia per il check sia per il download: il binario e'
// ~12 MB, su una linea scolastica lenta serve margine.
const timeoutHTTP = 5 * time.Minute

// Info descrive l'esito di un controllo aggiornamenti. Serializzato
// direttamente come risposta di /api/update/check.
type Info struct {
	Corrente    string `json:"corrente"`
	Ultima      string `json:"ultima"`
	Disponibile bool   `json:"disponibile"`
	Note        string `json:"note,omitempty"`
	PaginaURL   string `json:"paginaUrl,omitempty"`
	Asset       string `json:"asset,omitempty"`
	AssetByte   int64  `json:"assetByte,omitempty"`
	assetURL    string
}

// rispostaRelease e' il sottoinsieme della release GitHub che ci serve.
type rispostaRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Assets  []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// endpoint ritorna l'URL dell'API release, con override da env.
func endpoint() string {
	if v := os.Getenv("PLANCK_UPDATE_API"); v != "" {
		return v
	}
	return APIDefault
}

// NomeAsset e' il nome del file da cercare fra gli asset della release,
// dipendente dalla piattaforma. Oggi le release pubblicano solo
// planck.exe: su macOS/Linux il check dira' "asset non disponibile".
func NomeAsset() string {
	if runtime.GOOS == "windows" {
		return "planck.exe"
	}
	return "planck"
}

// Check interroga GitHub e confronta con la versione in esecuzione.
// Errori di rete risalgono al chiamante: la UI li mostra senza
// bloccare nulla.
func Check(ctx context.Context, corrente string) (*Info, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "planck-proxy")

	cli := &http.Client{Timeout: 30 * time.Second}
	res, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contatto GitHub: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub ha risposto %d", res.StatusCode)
	}

	var rel rispostaRelease
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("risposta GitHub non valida: %w", err)
	}

	info := &Info{
		Corrente:  corrente,
		Ultima:    strings.TrimPrefix(rel.TagName, "v"),
		Note:      rel.Body,
		PaginaURL: rel.HTMLURL,
	}
	for _, a := range rel.Assets {
		if a.Name == NomeAsset() {
			info.Asset = a.Name
			info.AssetByte = a.Size
			info.assetURL = a.URL
			break
		}
	}
	// Senza asset per questa piattaforma non c'e' niente da installare,
	// anche se il tag remoto e' piu' recente.
	info.Disponibile = info.assetURL != "" && Confronta(info.Ultima, corrente) > 0
	return info, nil
}

// Confronta due versioni tipo "2.9.28" o "2.0.0-alpha.5.5".
// Ritorna >0 se a e' piu' recente di b, <0 se piu' vecchia, 0 se pari.
//
// Regole: si confrontano i segmenti numerici uno a uno; a parita' di
// numeri, una versione CON suffisso pre-release e' piu' vecchia di una
// senza (2.0.0-alpha.1 < 2.0.0).
func Confronta(a, b string) int {
	numA, preA := spezza(a)
	numB, preB := spezza(b)
	for i := 0; i < len(numA) || i < len(numB); i++ {
		var x, y int
		if i < len(numA) {
			x = numA[i]
		}
		if i < len(numB) {
			y = numB[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	switch {
	case preA == "" && preB != "":
		return 1
	case preA != "" && preB == "":
		return -1
	case preA == preB:
		return 0
	case preA > preB:
		return 1
	default:
		return -1
	}
}

// spezza separa i segmenti numerici dall'eventuale suffisso
// pre-release: "2.0.0-alpha.5" -> ([2 0 0], "alpha.5").
func spezza(v string) ([]int, string) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	pre := ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		pre = v[i+1:]
		v = v[:i]
	}
	var nums []int
	for _, seg := range strings.Split(v, ".") {
		n, err := strconv.Atoi(seg)
		if err != nil {
			break
		}
		nums = append(nums, n)
	}
	return nums, pre
}

// Percorsi ritorna i tre path coinvolti nello swap, derivati
// dall'eseguibile in esecuzione: quello corrente, il .new e il .old.
func Percorsi() (corrente, nuovo, vecchio string, err error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", "", err
	}
	// EvalSymlinks: senza, un link simbolico farebbe rinominare il link
	// invece del binario vero.
	if risolto, e := filepath.EvalSymlinks(exe); e == nil {
		exe = risolto
	}
	dir := filepath.Dir(exe)
	base := filepath.Base(exe)
	est := filepath.Ext(base)
	senza := strings.TrimSuffix(base, est)
	return exe,
		filepath.Join(dir, senza+".new"+est),
		filepath.Join(dir, senza+".old"+est),
		nil
}

// Applica scarica l'asset, lo verifica e lo mette al posto del binario
// corrente. NON riavvia: il riavvio lo fa il chiamante, che sa come
// respawnare (e cosa passare al nuovo processo).
//
// In caso di errore dopo il primo rename, tenta il rollback: senza,
// resteresti con un planck.exe mancante e l'app non ripartirebbe piu'.
func Applica(ctx context.Context, info *Info) error {
	if info == nil || info.assetURL == "" {
		return fmt.Errorf("nessun asset da scaricare per questa piattaforma")
	}
	corrente, nuovo, vecchio, err := Percorsi()
	if err != nil {
		return fmt.Errorf("percorso eseguibile: %w", err)
	}

	dati, err := scarica(ctx, info.assetURL)
	if err != nil {
		return err
	}
	if err := verifica(dati, info.AssetByte); err != nil {
		return err
	}

	// Scrittura del .new accanto all'eseguibile: se la cartella non e'
	// scrivibile (installazione in Program Files) fallisce qui, prima
	// di aver toccato il binario in uso.
	if err := os.WriteFile(nuovo, dati, 0o755); err != nil {
		return fmt.Errorf("scrittura del nuovo binario: %w (la cartella e' scrivibile?)", err)
	}

	_ = os.Remove(vecchio) // residuo di un update precedente
	if err := os.Rename(corrente, vecchio); err != nil {
		_ = os.Remove(nuovo)
		return fmt.Errorf("spostamento del binario corrente: %w", err)
	}
	if err := os.Rename(nuovo, corrente); err != nil {
		// Rollback: rimetti a posto l'originale, altrimenti al prossimo
		// avvio non c'e' piu' nessun planck da lanciare.
		_ = os.Rename(vecchio, corrente)
		_ = os.Remove(nuovo)
		return fmt.Errorf("installazione del nuovo binario: %w", err)
	}
	return nil
}

// scarica legge l'intero asset in memoria (~12 MB, accettabile) cosi'
// da poterlo verificare prima di scriverlo su disco.
func scarica(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "planck-proxy")
	cli := &http.Client{Timeout: timeoutHTTP}
	res, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: il server ha risposto %d", res.StatusCode)
	}
	// Tetto difensivo a 200 MB: evita di riempire la RAM se l'URL
	// puntasse a qualcosa di inatteso.
	return io.ReadAll(io.LimitReader(res.Body, 200<<20))
}

// verifica scarta download troncati e file che non sono eseguibili
// (tipicamente una pagina HTML di errore servita al posto del binario).
func verifica(dati []byte, attesi int64) error {
	if len(dati) == 0 {
		return fmt.Errorf("download vuoto")
	}
	if attesi > 0 && int64(len(dati)) != attesi {
		return fmt.Errorf("download incompleto: %d byte invece di %d", len(dati), attesi)
	}
	if !eseguibileValido(dati) {
		return fmt.Errorf("il file scaricato non e' un eseguibile valido")
	}
	return nil
}

// eseguibileValido controlla i magic bytes del formato atteso per la
// piattaforma corrente: PE (MZ) su Windows, Mach-O su macOS, ELF altrove.
func eseguibileValido(d []byte) bool {
	if len(d) < 4 {
		return false
	}
	switch runtime.GOOS {
	case "windows":
		return d[0] == 'M' && d[1] == 'Z'
	case "darwin":
		// Mach-O 64 bit (little/big endian) e universal binary.
		m := uint32(d[0])<<24 | uint32(d[1])<<16 | uint32(d[2])<<8 | uint32(d[3])
		return m == 0xfeedfacf || m == 0xcffaedfe || m == 0xcafebabe || m == 0xbebafeca
	default:
		return d[0] == 0x7f && d[1] == 'E' && d[2] == 'L' && d[3] == 'F'
	}
}

// PuliziaResidui cancella il binario .old lasciato da un aggiornamento
// precedente. Va chiamata al boot: finche' il vecchio processo era vivo
// il file era in uso e non si poteva rimuovere.
func PuliziaResidui() {
	if _, _, vecchio, err := Percorsi(); err == nil {
		_ = os.Remove(vecchio)
	}
}

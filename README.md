# Planck Proxy

> **Stato dei branch**:
> - `main` — v1 (Node.js), tenuto stabile finché v2 non viene tagged 2.0.0.
> - `v2-go` — **rewrite Go**, dove vive lo sviluppo attivo (alpha pubblicate). Quanto descritto sotto si riferisce a v2.

Toolkit per la **vigilanza durante le verifiche in laboratorio di informatica**. Il PC del docente fa da proxy HTTP/HTTPS per tutti i PC degli studenti, una dashboard web mostra in tempo reale chi visita cosa, e (con Veyon) permette di lockare schermi, mandare messaggi, distribuire script automaticamente e monitorare USB/processi sospetti.

**Pensato per rete LAN di fiducia**, non è una soluzione di sicurezza enterprise. Rileva e disincentiva — uno studente smaliziato può bypassarlo (vedi [Limiti](#limiti-strutturali)).

## Caratteristiche v2

### Core
- **Single binary Go** (~12 MB Win/Linux), zero dipendenze esterne. Doppio click → parte. Niente Node, niente Qt, niente cgo.
- **Proxy HTTP+HTTPS** su singola porta (CONNECT tunneling, nessun MITM).
- **Dashboard web** (Live / Report / Storico / Impostazioni) con aggiornamento real-time via SSE.
- **Classificazione traffico** in 3 categorie: AI / Utente / Sistema (~180 pattern di rumore filtrati).
- **Blocklist o allowlist** con toggle globale di pausa.
- **Persistenza SQLite** (sessioni, mappe classe, preset, watchdog events) — tutto sopravvive ai restart.

### Veyon integration (Phase 3+4)
- **Client RFB+QDataStream nativo**: lock/unlock schermo, messaggio modale, lancio applicazioni, reboot/poweroff.
- **Distribuisci proxy_on con un click**: Planck invia `proxy_on.vbs` a tutti gli studenti via Veyon FileTransfer. Esecuzione 100% silenziosa lato studente (niente cmd flash, niente popup).
- **Multi-select** Ctrl/Shift+click sulle card studente per azioni mirate.

### Watchdog plugins (Phase 5)
- **USB monitor**: avvisa quando uno studente attacca chiavette/telefoni MTP/hard disk esterni. Filtra HID/audio integrati.
- **Process monitor**: avvisa su `cmd, powershell, regedit, taskmgr, mmc, gpedit, perfmon, msconfig`.
- **Framework estensibile**: nuovi plugin = un PowerShell script + 5 metodi Go.

### UX
- **Empty state guidate** con CTA chiare (es. "Distribuisci proxy ora" quando manca il setup).
- **Toast** non-modali per confirm/error (niente popup `alert()` molesti).
- **Keyboard shortcuts**: `Ctrl+1..4` switch tab, `Ctrl+S` start/stop sessione, `Ctrl+P` pausa, `Ctrl+F` filtro, `ESC` deseleziona, `Ctrl+A` seleziona tutti.
- **Tema chiaro/scuro** persistito.

## Quick start (5 min)

### 1. Download
Vai sulla pagina [Releases](https://github.com/DoimoJr/planck-proxy/releases) e scarica l'ultimo `planck.exe` (Windows) o `planck-linux` (Linux x64).

Mettilo in una cartella sul PC docente, esempio `C:\Planck\`.

### 2. Lancia
Doppio click su `planck.exe`. Si apre automaticamente il browser su `http://localhost:9999`. Login (se l'hai abilitato): `docente` / password che hai impostato.

Al primo boot Planck:
- Genera `planck.db` (SQLite) accanto al binario
- Genera `proxy_on.vbs` e `proxy_off.vbs` con il tuo IP LAN auto-detectato
- Apre la dashboard

Verifica nei log al boot la riga `Script studenti pronti: ... (IP X.Y.Z.W:9090)` — quello è l'IP che gli studenti useranno.

### 3. Configura la classe
Tab **Impostazioni**:
- Aggiungi gli studenti (IP → nome) nella card "Mappa studenti", o salva una combinazione `classe`+`laboratorio` per riusarla
- (Opzionale ma consigliato) Importa la chiave master Veyon nella card "Veyon"
- (Opzionale) Attiva i plugin watchdog (USB, Process)

### 4. Distribuisci proxy
Tab **Live** → toolbar **Azioni classe** → **📁 Distribuisci proxy**. Veyon trasferisce silenziosamente `proxy_on.vbs` su ogni PC studente, attivando il proxy + watchdog. Le card studente nel pannello iniziano a popolarsi col traffico.

A fine ora, **🚫 Rimuovi proxy** disattiva tutto.

## Setup Veyon (consigliato)

Veyon è il backbone per l'integrazione: distribuzione automatica del proxy, lock schermo, messaggi.

### Su un PC qualsiasi (può essere il PC docente)
- Installa Veyon (https://veyon.io)
- Apri **Veyon Configurator** → tab **Authentication keys** → **Create new key pair** → nome `teacher`
- Esporta la chiave pubblica → copiala su ogni PC studente

### Su ogni PC studente
- Installa Veyon (Service + Configurator)
- Importa la chiave pubblica via Configurator
- Imposta `Authentication method` = **Key file authentication**
- Riavvia il servizio Veyon

### Su Planck
- Tab **Impostazioni** → card **Veyon** → incolla la chiave **privata** PEM, nome chiave `teacher`, salva
- Test connessione verso un IP studente → deve diventare verde
- Da quel momento, tutti i bottoni Veyon (lock, msg, distribuisci) sono attivi

## Watchdog plugins

I plugin sono script PowerShell che girano sul PC studente e segnalano eventi a Planck via HTTP:

| Plugin | Cosa rileva | Severity |
|---|---|---|
| **USB** | Connessione di dispositivi USB di classe non sicura | warning su "added" |
| **Process** | Avvio di processi nella denylist (cmd, regedit, ...) | warning |

Per attivarli: **Impostazioni** → card **Watchdog plugins** → toggle ON → click **Distribuisci proxy** (la modifica si propaga al prossimo deploy).

Gli eventi appaiono nel pannello "Eventi watchdog" del tab Live, e come badge ⚠️ sulle card studente.

## File generati nella cartella di Planck

| File | Contenuto |
|---|---|
| `planck.db`, `planck.db-shm`, `planck.db-wal` | DB SQLite (sessioni, eventi, config) |
| `proxy_on.vbs`, `proxy_off.vbs` | Script studenti con IP+porta corretti, distribuibili anche manualmente |
| `veyon-master.pem` | Chiave Veyon importata (permessi 0600) |
| `*.v1.bak` | Migrazione automatica dal layout file-based v1 (al primo boot dopo upgrade) |

## Variabili d'ambiente

| Var | Scopo | Default |
|---|---|---|
| `PLANCK_DATA_DIR` | Cartella per `planck.db` + bat/vbs | dir del binario |
| `PLANCK_WEB_PORT` | Porta web/API/dashboard | 9999 |
| `PLANCK_PROXY_PORT` | Porta proxy HTTP/HTTPS | 9090 |
| `PLANCK_LAN_IP` | Override IP host (se l'auto-detect sbaglia) | UDP-dial trick |
| `PLANCK_NO_BROWSER` | Skip apertura automatica del browser | (apre Edge in modalità app) |

## API REST

Endpoint principali (tutti `/api/...`, dietro auth Basic se abilitata):

| Path | Cosa |
|---|---|
| `GET /api/config`, `/api/history`, `/api/settings` | Snapshot per idratazione UI |
| `GET /api/stream` | SSE per aggiornamenti real-time |
| `POST /api/block`, `/unblock`, `/block-all-ai`, ... | Mutazioni blocklist |
| `POST /api/session/{start,stop}` | Lifecycle sessione |
| `POST /api/veyon/configure`, `/test`, `/feature` | Veyon control |
| `POST /api/veyon/distribuisci-proxy` | Distribuisci `proxy_on.vbs` ai target |
| `POST /api/watchdog/config`, `/event` | Watchdog plugins |
| `GET /api/scripts/{proxy_on,proxy_off}.vbs` | Download script studente manuale |

## Limiti strutturali

- **Privilegi studente**: il proxy è settato in `HKCU` (no UAC). Lo studente può rimuoverlo manualmente se sa cercare. Veyon ScreenLock è overlay-based, bypassabile chiudendo Veyon Service da TaskManager (UAC permettendo).
- **HTTPS senza MITM**: vediamo SOLO il dominio (Host header / SNI), non il path. Va bene per "questo studente è andato su chatgpt.com", non per "questo studente ha scritto X".
- **AI list hardcoded**: ~129 domini "AI" nella lista interna, da aggiornare con PR. Auto-classification AI è in roadmap (v2.1).
- **No rete air-gapped**: il `PLANCK_LAN_IP` UDP-dial trick richiede connettività verso `8.8.8.8`. Su rete senza internet, override manuale dell'env var.

## Architettura

Vedi [`ARCHITECTURE.md`](./ARCHITECTURE.md) per il design e [`SPEC.md`](./SPEC.md) per la specifica funzionale completa (incluso il protocollo Veyon documentato e l'organizzazione dei plugin watchdog).

## Roadmap

| Fase | Stato |
|---|---|
| Phase 1 — Backend port + monitor sempre attivo | ✅ alpha.1-2 |
| Phase 2 — Persistenza SQLite | ✅ alpha.3 |
| Phase 3 — Veyon protocol (RFB + auth keyfile) | ✅ alpha.4 |
| Phase 4 — Veyon UI (Lock/Msg/Distribuisci/Power) | ✅ alpha.4-4.1 |
| Phase 5 — Watchdog plugins (USB, Process) | ✅ alpha.5-5.4 |
| Phase 8 — Polish + release v2.0 MVP | 🚧 in corso |
| Phase 5.x — Watchdog Network plugin / settings UI | future |
| Phase 6 — Auto-classification AI | v2.1 |
| Phase 7 — Reazioni automatiche | v2.1 |
| Tab Storico cross-session | v2.2 |

## Build da sorgenti

Richiede Go 1.22+:
```sh
git clone https://github.com/DoimoJr/planck-proxy
cd planck-proxy
git checkout v2-go
go build -ldflags="-s -w" -o planck.exe ./cmd/planck/      # Windows
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o planck-linux ./cmd/planck/   # Linux cross
```

Test:
```sh
go test ./...                                      # unit
go test -tags integration ./internal/veyon/        # contro Docker rig (vedi test/veyon-rig/)
```

## Licenza

MIT — vedi [`LICENSE`](./LICENSE).

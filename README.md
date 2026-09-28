# Planck Proxy

Toolkit per la **vigilanza durante le verifiche in laboratorio di informatica**. Il PC del docente fa da proxy HTTP/HTTPS per tutti i PC degli studenti, una dashboard web mostra in tempo reale chi visita cosa, e (con Veyon) permette di lockare schermi, mandare messaggi, distribuire script automaticamente e monitorare USB/processi sospetti.

**Pensato per rete LAN di fiducia**, non è una soluzione di sicurezza enterprise. Rileva e disincentiva — uno studente smaliziato può bypassarlo (vedi [Limiti](#limiti-strutturali)).

## Caratteristiche

### Core
- **Single binary Go** (~12 MB Win/Linux), zero dipendenze esterne. Doppio click → parte. Niente Node, niente Qt, niente cgo.
- **Proxy HTTP+HTTPS** su singola porta (CONNECT tunneling, nessun MITM).
- **Dashboard web** (Live / Report / Storico / Impostazioni) con aggiornamento real-time via SSE.
- **Classificazione traffico** in 3 categorie: AI / Utente / Sistema (~180 pattern di rumore filtrati).
- **Blocklist o allowlist** con toggle globale di pausa.
- **Persistenza SQLite** (sessioni, preset, watchdog events, config) — tutto sopravvive ai restart.
- **Aggiornamento dall'interfaccia**: Impostazioni → Sistema → Aggiornamenti scarica l'ultima release, la installa e riavvia da sola. Bloccato durante una verifica in corso.
- **Portatile fra laboratori**: nessun setup di classe da mantenere. A ogni boot Planck rigenera la mappa IP e la chiave Veyon dal PC su cui gira, quindi la stessa chiavetta funziona in qualsiasi aula.
- **Lista domini AI auto-aggiornata** dal repo GitHub, con copia integrata nel binario come fallback offline.

### Integrazione Veyon
- **Client RFB+QDataStream nativo**: lock/unlock schermo, messaggio modale, lancio applicazioni, reboot/poweroff.
- **Distribuisci proxy_on con un click**: Planck invia `proxy_on.vbs` a tutti gli studenti via Veyon FileTransfer. Esecuzione 100% silenziosa lato studente (niente cmd flash, niente popup).
- **Multi-select** Ctrl/Shift+click sulle card studente per azioni mirate.

### Watchdog plugins
- **USB monitor**: avvisa quando uno studente attacca chiavette/telefoni MTP/hard disk esterni. Filtra HID/audio integrati.
- **Process monitor**: avvisa su `cmd, powershell, regedit, taskmgr, mmc, gpedit, perfmon, msconfig`.
- **Network monitor**: avvisa quando compare una nuova interfaccia di rete — tethering da telefono, hotspot, VPN, dongle 4G.
- **Configurazione editabile dalla UI**: ogni plugin ha il suo JSON modificabile in Impostazioni → Watchdog.
- **Framework estensibile**: nuovi plugin = un PowerShell script + 5 metodi Go.

### UX
- **Empty state guidate** con CTA chiare (es. "Distribuisci proxy ora" quando manca il setup).
- **Toast** non-modali per esiti e errori. I confirm distruttivi (aggiorna e riavvia, reboot, poweroff, elimina) restano invece nativi **per scelta**: sono volutamente brutti, così non li si clicca per sbaglio.
- **Keyboard shortcuts**: `Ctrl+1..4` switch tab, `Ctrl+S` start/stop sessione, `Ctrl+P` pausa, `Ctrl+F` filtro, `ESC` deseleziona, `Ctrl+A` seleziona tutti.
- **Tema chiaro/scuro** persistito.

## Quick start (5 min)

### 1. Download
Vai sulla pagina [Releases](https://github.com/DoimoJr/planck-proxy/releases) e scarica l'ultimo `planck.exe`.

Mettilo in una cartella **scrivibile** sul PC docente — `C:\Planck\` o una chiavetta USB vanno bene, `Program Files` no: l'aggiornamento automatico deve poter sostituire il proprio eseguibile.

Il codice compila anche per Linux e macOS (un solo file platform-specific), ma le release pubblicano solo il binario Windows: altrove vai da sorgenti.

### 2. Lancia
Doppio click su `planck.exe`. Si apre automaticamente il browser su `http://localhost:9999`. Login (se l'hai abilitato): `docente` / password che hai impostato.

Al primo boot Planck:
- Genera `planck.db` (SQLite) accanto al binario
- Genera `proxy_on.vbs` e `proxy_off.vbs` con il tuo IP LAN auto-detectato
- Apre la dashboard

Verifica nei log al boot la riga `Script studenti pronti: ... (IP X.Y.Z.W:9090)` — quello è l'IP che gli studenti useranno.

### 3. Configura (poco)
Non c'è nessuna classe da registrare: le postazioni sono già lì. Planck genera da solo le card per gli IP `.1`–`.30` del /24 del PC docente, etichettate con l'IP.

Tab **Impostazioni**:
- (Consigliato) Importa la chiave master Veyon nella sezione **Veyon**
- (Opzionale) Attiva i plugin watchdog nella sezione **Watchdog** — da lì si modifica anche la loro configurazione JSON

### 4. Distribuisci proxy
Tab **Live** → toolbar **Azioni classe** → **📁 Distribuisci proxy**. Veyon trasferisce silenziosamente `proxy_on.vbs` su ogni PC studente, attivando il proxy + watchdog. Le card studente nel pannello iniziano a popolarsi col traffico.

A fine ora, **🚫 Rimuovi proxy** disattiva tutto.

### 5. Tenerlo aggiornato
Impostazioni → **Sistema** → **Aggiornamenti**: `Controlla ora`, e se c'è una versione nuova `Aggiorna e riavvia`. Planck scarica, si sostituisce e riparte da solo; la finestra resta aperta e si ricarica a riavvio finito.

Quando il controllo automatico al boot trova una versione nuova compare un badge in alto a destra.

A sessione attiva l'aggiornamento è **bloccato**: il riavvio stacca il proxy per un paio di secondi, e i browser degli studenti darebbero errore a metà verifica. Ferma la sessione, aggiorna, riparti.

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
| **USB** | Connessione di dispositivi USB di classe non sicura (chiavette, telefoni MTP, dischi esterni) | warning su "added" |
| **Process** | Avvio di processi nella denylist (cmd, powershell, regedit, taskmgr, ...) | warning |
| **Network** | Comparsa di una nuova interfaccia di rete: tethering USB, hotspot Wi-Fi, VPN, dongle 4G | warning |

Per attivarli: **Impostazioni** → card **Watchdog plugins** → toggle ON → click **Distribuisci proxy** (la modifica si propaga al prossimo deploy).

Gli eventi appaiono nel pannello "Eventi watchdog" del tab Live, e come badge ⚠️ sulle card studente.

## File generati nella cartella di Planck

| File | Contenuto |
|---|---|
| `planck.db`, `planck.db-shm`, `planck.db-wal` | DB SQLite (sessioni, eventi, config) |
| `proxy_on.vbs`, `proxy_off.vbs` | Script studenti con IP+porta corretti, distribuibili anche manualmente |
| `veyon-master.pem` | Chiave Veyon importata (permessi 0600) |
| `planck.log`, `planck.pid` | Log di esecuzione e PID dell'istanza in corso |
| `sessioni/*.v1.bak` | Migrazione automatica dal layout file-based v1 (al primo boot dopo upgrade). Contengono IP e domini per studente: esclusi da git |
| `planck.old.exe` | Eseguibile precedente, lasciato da un aggiornamento. Cancellato al boot successivo |

## Variabili d'ambiente

| Var | Scopo | Default |
|---|---|---|
| `PLANCK_DATA_DIR` | Cartella per `planck.db` + bat/vbs | dir del binario |
| `PLANCK_WEB_PORT` | Porta web/API/dashboard | 9999 |
| `PLANCK_PROXY_PORT` | Porta proxy HTTP/HTTPS | 9090 |
| `PLANCK_LAN_IP` | Override IP host (se l'auto-detect sbaglia) | UDP-dial trick |
| `PLANCK_NO_BROWSER` | Skip apertura automatica del browser | (apre Edge in modalità app) |
| `PLANCK_UPDATE_API` | Endpoint della release da cui aggiornare (per test contro un server finto) | API GitHub del repo |
| `PLANCK_ADOPT_BROWSER_PID` | PID della finestra browser da sorvegliare invece di aprirne una nuova. Lo imposta Planck stesso quando si riavvia dopo un aggiornamento | — |
| `PLANCK_DISCOVER_VEYON_ONLY` | Limita la discovery ai soli host con Veyon attivo | — |

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
| `GET /api/update/check` | Controlla se esiste una release più recente |
| `POST /api/update/apply` | Scarica, installa e riavvia. `409` se c'è una sessione in corso |

## Limiti strutturali

- **Privilegi studente**: il proxy è settato in `HKCU` (no UAC). Lo studente può rimuoverlo manualmente se sa cercare. Veyon ScreenLock è overlay-based, bypassabile chiudendo Veyon Service da TaskManager (UAC permettendo).
- **HTTPS senza MITM**: vediamo SOLO il dominio (Host header / SNI), non il path. Va bene per "questo studente è andato su chatgpt.com", non per "questo studente ha scritto X".
- **Lista AI per domini noti**: la lista viene aggiornata dal repo GitHub a ogni avvio (con fallback sulla copia integrata), ma resta un elenco di domini: un servizio AI nuovo o self-hosted non ci finisce finché qualcuno non apre una PR su [`data/ai-domains.txt`](./data/ai-domains.txt).
- **Aggiornamento automatico e antivirus**: Planck sostituisce il proprio `.exe` e riavvia. Su alcune configurazioni Windows SmartScreen o l'antivirus possono intervenire sul binario scaricato; in quel caso resta il download manuale dalle Releases.
- **No rete air-gapped**: il `PLANCK_LAN_IP` UDP-dial trick richiede connettività verso `8.8.8.8`. Su rete senza internet, override manuale dell'env var.

## Architettura

Vedi [`ARCHITECTURE.md`](./ARCHITECTURE.md) per il design e [`SPEC.md`](./SPEC.md) per la specifica funzionale completa (incluso il protocollo Veyon documentato e l'organizzazione dei plugin watchdog).

## Roadmap

| Fase | Stato |
|---|---|
| Backend port + monitor sempre attivo | ✅ v2.0 |
| Persistenza SQLite | ✅ v2.0 |
| Veyon protocol (RFB + auth keyfile) | ✅ v2.0 |
| Veyon UI (Lock / Msg / Distribuisci / Power) | ✅ v2.0 |
| Watchdog plugins (USB, Process) | ✅ v2.0 |
| Editor UI per la config dei plugin | ✅ v2.1 |
| Watchdog Network plugin + heartbeat detection | ✅ v2.2 |
| Lista AI auto-aggiornata da GitHub | ✅ v2.3 |
| Portabilità fra laboratori (niente setup classe) | ✅ v2.6 |
| Tab Storico cross-session | ✅ v2.9 |
| Lockdown Firefox via `policies.json` | ✅ v2.9.24 |
| Aggiornamento dall'interfaccia | ✅ v2.9.29 |
| Reazioni automatiche agli eventi watchdog | da fare |

## Build da sorgenti

Richiede Go 1.25+:
```sh
git clone https://github.com/DoimoJr/planck-proxy
cd planck-proxy
./build.sh      # Linux/macOS -> ./planck
build.bat       # Windows     -> planck.exe
```

Usa gli script, non un `go build` nudo: iniettano la versione dal tag git
(`-X main.Versione=...`). Con un build senza quel flag il binario si dichiara
`0.0.0-dev` e il controllo aggiornamenti si comporta di conseguenza.

Cross-compile da qualsiasi piattaforma (niente cgo, SQLite è Go puro):
```sh
GOOS=windows GOARCH=amd64 go build -trimpath \
  -ldflags="-s -w -H=windowsgui -X main.Versione=$(git describe --tags --abbrev=0 | sed 's/^v//')" \
  -o planck.exe ./cmd/planck
```

Test:
```sh
go test ./...                                      # unit
go test -tags integration ./internal/veyon/        # contro Docker rig (vedi test/veyon-rig/)
```

## Licenza

MIT — vedi [`LICENSE`](./LICENSE).

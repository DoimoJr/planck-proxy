# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Purpose

Classroom-invigilation toolkit used by a teacher during exams: the teacher's PC runs a proxy that all student PCs are forced through, and a web dashboard (3 tabs: Live / Report / Impostazioni) shows live traffic per-IP, flags AI-assistant domains, blocks sites on the fly, supports session lifecycle (pause/deadline/archive), and exports session logs. Everything (code, UI, commits) is in **Italian** and should stay that way.

**Portable by design**: the folder is self-contained. No `npm install`, no build step. `.bat` files on student PCs are distributed manually (Veyon or similar).

Pubblicato come **`DoimoJr/planck-proxy`** su GitHub (MIT). `node.exe` (~91 MB) e' escluso dal repo via `.gitignore`: l'utente lo scarica da [nodejs.org](https://nodejs.org/en/download) e lo mette nella radice del progetto (vedi README). In locale la cartella si chiama ancora `consegna_portable/` (path originale), sul repo il nome e' `planck-proxy`.

## Layout

```
consegna_portable/
├── node.exe                    Node.js runtime (~91 MB, NON in git — scaricato separatamente)
├── avvia.bat                   launcher
├── server.js                   proxy + web + API + SSE (single process)
├── domains.js                  DOMINI_AI + PATTERN_SISTEMA + classifica()
├── config.json                 ports, auth, titolo, classe, modo, inattivita, dominiIgnorati
├── studenti.json               IP -> nome mapping (keys prefixed "_" are ignored)
├── blocked.html                403 page served to blocked students (replaceable)
├── presets/                    saved blocklist snapshots
├── classi/                     saved student-map snapshots (swap classe/lab via dropdown)
├── sessioni/                   auto-archived past sessions (JSON per session)
├── public/                     static assets served on web port
│   ├── index.html              3 tab structure (Live / Report / Impostazioni)
│   ├── monitor.css             CSS custom props + darkmode via body.dark
│   └── js/                     ES modules
│       ├── app.js              entry: init + event delegation
│       ├── state.js            shared state + persistence helpers + aggregaPerReport
│       ├── render.js           pure render functions; renderAll() throttled via requestAnimationFrame
│       ├── actions.js          API calls + state mutations (no DOM)
│       ├── sse.js              EventSource + reconnect + beep + desktop notif
│       └── util.js             $, escapeHtml, ip2long, parseOra, formatRelativo, formatDurata
├── proxy_on.bat                student-side (distribute via Veyon)
└── proxy_off.bat
```

Runtime state (auto-created, not committed): `_blocked_domains.txt`, `_traffico_log.txt`, `sessioni/*.json`.

## Run / test

- **Start**: `avvia.bat` → runs `server.js`. Proxy on `:9090`, monitor UI on `:9999`, both `0.0.0.0`.
- **Student setup**: edit `IP_PROF` in `proxy_on.bat`, distribute via Veyon, run.
- **Smoke tests** (server running):
  - `./node.exe --check server.js domains.js public/js/app.js ...`
  - `curl -x http://127.0.0.1:9090 http://example.com` → 200 (or 403 if blocked / paused)
  - `curl http://127.0.0.1:9999/api/config` → config + domains + studenti + presets
  - `curl http://127.0.0.1:9999/api/pausa/on` → subsequent proxy requests get 403
  - `curl 'http://127.0.0.1:9999/api/deadline/set?time=23:59'` → deadline ISO returned
  - `curl http://127.0.0.1:9999/api/session/start` → archives previous session into `sessioni/`
  - `curl http://127.0.0.1:9999/api/sessioni` → list of archived session files
  - `curl -I http://127.0.0.1:9999/api/export` → `Content-Disposition: attachment`

## Architecture

Single Node process, two HTTP servers, shared in-memory state:

```
student PC ──HTTP/HTTPS──► :9090 proxy ──► origin
                              │
                              ▼ ring buffer (MAX_STORIA=5000) + SSE broadcast
                          :9999 web/API ──► monitor UI (public/)
                              │
                              ▼ persistence
                          _blocked_domains.txt   (blocklist / allowlist, one line per domain)
                          _traffico_log.txt      (audit append, not read back)
                          presets/*.json         (saved blocklist snapshots)
                          sessioni/*.json        (auto-archived past sessions)
```

### Session lifecycle

- **Esplicito Avvia/Ferma**: al boot `sessioneAttiva = false`, `sessioneInizio = null`. Il proxy e' gia' attivo (blocchi applicati, watchdog funzionante), ma **nessun traffico viene registrato** finche' l'utente non preme "Avvia sessione" nella UI (→ `GET /api/session/start`). Rationale: il comportamento "registra sempre e archivia alla nuova" era scomodo sul campo (test 2026-04-22) — la teacher vuole decidere esplicitamente quando un esame inizia e quando finisce.
- `GET /api/session/start`: se esiste gia' una sessione con dati, la archivia in `sessioni/<sessioneInizio>.json`; azzera ring buffer + log; imposta `sessioneInizio = now`, `sessioneFineISO = null`, `sessioneAttiva = true`.
- `GET /api/session/stop`: setta `sessioneAttiva = false`, memorizza `sessioneFineISO = now`, **archivia subito** (se ha dati) in `sessioni/<sessioneInizio>.json`. Il buffer resta visibile in UI per revisione. Rationale: se il PC si spegne dopo lo stop i dati sono gia' salvati; la teacher puo' sempre "Elimina archivio" se vuole scartare.
- `GET /api/session/start`: nel flusso normale (stopped → start) NON archivia (lo ha gia' fatto Stop) — azzera buffer, nuovo `sessioneInizio`, `sessioneAttiva=true`. Caso difensivo: se la sessione era ancora attiva (chiamata API diretta che salta Stop), prima archivia, altrimenti il buffer andrebbe perso.
- Nel proxy, `registraTraffico()` ha un early-return su `!sessioneAttiva`: traffico "fuori sessione" non viene loggato ne' messo nel buffer ne' broadcastato via SSE. Watchdog (`/_alive`) funziona comunque — la `aliveMap` si aggiorna a prescindere.
- **Graceful shutdown** (SIGINT/SIGTERM) triggera `archiviaSessioneCorrente()` anche se la sessione e' ferma, purche' ci siano dati in buffer: Ctrl+C non perde i dati della sessione appena fermata.
- `GET /api/export` streama il buffer corrente come JSON download — funziona sia a sessione attiva che ferma.
- **UI**: bottone toggle nella toolbar Live (id `btn-sessione`): label "Avvia sessione" (verde) ↔ "Ferma sessione" (rosso). L'indicatore `stat-modo` mostra "SESSIONE FERMA" quando `!sessioneAttiva`. La durata (`stat-durata`) si congela al momento del Ferma (calcolata su `sessioneFineISO` invece di `Date.now()`).
- **SSE**: messaggio `session-state` con `{sessioneAttiva, sessioneInizio, sessioneFineISO}` broadcastato ad ogni start/stop. Il vecchio messaggio `reset` continua a essere broadcastato su `/api/session/start` per trigger pulizia buffer client.
- Il Report tab mostra aggregazioni sulla sessione corrente (anche se ferma) o su una archiviata (dropdown in Impostazioni + Report).

### Modes: blocklist vs allowlist vs pause

`dominioBloccato()` in `server.js` combines three checks in order:
1. If `pausato === true` and the domain is not in `dominiIgnorati`, block.
2. Otherwise, if `config.modo === 'allowlist'`: block unless the domain matches `bloccati` or is in `dominiIgnorati`.
3. Otherwise (default blocklist): block only if matches `bloccati`.

`dominiIgnorati` is **always allowed** (must be — without it, in pause or allowlist the student browser can't even reach localhost, Windows background services, etc.).

### Deadline / countdown

- `GET /api/deadline/set?time=HH:MM` interprets the time as local, resolves to "today or next day if already past", stores as ISO, and schedules a `setTimeout` that broadcasts `{type: 'deadline-reached'}` when it expires.
- `GET /api/deadline/clear` cancels.
- The client shows a live countdown (updated every second by a `setInterval` tied to `renderCountdown()` only — cheaper than full render). It turns warning orange < 5 min, red+blink < 1 min, "SCADUTO" at zero.
- On `deadline-reached`, the client flashes the AI banner with "TEMPO SCADUTO", fires 3 beeps, and surfaces a desktop notification.

### API surface

| Endpoint | Purpose |
|---|---|
| `GET /api/config` | `{titolo, classe, modo, inattivitaSogliaSec, dominiAI, patternSistema, studenti, presets}` |
| `GET /api/history` | `{entries, bloccati, sessioneAttiva, sessioneInizio, sessioneFineISO, pausato, deadlineISO, alive}` - full hydrate |
| `GET /api/block?domain=X` / `unblock?domain=X` | Toggle single |
| `GET /api/block-all-ai` / `unblock-all-ai` | Bulk via `DOMINI_AI` |
| `GET /api/clear-blocklist` | Empty list |
| `GET /api/presets` / `preset/load?nome=X` / `preset/save?nome=X` | Blocklist snapshots. Filename sanitized to `[a-zA-Z0-9_-]`. |
| `GET /api/session/start` | Archive previous (if any) + clear + new `sessioneInizio` + set `sessioneAttiva = true` |
| `GET /api/session/stop` | Set `sessioneAttiva = false` + store `sessioneFineISO` + archivia subito (se ha dati). Non azzera il buffer. |
| `GET /api/session/status` | `{sessioneAttiva, sessioneInizio, sessioneFineISO, durataSec, richieste, bloccati, pausato, deadlineISO}` |
| `GET /api/export` | Current session as JSON download |
| `GET /api/reset` | Clear ring buffer only (keep blocklist / session start) |
| `GET /api/sessioni` | List archived filenames (newest first) |
| `GET /api/sessioni/load?nome=X.json` | Read archived session. Filename strictly validated. |
| `GET /api/sessioni/delete?nome=X.json` | Delete archived. |
| `GET /api/sessioni/archivia` | Force archive current (without starting new) |
| `GET /api/pausa/toggle` / `on` / `off` | Global pause |
| `GET /api/deadline/set?time=HH:MM` / `clear` | Countdown |
| `GET /api/settings` | Full config (password masked: `{password:"",passwordSet:true}`) |
| `POST /api/settings/update` | Body JSON `{key: value, ...}` with dotted keys (e.g. `"web.auth.enabled"`). Validates per-key, rejects invalid, persists to `config.json`, reports `richiedeRiavvio` for keys that don't apply at runtime |
| `GET /api/settings/ignorati/add?dominio=X` / `remove?dominio=X` | Mutate `dominiIgnorati` at runtime |
| `GET /api/reload-studenti` | Re-read `studenti.json` from disk |
| `GET /api/studenti/set?ip=X&nome=Y` | Upsert one student; persists `studenti.json`; broadcasts |
| `GET /api/studenti/delete?ip=X` | Remove one; persists; broadcasts |
| `GET /api/studenti/clear` | Empty the map; persists; broadcasts |
| `GET /api/classi` | List saved maps as `[{classe, lab, file}, ...]` |
| `GET /api/classi/load?classe=X&lab=Y` | Replace current student map with that combo's contents; persists |
| `GET /api/classi/save?classe=X&lab=Y` | Snapshot current map as `classi/<classe>--<lab>.json` with `{classe, lab, mappa}` metadata |
| `GET /api/classi/delete?classe=X&lab=Y` | Remove snapshot |
| `GET /api/stream` | SSE. Messages: `traffic`, `blocklist`, `reset`, `studenti`, `classi`, `settings`, `pausa`, `deadline`, `deadline-reached`, `alive`, `session-state` |
| `GET /_alive` (on **proxy** port) | Watchdog keepalive. Not a JSON API: served by the proxy, not the web server. Returns `ok` and registers the client IP + timestamp in `aliveMap`. |

HTTP Basic auth guards **all** endpoints when `config.web.auth.enabled: true`.

### Vista IP: griglia vs lista

Il pannello "Traffico per IP" nella Live tab supporta due modi di visualizzazione (toggle nell'header del pannello, persistito in `localStorage.vistaIp`):
- **griglia** (default): una card per studente in una CSS grid auto-fill (`grid-template-columns: repeat(auto-fill, minmax(200px, 1fr))`). Ogni card mostra WD dot, nome+IP, conteggio N (grande), ultima attivita', e i primi `DOMINI_CARD_MAX=6` domini come tag (piu' recenti in cima) + "+N" per gli altri. Pensata per vedere 15–30 studenti in un colpo d'occhio senza scroll.
- **lista**: la tabella precedente (5 colonne). Utile quando servono i domini completi per riga.

Implementazione: `renderTabellaIp()` e' un dispatcher che legge `state.vistaIp` e delega a `renderListaIp()` o `renderGrigliaIp()`. Entrambe riusano `calcolaStatoIp(ip)` per i dati derivati (listaAttive, dominiMap, wd, inattivo) — zero duplicazione della logica di business. Il container e' un unico `<div id="ip-container">` che viene riscritto completamente (table o grid) a ogni render throttled; la toolbar `.view-toggle` ha due bottoni `data-action="vista-griglia"/"vista-lista"`.

### Frontend architecture

- **Single mutable `state` object** in `state.js`; persistence in `localStorage` per: `nascosti`, `darkmode`, `notifiche`, `tabAttivo`, `vistaIp` (griglia|lista), `sidebarCollassata`, `richiesteCollassate`.
- **Render throttling**: `renderAll()` wraps `_renderAllSync()` with a `requestAnimationFrame` guard — multiple calls in the same frame coalesce into one paint. Critical during SSE event bursts (20+ students simultaneously).
- **Countdown ticks independently** every 1s via a dedicated `setInterval(renderCountdown, 1000)` — doesn't trigger full renders.
- **Tabs**: three `<section class="tab-panel">`; `renderTabs()` toggles the `.active` class. Switching into Report or Impostazioni triggers a fresh `ricaricaSessioni()`.
- **Event delegation**: zero inline `onclick`. Every actionable element carries `data-action` (+ optional `data-dominio`/`data-ip`/`data-nome`/`data-tab`/`data-sezione`). Three delegated listeners (`click`, `input`, `change`) on `document.body` in `app.js`.
- **Client-side aggregation**: `state.perIp`, `state.perDominio`, `state.ultimaPerIp` rebuilt on hydrate and incrementally on each SSE `traffic` event. `aggregaPerReport(entries)` is called on demand when Report tab renders — operates on either current `state.entries` or the loaded archive.

### Archive viewing

When the user selects an archived session in Impostazioni (dropdown) or clicks a row in the sessioni list:
- `state.sessioneVisualizzata` is set to the filename
- `state.datiSessioneVisualizzata` holds the fetched content
- `state.tabAttivo` is forced to `'report'`
- `renderReport()` branches on `state.datiSessioneVisualizzata` to show archived data instead of live state
- Leaving the Report tab clears the archive view; returning shows current session again

## Conventions and gotchas

- **Italian everywhere**: `dominio`, `bloccato`, `sessione`, `pausato`, `deadlineISO`. Keep it.
- **Substring matching for blocks**: `instagram` catches `www.instagram.com`. Intentional.
- **HTTPS blocking uses CONNECT hostname only**. No MITM, no fake cert.
- **Two ports, one process**. Changing `config.proxy.port` requires updating `PORTA=` in `proxy_on.bat` (distributed separately, does not read `config.json`).
- **`dominiIgnorati` is pre-filter + whitelist**: matching hostnames are dropped before classification **and** are always allowed through the proxy, even in pause/allowlist modes. This is essential or nothing works (Windows background, localhost, etc.).
- **`DOMINI_AI` / `PATTERN_SISTEMA` live only in `domains.js`**. Frontend gets them via `/api/config`.
- **"Sistema" e' rumore: escluso dai conteggi per-studente**. `PATTERN_SISTEMA` e' una lista estesa (~180 pattern) che cattura telemetria, ad tech/RTB, CMP, push services, update channel, ecc. La UI esclude le entry `tipo === 'sistema'` dai conteggi per IP (colonna N nella tabella Live), da "ultima attivita'" (un PC che solo pinga telemetria risulta inattivo) e dal totale richieste nell'header. Le entry di sistema restano visibili nella sidebar "Sistema" per trasparenza. Nel report, `aggregaPerReport()` espone sia `perIp` (totale, per la dt "Richieste totali") sia `perIpAttive` (solo utente+ai, usato per il ranking "Top studenti"). Rationale: dal test sul campo del 2026-04-22, il 93% del traffico registrato era rumore — contarlo nei per-studente appiattisce il segnale.
- **Event delegation, not inline `onclick`**. Nested clickable elements (e.g. block button inside a clickable row) must `e.stopPropagation()` in their case branch of `app.js`.
- **Toolbar Live ridotta all'essenziale** (decluttering 2026-04-22): `[Avvia/Ferma sessione]`, `[Pausa]`, `[Blocca AI]`, `[Sblocca AI]`, `[⋮]`, e sulla destra `Fine: [HH:MM] [x]`. Solo le azioni meno frequenti (svuota blocklist, preset load/save, esporta JSON) vivono nel menu overflow `<details class="menu-overflow">`. L'implementazione usa `<details>` nativo per lo stato aperto/chiuso; `app.js` ha un listener in capture phase su `document.body` che chiude il menu quando si clicca fuori, e chiude anche dopo un click su un item all'interno (tramite `dentroMenu = el.closest('.menu-overflow-panel')`). Il `change` su `preset-load` chiama anche `chiudiMenuOverflow()`.
- **Colonne collassabili (Live tab)**: la sidebar domini (sinistra) e il pannello "Ultime richieste" (destra) sono collassabili tramite i bottoni `«`/`»` nei rispettivi header. Quando collassate, la colonna diventa un rail verticale di 28px con un bottone di ri-espansione. Lo stato e' persistito in `localStorage` (`sidebarCollassata`, `richiesteCollassate`) e applicato a init prima del primo render tramite `applicaCollassi()` in `actions.js` — senza di questo il primo paint mostra la colonna piena, poi si collassa con flicker. Le transizioni CSS (0.15s) sulla width e flex-basis sono usate per l'animazione.
- **`studenti.json` keys starting with `_` are treated as comments** and dropped on load. The file is also **rewritten automatically** by the server whenever the map is mutated via `/api/studenti/*` or `/api/classi/load` — so inline comments are lost after the first UI edit. Reloadable via `/api/reload-studenti` if you edit the file by hand.
- **Classi use a 2-dimensional model**: every saved snapshot is keyed by the pair `(classe, lab)`. File on disk is `<classe>--<lab>.json` with body `{classe, lab, mappa}` (both segments sanitized to `[a-zA-Z0-9_-]`, joined by `--`). The UI has two independent dropdowns — selecting both is what activates Load/Delete. This models the real-world use case: same class visiting different labs (same names, different IPs) and same lab hosting different classes (same IPs, different names).
- **Inline editing preserves focus**: `renderMappaStudenti()` captures `document.activeElement` + selection range before swapping `innerHTML` and re-focuses after. Without this, every SSE `studenti` broadcast (including the one triggered by the user's own edit) would steal focus mid-keystroke.
- **Settings UI in Impostazioni tab**: form with `data-action="settings-field"` + `data-key="<dotted.path>"`. On `change`, `settingsCampoModificato()` POSTs `{[key]: value}` to `/api/settings/update`. Server applies via `setDeep()` using the per-key validator map (`SETTINGS_VALIDATORI`) and returns `{updated, rejected, richiedeRiavvio}`. Keys in `SETTINGS_RESTART` (ports, auth) are saved to `config.json` but don't apply until restart — the client latches `state.riavvioRichiesto` and shows a persistent orange banner.
- **Runtime-vs-boot config reads**: `config.web.auth`, `config.modo`, `config.dominiIgnorati` are read on every request via `config.xxx` so UI edits apply immediately. Ports (`PORTA_PROXY`, `PORTA_WEB`) stay as boot-time consts — reassigning them wouldn't re-bind the listening socket anyway.
- **Password never leaves the server**: `sanitizeConfig()` strips `web.auth.password` and replaces with `{password:"", passwordSet:boolean}`. The UI password field only sends a new value if non-empty (empty submit is ignored client-side — the field placeholder reflects whether a password is already set).
- **Filename sanitization**: presets use `[a-zA-Z0-9_-]`; sessions use `[a-zA-Z0-9_.\-]` + mandatory `.json` suffix. Both refuse bypass attempts — see `nomeSessioneSafe()`.
- **No tests, no linter, no CI.** Validate with the smoke tests above and by loading `http://localhost:9999` in a browser.

## Known structural limits

- **Watchdog keepalive**: `proxy_on.bat`'s VBS ping the proxy at `GET /_alive` every 5s via `MSXML2.ServerXMLHTTP.6.0` with `setProxy 1` (bypasses the local proxy config to avoid a loop). The proxy intercepts `/_alive` before attempting to forward, updates `aliveMap[ip] = now`, and broadcasts `{type:'alive', ip, ts}` via SSE. Frontend renders a colored dot in the IP table column "WD": green <15s since last ping, yellow 15–60s, red >60s (possible bypass), gray = never seen. IPs present only in `aliveMap` (no traffic yet) still appear in the table — they show as rows with 0 traffic and the dot. This is **detection, not prevention**: a student killing `wscript.exe` turns their dot red within 60s, but the red dot alone isn't proof — students legitimately finishing the test also stop pinging.
- **Hotspot bypass**: phone hotspot circumvents the LAN proxy. Unsolvable at this layer. If the PC itself joins a hotspot, the browser can't reach the proxy → visible errors, and also `/_alive` stops arriving → watchdog dot turns red. If the student uses the phone *directly* (PC still on LAN and pinging), the watchdog stays green — only visual supervision catches this.
- **Auth is HTTP Basic**: trusted-LAN only; not TLS-protected. Disabled by default. The advice to bind the web server to `127.0.0.1` instead of `0.0.0.0` for teacher-only access is deliberately **not** implemented — the user wanted LAN access left open.
- **Archive saves at `session/stop`, at `session/start` (difensivo per API calls che saltano Stop) e at graceful shutdown**. Lo stop archivia subito (se ha dati), poi il buffer resta visibile in UI per revisione finche' non premi Avvia. A `kill -9` o power loss prima di uno stop esplicito, il buffer in memoria si perde — esporta manualmente se la sessione e' importante.
- **Preset save overwrites silently.** Same name = overwrite.
- **Deadline is in-memory**: lost on restart.

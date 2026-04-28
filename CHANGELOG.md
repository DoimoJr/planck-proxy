# Changelog

Tutti i cambiamenti rilevanti del progetto sono raccolti qui.
Il formato segue [Keep a Changelog](https://keepachangelog.com/it/1.1.0/) e il
versioning segue [Semantic Versioning](https://semver.org/lang/it/) (con tag
pre-release `-alpha.N` / `-beta.N` per le versioni intermedie del rewrite v2).

## [v2.0.0-alpha.1] — 2026-04-29

Primo release del rewrite v2 in Go. Pensato per uso interno scuola e per
provare il binario singolo sul campo. **Feature parity con v1 + miglioramenti
strutturali**; le funzionalita' davvero "nuove" (integrazione Veyon, SQLite,
auto-classification AI, reazioni automatiche, storico cross-session)
arriveranno nelle release `alpha.2+` come previsto da [SPEC.md](./SPEC.md).

### Aggiunto

- **Single binary Go** (~7.4 MB) che sostituisce il bundle Node.js+script di
  v1 (~91 MB). Niente piu' `node.exe` da installare: `planck.exe` e basta.
- **Frontend embeddato** nel binario via `//go:embed`: niente file accessori
  in produzione.
- **Auto-detection IP LAN + generazione automatica di `proxy_on.bat`/
  `proxy_off.bat`**: i `.bat` vengono scritti accanto al binario al primo
  avvio (e ad ogni boot successivo) con IP+porta gia' compilati. Niente
  piu' edit manuale prima della distribuzione via Veyon. Override IP via
  env var `PLANCK_LAN_IP` per macchine multi-interfaccia.
- **Endpoint download script**: `GET /api/scripts/proxy_on.bat` e
  `proxy_off.bat` ritornano i file con `Content-Disposition: attachment`,
  comodi da linkare ai colleghi.
- **Persistenza file-based** (precursore della Phase 2 SQLite): la blocklist,
  la mappa studenti, la config, i preset, le combo classe+lab e gli archivi
  sessione sopravvivono ai restart automatically. La sessione attiva scrive
  un NDJSON append-only che alla `stop` viene serializzato come JSON snapshot.
- **Crash recovery**: se al boot esiste un NDJSON residuo (sessione
  interrotta da crash), viene archiviato automaticamente come file
  `recovered-*.json`.
- **Monitor sempre attivo**: il monitor live (proxy + classificazione +
  banner AI + watchdog + SSE broadcast) e' attivo a prescindere dalla
  sessione, anche durante lezioni normali. v1 conflate-va monitoraggio e
  registrazione: questo era un bug strutturale ora corretto.
- **Auth HTTP Basic con bcrypt**: la password e' hashata invece che in
  chiaro come v1. Default disabilitata.
- **Test suite Go**: ~50 test in 6 pacchetti (`classify`, `proxy`, `state`,
  `web`, `persist`, `scripts`).

### Cambiato

- **API REST POST + JSON body**: tutte le mutazioni passano da `POST` con
  `Content-Type: application/json`. v1 usava `GET` con query params,
  comodo per testare con curl ma non RESTful e meno robusto contro CSRF.
- **Endpoint rinominati per consistenza**: `/api/pausa/*` → `/api/pause/*`;
  `/api/studenti/*` → `/api/students/*`. La UI e' aggiornata; consumatori
  esterni dovranno adattarsi.
- **Niente piu' `config.json` come fonte unica di config**: la config
  dinamica vive in `config.json` accanto al binario, default hardcoded
  per il primo boot. (In Phase 2 migrera' tutto in SQLite `planck.db`.)

### Limitazioni note di questa alpha

- **Niente integrazione Veyon** ancora — Phase 3-4 della roadmap. Il watchdog
  detection funziona, ma non si possono lanciare comandi (lock/screenshot/...)
  dalla dashboard.
- **Niente SQLite** ancora — Phase 2. Persistenza su file. Va bene per
  decine/centinaia di studenti per istanza, niente reportistica cross-session.
- **Niente auto-classification AI** (Phase 5): la lista AI e' hardcoded
  come in v1 (~129 domini). Aggiornamenti via PR sul repo.
- **Niente reazioni automatiche** (Phase 6): tutto manuale (banner avvisa,
  prof clicca per agire).
- **Niente tab Storico** (Phase 7): vedi una sessione alla volta.

### Migrazione da v1

Non c'e' un percorso di migrazione automatica in questa alpha (i file di v1
hanno formati simili ma path diversi). Per ora si parte da fresh. La
migrazione automatica al primo boot v2 verra' aggiunta nelle prossime alpha.

### Setup

1. Scarica `planck.exe` dalla release.
2. Mettilo in una cartella sul PC docente.
3. Doppio click → si avvia. Apri `http://localhost:9999`.
4. Trova `proxy_on.bat`/`proxy_off.bat` auto-generati nella stessa cartella
   e distribuiscili agli studenti via Veyon.

Per esposizione su LAN scuola con piu' insegnanti, mettilo su un PC
condiviso o usalo singolarmente sul PC docente del laboratorio (modello
decentralizzato, una istanza per docente — vedi [SPEC.md §1.5](./SPEC.md)).

---

Per la storia completa di v1 (Node.js, prima del rewrite), vedi i commit
sul branch `main`.

# Changelog

Tutti i cambiamenti rilevanti del progetto sono raccolti qui.
Il formato segue [Keep a Changelog](https://keepachangelog.com/it/1.1.0/) e il
versioning segue [Semantic Versioning](https://semver.org/lang/it/) (con tag
pre-release `-alpha.N` / `-beta.N` per le versioni intermedie del rewrite v2).

## [v2.0.0-alpha.5.2] — 2026-04-29

Hotfix dell'auto-detection LAN IP. Su PC con piu' interfacce
(Wi-Fi + Ethernet + VirtualBox host-only + VPN), la scansione
euristica precedente prendeva la prima interfaccia privata trovata,
spesso non quella della LAN reale. Risultato: il docente doveva
settare PLANCK_LAN_IP a mano ogni volta che cambiava lab.

### Fixato

- **`scripts.LocalLANIP()` con UDP dial trick**: prova prima a fare
  un `net.Dial("udp", "8.8.8.8:80")` — niente pacchetti spediti,
  ma il kernel risponde con l'IP sorgente della default route.
  Quello e' l'IP della LAN reale, sempre. Solo se non c'e' connettivita'
  internet (lab air-gapped) cade alla scansione manuale.

L'override via env var `PLANCK_LAN_IP` resta disponibile per setup
particolari.

## [v2.0.0-alpha.5.1] — 2026-04-29

Hotfix di alpha.5: il toggle dei plugin watchdog non era effettivamente
rispettato dal flow di distribuzione, e ridistribuire proxy_on creava
istanze duplicate dei watchdog.

### Fixato

- **Endpoint `/api/scripts/watchdog/<id>.ps1` ora rispetta enable/disable**:
  ritorna 404 se il plugin e' disabled, 200 se enabled. Prima serviva
  sempre lo script — il toggle in UI non aveva effetto pratico.

- **`proxy_on.bat` non duplica piu' i watchdog su redistribuzione**:
  ora, al primo step, killa eventuali watchdog precedenti via WMIC
  filter su CommandLine, cancella i `.ps1` vecchi, poi riprova il
  download. Curl con `-f` fallisce su 404 senza scrivere il file →
  plugin disabled = file mancante = `if exist` skippa lo start.

### UX

Il flow per applicare un cambio di config plugin diventa:

1. Toggle plugin in Settings
2. Click "Distribuisci proxy_on" → propaga a tutti gli studenti

Il binario non va piu' riavviato (era una mia assunzione errata). Il
`.bat` non si rigenera al toggle (non serve — e' "stupido" e generico,
e' il server che decide 200/404).

## [v2.0.0-alpha.5] — 2026-04-29

Phase 5: framework di **Watchdog plugins**. Planck monitora gli
studenti oltre al traffico web: rileva connessioni USB sospette,
processi della denylist, e in futuro altri eventi. Il framework
e' estensibile: aggiungere un nuovo plugin = scrivere un .ps1 +
implementare 5 metodi Go.

### Aggiunto

- **Framework `internal/watchdog`**: interfaccia `WatchdogPlugin` +
  `Registry` con plugin registrati a boot. Ogni plugin ha 3 componenti:
  Go server-side (validazione + format + severity), PowerShell client-
  side (polling + POST eventi), UI Planck (toggle + display).

- **Plugin USB built-in**: rileva collegamento/scollegamento di
  dispositivi USB di classe non sicura (chiavette, telefoni MTP, hard
  disk esterni, camere). Filtra automaticamente classi HID/Mouse/
  Keyboard/Audio integrati. Polling 5s lato studente.

- **Plugin Process built-in**: rileva avvio di processi nella denylist
  (cmd, powershell, regedit, taskmgr, mmc, gpedit, perfmon, resmon,
  msconfig). Strumenti che lo studente puo' usare per aggirare il
  proxy o accedere a config di sistema.

- **DB schema v2**: nuove tabelle `watchdog_events` (eventi storicizzati,
  linkati alla sessione corrente se attiva) e `watchdog_config`
  (enable/disable + config per-plugin). Migration automatica al boot.

- **API REST**:
  - `GET  /api/watchdog/plugins` — lista plugin + stato + config
  - `POST /api/watchdog/config` — toggle enable/disable + update config
  - `POST /api/watchdog/event` — endpoint per gli script studenti (no auth)
  - `GET  /api/watchdog/events?plugin=&ip=&limit=` — storico eventi
  - `GET  /api/scripts/watchdog/usb.ps1` — serve lo script PS1 (con IP/port templated)
  - `GET  /api/scripts/watchdog/process.ps1` — analogo

- **UI**:
  - Tab Impostazioni → nuova card "Watchdog plugins" con toggle per
    ogni plugin disponibile + descrizione.
  - Tab Live → badge ⚠️ N sulla card studente quando ci sono eventi
    rilevanti negli ultimi 5 minuti.
  - Tab Live → pannello "Eventi watchdog" sopra la griglia IP che
    mostra gli ultimi 5 eventi warning/critical degli ultimi 5 min.
  - Notifica desktop / beep su eventi warning+ se notifiche sono on.

- **Integrazione `proxy_on.bat` / `proxy_off.bat`**: il bat distribuito
  agli studenti scarica gli script watchdog enabled e li lancia in
  background hidden. `proxy_off.bat` killa solo i processi PowerShell
  watchdog (filtro per CommandLine via WMIC), evitando di toccare
  altri script PS dell'utente.

### Note di upgrade

- Per attivare i watchdog: Impostazioni → card "Watchdog plugins"
  → toggle. Gli script vengono distribuiti alla prossima
  esecuzione di "Distribuisci proxy_on.bat" (le istanze gia'
  distribuite continueranno con la versione precedente fino al
  successivo restart del PC studente o a una nuova distribuzione).

- Trust model: `/api/watchdog/event` non richiede auth (stesso
  modello di `/_alive` — fiducia LAN). `/api/watchdog/config` e
  `/events` richiedono HTTP Basic come gli altri endpoint admin.

- I watchdog sono **best-effort**: girano come PowerShell user-mode,
  uno studente smaliziato puo' killarli da TaskManager. La detection
  di "watchdog killato" non e' implementata (Phase 5.x).

### Limitazioni note

- Niente UI per modificare la **denylist process** o la **allowlist USB
  VID:PID** — solo i default. Phase 5.x.
- Niente plugin Network (cambio interfaccia / VPN / tethering). Phase
  5.x se richiesto.
- Tab "Storico eventi watchdog" non c'e' (gli eventi sono solo nella
  toolbar Live ultimi 5 min). Phase 5.x.

## [v2.0.0-alpha.4.1] — 2026-04-29

Hotfix di alpha.4 dopo testing sul campo (4 VM Windows con Veyon
4.10). Tre bug nascosti dietro recon agenti che hanno restituito
informazioni tecnicamente plausibili ma sbagliate.

### Fixato

- **Distribuzione proxy_on/proxy_off ora funziona davvero**.
  Sostituito il workaround `StartApp + powershell + curl` (fragile,
  problemi di parsing/PATH) con il vero **FileTransfer feature di
  Veyon** (UUID `4a70bd5a-fab2-4a4b-a92a-a1e81d2b75ed`). Sequenza
  StartFileTransfer → ContinueFileTransfer (chunked 256 KB) →
  FinishFileTransfer con `OpenFileInApplication=true`. Il file arriva
  nella cartella Downloads dello studente e viene aperto da `cmd.exe`
  che lo esegue.

- **UUID FileTransfer corretto**. Era `...a1660a6105b7` (dalla recon
  iniziale dello SPEC), ma il vero e' `...a1e81d2b75ed`. Le prime
  versioni di FileTransfer non funzionavano per niente.

- **Argument keys nei FeatureMessage erano sbagliate dappertutto**.
  Avevo assunto chiavi camelCase tipo `"fileName"`, `"text"`,
  `"applications"`. In realta' sono **gli integer dell'enum
  Argument convertiti a stringa**: `"0"`, `"1"`, `"2"`, ecc. Vedi
  `FeatureMessage::argument()` in core/src/FeatureMessage.h:
  `m_arguments[QString::number(static_cast<int>(index))]`. Sistemati
  i flow di TextMessage, StartApp, OpenURL e tutta la catena
  FileTransfer. Lock/Unlock funzionavano per fortuna (niente
  argomenti).

- **IP host auto-detected esposto via /api/config** (`lanIP`). La UI
  Distribuisci non chiede piu' all'utente l'IP — usa quello che
  Planck stesso ha inserito nel proxy_on.bat al boot. Bottone
  "Rimuovi proxy" aggiunto nella toolbar classe.

- **Card Live ora visibili da subito**: anche IP della mappa
  studenti senza traffico/watchdog appaiono come card. Permette di
  inviare comandi Veyon prima della distribuzione del proxy.

### Note di upgrade

- Niente azione manuale richiesta. La master key Veyon importata in
  alpha.4 resta valida.
- Su Windows multi-interfaccia, esporta `PLANCK_LAN_IP=<ip-LAN>`
  prima di lanciare planck.exe se l'auto-detect prende l'interfaccia
  sbagliata (es. VirtualBox host-only invece della LAN reale).

## [v2.0.0-alpha.4] — 2026-04-29

Phase 3 + 4: integrazione Veyon completa. Planck può ora controllare i
PC studenti che hanno `veyon-server` in esecuzione: lock schermo, lancio
applicazioni, messaggi modali, reboot/poweroff, distribuzione automatica
di `proxy_on.bat` con un click. Niente cgo, niente Qt SDK: client
nativo del protocollo RFB+Veyon implementato in Go puro.

### Aggiunto

- **`internal/veyon/qds`** (Phase 3a, ~500 LoC + 39 unit test): encoder/
  decoder Qt `QDataStream` versione `Qt_5_5`. Tipi supportati: `bool,
  qint32, qint64, double, QString, QByteArray, QStringList, QUuid,
  QRect, QVariant, QVariantList, QVariantMap`. Determinismo dell'ordine
  delle chiavi `QVariantMap` (Qt's `QMap` e' sorted) per byte-equality
  con messaggi prodotti da Qt.
- **`internal/veyon`** (Phase 3b/c/d, ~400 LoC + 4 integration test):
  client del protocollo Veyon su RFB v3.8. Greeting + security type
  custom (`0x28`) + `KeyFile` auth con firma RSA SHA-512 PKCS#1 v1.5
  + `ClientInit`/`ServerInit` + invio `FeatureMessage` come RFB
  extension (tipo `0x29`).
- **API REST `/api/veyon/*`** (Phase 3e):
  - `GET  /status`     ritorna `{configured, keyName, port}`
  - `POST /configure`  importa la master key (PEM) + nome chiave
  - `POST /clear`      rimuove la configurazione (file su disco + DB)
  - `POST /test`       prova dial+auth verso `{ip}`
  - `POST /feature`    invia `{ip, feature, command, arguments}`. Il
    campo `feature` accetta UUID raw o un nome simbolico tra
    `screenLock, startApp, reboot, powerDown, logoff, textMsg, openURL`.
- **UI Veyon** (Phase 3e + 4):
  - Card "Veyon (controllo studenti)" nel tab Impostazioni: status
    indicator, importazione master key PEM, override porta, test
    connessione verso un IP.
  - Bottoni inline 🔒 / 💬 sulle card studente nella vista griglia
    Live (visibili solo se Veyon e' configurato).
  - Toolbar "Azioni classe" sotto la toolbar principale Live: 🔒 Lock,
    💬 Messaggio, 📁 Distribuisci proxy_on. Tutti applicati su tutti
    gli IP attivi (chi ha pingato il watchdog).
  - "Distribuisci proxy_on.bat" sfrutta che Planck gia' serve lo
    script su `/api/scripts/proxy_on.bat`. Lancia su ogni studente
    via `StartApp` un comando `powershell -c "iwr ... -OutFile ...; & ..."`.
    Niente `FileTransfer` (Veyon WebAPI non lo espone via plugin RFB).
- **`test/veyon-rig`**: Dockerfile + entrypoint per Veyon server
  headless (Ubuntu 24.04 + PPA `veyon/stable` + Xvfb). Setup
  one-shot: `docker build -t planck-veyon-rig test/veyon-rig`,
  poi `docker run -p 11100:11100 planck-veyon-rig`. Genera coppie
  RSA al boot, accessibili in `/export/`.
- **Update SPEC §5.16**: riscritto il protocollo Veyon dopo
  l'implementazione effettiva. La versione iniziale assumeva un
  protocollo Qt-puro su TCP raw; in realta' Veyon e' RFB v3.8 (VNC)
  con security type custom e message extension. Il nuovo testo
  documenta la sequenza byte-per-byte validata vs server vivo.

### Note di upgrade

- Niente azione manuale richiesta. Aggiorna l'eseguibile e Planck parte
  come prima. Se vuoi usare Veyon, vai su Impostazioni → card "Veyon"
  e importa la master key generata con Veyon Configurator (PEM PKCS#8).
- La master key viene salvata in `<dataDir>/veyon-master.pem` con
  permessi `0600`. Cancellala con il bottone "Rimuovi" se vuoi
  disabilitare il controllo.
- I PC studenti devono avere `veyon-server` in esecuzione e la
  chiave pubblica corrispondente importata via Veyon Configurator
  (workflow standard Veyon).
- `internal/persist` (file-based v1.6) e' stato rimosso dal binario
  in alpha.3 ma resta nel repo come reference. Verra' eliminato in
  alpha.5.

### Limitazioni note

- **Niente Screenshot** — la WebAPI di Veyon lo espone (`GET
  /framebuffer`) ma RFB FeatureMessage no. Phase 4 polish.
- **Multi-select** sulle card studente non ancora — i comandi vanno
  o su un singolo IP (bottoni inline) o su tutti gli attivi (toolbar
  classe). In arrivo come polish.
- **Niente integrazione Veyon "discovery"**: Planck non scopre
  automaticamente quali IP della classe hanno veyon-server attivo.
  Per ora si fida del watchdog del proxy ("studente ha pingato"
  -> "ha veyon-server attivo, probabilmente"). Per diagnosi punto
  per punto c'e' il bottone Test connessione.

## [v2.0.0-alpha.3] — 2026-04-29

Phase 2: persistenza migrata da file-based a SQLite. Niente piu' rotolamento
NDJSON + JSON snapshot per le sessioni — ogni richiesta finisce in una riga
della tabella `entries`, le sessioni in `sessioni`. Il binario resta single-
file e cross-compilabile (driver pure-Go `modernc.org/sqlite`, niente CGO).

### Aggiunto

- **`internal/store`**: nuovo package SQLite-backed che sostituisce il
  file-based `internal/persist`. Apre `planck.db` accanto al binario con
  `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`. Schema gestito
  via tabella `schema_version` + migration ordinate (v1 = init).
- **Migrazione one-shot da v1 file-based**: al primo boot dopo l'upgrade,
  Planck importa `config.json`, `studenti.json`, `_blocked_domains.txt`,
  `presets/*.json`, `classi/*.json` e `sessioni/*.json` nel DB. I file
  sorgente vengono rinominati a `*.v1.bak` per non re-importare. Marker
  di idempotenza in `kv.migrated_from_files`.
- **BOM-tolerant**: il parser tollera l'UTF-8 BOM che Notepad su Windows
  aggiunge ai file `Save As UTF-8`. Necessario perche' molti docenti
  editano `config.json` a mano.
- **Crash recovery sessione**: al boot, eventuali sessioni con
  `sessione_fine NULL` (crash mid-sessione) vengono chiuse forzatamente.

### Cambiato

- **Lifecycle sessione**: `SessionStart` apre una riga in `sessioni` e
  ritorna l'id; le entries del proxy vanno via `SessionAppendEntry` (insert
  per riga) invece di NDJSON append. `SessionStop` fa `UPDATE sessioni SET
  sessione_fine = ?, durata_sec = ?, archiviata_at = ?`.
- **`/api/sessioni`**: il filename ritornato e' `<id>-<inizio>.json` invece
  che il path del JSON snapshot. La UI continua a funzionare perche' usa
  l'id-stringa come opaco; load/delete passano per `ParseSessionFilename`
  che estrae l'id numerico.
- **`/api/sessioni/archivia`** (checkpoint): non scrive piu' uno snapshot
  separato. Chiude la sessione corrente e ne apre una nuova con stessi
  metadata (rotazione in-place).

### Note di upgrade

- Niente azione manuale richiesta: aggiorna l'eseguibile e al primo boot
  Planck importa i file legacy automaticamente. I file `*.v1.bak` possono
  essere cancellati a mano dopo aver verificato che il DB ha i dati
  corretti (consigliato: tieni una copia di backup della cartella prima
  dell'upgrade).
- `internal/persist` resta nel repo (per reference / fallback) ma non e'
  piu' importato dal binario. Verra' rimosso in alpha.4.



Patch QoL su alpha.1 — feel desktop app + shutdown dalla UI.

### Aggiunto

- **Auto-launch browser in modalita' app** al boot: Planck cerca Edge
  (poi Chrome) e lo apre con `--app=http://localhost:9999` su una finestra
  senza barra URL, senza tab, senza menu. Il risultato e' una "vera"
  finestra app desktop, non un tab del browser. Override:
  `PLANCK_NO_BROWSER=1` per server headless.
- **Endpoint `POST /api/shutdown`**: spegne il binario via os.Exit dopo
  aver risposto al client (graceful HTTP, 200ms delay per garantire la
  risposta).
- **Bottone ⏻ in topbar** (UI): chiede conferma e chiama `/api/shutdown`,
  poi mostra un overlay "Planck spento". Permette di chiudere il server
  senza dover andare in console.

### Cambiato

- Niente: alpha.1 resta compatibile, nessuna API rimossa.

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

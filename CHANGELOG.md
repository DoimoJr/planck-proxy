# Changelog

Tutti i cambiamenti rilevanti del progetto sono raccolti qui.
Il formato segue [Keep a Changelog](https://keepachangelog.com/it/1.1.0/) e il
versioning segue [Semantic Versioning](https://semver.org/lang/it/) (con tag
pre-release `-alpha.N` / `-beta.N` per le versioni intermedie del rewrite v2).

## [v2.3.1] — 2026-05-01

Patch: fix UI "Domini ignorati" che appariva vuota al primo load.

### Risolto

- **Domini ignorati vuoti in Impostazioni**: il client leggeva la
  risposta di `GET /api/settings` come `setRes.settings` (proprieta'
  inesistente), quindi `state.settings` restava `null` al boot. La
  lista si popolava solo al primo SSE `settings` (es. dopo un cambio
  manuale). Fix in `app.js`: la API ritorna l'oggetto piatto, viene
  letto direttamente.

### Pulizia

- Rimossi i file legacy della v1 dalla root del repo: `node.exe`
  (~91 MB), `server.js`, `domains.js`, `avvia.bat`, `blocked.html`,
  e i backup `.v1.bak` di `config.json`, `studenti.json`,
  `_blocked_domains.txt`, `presets/*.json`. Il DB SQLite e' ormai
  l'unica fonte di verita'; i backup erano stati lasciati per
  paracadute post-migrazione.

## [v2.3.0] — 2026-04-30

Phase 6: **auto-classification AI**. La lista dei domini AI ora vive
in un file di testo nel repo (`data/ai-domains.txt`) invece che hardcoded
nel codice Go. Planck pulla l'ultima versione da GitHub al boot e
quando l'utente clicca "Aggiorna ora" nelle Impostazioni, con
fallback graceful sulla copia embedded se non c'e' connettivita'.

### Aggiunto

- **`data/ai-domains.txt`** (root del repo): file canonico, una entry
  per riga, commenti `#` e sezioni. Aggiungere domini = aprire una
  PR su questo file. Tutti i Planck installati raccolgono la modifica
  al prossimo refresh.

- **`classify.RefreshAIList(url, dataDir)`**: scarica + valida +
  salva cache in `<dataDir>/ai-domains-cache.txt` + promuove la
  lista corrente. Timeout 10s, sanity check `>= 10` domini per evitare
  di sovrascrivere con HTML 404 o pagine error.

- **`classify.LoadAICache(dataDir)`**: carica la cache scritta da
  un refresh precedente. Usato a boot prima del fetch async.

- **3 layer di fallback**:
  1. Try cache (`<dataDir>/ai-domains-cache.txt`) — se presente, usa
  2. Async tenta refresh remote — se ok, sovrascrive cache
  3. Embedded (built-in nel binario, ~129 domini) — sempre disponibile

  Niente blocco al boot: la lista parte sempre dall'embedded e si
  promuove asincronicamente quando il fetch ritorna.

- **`POST /api/ai/refresh`** + **`GET /api/ai/status`**: refresh
  manuale dalla UI. Lo status mostra count + source
  (`embedded`/`cache`/`remote`) + timestamp ultima update.

- **UI Settings** (Impostazioni → nuova card "Lista AI auto-aggiornata"):
  status corrente + bottone "🔄 Aggiorna ora". Toast con esito.

- **SSE event `ai-list`**: broadcast quando la lista viene
  aggiornata. La UI ricarica il count + sorgente in tempo reale.

### Cambiato

- **`classify.DominiAI` (var) → `classify.AIDomains()` (func)**.
  La nuova funzione ritorna uno snapshot atomico della lista
  corrente. I 3 chiamanti aggiornati (`Classifica`, `BlockAllAI`,
  `ConfigSnapshotData`).

- **Test `TestEmbeddedMatchesDataFile`** (in `source_test.go`)
  assicura che `internal/classify/embedded_ai_domains.txt` e
  `data/ai-domains.txt` siano sempre identici. Se si aggiorna uno
  dei due, l'altro va sincronizzato (`cp`) — il test fallisce
  altrimenti.

### Note di upgrade

- Niente azione manuale richiesta. Al primo boot Planck prova il
  fetch da GitHub: se va, scarica la lista corrente; se no, usa
  l'embedded che e' la stessa di alpha.5.5 (~129 domini).
- L'URL del fetch e' hardcoded a `raw.githubusercontent.com/DoimoJr/
  planck-proxy/main/data/ai-domains.txt`. Override via PR sul codice.
- Il binary resta ~12 MB (lista AI ~3 KB embedded).

## [v2.2.0] — 2026-04-30

Phase 5.x parte 2: **plugin Network** (rileva nuove interfacce di rete)
+ **heartbeat detection** (alert se uno studente killa il watchdog).

### Aggiunto

- **Plugin Network** (`internal/watchdog/builtin/network.go`):
  rileva la comparsa di nuove interfacce "Up" rispetto al baseline
  catturato al boot dello script. Pattern di config:
  - `suspiciousPatterns`: substring nel nome interfaccia che alzano
    severity a critical (default: VPN, TAP, TUN, WireGuard, OpenVPN,
    Cellular, Mobile, Broadband, USB, Hotspot, Tether, PAN)
  - `ignorePatterns`: substring per skippare completamente (default:
    Loopback, Hyper-V, VirtualBox Host-Only)
  Tutto editabile dall'editor JSON delle Impostazioni.

  Caso d'uso: studente attacca un dongle 4G LTE, attiva un client
  VPN, fa tethering dal telefono → Planck rileva l'interfaccia nuova
  e fa scattare un alert critical.

- **Watchdog heartbeat** (`internal/state/watchdog.go`):
  ogni script `.ps1` ora invia un POST `/api/watchdog/heartbeat`
  ogni 30 secondi (in aggiunta agli eventi reali). Il server traccia
  `lastSeen[ip][plugin]` e una goroutine ogni 30s controlla:
  - Se un IP e' "alive" (proxy ping recente) MA un suo plugin enabled
    e' silente da piu' di 90 secondi → emette evento meta
    `watchdog-<plugin> stopped` con severity `warning` ("probabilmente
    killato").
  - Quando l'heartbeat torna → evento `watchdog-<plugin> resumed` con
    severity `info`.
  Il flag stopped/alerted evita flooding (un solo allarme per silenziamento).

  Caso d'uso: studente apre Task Manager, killa `wscript.exe` o
  `powershell.exe`, prosegue il proprio piano. Planck se ne accorge
  entro 2 minuti e lo segnala al docente.

### API

- `POST /api/watchdog/heartbeat {plugin}` — endpoint senza auth (LAN
  trust come `/_alive` e `/event`). Body minimal, niente persistenza.
- `GET  /api/scripts/watchdog/network.ps1` — script PowerShell
  template-substituted con la config network.

### Limiti

L'attacker model resta lo stesso: lo studente ha l'ultima parola sul
suo PC. Il framework heartbeat non impedisce il kill — RILEVA il kill.
Per "blindare" davvero servirebbe un servizio Windows con privilegi
admin. Phase 5.x non ci si addentra (richiederebbe firma codice +
UAC + setup IT scuola).

## [v2.1.0] — 2026-04-30

Phase 5.x: editor UI per la config dei watchdog plugins. Le denylist
process e le classi/VID:PID ignorati USB sono ora editabili dal browser
e iniettati al volo nei `.ps1` serviti agli studenti.

### Aggiunto

- **Editor JSON inline** nella card "Watchdog plugins" delle Impostazioni:
  `<details>` collassabile con textarea, bottoni "Salva configurazione"
  e "Ripristina default". Il JSON e' validato lato client prima
  dell'invio; errori parse mostrati come toast.

- **Allowlist USB VID:PID**: il plugin USB ora rispetta una whitelist di
  coppie hex (es. `"1234:5678"`) per device legittimi del docente — la
  chiavetta personale non genera piu' false positive ad ogni inserimento.
  L'estrazione VID:PID dal PnP InstanceId e' fatta lato PowerShell.

- **Denylist process editabile**: il plugin Process aveva una lista
  hardcoded nel template; ora la lista viene letta dalla config in DB
  e iniettata nello script al momento del download. Aggiungere/rimuovere
  processi e' immediato (effetto alla prossima Distribuisci proxy).

### Cambiato

- `internal/scripts/watchdog_*.go`: le signature di `WatchdogUsbScript`
  e `WatchdogProcessScript` ora accettano gli array di config in input.
  Il template ha placeholder `__IGNORED_CLASSES__`, `__ALLOW_VID_PID__`,
  `__DENY_LIST__` sostituiti via `psStringArray()` (formatter di array
  PowerShell con escape di apici singoli).

- `/api/scripts/watchdog/<id>.ps1`: l'endpoint ora legge la config del
  plugin da `LoadWatchdogConfig`, deserializza in struct typed, e la
  passa al generator. Comportamento per plugin non abilitato invariato
  (404 → bat skippa via `if exist`).

### UX

Modificare la config NON propaga automaticamente agli studenti già
attivi. Il flow corretto:

1. Modifica JSON nella UI → click Salva
2. Toolbar Live → Distribuisci proxy
3. Sui PC studenti: il bat killa il watchdog vecchio + scarica il
   `.ps1` con la nuova config

Toast post-save lo ricorda esplicitamente.

## [v2.0.0] — 2026-04-30

🎉 **Prima release stable di Planck v2.** Niente piu' alpha. Cumulativa
di tutte le 5 fasi del rewrite Go: monitor proxy + dashboard, persistenza
SQLite, integrazione Veyon completa (lock/msg/distribuisci/power),
watchdog plugins (USB/Process), polish UX.

`main` ora punta a v2 (era rimasto su v1 Node.js fino a questo cut).
v1 vive nei tag legacy `v1.x` per retrocompatibilita'.

### Highlights

- **Single binary** Go (~12 MB) per Windows e Linux, niente
  dipendenze esterne. Sostituisce il bundle Node.js+script di v1
  (~91 MB). Doppio click → parte.
- **Proxy HTTP+HTTPS** con CONNECT tunneling, no MITM. Vede solo
  hostname, mai contenuti.
- **Dashboard web** real-time via SSE. 4 tab (Live / Report /
  Storico / Impostazioni). Tema chiaro/scuro, tema italiano.
- **SQLite** per sessioni, eventi watchdog, mappe classe, preset.
  Migrazione automatica dal layout file-based v1 al primo boot.
- **Veyon nativo** (RFB v3.8 + KeyFile auth + FeatureMessage,
  protocollo reverse-engineered byte-per-byte): lock/unlock schermo,
  text message, start app, reboot, poweroff, distribuisci proxy_on.vbs.
  Multi-select Ctrl/Shift+click sulle card studente.
- **Watchdog plugins** estensibili: USB monitor, Process monitor.
  PowerShell client-side, REST + SSE server-side. Toggle in Settings.
- **UX**: toast non-modali, empty state guidate, connection banner,
  keyboard shortcuts (Ctrl+1..4 / Ctrl+S/P/F/A / ESC).

### Migrazione da v1

Niente azione manuale. Sostituisci il bundle Node.js con `planck.exe`
nella stessa cartella, e al primo boot Planck importa automaticamente
`config.json`, `studenti.json`, `_blocked_domains.txt`, `presets/`,
`classi/`, `sessioni/` nel `planck.db` SQLite. I file legacy restano
sul disco come `*.v1.bak` (cancellabili a mano dopo verifica).

### Breaking changes vs v1

- Endpoint API REST tutti **POST + JSON body** (v1 usava GET + query
  params). I client esterni vanno aggiornati. La UI built-in funziona
  out-of-the-box.
- `proxy_on.bat` → `proxy_on.vbs` (esecuzione silenziosa lato
  studente). Distribuisci-proxy via Veyon usa la versione `.vbs`.
- Niente piu' `node.exe` da scaricare separatamente.

### Limiti noti

Vedi [README — Limiti strutturali](./README.md#limiti-strutturali).
In sintesi: lo studente ha sempre l'ultima parola sul proprio PC
(proxy in HKCU = no UAC, watchdog user-mode killabile). Planck
**rileva e disincentiva**, non blinda.

---

## [v2.0.0-alpha.5.5] — 2026-04-30

Phase 8 polish (parte 1): UX della dashboard. Sostituiti gli `alert()`
modali con toast non-bloccanti, aggiunto banner "riconnessione" per
SSE persa, empty state guidato sul Live tab, keyboard shortcuts.
Build Linux ora pubblicata accanto alla Windows.

### Aggiunto

- **Toast system** (`toast.success/error/info`) sostituisce 24 chiamate
  `alert()` sparse nel frontend. Toast bottom-right, auto-dismiss
  3s/5s, dismissable click, niente blocco del flow utente.

- **Empty state Live** con CTA contestuale:
  - Se Veyon configurato + studenti in mappa → bottone "📁 Distribuisci proxy ora"
  - Se solo studenti in mappa → suggerimento di configurare Veyon
  - Se nulla → suggerimento di compilare la mappa

- **Connection lost banner**: quando SSE si disconnette (server riavviato,
  rete persa), banner arancione in cima "Tentativo di riconnessione...".
  Auto-dismiss alla riconnessione.

- **Keyboard shortcuts** (SPEC §6.7):
  - `Ctrl+1..4` switch tab Live/Report/Storico/Impostazioni
  - `Ctrl+S` toggle Avvia/Ferma sessione
  - `Ctrl+P` toggle pausa
  - `Ctrl+F` focus sul filtro testuale
  - `Ctrl+A` (in Live) seleziona tutti gli IP della mappa studenti
  - `ESC` deseleziona / clear focus IP
  - Shortcut con Ctrl funzionano anche da input (utili per cambio tab);
    gli altri sono inibiti se stai digitando.

### Cambiato

- **Build Linux**: l'eseguibile `planck-linux` (~11.7 MB ELF
  statically-linked) viene ora cross-compilato e pubblicato accanto a
  `planck.exe` in ogni release.

- **README riscritto per v2**: tutta la doc utente aggiornata
  (download release, setup Veyon, watchdog plugins, env vars,
  shortcuts, roadmap).

## [v2.0.0-alpha.5.4] — 2026-04-29

Refactor distribuzione proxy: **da .bat a .vbs**. Niente piu' flash di
console / cmd minimizzata sugli studenti — esecuzione 100% silenziosa.

### Cambiato

- **`proxy_on.bat` → `proxy_on.vbs`**, idem per `proxy_off`.

  Su Windows i `.bat` sono associati a `cmd.exe` (subsystem console),
  che apre **sempre** una finestra console quando viene invocato. Con
  `OpenFileInApplication=true` di Veyon FileTransfer, ogni studente
  vedeva un cmd lampeggiare/restare minimizzato col messaggio
  "Proxy attivato". Anche con `start /min` e `WindowStyle Hidden` sui
  child, c'era sempre un flash.

  Soluzione strutturale: i `.vbs` sono associati a `wscript.exe`
  (subsystem GUI), che NON crea nessuna finestra. La logica del bat
  (set proxy in HKCU, kill watchdog precedenti, lancia watchdog VBS,
  scarica + lancia plugin .ps1) viene riscritta come VBScript usando
  `WScript.Shell.RegWrite` per il registry e `Run "wscript ...", 0,
  False` per i child hidden.

- **API endpoint per il download manuale rinominato**:
  `/api/scripts/proxy_on.bat` → `/api/scripts/proxy_on.vbs` (idem off).
  Content-Type: `text/vbscript`.

- **API distribuzione**: `/api/veyon/distribuisci-proxy` e
  `/api/veyon/disinstalla-proxy` ora trasferiscono i file `.vbs`
  invece di `.bat`. Comportamento client-side identico, solo nessun
  cmd visibile.

### Note di upgrade

- I `.bat` vecchi gia' distribuiti agli studenti continuano a girare
  finche' non si fa Distribuisci proxy_off oppure il PC viene
  riavviato. La prossima Distribuisci proxy_on inviera' la versione
  `.vbs`, che killa il watchdog vecchio e installa il nuovo flow.
- Nessun cambio per il docente: stessa UI, stessi bottoni.

## [v2.0.0-alpha.5.3] — 2026-04-29

Hotfix: cmd window non si chiudeva sul PC studente dopo Distribuisci.

### Fixato

- **`proxy_on.bat` non lascia piu' un cmd minimizzato aperto**.
  Il bat lanciava i child watchdog (VBScript + PowerShell) con
  `start "" /b ...` — `/b` significa "no new window", ma il
  side-effect e' che i child ereditano la console del cmd parent.
  Cmd finisce il bat, vorrebbe chiudersi, ma non puo' perche' la
  console serve ai child.

  Fix: `start "" /min` invece di `/b`. I child partono in una
  nuova finestra minimizzata; `wscript.exe` essendo subsystem GUI
  non mostra finestra; `powershell.exe -WindowStyle Hidden` si
  nasconde subito. La console del bat parent si chiude come
  dovrebbe.

- **Rimosso `echo Proxy attivato: ...`** dal bat: era solo log
  di debug, sullo studente lasciava una stringa visibile nel
  cmd minimizzato (l'altro lato del bug sopra).

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

# Changelog

Tutti i cambiamenti rilevanti del progetto sono raccolti qui.
Il formato segue [Keep a Changelog](https://keepachangelog.com/it/1.1.0/) e il
versioning segue [Semantic Versioning](https://semver.org/lang/it/) (con tag
pre-release `-alpha.N` / `-beta.N` per le versioni intermedie del rewrite v2).

## [v2.9.22] — 2026-05-06

### Modificato

- **Distinzione "Remove esplicito" vs "kill sospetto del proxy"**
  nei pallini plugin. v2.9.21 li metteva entrambi a grigio. Ora:
  - **Mai visto / Remove esplicito** (`aliveMap[ip]` non esiste o
    e' stato cancellato dall'SSE proxy-removed) → grigio
    "Proxy non attivo: stato plugin sconosciuto"
  - **Era vivo, ora silente** (`aliveMap[ip]` ha ts vecchio oltre
    15s, lo studente ha killato il processo proxy) → rosso "Proxy
    silente da Xs: plugin killati col processo"
  - **Proxy fresco** → valutazione plugin normale

  Stessa logica in card grid (`statoPlugins`) e detail pane
  (`pluginRows`): per-plugin pallino diventa alert con detail
  "proxy silente — kill sospetto". Entrambi i pallini bottom-left e
  bordo della card seguono il rosso. Scenario "studente killa il
  proxy via Task Manager" ora distinguibile a colpo d'occhio.

## [v2.9.21] — 2026-05-06

### Risolto

- **Post Remove proxy: card grid bottom-left rosso, bordo rosso,
  detail pane plugin verdi** (incoerenza). Causa: `statoPlugins`
  contava 0 plugin alive (mappa pulita) → "tutti mancanti = rosso";
  pluginRows del detail pane invece guardava solo gli eventi → 0
  eventi → tutti verdi.

  Fix:
  - `statoPlugins` ora gate sul proxy: se `aliveMap[ip]` non e'
    fresco (proxy grigio o rosso), ritorna grigio "stato plugin
    sconosciuto". Lo studente e' offline, non sta evadendo.
  - `pluginRows` del detail pane: stesso gate, plugin diventano
    grigi con detail "proxy non attivo". Inoltre per ogni plugin
    verifica anche l'aliveness specifica (`alivePluginMap[ip][p.id]`):
    se il proxy pinga ma il plugin singolo no, il pallino diventa
    warn con detail "plugin silente — possibile kill" (coerente:
    il watchdog di quel plugin e' stato killato mentre il proxy
    sopravvive).

## [v2.9.20] — 2026-05-06

### Modificato

- **Pulse traffic ora ben visibile**: l'animazione precedente usava
  `box-shadow` rgba tenue (0.55 → 0 alpha) su `::after`, poco
  percepibile su sfondi chiari. Ora usa `outline` che parte dal
  bordo e si espande verso fuori (8px) sfumando — segue il
  border-radius, nitido, indipendente dal box-shadow di stato.
  Durata 650ms.

## [v2.9.19] — 2026-05-06

### Modificato

- **Risoluzione warning con "Ignora + info successivo"**: ora la
  semantica del pallino plugin (sia card grid che detail pane) e':
  - warning attivo → giallo per 5 min
  - utente clicca **Ignora** sul log eventi → warning resta GIALLO
    (l'utente ha preso visione ma il problema e' ancora attivo, es.
    USB ancora collegata)
  - studente toglie USB → arriva evento info "removed"
  - se l'evento era stato ignorato → pallino torna VERDE subito
    (stato risolto)
  - se non era stato ignorato → resta giallo per i 5 min residui
    (l'utente non ha ancora preso visione, deve poterlo vedere)

  Helper unificata `valutaWdPlugin(ip, pluginId, evts, cutoff,
  ignoredIds)` riusata da `statoPlugins` (card grid) e dalle pluginRows
  del detail pane: stessa logica ovunque.

## [v2.9.18] — 2026-05-06

### Risolto

- **Detail pane plugin tornava verde subito dopo USB rimossa, ma card
  grid restava gialla** (incoerenza). Causa: il detail pane guardava
  l'**ultimo** evento del plugin, mentre `statoPlugins` (card grid)
  cerca qualsiasi warning/critical nei 5 min. Quando l'utente toglie
  la USB e arriva l'evento "removed" (info), questo diventava l'ultimo
  → detail pane verde, ma il warning "USB inserita" 30s prima era
  ancora nei 5 min → card gialla. Ora il detail pane usa la stessa
  logica: l'evento di severity piu' alta nei 5 min determina il
  colore, info "removed/stopped" successivi non azzerano il warning.

## [v2.9.17] — 2026-05-06

### Risolto

- **Pallino watchdog-dot e bordo card non riflettevano gli eventi
  warning/critical**: quando arrivava un evento (es. USB inserita) il
  detail pane coloriva il pallino del plugin specifico, ma sulla card
  grid il pallino bottom-left e il bordo restavano verdi. Causa:
  `statoPlugins` valutava SOLO l'aliveness dei plugin (che continuavano
  a pingare regolarmente). Ora dopo aver verificato che tutti i plugin
  sono attivi, controlla anche gli eventi recenti (5 min): warning →
  giallo, critical → rosso. Aliveness rotta resta priorita' alta sul
  filtro eventi (kill manuale = segnale piu' forte di un singolo evento).

- **Card non si puliva dopo Remove+Rimando proxy**: gli eventi watchdog
  recenti per quell'IP restavano in `state.watchdogEventsPerIp` e il
  pallino restava giallo anche dopo aver "azzerato" lo studente. Ora
  l'SSE `proxy-removed` cancella anche `watchdogEventsPerIp[ip]`: la
  card riparte verde non appena il proxy ricomincia a pingare.

## [v2.9.16] — 2026-05-06

### Risolto

- **Card studente non andavano mai in stato "idle"** anche dopo
  minuti senza navigazione. Bug in `ipSignals`: calcolava il
  `trafficoAgo` dall'ultima entry **qualsiasi**, includendo il
  traffico tipo "sistema" (OCSP, telemetry Microsoft, captive
  portal, ecc.) che arriva continuamente a finestra Edge chiusa.
  Risultato: il timestamp dell'ultima entry era sempre <3 min →
  card perennemente "active". Fix: cerco l'ultima entry NON sistema
  (web/ai).

### Modificato

- **Soglia idle abbassata da 3 min a 15 secondi**
  (`IDLE_TRAFFIC_MS`). Feedback molto piu' reattivo: la card
  sbiadisce subito quando lo studente smette di navigare.

## [v2.9.15] — 2026-05-06

### Aggiunto

- **Pulse animation sulla card studente ad ogni richiesta**: feedback
  visivo in tempo reale quando un'entry SSE arriva per un IP. Onda
  verde tenue di ~550ms che si espande dal bordo tramite pseudo-
  elemento `::after` (isolato dal bordo/box-shadow di stato:
  funziona insieme a data-border, data-state="ai", "selected"). Skip
  sulle entry tipo "sistema" per evitare vibrazione costante. Riavvio
  pulito su eventi consecutivi via reflow + remove/add classe.

## [v2.9.14] — 2026-05-06

### Risolto

- **Watchdog Process non emetteva mai eventi**: bug nel template
  PowerShell. `Get-Process` ritorna `Name` SENZA estensione (es.
  `"cmd"`, `"powershell"`), mentre la denylist default contiene
  `"cmd.exe"`, `"powershell.exe"`. Lo `Strip-Exe` lato script veniva
  applicato solo al nome processo, non alla denylist, quindi
  `$denyList -contains $clean` falliva SEMPRE. Fix: normalizzazione
  una-tantum della denylist al boot dello script.

### Aggiunto

- **Pulsante "Invia proxy" nella scheda di dettaglio studente**:
  shortcut accanto a "Rimuovi proxy" per distribuire `proxy_on.vbs`
  al singolo studente senza dover passare per la toolbar Veyon globale.

### Note

- **Watchdog Network**: il plugin segnala SOLO interfacce che
  appaiono DOPO che lo script e' partito. Tutte le interfacce gia'
  Up al boot del proxy_on entrano nella baseline "trusted". Per
  testare: lanciare il proxy con il dongle/hotspot scollegato, poi
  collegarlo → l'evento "added" arrivera' entro 5s.

## [v2.9.13] — 2026-05-06

### Risolto

- **Pallino top-left blu in vista Lista quando la card e' selected**:
  il col-status della tabella usava una mappa `selected→info` che
  forzava blu sul dot. Ora il dot rispecchia SOLO lo stato proxy
  (verde/rosso/grigio), coerente con la card grid. Il bordo/row
  selezionato continua a essere segnalato dall'highlight del row.

- **Remove proxy generava alert "watchdog stopped" spuri**: dopo aver
  cliccato "Rimuovi proxy", i watchdog vengono killati e il server
  tirava su l'alert "il watchdog e' silente". Ovviamente atteso, ma
  rumoroso. Nuovo metodo server `MarkProxyRemoved(ips)` chiamato dal
  handler `/api/veyon/disinstalla-proxy`:
  - pulisce `aliveMap`, `watchdogHeartbeats`, `*StoppedAlerted` per
    quegli IP (la card si pulisce subito → grigia su entrambi i dot);
  - imposta `proxyRemovedAt[ip] = now`. Per i prossimi 60s
    (`ProxyRemovedGrace`) `checkHeartbeats` salta gli alert per
    quell'IP. Cancellato anticipatamente se il proxy torna a pingare.
  - broadcast SSE `{type:"proxy-removed", ip}` per pulizia immediata
    lato UI.

### Modificato

- **Banner alert non scade piu' automaticamente**: prima gli eventi
  spirivano dal banner dopo 5-10 min. Ora restano fino a quando
  l'utente non li ignora esplicitamente (click "Ignora" nel log
  eventi, "Ignora tutto", o un Reset). Cap implicito invariato:
  `state.entries` (5000) e `state.watchdogEvents` (200) garantiscono
  roll-off naturale.

- **Bottone "Reset" ora svuota davvero tutto**: prima cancellava solo
  blocklist + pausa + lock Veyon. Ora chiama anche `/api/reset-runtime`
  che svuota `s.storia` e marker watchdog server-side, e il broadcast
  SSE `reset-runtime` pulisce `state.entries`, `state.perIp`,
  `state.perDominio`, `state.watchdogEvents`, `state.watchdogEventsPerIp`,
  `state.eventiIgnoredIds`, `bannerDismissed`. Il DB persistito
  (sessioni, eventi storici) NON viene toccato.

### Aggiunto

- **Bottone "Ignora tutto" nel pannello Log eventi**: appare quando
  ci sono eventi correnti. Click → `state.eventiIgnoredIds` ingloba
  tutti gli id correnti → spariscono da banner + log senza doverli
  cliccare uno a uno. Toast di conferma con il numero di eventi
  ignorati.

## [v2.9.12] — 2026-05-06

### Modificato

- **Card studente — semantica dei 3 indicatori chiarita**:
  1. **Pallino top-left** (`.status`): rappresenta SOLO lo stato del
     proxy.
     - verde: heartbeat proxy recente (<15s)
     - rosso: heartbeat in passato ma silente ora → bypass sospetto
     - grigio: proxy mai visto (PC scoperto via LAN scan ma proxy
       non installato/avviato)
  2. **Pallino bottom-left** (`.watchdog-dot` nel piede): rappresenta
     lo stato dei plugin watchdog **abilitati nelle Impostazioni**.
     - verde: tutti i plugin abilitati pingano regolarmente (<15s)
     - giallo: alcuni mancanti (ma non tutti)
     - rosso: TUTTI mancanti — kill globale dei watchdog
     - grigio: nessun plugin abilitato
  3. **Bordo della card**: prende il **peggior colore** tra i due
     pallini (verde<giallo<grigio<rosso). Override speciali: AI
     (rosso saturato + glow) e selected (info + outline) vincono sul
     bordo "peggio".

### Aggiunto

- **Tracking per-plugin heartbeat lato client**: nuova mappa
  `state.alivePluginMap` (ip → plugin → lastTs). Idratata a boot via
  `/api/history` (campo `alivePlugins`) e aggiornata in tempo reale
  via SSE `{type:"plugin-alive", ip, plugin, ts}` emesso dal server
  ad ogni `/api/watchdog/heartbeat` (un evento ogni 5s per ogni
  coppia ip+plugin attiva).

## [v2.9.11] — 2026-05-05

### Risolto

- **Pallino top-left della card si coloriva di arancione per eventi
  watchdog** (bug UI v2.9.10). Il pallino top-left rappresenta lo
  stato del proxy (online/offline/idle/AI/selected): non deve
  cambiare colore quando un plugin watchdog emette un warning.
  Ora il segnale visivo per eventi watchdog e' SOLO il bordo
  arancione della card + il pallino watchdog-dot in basso.

- **Pallino watchdog-dot in basso non rifletteva eventi plugin**:
  prima si basava SOLO sul ping del proxy_watchdog.vbs (alive map).
  Ora considera anche eventi plugin recenti (5 min):
  - rosso: c'e' un evento `critical` recente (es. tutti i plugin
    silenti = sospetto kill globale);
  - giallo: c'e' un evento `warning` recente (es. plugin singolo
    stopped, USB inserita, processo sospetto avviato);
  - verde: proxy alive, nessun evento;
  - grigio: proxy mai visto.

- **Detail pane studente — pallini per-plugin non si aggiornavano
  quando il plugin veniva killato**. Il filtro eventi cercava solo
  `ev.plugin === "usb"`, ma i meta-eventi emessi dal server quando
  un plugin va silente hanno plugin = `"watchdog-usb"` (con
  prefisso). Il filtro ora matcha entrambi: cosi' se lo studente
  killa il watchdog USB, il pallino del plugin USB nel detail pane
  si colora correttamente.

## [v2.9.10] — 2026-05-06

### Risolto

- **Plugin PowerShell che si accumulano ad ogni Send proxy** (bug
  noto da v2.9.7-9, fix incompleto in v2.9.9). Causa duplice:
  1. Il kill PowerShell del Step 2 di proxy_on.vbs poteva fallire
     anche col self-skip (es. `Get-CimInstance` ritorna alcuni
     processi con CommandLine null, che non vengono filtrati e
     passano oltre senza essere killati).
  2. Anche se killasse correttamente, c'era un race tra il kill
     async e i `Start-Sleep -Seconds 5` dentro i loop dei plugin —
     potevano "non arrendersi" subito e lasciare timestamp lock.

  **Fix definitivo**: i 3 plugin .ps1 ora controllano un **flag file
  `$env:TEMP\planck_stop.flag` ad ogni tick** (sia prima che dopo
  lo `Start-Sleep`); quando il flag esiste, escono con `exit 0`.
  Indipendente dal kill esterno.

  proxy_on.vbs Step 2 ridisegnato:
    A) crea il flag → watchdog vecchi exit gentile entro 5s
    B) sleep 7s per dare tempo
    C) kill PowerShell defensivo come backup
    D) delete flag prima di lanciare i nuovi watchdog

  proxy_off.vbs gia' usava il flag per il proxy_watchdog.vbs;
  ora i plugin .ps1 lo rispettano allo stesso modo.

- **Remove proxy non killava i plugin PowerShell**: stessa causa.
  Il proxy_off creava il flag ma SOLO il proxy_watchdog.vbs lo
  controllava — i plugin .ps1 ignoravano il flag e venivano
  killati solo dal PowerShell defensivo, che spesso falliva.
  Ora i plugin si auto-terminano al vedere il flag.

## [v2.9.9] — 2026-05-06

Patch focus: detection real-time degli stati studente, fix bug kill
processi PowerShell che si accumulavano a ogni redistribuzione,
arricchimento liste classificazione domini.

### Aggiunto

- **Detection "all-stopped" critical**: quando TUTTI i plugin watchdog
  di uno stesso IP vanno silenti entro la finestra di check, Planck
  emette un evento aggregato `watchdog-all/all-stopped` con severity
  **critical** (oltre ai warning singoli per ogni plugin). Segnala
  con alta probabilita' un kill manuale dei processi PowerShell sullo
  studente (Task Manager → End Task globale). Implementato in
  `state/watchdog.go` `checkHeartbeats()` Pass 2 + nuovo flag
  `watchdogAllStoppedAlerted` per evitare doppi alert. Reset al primo
  heartbeat ricevuto da quell'IP.

- **Tools di debug** sotto `tools/`:
  - `analizzaeventi`: dump eventi watchdog di una sessione, breakdown
    per plugin/severity + tipi di evento + per IP. Usato per capire
    cosa è rumore (VolumeSnapshot Windows) vs azione studente.
  - `checkstudente`: dump dei domini di un IP specifico in una
    sessione, classificati con `classify.Classifica` + euristica
    candidati AI nei "utente".
  - `ultimirichieste`: ultime N richieste di un IP in una sessione.
  - `inspectsession`: dump della tabella sessioni di un planck.db.
  - `rmsession`: cancella una sessione dal DB (CASCADE su entries
    + watchdog_events).

### Cambiato

- **Heartbeat real-time**: detection silent-plugin ridotta da ~2.5min
  worst case a ~10-15s tipico (max 25s):
  - Plugin watchdog (USB/process/network) ora pingano ogni **5s**
    invece di ogni 30s (`heartbeatEvery = 1` invece di 6).
  - `HeartbeatTimeout` ridotto da **90s** a **15s** (3 ping mancati
    consecutivi = solido segnale di kill, no glitch transitori).
  - `HeartbeatCheckInterval` ridotto da **30s** a **5s**.

- **Card "offline" real-time**: `OFFLINE_PING_MS` lato client ridotto
  da 60s a **15s** (3 ping mancati). Coerente coi plugin: se uno
  studente killa il proxy_on.vbs o spegne il PC, la card diventa grigia
  entro 15-20s anziche' 60s.

- **Pattern AI `kimi.com` → `.kimi.com`**: il bare `kimi.com` matchava
  come substring `eskimi.com` (Eskimi e' una DSP ad-tech, non AI). Fix
  in `embedded_ai_domains.txt`, `data/ai-domains.txt`, cache locale.
  L'aggiunta del leading dot vincola il match al suffisso di sottodominio.

- **`classify.PatternSistema`** arricchita con 23 nuovi pattern adtech
  scoperti dall'analisi della sessione 12 (`Test 4DII Grafici`):
  `.yieldmo.com`, `.pub.network`, `.primis.tech`, `.gumgum.com`,
  `.ccgateway.net`, `.doubleverify.com`, `floors.dev`, `.yellowblue.io`,
  `.2mdn.net`, `.ingage.tech`, `.quantserve.com`, `.admanmedia.com`,
  `measureadv.com`, `.tiktokw.us`, `publisher-services.amazon.dev`,
  `.monetixads.com`, `.betweendigital.com`, `.avads.net`,
  `adx.opera.com`, `oa.opera.com`, `ads.linkedin.com`,
  `ads.unity3d.com`, `config.uca.cloud.unity3d.com`, `.astra.dell.com`,
  `kinesis.` (AWS data streaming).

- **`tools/importsession`**: ora importa anche sessioni "orfane"
  (`sessione_fine NULL`, es. crash o `planck.exe` chiuso senza Stop).
  Per quelle stima fine + durata dall'ultima entry registrata.

### Risolto

- **Processi PowerShell si accumulano a ogni Send proxy**: nello script
  `proxy_on.vbs` Step 2, il PowerShell di kill aveva un filtro
  `Where-Object { $_.CommandLine -match 'planck_.*_watchdog.ps1' }`
  che matchava **se stesso** (la sua CommandLine contiene
  letteralmente quella regex come parametro `-Command`) e si
  auto-uccideva a meta' iterazione → i plugin precedenti
  sopravvivevano. Stessa cosa in `proxy_off.vbs` Step 4. Fix:
  aggiunto `$_.ProcessId -ne $PID` come prima condizione + irrigidita
  la regex con `(usb|process|network)` esplicito e `\.` literal.

- **Banner AI alert legacy** rimosso in v2.9.8: ora il banner alert
  unificato in alto e' l'unica notifica AI visiva (oltre a beep +
  notifica desktop).

## [v2.9.8] — 2026-05-06

### Aggiunto

- **Sezione "Eventi watchdog" nel Report** (full-width, sotto Attività
  studente): lista cronologica inversa degli eventi USB / processi /
  network accaduti durante la sessione. Per archivio: presi da
  `state.datiSessioneVisualizzata.watchdogEvents` (nuovo campo nel
  payload `SessionWithEntries`); per sessione corrente: presi da
  `state.watchdogEvents` filtrati per `ts >= sessioneInizio`.
  - Backend: `SessionLoad` ora query anche `watchdog_events WHERE
    sessione_id = ?`, payload deserializzato + nome studente. Campo
    `WatchdogEvents []json.RawMessage` aggiunto a `SessionWithEntries`.
  - Frontend: `renderReportEventi(eventi, studentiMap)` produce
    righe con dot severity (warn/alert/muted) + timestamp mono +
    plugin label (USB/Processi/Network) + studente·IP + dettaglio
    payload key:value.

### Rimosso

- **Banner AI legacy** (`lampeggiaBannerAI`): popup rosso "ATTENZIONE
  accesso AI rilevato" che lampeggiava 5s al primo dominio AI di una
  sessione. Era ridondante col banner alert unificato in alto
  (`renderAlertBanner`) introdotto in v2.9.0. Beep + notifica desktop
  spostati direttamente nell'handler SSE `traffic` quando l'entry
  e' di tipo AI.

## [v2.9.7] — 2026-05-06

Patch release.

### Risolto

- **Floating selection bar flickerava** rendendo i bottoni
  (Blocca/Sblocca schermo, Messaggia, Proxy on/off, Blocca dominio,
  ✕) impossibili da cliccare durante la multi-selezione. Causa: ogni
  `renderAll()` ricostruiva l'innerHTML della pillola → bottoni
  distrutti durante il click. Fix: content-key skip basato su
  `selectedIps.size + veyonConfigured` — la pillola si rebuilda solo
  quando cambia il count o lo stato Veyon, non ad ogni evento SSE.
  Stesso pattern dei fix flicker di detail-pane e log-pane in v2.9.0.

## [v2.9.6] — 2026-05-06

Polish dopo testing reale a scuola: rifacimento tab Report e Impostazioni
in stile Linear, naming sessione, hot-swap auth, rec come toggle, alert
nativi sostituiti con toast/modal, scan analizzato per nuovi domini di
sistema.

### Aggiunto

- **Naming sessione dopo Stop**: alla fine della registrazione appare un
  modal in stile app per dare un nome custom (es. "Verifica Storia 5B"),
  che diventa il display name dell'archivio nel dropdown Report e nella
  lista Impostazioni → Archivio. Saltabile (resta default "Planck Proxy").
  Endpoint `POST /api/session/rename {id, titolo}` + `SessionRename`
  store helper. `GET /api/sessioni` ora ritorna `[{filename, titolo,
  inizio, fine, durataSec}]` invece di array di filename.

- **Modal custom in stile app** (`js/modal.js`) sostituisce `prompt()`
  nativo per "Invia messaggio" Veyon e "Naming sessione". Overlay
  scuro + card 440px centrata, header surface-2 con titolo + X,
  textarea ridimensionabile, bottoni Annulla/Invia. Esc / click fuori
  / ✕ chiudono. Ctrl/Cmd+Enter conferma. API riusabile via
  `showPromptModal({title, message, defaultValue, placeholder, okLabel,
  cancelLabel})` → `Promise<string|null>`.

- **Pulsante Rec sessione come toggle**: rimosso bottone Stop separato.
  Click su "Rec sessione" (idle, primary rosso) avvia. Click su "Sta
  registrando" (filled alert + dot pulse) ferma e archivia. Pattern
  uniforme a Blocca tutto / Blocca AI.

- **Tab Report ridisegnata** in stile Linear coerente con Live:
  - Toolbar in alto: dropdown sessione (con titolo custom + data/ora)
    + Elimina archivio + titolo report a destra.
  - Stat strip 5 col (riusa `.stat-row` della Live): Durata / Richieste
    / Domini / Bloccate / In blocklist, ognuno con label uppercase 10px
    + numero 22px tabular + sub-line.
  - 3 sezioni dense (Top AI / Top 10 domini / Attività studente):
    righe `nome | barra proporzionale | count tabular-right` invece dei
    box con barre orizzontali viola di prima.

- **Tab Impostazioni in 7 sub-tab** (rectangular pill nav coerente con
  i tab principali):
  - **Generale**: profilo, comportamento proxy (modo/inattività),
    discovery & notifiche.
  - **Rete & Auth**: porte (richiede riavvio) + auth (applicata subito,
    hot-swap del middleware `RequireAuth`).
  - **Domini & AI**: domini ignorati + lista AI auto-aggiornata.
  - **Watchdog**: lista plugin USB/process/network.
  - **Archivio**: sessioni archiviate.
  - **Veyon**: stato + test connessione.
  - **Sistema**: endpoint API debug (export/status/sessioni/health/version).
  - Sub-tab persistito in `localStorage` (`settingsSubtab`).

- **Toggle EVENTI button** in toolbar (testuale, sempre disponibile)
  per aprire/chiudere il pannello Log eventi anche senza alert attivi.

- **Bottone "Sblocca schermo"** nelle azioni rapide del detail pane
  (era raggiungibile solo dalla multi-selezione). Layout 5 bottoni:
  Blocca/Sblocca schermo + Messaggia + Disconnetti proxy in 2x2 +
  Blocca dominio (rosso, full-width).

- **Tools**:
  - `tools/importsession`: copia sessioni archiviate da un planck.db
    sorgente al destinazione (rimappa id, copia entries + watchdog
    events). Usato per recuperare la sessione di test fatta a scuola
    da un binario v2.9.0.
  - `tools/analizzasessione`: classifica i domini di una sessione con
    `classify.Classifica` e suggerisce candidati per nuove voci nelle
    liste AI / Sistema, basato su euristica nome.

### Cambiato

- **`HideOwnConsole`** ora chiama anche `FreeConsole()` dopo
  `ShowWindow(SW_HIDE)`. Senza il detach esplicito, la finestra cmd
  restava "minimizzata in taskbar" lasciando un'icona persistente.
  Ora il processo non ha piu' una console attached: zero icone fantasma.

- **EVENTI / STREAM / DOMINI** in toolbar (testuali) sostituiscono le
  icone SVG dei toggle log/stream/sidebar. Bottone EVENTI spostato a
  destra del search input. Feedback visivo `.attivo` (verde) coerente
  quando il rispettivo pannello e' aperto.

- **Confirm popup nativi rimossi** dalle azioni non distruttive:
  Rec sessione (toast "Registrazione avviata"), Send proxy, Remove
  proxy, Lock screens, Unlock screens. Solo le azioni davvero
  irreversibili (Reset, Reboot/Spegni classe, Elimina sessione,
  Spegni server) mantengono il `confirm()` nativo.

- **`veyonForEachTarget(label, fn, skipConfirm)`**: parametro nuovo
  per saltare il confirm su lock/unlock/msg/proxy on/off mantenendolo
  per reboot/poweroff.

- **Stati card refactored a logica `aliveAgo` + `trafficoAgo`** separati:
  - `offline` (grigio scuro): nessun ping watchdog da > 60s.
  - `idle` (grigio chiaro): ping ok ma traffico utente assente da > 3
    min (incluso "mai navigato dopo Send proxy").
  - `active` (verde): ping ok + traffico recente.
  - Override: `watchdog` (5 min cutoff) → `ai` (10 min) → `selected`.
  - Risolve "card resta colorata anche dopo riavvio Planck" e
    "non distinguibile online ma non naviga vs online attivo".

- **Stato `locked` della card**: tracking client-side via
  `state.lockedIps`. Quando l'utente blocca lo schermo via Veyon
  da Planck, la card mostra un overlay scuro semitrasparente (.55
  alpha) con icona lucchetto centrata. Sblocco → overlay sparisce.
  Vista lista: riga con bg `info-bg` + lucchetto blu nella colonna
  status.

- **`classify.PatternSistema`** arricchita con 10 nuovi pattern
  scoperti dall'analisi della sessione di scuola: `events.data.msn.`
  (variante CN), `.gfx.ms`, `.bromium-online.com`, `analytics*.istruzione.it`,
  `.sprig.com`, `.imrworldwide.com`, `ad-delivery.net`, `.trustarc.com`.

- **Click sui chip dominio nelle card** non blocca piu' il dominio:
  apre il detail pane (uniforme col resto della card). Il blocco
  per-IP avviene dal bottone "Blocca dominio" del detail pane.

- **Watchdog events panel sopra la grid** rimosso: era ridondante col
  banner alert + log eventi.

- **Single-instance lock** via `planck.pid`: kill istanza precedente
  al boot (sleep 800ms per il rilascio della porta TCP).

- **ESC chain finale**: multi-sel > detail pane > log pane > focus IP
  > sidebar dx > sidebar sx.

### Risolto

- **Timer report archive che continuava ad aumentare**: `renderReport`
  usava `state.datiSessioneVisualizzata.esportatoAlle || Date.now()`
  come "fine sessione" per calcolare la durata. `esportatoAlle` era
  un campo legacy v1, mai popolato dal backend SQLite v2 → cadeva
  sempre su `Date.now()` → durata cresceva ogni 5s mentre guardavi
  un archivio. Ora calcolata da `durataSec` (persistito in
  `SessionClose`).

- **Dropdown sessione vuoto in tab Report**: il `<select>` era
  popolato solo in `renderImpostazioni`, che fa early-return se non
  sei in tab Impostazioni. Estratta la logica in
  `aggiornaSelectSessioniArchivio()`, chiamata anche da
  `renderReport`.

- **Stat strip Report invisibile**: avevo usato `class="stats"` (non
  esiste) invece di `class="stat-row"` (la classe usata dalla Live
  per il grid 5 col).

- **"Bottone Eventi" / "Stream" / "Sidebar" SmartScreen Defender**:
  build cambiata a console subsystem + runtime hide → niente piu'
  popup "App non riconosciuta" al primo avvio. Effetto a cascata:
  anche i `.vbs` distribuiti via Veyon FileTransfer non vengono piu'
  flaggati (il processo master non e' piu' "sospetto").

- **Hint "richiede riavvio"** mostrato sulla sezione Autenticazione
  nelle Impostazioni: era sbagliato. Auth e' hot-swap (middleware
  `RequireAuth` legge `state.AuthInfo()` ad ogni request).

### Known issues

- Le porte (`proxy.port`, `web.port`) richiedono ancora riavvio: il
  TCP listener e' bound una sola volta in `main.go`. Restart graceful
  del listener possibile ma non implementato.
- Stato locked tracciato client-side: se l'utente blocca da un altro
  Veyon Master non Planck, lo state non si aggiorna.

## [v2.9.5] — 2026-05-05

Iterazione di polish su v2.9.0 dopo testing reale su VM. Focus: multi-
selezione completa, banner alert unificato + log eventi, scan subnet
adattivo, stati card a 3 step, fix Defender false positive.

### Aggiunto

- **Multi-selezione completa** secondo design Claude:
  - Cmd/Ctrl+click toggla l'IP nella selezione; se c'era un detail
    aperto, il suo IP viene incorporato nella selezione (non perso).
  - Shift+click range da anchor a IP; se c'era detail aperto, il suo
    IP entra nel range. Detail/focus chiusi in entrambi i casi.
  - Plain click toggla detail pane, azzera selezione multi.
  - Anchor aggiornato sempre all'IP cliccato.
  - Floating selection bar pillola: centrata bottom 24px, pillola
    `radius: 999px` con shadow soft, contenuto `N selezionati · sep ·
    bottoni azione (Blocca schermo / Sblocca / Messaggia / Proxy on /
    Proxy off / Blocca dominio) · sep · ×`. Le azioni agiscono SOLO
    sul subset selezionato (gli endpoint Veyon usano `targetIps()`).
    Card multi: outline `info` + bg `info-bg`. Riga lista multi: bg
    `info-bg` + border-left `info`.

- **Banner alert unificato AI + Watchdog** (sotto topbar, sopra layout):
  - Visibile solo se `total > 0 && !dismissed`. Auto-reset al cambio
    di event-key (nuovo evento riapre il banner se l'utente l'aveva
    chiuso).
  - Pill `AI` rosso e/o `WD` giallo (animation `planck-pulse 2.4s` se
    kind=`pulse`, default).
  - Headline `N event* · M AI · K watchdog` + sample text con ellipsis.
  - Bottone "Apri log" + ✕ dismiss.
  - 3 varianti `bannerKind`: `pulse` (default tinted+animato), `sticky`
    (tinted no-animation), `slide` (filled solid).
  - `prefers-reduced-motion: reduce` disabilita animation.

- **Log eventi pane** (pannello laterale destro, mutex con stream/detail):
  - Header: icona log + "Log eventi" + count totali + ✕.
  - Filtri rapidi: Tutti · N / AI · N (rosso) / WD · N (giallo).
  - Feed cronologico inverso: dot color + ts mono + (pill tipo + nome) +
    dettaglio + 3 azioni mini (Apri studente / Blocca dominio AI /
    Ignora).
  - "Ignora" aggiunge id evento a `state.eventiIgnoredIds`, sparisce
    dal feed e dal banner.
  - Bottone toggle `LOG` nella toolbar primaria (testuale, non icona)
    a sinistra del search input — sempre disponibile anche senza
    eventi attivi. `.btn.attivo` (verde) quando il pannello e' aperto.

- **Subnet-aware LAN scan**: `discover.LocalSubnet(lanIP)` legge la
  mask reale dell'interfaccia di rete del docente. Lo scan adatta
  automaticamente al /24, /23, /22, ecc. Subnet > /22 (>1024 host)
  ricadono su /24 implicito attorno a lanIP per evitare scan massivi.
  Worker pool a 128 connect concorrenti. Risolve il problema "a casa
  i PC sono .78-.80 invece di .1-.30".

- **Toggle "Solo PC con Veyon"** in Impostazioni → Generale:
  - Setting persistito (`discoverVeyonOnly`, default `true`).
  - Quando attivo, lo scan considera vivi SOLO i PC con :11100 aperto
    (Veyon Service installato). Esclude router/NAS/altri Windows.
  - Default OFF probe `:11100, :445, :135` per setup nuovi.
  - Override env `PLANCK_DISCOVER_VEYON_ONLY=0/1` al boot.

- **Bottone "Sblocca schermo"** nel detail pane azioni rapide (era
  raggiungibile solo dalla multi-selezione). Layout 5 bottoni: 2x2
  `(Blocca schermo, Sblocca schermo, Messaggia, Disconnetti proxy)`
  + riga "Blocca dominio" (rosso) full-width.

- **Stato "screen locked" della card**: tracking client-side via
  `state.lockedIps`. Quando l'utente blocca lo schermo via Veyon
  da Planck, la card mostra un overlay scuro semitrasparente (.55
  alpha) con icona lucchetto centrata. Sblocco → overlay sparisce.
  Vista lista: riga con bg `info-bg` + lucchetto blu nella colonna
  status. Reset toolbar pulisce anche `lockedIps`.

- **ESC chain esteso**: priorita' `multi-sel > detail pane > log
  pane > focus IP > sidebar dx > sidebar sx`. Premendo ESC
  ripetutamente smonta tutto step-by-step.

- **Single-instance lock** via `planck.pid` accanto al binario:
  killa l'istanza precedente al boot (con sleep 800ms per il
  rilascio della porta TCP).

### Cambiato

- **Build flag**: rimosso `-H=windowsgui` (subsystem GUI) — era il
  trigger principale di SmartScreen/Defender false positive ("App
  non riconosciuta" + bloccata). Tornato a console subsystem normale
  (come v2.6.x). La finestra cmd viene nascosta a runtime via
  `sysutil.HideOwnConsole()` (`ShowWindow(GetConsoleWindow(), SW_HIDE)`):
  flash brevissimo al boot ma niente piu' avviso anti-virus. Effetto
  collaterale: anche i `.vbs` distribuiti via Veyon FileTransfer non
  vengono piu' flaggati (Defender non li associa piu' a un processo
  master sospetto).

- **Stati card a 3 step** con cutoff temporali separati:
  - `offline` (grigio scuro, opacity .35): nessun ping watchdog da
    > 60s (proxy non attivo).
  - `idle` (grigio chiaro, opacity .55): ping ok ma traffico utente
    assente da > 3 min (incluso "mai navigato dopo Send proxy").
  - `active` (verde, piena): ping ok + traffico recente.
  - Override semantici: `watchdog` (arancio) entro 5 min dall'ultimo
    evento warn/critical, `ai` (rosso) entro 10 min dall'ultima
    richiesta AI, `selected` (blu) detail aperto.
  - Risolve "card resta colorata anche dopo riavvio Planck" e
    "non distinguibile online ma non naviga vs online attivo".

- **Detail pane flicker fix**: ora salta il rebuild di `innerHTML`
  se la "content key" (ip + stato + count entries + count blocchi
  + count plugin) e' invariata. Senza, ogni renderAll (ad ogni
  evento SSE + setInterval 5s) distruggeva e ricostruiva il DOM
  dei bottoni → click "perso" → flicker. Stesso fix su `renderLogPanel`.

- **Card studente click sui chip dominio**: rimosso `data-action="blocca"`
  dai chip → cliccare ovunque sulla card apre il detail pane (prima
  cliccare un chip bloccava il dominio globalmente). Per bloccare
  un dominio: bottone "Blocca dominio" del detail pane (per-IP) o
  sidebar sx Domini.

- **Watchdog events panel** (la striscia gialla sopra la grid IP)
  rimosso: era ridondante col banner alert + log eventi.

- **`UnblockAllAI`** doppio sweep: prima rimuove le stringhe esatte
  da `classify.AIDomains()`, poi rimuove qualsiasi dominio nel set
  bloccati che `Classifica()` riconosce come `TipoAI` (cattura voci
  residue di vecchie liste AI dopo refresh remote).

- **`SessionStart` no-op se gia' attiva**: archivio solo on-Stop.
  Toast info "Premi Stop per archiviarla" se l'utente clicca Rec
  con sessione gia' attiva.

- **`SessionStop` confirm** arricchito con durata HH:MM:SS + count
  eventi non-sistema calcolati al volo dal client.

- **Bottone Rec sessione** stato visivo "recording": classe
  `.btn-primary.recording` (filled alert + alone box-shadow + dot
  bianco con animazione `rec-pulse`), label "Sta registrando",
  disabilitato durante registrazione (no double-start).

- **Icona dell'app** rifatta dal brand-dot del logo (quadrato nero
  arrotondato, sostituisce la "P" su cerchio viola). 6 risoluzioni
  embeddate via `rsrc_windows_amd64.syso` + servito come
  `favicon.ico` per il browser.

### Risolto

- **Render iniziale rotto** (sintomo: i dispositivi comparivano solo
  dopo aver toggleato sidebar o cambiato tab). Causa: due null
  reference in renderer chiamati durante `init()`
  (`aggiornaSelectPresets` su `#preset-select`, `renderFocus` su
  `#panel-ip-titolo` rimossi nel redesign). Throw che interrompeva
  `init()` prima di `avviaSSE()` → SSE mai connesso → toggle non
  riflettevano lo stato reale. Fix: null guards + try/catch isolato
  per ogni renderer in `_renderAllSync` (logga `[planck] renderer
  crash: <name>` e prosegue).

- **`renderCountdown` riga 370 NPE**: countdown rimosso dalla topbar
  ma il renderer non aveva null guard; crashava bloccando la cascata.

- **`setStato` SSE crasha su `parentElement` di null**: nuova logica
  che aggiorna la pill `.stat-card.status .live-pill` (toggle
  `dot.ok ↔ dot.alert` + label LIVE/OFF), null-guard se la card e'
  assente in qualche layout.

- **"Pagina non raggiungibile" al primo avvio**: Edge si attaccava
  a istanza fantasma (lock files Chromium residui) → `cmd.Wait()`
  ritornava in 13ms → Planck `os.Exit(0)`. Fix: cleanup di
  `SingletonLock`, `SingletonSocket`, `SingletonCookie`, `lockfile`
  prima di lanciare Edge + soglia 2s che mantiene Planck attivo se
  l'attach e' prematuro.

- **Toggle "Blocca AI" non aggiornava lo stato del proxy** sebbene
  cambiasse grafica: stessa causa del render iniziale (SSE non
  connesso a causa del crash in init). Risolto coi null guards.

- **Defender flagga il binario .exe** (false positive tipo
  "Trojan:Win32/Wacatac"): risolto rimuovendo `-H=windowsgui`.
  Pattern PE "console subsystem unsigned" non e' piu' suspicious.

- **Bottoni del detail pane non cliccabili** per flicker: il
  rebuild di innerHTML ad ogni renderAll distruggeva il bottone
  durante il click. Risolto con content-key skip.

### Known issues / da fare

- "OS / browser / MAC" nella sezione Sessione del detail pane sono
  segnaposto `—`: Planck non raccoglie ancora User-Agent o ARP-MAC.
- Stato "screen locked" e' tracciato client-side: se l'utente blocca
  da un altro Veyon Master (non Planck), Planck non lo sa.
- Bottone "Disconnetti proxy" del detail pane filtrato a singolo IP
  — non ancora testato su PC studente reale.

## [v2.9.0] — 2026-05-05

Iterazione successiva al redesign v2.8.0: chiusura del lavoro su detail
pane, vista lista, sidebar dx full-height, blocchi per-IP, fix avvio,
fix render iniziale che impediva il funzionamento dei toggle.

### Aggiunto

- **Blocchi per-IP** (additivi rispetto alla blocklist globale):
  - Schema: nuova tabella `bloccati_per_ip(ip, dominio, added_at)`
    (migration v3, applicata al primo avvio).
  - State: `blocchiPerIp map[ip]map[dominio]struct{}` con
    `BlockForIp` / `UnblockForIp` / `ClearBlocksForIp` + broadcast
    SSE `blocchi-per-ip`.
  - Logica blocco: `DominioBloccato(dominio, clientIP)` controlla
    sia la blocklist globale sia `blocchiPerIp[clientIP]` (OR).
  - API: `POST /api/block-per-ip {ip, dominio}`,
    `POST /api/unblock-per-ip {ip, dominio}`,
    `POST /api/clear-blocks-for-ip {ip}`.
  - `ClearBlocklist` svuota anche tutti i blocchi per-IP per
    coerenza con il bottone Reset.
  - UI: sezione "Blocchi attivi" nel detail pane con bottone × per
    rimuovere singoli; badge `⊘N` rosso accanto al nome (card) o
    all'IP (riga lista) se l'IP ha blocchi per-IP attivi.

- **Detail pane laterale destro** (`#detail-pane`, 280px):
  - Click su una card o riga lista lo apre per quell'IP; click sulla
    stessa card o sulla X chiude. Mutex con stream live (l'altro
    pannello narrow): quando detail aperto, stream nascosto.
  - Header: status dot + nome studente + IP mono + bottone X.
  - Banner AI condizionale (rosso) con dominio AI rilevato + ts.
  - Azioni rapide 2×2: Blocca schermo / Messaggia / Disconnetti
    proxy / Blocca dominio (rosso, prompt() pre-popolato con
    ultimo dominio AI dell'IP, blocco per-IP via `/api/block-per-ip`).
  - Watchdog: lista plugin con label corte (USB / Processi / Network)
    e messaggio "ok" specifico per stato verde, oppure dettaglio
    evento warning/critical troncato.
  - Domini recenti · N richieste: top 12 domini ordinati per ultima
    attività; AI in rosso. Sessione: connesso / durata / ultima.

- **Vista lista 8 colonne** (Status dot / Studente / IP / REQ /
  Ultima / Domini recenti / Watchdog / Stato testuale): righe `idle`
  in `text-3` con opacità ridotta, header sticky uppercase 10px.

- **Bottone Reset** in toolbar accanto a Blocca tutto / Blocca AI:
  svuota blocklist (globale + per-IP), disattiva pausa globale,
  sblocca tutti gli schermi via Veyon (silenzioso, no confirm
  secondari, una sola conferma utente all'inizio).

- **Endpoint `/api/health`** non autenticato per `WaitForHTTP`
  affidabile in fase di boot.

- **Single-instance lock** via `planck.pid` accanto al binario:
  killa l'istanza precedente al boot (con sleep 800ms per il
  rilascio della porta TCP) — risolve "porta già in uso" al
  secondo avvio se il processo precedente era rimasto orfano.

- **Stream live header**: dot ok + label "Stream live" + rate
  `~X.X/s` calcolato come `last60 / 60`.

- **Hide scrollbars globali**: scroll resta funzionante (rotella,
  trackpad) ma le barre laterali non sono più rese.

- `internal/sysutil/`: `HideConsoleWindow` (CREATE_NO_WINDOW Windows,
  no-op altri OS), `WaitForPort` (TCP), `WaitForHTTP` (GET reali
  con timeout 300ms per chiamata).

### Cambiato

- **Sidebar destra full-height**: stream e detail pane sono ora
  sibling diretti di `.sidebar` e `.main` dentro `.layout`. Si
  estendono dalla topbar al fondo, fianco a fianco con tutte le
  toolbar (che restano confinate in `.main`).

- **Sidebar sinistra "Domini"** ridisegnata: chevron > cliccabile
  + label uppercase + count `text-3` allineato a destra (niente più
  parentesi). Sezioni AI / Siti / Sistema / Bloccati / Nascosti
  sempre visibili (anche vuote). Solo "AI" ha label rosso (`var(--alert)`),
  niente più sfondo tinted. Voci dominio mono 10.5px con padding
  allineato sotto al chevron. I bottoni block/unblock sono icone
  SVG che appaiono solo on hover (sempre visibili in alert sui
  bloccati). `+`/`−` di Sistema/Nascosti rimossi (chevron unificato).

- **Card studente**: badge `⊘N` accanto al nome quando l'IP ha
  blocchi per-IP attivi; al click la card apre il detail pane
  invece del solo focus-ip filter.

- **Vista griglia/lista**: stato derivato uniforme
  (active → idle → watchdog → ai → selected) sia nelle card sia
  nelle righe della tabella, con `data-state` come fonte di verità.

- **Pulsante Rec sessione**: stato visivo "recording" con classe
  `.btn-primary.recording` (filled alert + alone box-shadow + dot
  bianco con animazione `rec-pulse`), label cambia in "Sta
  registrando", disabilitato durante la registrazione (no
  double-start). Stop diventa enabled solo quando c'è una sessione
  attiva. Timer `● rec HH:MM:SS` mono visibile solo in recording.

- **Stop sessione**: confirm dialog arricchito con durata
  (HH:MM:SS) e count eventi non-sistema, calcolati al volo.

- **Archiviazione sessione**: ora avviene **solo** premendo Stop.
  `SessionStart` su una sessione già attiva è un no-op (l'utente
  vede un toast: "Premi Stop per archiviarla prima di avviarne
  un'altra"). Era prima auto-archiviante in fase di Start, dietro
  alle quinte.

- **Detail pane Watchdog labels**: USB / Processi / Network (corte)
  con `okDetail` specifico per ognuno (`nessun dispositivo`,
  `nessun processo sospetto`, `instradato via proxy`).

- **Stream live (panel destro)**: header design-aligned (dot ok +
  label + rate auto-right), riga `.stream-row` grid 52px+1fr mono
  10px, no border-bottom righe, AI con bg+color alert su tutta la
  riga, no più nome studente / parentesi quadre / pill solo sul
  dominio.

- **Toggle Blocca AI**: ora legge lo stato dal DOM (`btn.classList`)
  e fa feedback ottimistico immediato. `UnblockAllAI` lato server
  fa doppio sweep — prima per stringa esatta da
  `classify.AIDomains()`, poi per `Classifica()` su ogni dominio
  bloccato — per catturare voci residue di vecchie liste AI.

- **Net.Listen esplicito** + `http.Serve` in goroutine: il TCP è
  in stato LISTEN al ritorno di `net.Listen`, le connessioni entrano
  nella backlog del kernel anche prima che `Serve` chiami `Accept`.

- **WaitForPort + WaitForHTTP** prima del browser launch:
  - `127.0.0.1:9090` (proxy) — se il PC docente ha proxy_on attivo,
    Edge passa per il proxy anche per localhost:9999.
  - `/api/health` — verifica HTTP reale (non solo TCP listening).
  - `time.Sleep(500ms)` aggiuntivo: Edge a freddo crea il profilo
    `--user-data-dir` e può fare la prima GET prima di stabilire
    la connessione.

- **Browser launch**: cleanup dei lock files Chromium
  (`SingletonLock`, `SingletonSocket`, `SingletonCookie`, `lockfile`)
  prima di lanciare Edge per evitare l'attach a istanza fantasma.
  Se `cmd.Wait()` ritorna in <2 secondi, Planck **non** si auto-spegne
  (era attach a istanza esistente, non un vero exit utente).

- **Icona dell'app** rifatta a partire dal brand-dot del logo:
  quadrato nero pieno centrato (~62% del canvas) con angoli
  arrotondati 2/10 del lato. Sostituisce la "P" su cerchio viola.
  6 risoluzioni (16/32/48/64/128/256) embeddato via
  `rsrc_windows_amd64.syso` + servito come `favicon.ico` per il
  browser.

### Risolto

- **Render iniziale rotto** (sintomo: i dispositivi comparivano
  solo dopo aver toggleato sidebar o cambiato tab). Causa: due
  null reference in renderer chiamati durante `init()` →
  `aggiornaSelectPresets` su `#preset-select` rimosso, e
  `renderFocus` su `#panel-ip-titolo` rimosso, throw che
  interrompeva `init()` prima di `avviaSSE()`. Fix: null guards
  + try/catch isolato per **ogni** renderer in `_renderAllSync`
  (logga `[planck] renderer crash: <name>` e prosegue).

- **Toggle Blocca AI / Blocca tutto non funzionavano** (cambiavano
  grafica ma non lo stato). Stessa causa del render iniziale: SSE
  non veniva mai connesso perché `init()` crashava prima di
  `avviaSSE()`. Risolto coi null guards di cui sopra.

- **"Pagina non raggiungibile" al primo avvio** (sintomo: dovevi
  chiudere e riaprire il binario). Causa: lock files Chromium
  residui causavano `cmd.Wait()` di Edge a ritornare in
  pochi millisecondi → vecchio comportamento `os.Exit(0)` →
  Planck moriva prima che la pagina caricasse. Risolto con
  cleanup lock files + soglia 2s che mantiene Planck attivo
  invece di auto-spegnersi.

- **`setStato` SSE crasha su `parentElement` di null** (cambiamento
  delle stat card nel redesign): nuova logica che aggiorna la pill
  `.stat-card.status .live-pill` (toggle dot.ok ↔ dot.alert + label
  LIVE/OFF), null-guard se la card è assente in qualche layout.

### Known issues / da fare

- "OS / browser / MAC" nella sezione Sessione del detail pane sono
  segnaposto `—`: Planck non raccoglie ancora User-Agent o ARP-MAC.
- Bottone "Disconnetti proxy" del detail pane usa lo stesso endpoint
  Veyon di "Remove proxy" filtrato a un IP — non ancora testato su
  PC studente reale.

## [v2.8.0] — 2026-05-04

UI redesign basato su un mockup di **Claude Designer** (claude.ai/design)
condiviso con l'utente come bundle handoff. Iterato fino a una direzione
"Linear-style" precisa: palette neutra grayscale + 4 colori semantici
(ok/warn/alert/info), zero accent decorativo, densita' alta, dark mode
prima-class.

### Cambiato

- **Palette completa rifatta** in `monitor.css` `:root` / `body.dark`:
  - Light: `#fafaf9` bg / `#fff` surface / `#1a1a18` text / `#e2e2df`
    border. Semantici: `#2f9e6a` ok / `#c98a2b` warn / `#c63d3d` alert
    / `#4a7bd1` info / `#9a9a93` muted (con varianti `*-bg` rgba .10-.16).
  - Dark: `#0e0e0d` bg / `#181816` surface / `#f0f0ec` text / `#2a2a27`
    border. Semantici scuri matcheggiati per contrasto.
  - Variabili vecchie (`--card`, `--accent`, `--danger`, ecc.) restano
    come **alias** dei nuovi token: il codice esistente continua a
    funzionare senza modifiche.

- **Spacing/radius scale** standardizzata: `--s-1 .. --s-6` (4-32px),
  `--r-1 .. --r-4` (3-12px). Tipografia: `--font-ui` (system-ui),
  `--font-mono` (ui-monospace) per IP e log.

- **Topbar** rifatta a 38px: brand "■ Planck · Proxy" a sinistra, tabs
  rectangular pill (active = surface-2 + border), topbar-right con
  pausa-indicator + countdown + theme toggle + spegni server.

- **Tab buttons**: stile rectangular (radius `--r-2`, bg `--surface-2`
  on active) invece del precedente underline.

- **Toolbar sessione** Linear-style: bottoni height 26px, radius `--r-2`.
  - "Avvia sessione" → "Rec sessione" (testo allineato al design).
  - "Pausa" → **"Blocca tutto"** con classe `.btn.block` (toggle on/off).
    Off: bordo+testo rosso su `--alert-bg` tinted. On: filled rosso pieno
    + dot bianco pulse top-right.
  - "Blocca AI" stessa pattern `.btn.block` (consistenza visiva).
  - Separator piu' marcato (`.sep.big`) tra Rec/Stop e Blocca.

- **Action toolbar (Veyon)** sempre visibile sotto la toolbar sessione
  (bg `--surface-2`, border-bottom). Era nascosta finche' Veyon non era
  configurato; ora che l'auto-import via veyon-cli rende la chiave
  sempre disponibile (v2.6.0+), ha senso averla sempre presente.

- **Stat strip** 5 colonne in grid con border 1px tra le celle (no piu'
  card separate con shadow). Label uppercase 10px + numero 22px tabular
  semibold + sub line. Status mostra "LIVE" / "OFF" in maiuscolo.

- **Card studente**: padding 8×10 invece di 10×12, radius `--r-2`,
  status dot 6px posizionato top-left fuori dal bordo (`-3px`). Stati:
  `.ai` border alert + box-shadow inset (alta visibilita'),
  `.watchdog` border warn, `.inattivo` opacity .55, `.focus`/`.selected`
  border info + box-shadow inset.

- **Banner AI** sottile: padding `--s-3 --s-4` (stesso della toolbar),
  pill "AI" inline con animation `planck-pulse` (opacity 1→.55, 2.4s).
  Sostituito il `flash-banner` di tipo alternato che era affaticante.

- **Sidebar items**: monospace 10.5px, padding sottile, hover background
  `--hover` morbido, niente border-bottom tra item.

- **Pausa indicator + countdown** sottili (font 10/11px, padding
  ridotto, radius `--r-1`).

### Note

- Il design **Liquid Glass** che era stato esplorato in v2.4.0 e poi
  revertato resta deprecato. Il designer ha chiesto inizialmente
  Liquid Glass ma dopo il primo confronto fianco-a-fianco con la
  variante Flat ha confermato che il glass non funzionava neanche
  come "vetro su chrome / flat su contenuti".
- Il bundle handoff e' ignorato in `.gitignore` (`design/`): contiene
  solo file di lavoro (HTML/JSX/CSS prototype + chat transcript), non
  e' source code da mantenere.
- DB e API sono **invariati**: solo CSS + due microcambi al markup
  (brand topbar, classi `.btn.block` su Pausa/Blocca AI) e al render
  JS (label "Blocca tutto"/"Sblocca tutto", "Rec sessione"/"Stop").

### Per dopo (v2.9+)

Componenti del design non ancora portati:
- Vista lista commutabile (segmented control griglia/lista in toolbar).
- Click su card → detail pane laterale con azioni rapide + storico
  domini + watchdog espanso.
- Multi-select Cmd/Ctrl+click + floating selection bar in basso al
  centro (esiste parzialmente come `selection-bar`).
- Event log panel (feed cronologico AI+watchdog filtrabile).

## [v2.7.0] — 2026-05-04

UX cleanup all'avvio: niente flash della finestra `cmd`, icona Planck
sulla taskbar, lifecycle "single-process" (chiudi la finestra app =
spegni il server).

### Cambiato

- **Subsystem GUI** (Windows): build con `-ldflags "-H=windowsgui"`.
  Il binario non e' piu' un'app console → niente console che lampeggia
  o resta aperta dietro la finestra Edge. L'icona embeddata (v2.6.2)
  diventa anche l'icona del processo nella taskbar.

- **Log su file**: con subsystem GUI non c'e' stderr/stdout, quindi i
  `log.Printf` venivano persi. Ora redirezionati a `planck.log` accanto
  al binario, troncato ad ogni boot (file di sessione corrente, comodo
  per debug post-mortem). Timestamp con microsecondi.

- **Lifecycle browser → server**: la funzione che apriva Edge usava
  `cmd.Start()` e abbandonava il sub-process. Ora usa `cmd.Wait()` +
  `os.Exit(0)`: chiudere la finestra dell'app spegne automaticamente
  Planck. Niente piu' processi orfani da uccidere a mano dopo l'uso.

  Trick chiave: passa `--user-data-dir=<path>` al browser per forzare
  un'istanza isolata e dedicata. Senza, Edge si attaccherebbe a
  un'istanza esistente (se ne hai una) e `cmd.Wait()` ritornerebbe
  immediatamente lasciando la finestra orfana.

- **Favicon servito** dal web server: `internal/web/public/favicon.ico`
  (copia sincronizzata da `assets/planck.ico` via `tools/genicon`) +
  `<link rel="icon">` in `index.html`. Edge in app mode usa il
  favicon come icona della finestra → adesso vedi l'icona Planck
  sulla taskbar invece di quella generica di Edge.

### Aggiunto

- `.gitignore`: ignora `planck.log` e `.planck-browser-profile/`
  (profile dir Edge dedicata e isolata, contiene cookies/cache della
  finestra app — non ha nulla a che vedere col profilo personale di
  Edge dell'utente).

### Note

- `PLANCK_NO_BROWSER=1` resta valido per modalita' headless: niente
  browser, niente auto-shutdown, Planck gira finche' lo killi a mano.
- `planck-linux` non e' interessato dal flag `-H=windowsgui` (e'
  Windows-only). La modalita' app sub-process resta uguale a prima.

## [v2.6.2] — 2026-05-04

planck.exe ora ha un'icona embedded (placeholder).

### Aggiunto

- **`assets/planck.ico`**: icona multi-risoluzione (16/32/48/64/128/256
  px) — "P" bianca su cerchio viola Planck `#8e44ad`. Generata
  programmaticamente da `tools/genicon`. Sostituibile rimpiazzando
  il file con una versione migliore (vedi build.bat per il setup).

- **`cmd/planck/rsrc_windows_amd64.syso`**: blob risorse Windows con
  l'icona embedded. Generato via `github.com/akavel/rsrc` da
  `assets/planck.ico`. Il suffix `_windows_amd64` fa si' che `go build`
  lo linki solo per quel target (cross-compile Linux non lo include).

- **`tools/genicon/main.go`**: tool standalone che disegna l'icona
  programmaticamente (cerchio anti-aliased + "P" composta da
  primitive geometriche, niente font esterni richiesti) e scrive
  `assets/planck.ico` come container ICO multi-risoluzione con PNG
  embedded (formato Vista+).

### Note

- Per rigenerare l'icona (es. dopo aver scelto un design migliore):
  `go run ./tools/genicon` → `rsrc -ico assets/planck.ico -o cmd/planck/rsrc_windows_amd64.syso -arch amd64`.
- Il binario Linux (`planck-linux`) non e' interessato — gli .syso
  Windows non vengono linkati durante il cross-compile.

## [v2.6.1] — 2026-05-04

Patch: fix bug "Rimuovi proxy a volte non si toglie davvero".

### Risolto

- **proxy_off.vbs su Windows 11 24H2+**: usava `wmic process delete`
  per killare il watchdog VBS che riapplica il proxy ogni 5 secondi.
  WMIC e' **deprecato in Windows 11 e rimosso in 24H2** → il kill
  falliva silenziosamente, il watchdog continuava a girare e dopo
  ~5s riapplicava `ProxyEnable=1` in HKCU sovrascrivendo il
  `ProxyEnable=0` settato da proxy_off. Effetto: lato docente
  l'azione "Rimuovi proxy" sembrava riuscita, ma sul PC studente
  il proxy "tornava" subito.

### Cambiato

- **Kill via PowerShell** (`Get-CimInstance Win32_Process` +
  `Stop-Process`): disponibile su Windows 7+, sostituisce
  completamente WMIC. Filtraggio per `CommandLine -match`
  (regex) per ammazzare i processi giusti senza colpire proxy_off
  stesso.

- **Stop flag come kill "gentile"**: proxy_off ora crea
  `%TEMP%\planck_stop.flag`. Il watchdog VBS controlla questo
  file ad ogni iterazione e si auto-termina (settando
  `ProxyEnable=0` per buona misura) quando lo trova. Funziona
  anche se PowerShell fallisse per qualche edge case.

- **Sequenza proxy_off**: 1) crea flag → 2) sleep 6s (un ciclo
  watchdog + margine) → 3) ProxyEnable=0 → 4) kill defensivo via
  PowerShell → 5) cleanup file + flag + self-delete.

- **proxy_on.vbs**: anche il kill dei watchdog precedenti (per
  evitare duplicati al ridistribuire il proxy) ora usa PowerShell
  invece di WMIC. In piu' cancella `planck_stop.flag` al boot,
  altrimenti il watchdog appena lanciato uscirebbe immediatamente.

### Note

- I PC studente che hanno gia' ricevuto il vecchio proxy_on.vbs
  v2.6.0 hanno il watchdog senza check del flag stop. Il fix
  diventa effettivo dopo una "Distribuisci proxy" con questa
  release: il nuovo watchdog include il check.
- Per fermare un watchdog v2.6.0 residuo su un PC studente: una
  sola "Rimuovi proxy" v2.6.1 lo killa via PowerShell (il path
  defensivo); non si appoggia piu' al flag.

## [v2.6.0] — 2026-05-04

**Portabile tra laboratori senza setup.** Rimosso il concetto di
"classe + laboratorio salvato" e l'editing manuale dei nomi studente.
Il binario in chiavetta funziona out-of-the-box in qualsiasi lab della
scuola: ogni boot rigenera mappa studenti e chiave Veyon dal contesto
locale (LAN del PC docente corrente + veyon-cli del PC docente
corrente).

> **Breaking change**: rimosse 6 API (`/api/students/*`, `/api/classi/*`).
> Le tabelle DB `studenti_correnti` e `combo` vengono ignorate ma non
> droppate (preservano integrita' delle sessioni archiviate v2.5.x).

### Cambiato

- **Mappa studenti**: ora una semplice lista degli IP `.1`-`.30` del /24
  del docente, generata in-memory ad ogni boot. Niente piu' nomi
  modificabili, niente piu' persistenza, niente residui da lab
  precedenti. Le card Live mostrano l'IP come label.

- **Chiave Veyon**: NON piu' persistita in DB. Ad ogni boot:
  1. `VeyonClear()` resetta lo stato precedente.
  2. `AutoImportVeyonKey()` re-importa la master key da `veyon-cli`
     del PC docente corrente. Se sei in un lab diverso, prendi la
     chiave di quel lab. Niente residui dalla chiavetta usata altrove.

- **Tab Impostazioni**: rimossa la card "Mappa studenti" (con dropdown
  classe/lab, tabella IP→nome, form aggiungi). La card "Veyon" ora
  mostra solo lo stato (importazione automatica) + il test connessione.

- **Filtro Live**: placeholder cambiato da "Filtra domini/IP/studenti…"
  a "Filtra domini/IP…" (i nomi non esistono piu').

### Rimosso

- API: `POST /api/students/{set,delete,clear}`, `GET /api/classi`,
  `POST /api/classi/{save,load,delete}`.
- Backend: `state.SetStudent/DeleteStudent/ClearStudents/SetStudenti/
  MergeStudentiAuto`. Sostituiti da un singolo `SetStudentiIPs(ips)`
  in-memory only.
- Backend: `store.LoadStudenti/SaveStudenti/LoadClasse/SaveClasse/
  DeleteClasse/ListaClassi`. File `internal/store/{studenti,classi}.go`
  cancellati.
- Backend: package `internal/persist/` (legacy v1.6 file-based, gia'
  non importato dal binario runtime).
- Frontend: `renderMappaStudenti`, `renderSelectCombo`,
  `modificaStudente`, `aggiungiStudente`, `svuotaMappaStudenti`,
  `caricaCombo`, `salvaCombo`, `eliminaCombo`, dispatcher associati.
- Migrate v1: blocchi per `studenti.json` e `classi/*.json` (ora skip).

### Note

- **Restano nei DB esistenti** (sia v2.5.x che migrazioni v1) le righe
  delle tabelle `studenti_correnti` e `combo`. Sono dormienti — il
  binario non le legge piu'. Le sessioni archiviate con
  `entries.nome_studente != ''` mantengono il valore storicizzato.
- **Veyon richiede `veyon-cli`**: ogni PC docente che vuoi usare
  come laboratorio deve avere Veyon Configurator installato (con la
  master key del lab importata nel suo keystore). Senza, Veyon resta
  disabled per quella sessione.

## [v2.5.1] — 2026-05-04

Patch: il ping sweep del /24 introdotto in v2.5.0 si comportava male su
LAN reali (timeout intermittenti, IP non-studente inclusi, latenza
variabile). Sostituito con un approccio piu' semplice e prevedibile.

### Cambiato

- **`internal/discover`** ora genera un range fisso `.1`–`.30` del /24
  del docente (convenzione standard laboratorio scolastico). Una sola
  chiamata sincrona al boot, niente probing periodico, niente shell-out
  a `ping.exe`.

- Le card studente nella grid Live appaiono subito anche per i PC
  spenti — quando lo studente accende e installa il proxy, la card
  esistente comincia a popolarsi senza refresh.

### Note migrazione da v2.5.0

Se hai gia' v2.5.0 installato, la tua mappa studenti puo' contenere
voci scoperte dal vecchio ping sweep (router, NAS, telefoni nel /24).
Apri **Impostazioni → Mappa studenti** e usa il bottone **X** per
rimuovere quelle che non sono PC studente. Le voci `.1`–`.30` sono
quelle generate dalla nuova logica.

### Rimosso

- `discover.Sweep()`, `discover.DefaultTimeout`, `discover.DefaultConcurrency`
  e `pingOnce()`. Il package `discover` ora espone solo `DefaultRange()`,
  `DefaultFirst` e `DefaultLast`.

## [v2.5.0] — 2026-05-04

Auto-setup all'avvio: il software prende da solo la chiave master Veyon
e popola la mappa studenti con i PC attivi nella LAN del docente. Niente
piu' creazione manuale di classe/laboratorio prima di poter usare le
funzioni base.

### Aggiunto

- **`internal/discover/`**: ping sweep concorrente del /24 derivato dal
  LAN IP del docente. Implementazione tramite shell-out a `ping`/`ping.exe`
  (no raw ICMP → niente privilegi admin richiesti). Concorrenza
  semaforica a 32, timeout 250ms per IP, sweep totale ~2s per /24.

  Sweep iniziale al boot (sincrono) + loop periodico ogni 60s per
  scoprire PC accesi successivamente. Gli IP gia' mappati con un nome
  reale non vengono mai sovrascritti.

- **`state.MergeStudentiAuto(ips)`**: aggiunge IP alla mappa studenti
  preservando le voci esistenti. Persiste e broadcasta solo se la
  mappa cambia davvero.

- **`internal/veyon.AutoImport(dataDir)`**: cerca `veyon-cli` nei path
  standard Windows/Linux/macOS, lista le auth keys via
  `veyon-cli authkeys list`, sceglie la prima master (preferendo il
  nome `master` se presente) e la esporta via
  `veyon-cli authkeys export <name>/private`. Fallback silenzioso al
  flusso manuale se veyon-cli manca o nessuna chiave e' importata nel
  Configurator.

- **`state.AutoImportVeyonKey()`**: wrapper boot-time. No-op se
  l'utente ha gia' configurato Veyon manualmente (preserva la sua
  scelta). Errore non-fatale: il flusso `POST /api/veyon/configure`
  resta sempre disponibile come fallback.

### Cambiato

- Il primo avvio non richiede piu' di passare dalle Impostazioni: la
  UI parte gia' con le card studente popolate (IP visti via ping) e
  con i bottoni Veyon attivi (chiave importata da veyon-cli).

### Note

- L'auto-discovery puo' includere device non-studente del /24 (router,
  printer, NAS, telefoni). Si rimuovono dalla UI Impostazioni →
  Mappa studenti → bottone X sulla riga.
- L'auto-import Veyon si attiva solo se `veyon-cli` e' raggiungibile.
  In Linux su distro senza Veyon Configurator l'utente continua col
  flusso manuale.

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

# Planck Proxy — Specifica funzionale

> Reference document del progetto Planck Proxy. Cattura visione, casi d'uso, funzionalita', architettura e roadmap. Costruito a sezioni in modo iterativo con il maintainer; ogni decisione non banale e' discussa esplicitamente qui prima di finire in codice.

---

## Indice

1. [Visione](#1-visione)
2. [Casi d'uso](#2-casi-duso)
3. [Funzionalita'](#3-funzionalita)
4. [Modello dati](#4-modello-dati)
5. [API e protocolli](#5-api-e-protocolli)
6. [UI map](#6-ui-map)
7. [Non funzionali](#7-non-funzionali)
8. [Roadmap a fasi](#8-roadmap-a-fasi)

---

## 1. Visione

### 1.1 Cos'e' Planck Proxy

Software per monitorare i computer degli studenti in laboratorio durante lezioni e verifiche. Il PC del docente fa da proxy HTTP/HTTPS per i PC studente; una dashboard web mostra il traffico in tempo reale, segnala accessi a tool AI (chatbot, code assistant, scrittura assistita, ...), permette di bloccare siti al volo, archivia le sessioni per consultarle dopo.

Da v2.0 in poi, integra anche le funzionalita' di vigilanza tipiche di Veyon (lock schermo, screenshot, messaggi, lancio programmi remoti) parlando direttamente il protocollo Veyon — un'unica dashboard per traffico + controllo PC.

### 1.2 Utente target

Software pensato per **professori di informatica** (target tecnico: insegnano programmazione, sanno cos'e' un proxy, sono autonomi nel setup).

**Bar di UX**: l'interfaccia deve risultare comprensibile e gradevole anche a un prof di altra materia che dovesse trovarsi a usarla — niente tecnicismi gratuiti, niente terminologie che richiedono il manuale.

Distribuzione: maintainer + colleghi della stessa scuola. Niente roadmap di massa per ora.

### 1.3 Problema risolto

Le verifiche di informatica/programmazione sui PC del laboratorio sono tradizionalmente vigilate a vista (il docente cammina, guarda gli schermi). Con la proliferazione di chatbot e tool AI (oltre 100 servizi diversi tracciati attualmente in `domains.js`), la vigilanza visiva non scala: uno studente nascosto apre un chatbot per 30 secondi, copia, chiude.

Veyon permette gia' di vedere lo schermo e bloccarlo a comando, **ma non sa cosa va in rete**. Planck colma il gap: classifica il traffico (AI / utente / sistema), distingue rumore di background da attivita' reale, segnala in tempo reale i tentativi di accesso a servizi AI.

La v2 aggiunge la possibilita' di **reagire** dalla stessa dashboard (lock, screenshot, messaggio) senza dover saltare ad un'altra applicazione, e di automatizzare reazioni (es. "AI rilevata -> lock immediato").

### 1.4 Out of scope

Decisioni consapevoli per evitare scope creep:

- **Nessun MITM HTTPS / decrypt del traffico**. Il proxy vede solo l'hostname del tunnel CONNECT, mai i body delle richieste. La classificazione si basa esclusivamente sul nome del dominio. Aggiungere MITM richiederebbe deploy di certificati custom su ogni studente — battaglia non combattuta.
- **Niente cloud / multi-tenancy / SaaS**. Ogni istanza gira locale, dati locali. Non esiste "Planck Cloud".
- **Niente integrazione con registro elettronico, SSO scuola, LDAP centrale**. Il perimetro e' la rete del laboratorio.
- **Niente supporto a BYOD / mobile / hotspot cellulare**. Il proxy funziona dentro la LAN scuola sui PC della scuola; un hotspot dal telefono dello studente e' un limite strutturale gia' documentato e si gestisce a vista.
- **Niente multi-docente concorrente sulla stessa istanza** (per il modello decentralizzato; vedi 1.5).

### 1.5 Scala attesa e modello di deployment

**v2.x — modello decentralizzato (committed)**: ogni docente avvia Planck sul PC in cattedra del laboratorio in cui sta facendo lezione/verifica. Una istanza = un laboratorio, una classe alla volta. Numeri attesi:
- ~30 studenti per istanza
- ~10-30 docenti potenziali nella scuola
- ~5-10 sessioni/giorno per docente nei picchi

**v2.x+ — modello centralizzato (deferred, non committed)**: una singola istanza Planck per tutta la scuola (~10 laboratori, ~300 studenti concorrenti). Vincolato dalla capacita' della rete dell'istituto di reggere il funnel: 300 studenti × ~100 KB/s ≈ 30 MB/s di picco, dentro la portata di una rete gigabit ma da validare con misure reali sull'infrastruttura specifica. **Decisione rimandata a quando avremo dati di deployment reale dalla v2 decentralizzata**.

---

## 2. Casi d'uso

### 2.1 Modi d'uso: Monitor vs Sessione

Architettura concettuale: **monitoraggio** e **registrazione** sono assi indipendenti.

| Concetto | Cosa fa | Quando attivo |
|---|---|---|
| **Monitor** | proxy + classificazione + UI realtime + banner AI + watchdog + blocklist enforcement | sempre, mentre Planck gira |
| **Sessione** | persistenza su disco (NDJSON + archivio) per consultazione/report | on/off esplicito (default OFF) |

> **Nota sul rewrite v2**: in v1 attuale "sessione ferma" significa "niente broadcast traffic, niente buffer in RAM" — il monitor live era conflato con il recording. La v2 separa i due concetti: il monitor live e' sempre attivo, la sessione governa SOLO la persistenza. Questo abilita l'uso quotidiano durante le lezioni normali senza inquinare l'archivio.

### 2.2 UC1 — Lezione (no sessione)

**Pre**: Planck installato, studenti col proxy gia' configurato (o lo distribuisco al volo via Veyon).

**Flusso**:
1. Avvio Planck (doppio click su `planck.exe`)
2. Apro `http://localhost:9999`, lascio la dashboard sul monitor secondario
3. Durante la lezione lavoro normalmente. Se il banner AI lampeggia, vado a verificare a vista
4. Eventualmente blocco un dominio al volo (click sul tag) o pausa globale

**Post**: chiudo Planck (o lo lascio aperto per la lezione successiva). **Nessun archivio creato**.

### 2.3 UC2 — Verifica programmata

**Pre**: 5 minuti prima della verifica.

**Setup**:
1. Avvio Planck
2. Tab Impostazioni → carico combo `(classe, lab)` → mappa IP→nome popolata
3. Carico preset blocklist (es. `verifica-prog`)
4. **Distribuisci proxy_on.bat** (V2): un click → Planck lancia `RunProgram` Veyon su tutti i PC della mappa
5. Imposto deadline (HH:MM)
6. **Avvia sessione** → inizia recording

**Durante**:
- Griglia/tabella IP a colpo d'occhio (watchdog dot, count attivita', top domini per studente)
- Detection AI → banner rosso + sound + (V2 opt-in) auto-lock dello studente
- Posso filtrare per studente, bloccare al volo, mandare message Veyon a uno specifico, screenshot a richiesta

**Scadenza**:
- Banner "TEMPO SCADUTO" lampeggia, suono triplo

**Post**:
1. **Ferma sessione** → archive automatico (V2: SQLite + export JSON on demand)
2. Tab Report → riepilogo, top domini, top studenti, % bloccate
3. Esporto JSON o screenshot per archivio docente / verbali

### 2.4 UC3 — Piu' verifiche in giornata

**Stesso laboratorio** (es. lab2 ore 8, 10, 12): **una sola istanza** Planck. Tra una verifica e l'altra: Ferma → carico nuova mappa → Avvia. L'archivio cresce, 3 sessioni a fine giornata.

**Laboratori diversi** in contemporanea (es. lab1 e lab3 alle 10): **istanze separate** sui rispettivi PC cattedra. Nessuna interazione, ognuno per se'.

### 2.5 UC4 — Consultazione storica (anytime)

Apro Planck anche fuori da verifiche → tab Report → carico una sessione archiviata dal dropdown → visualizzo aggregazioni → esporto JSON. (V2) Confronto due sessioni della stessa classe per vedere trend.

### 2.6 UC5 — Auto-detection nuovo dominio AI (V2)

Quando Planck vede un dominio mai visto:
1. Heuristic check sul nome (`*ai.*`, `chat.*`, `*gpt*`, `*llm*`, ...)
2. Se sospetto → badge "🤔 Sospetto AI" nella sidebar accanto al dominio
3. Click sul badge → modal "E' AI? [Si', classifica come AI] [No, ignora]"
4. La scelta aggiorna immediatamente la classificazione locale (file utente, separato dalla lista upstream)

**Locale-only**: nessun PR automatico verso il repo community. Le scelte del docente restano sul suo PC. La lista upstream si aggiorna comunque periodicamente in modo automatico (sync), ma la curation upstream resta umana e separata.

### 2.7 UC6 — Reazione automatica (V2, opt-in)

Configurabile in Impostazioni → "Reazioni automatiche":
- Es. su detection AI non bloccato → **ScreenLock** immediato del PC studente (in v2.x: anche Message popup, una volta che TextMessage e' supportato)
- Granularita' a scelta: solo durante sessione attiva, o sempre
- **Default disabilitato**: l'utente lo abilita esplicitamente solo se vuole, per evitare lock accidentali su falsi positivi
- In v2.x verra' ampliato con piu' regole configurabili

### 2.8 UC7 — File transfer agli studenti (V2)

**Use case**: il docente deve consegnare un file a tutti gli studenti del laboratorio (testo della verifica, esercizio starter, scheda riferimento).

**Flusso**:
1. Click "Invia file" nella toolbar (o sulla griglia per inviare a un singolo studente)
2. File picker locale, selezione file/cartella
3. Conferma → Planck usa la feature `FileTransfer` di Veyon per trasferire ai PC target
4. Feedback in UI: barra di avanzamento per studente, success/failure individuali

**Nota**: scope minimo per v2 = "invia a tutti / invia a uno". Funzionalita' piu' raffinate (raccolta file, broadcast cartelle, ecc.) deferred a v2.x.

---

## 3. Funzionalita'

> Catalogo dettagliato di cosa fa Planck v2. Le sotto-sezioni 3.1-3.7 e 3.12 sono in larga parte un porting del comportamento v1; le sotto-sezioni 3.8-3.11 sono nuove e contengono le decisioni piu' rilevanti del rewrite.

### 3.1 Proxy + classificazione traffico

#### 3.1.1 Architettura proxy

Singolo processo Go con due server HTTP:
- **Server proxy** su `config.proxy.port` (default 9090): HTTP forwarding + HTTPS CONNECT tunneling, su `0.0.0.0`.
- **Server web** su `config.web.port` (default 9999): UI + API REST + SSE.

Il server proxy:
- Per HTTP: parse URL, applica `dominioBloccato()`, forwarda con `net/http` client (no body inspection)
- Per HTTPS: handler `connect`, applica `dominioBloccato()` sull'hostname, apre tunnel TCP raw bidirezionale (`net.Dial` + `io.Copy` x2). **Nessun MITM**, nessun cert custom.

Il proxy intercetta `GET /_alive` per il watchdog (vedi 3.7) prima di tentare il forwarding.

#### 3.1.2 Classificazione

Funzione `classifica(dominio)` consulta in ordine:

1. Lista AI (upstream + locale, vedi 3.9) → `tipo = "ai"`
2. Pattern sistema (lista hardcoded in `domains.go`) → `tipo = "sistema"`
3. Default → `tipo = "utente"`

Match per **sostringa case-insensitive** sull'hostname (stessa semantica di v1).

`dominiIgnorati` (configurabile da UI) e' valutato **prima** della classificazione: i domini matchati sono droppati dal log e dalla SSE, e bypassano sempre i blocchi (necessario per `localhost`, `wpad`, ecc.).

⚠️ **Diff vs v1**: la lista AI ora ha tre fonti (upstream sync + locale del docente + heuristic flag). Vedi 3.9.

#### 3.1.3 Buffer in RAM

Ring buffer in memoria delle ultime N entry (per la UI "Ultime richieste" e per l'idratazione dei client SSE che si connettono a meta' sessione).

⚠️ **Diff vs v1**: in v1 c'era `MAX_STORIA = 5000` sia per il buffer UI sia per l'archivio. In v2 il buffer UI mantiene il cap (es. 1000-2000 entry, basta per la live view) mentre la **persistenza** della sessione va in NDJSON append-only senza cap (vedi 3.3).

---

### 3.2 Monitor (sempre attivo)

Mentre Planck gira, il monitor e' **sempre operativo** indipendentemente dalla sessione. Comprende:

- **Proxy attivo + classificazione live**: tutto il traffico HTTP/HTTPS degli studenti passa, viene classificato e broadcastato via SSE alla UI
- **Banner AI**: detection in tempo reale di accessi a domini AI (banner rosso lampeggiante + suono + notifica desktop se abilitate)
- **Watchdog visualization**: dot colorato per ogni studente, sempre aggiornato
- **Blocklist enforcement**: i blocchi sono applicati a prescindere dalla sessione
- **Aggregazioni client-side**: `perIp`, `perDominio`, `perTipo` ricostruite in tempo reale a ogni entry SSE

⚠️ **Cambio chiave rispetto a v1**: in v1 con `sessione ferma` il proxy faceva early-return prima di broadcastare via SSE → il monitor live era inutilizzabile durante le lezioni normali. In v2 il monitor live e' garantito.

#### 3.2.1 Stato buffer

Il monitor mantiene un ring buffer in RAM delle ultime entry per la UI live, **anche fuori sessione**. Dimensione: configurabile, default 1000 entry. Quando una nuova istanza UI si connette (browser apre dashboard), riceve in idratazione le ultime N entry del buffer.

Il buffer e' **separato dalla persistenza sessione**: durante una sessione attiva, le entry vanno sia nel buffer UI sia nel log NDJSON.

---

### 3.3 Sessione (registrazione opt-in)

#### 3.3.1 Lifecycle

| Stato | Trigger | Effetto |
|---|---|---|
| Idle (default al boot) | — | `sessioneAttiva = false`. Monitor live OK, niente persistenza |
| In corso | UI: bottone "Avvia sessione" o `POST /api/session/start` | `sessioneAttiva = true`, nuovo `sessioneInizio` ISO, log NDJSON aperto in append |
| Ferma | UI: bottone "Ferma" o `POST /api/session/stop` | `sessioneAttiva = false`, archive automatico, NDJSON chiuso |

L'archive di stop persiste la sessione in SQLite (vedi 3.11 per il modello report e §4 per lo schema), e produce un export JSON snapshot in `sessioni/<timestamp>.json` per backup / portabilita' (lossless, importabile in altro Planck).

#### 3.3.2 Persistenza durante sessione

Durante una sessione attiva, ogni entry viene:
1. Aggiunta al ring buffer UI (broadcast SSE) — come fa il monitor sempre
2. Appended al file NDJSON `sessioni/_corrente.ndjson` (una entry per riga JSON)
3. Eventualmente acceso il flag `flagged: true` se una regola di auto-reazione l'ha matchata

⚠️ **NDJSON vs SQLite live**: durante la sessione si scrive su NDJSON (append-only, fast, robusto a crash). Allo `stop` il file viene letto e l'intera sessione importata in SQLite in una transazione. Motivo: scrivere ogni entry in SQLite live ha overhead, NDJSON e' append O(1) e crash-safe.

#### 3.3.3 Comportamento "buffer che precede l'Avvia"

Quando il prof preme "Avvia sessione":
- Il NDJSON viene **resettato** (file vuoto)
- Le entry gia' nel buffer UI **non** vengono retroattivamente persistite
- La sessione comincia "da adesso"

⚠️ Decisione esplicita: niente "registrazione retroattiva". Se serve, l'utente avrebbe dovuto premere Avvia prima.

#### 3.3.4 Crash / shutdown

- **Graceful shutdown** (Ctrl+C, signal): se sessione attiva, esegue stop + archive automatico
- **Crash / kill -9 / power loss**: il file NDJSON resta su disco. Al boot successivo Planck rileva `sessioni/_corrente.ndjson` non vuoto → propone "Sessione interrotta trovata: importala?". L'utente puo' importare (archive in SQLite) o scartare.

⚠️ Questo e' un miglioramento sostanziale rispetto a v1 dove il crash perdeva tutto il buffer in RAM.

---

### 3.4 Blocchi (blocklist / allowlist / pausa)

#### 3.4.1 Modi proxy

`config.modo` puo' essere:

- **`blocklist`** (default): tutto passa, tranne i domini in `bloccati`
- **`allowlist`**: niente passa, tranne i domini in `bloccati` (la lista e' interpretata "al contrario") + `dominiIgnorati`

#### 3.4.2 Pausa globale

Toggle in toolbar. Quando `pausato = true`: il proxy blocca **tutto** tranne `dominiIgnorati` (per non rompere localhost / Windows background).

Stato in-memory, perso al restart (come v1). Indicatore visivo: `[IN PAUSA]` lampeggiante in topbar.

#### 3.4.3 Logica di blocco

`dominioBloccato(d)`:
1. Se `d` matcha `dominiIgnorati` → false (passa sempre)
2. Se `pausato` → true (blocca)
3. `match = bloccati.some(b => d.toLowerCase().includes(b.toLowerCase()))`
4. Se `modo === 'blocklist'` → return `match`
5. Se `modo === 'allowlist'` → return `!match`

#### 3.4.4 Persistenza

In v2: tabella SQLite `bloccati(dominio, added_at)`. Mutazioni live via API (`POST /api/block`, `unblock`, `block-all-ai`, `clear-blocklist`), broadcast SSE `blocklist` ai client.

`dominiIgnorati`: tabella SQLite `ignorati(dominio)` o lista in `config.json` (decidi in §4).

---

### 3.5 Preset blocklist

#### 3.5.1 Modello

Un preset = snapshot della blocklist + un nome. Tabella SQLite `presets(nome, descrizione, domini_json)`.

Operazioni:
- **Salva preset**: `POST /api/preset/save?nome=X` → snapshot della blocklist corrente con nome X (overwrite silenzioso se nome esiste)
- **Carica preset**: `POST /api/preset/load?nome=X` → sostituisce la blocklist corrente con il contenuto del preset (broadcast SSE)
- **Elenco**: `GET /api/presets` → lista nomi
- **Elimina preset**: `POST /api/preset/delete?nome=X`

#### 3.5.2 UI

Dropdown in menu overflow (⋮) della toolbar. Voci: lista preset esistenti + voce "+ Salva corrente come preset...".

#### 3.5.3 Niente preset built-in

Planck **non** ships con preset precompilati. La tabella `presets` parte vuota al primo boot. L'utente crea i propri preset salvando snapshot della blocklist corrente quando ne ha bisogno (es. dopo aver costruito una blocklist tipica per la verifica di programmazione, salva `verifica-prog`).

Motivazione: i casi d'uso e le esigenze pedagogiche variano molto da docente a docente; preset standard rischiano di essere o troppo restrittivi o troppo permissivi per il caso reale, e l'utente li ignora.

---

### 3.6 Mappe studenti, classi, laboratori

#### 3.6.1 Modello dati

Due livelli:

**Mappa attiva**: la mappatura `IP -> nome studente` correntemente caricata. Vive in memoria + tabella SQLite `studenti_correnti(ip, nome)`. Una sola mappa attiva alla volta.

**Combo (classe, lab) salvate**: snapshot della mappa attiva con un nome. Tabella SQLite `combo(classe, lab, mappa_json)`. Modello bidimensionale che riflette il caso reale: la stessa classe in lab diversi ha IP diversi, lo stesso lab con classi diverse ha nomi diversi.

#### 3.6.2 Operazioni UI (tab Impostazioni → Mappa studenti)

- Edit inline della mappa attiva (riga per IP, click sul nome → input)
- Aggiungi singolo IP+nome (form sotto la tabella)
- Svuota tutto
- Carica combo `(classe, lab)`: due dropdown → click "Carica" → sostituisce mappa attiva
- Salva combo: prompt di nome classe + lab → snapshot della mappa attiva
- Elimina combo

#### 3.6.3 Persistenza nei dati di sessione

Le sessioni archiviate **fotografano** la mappa al momento dell'archive. Modificare la mappa dopo non altera i dati storici.

Al boot Planck parte sempre con la mappa attiva vuota: il prof carica esplicitamente la combo `(classe, lab)` che gli serve. Niente memoria implicita dell'ultima combo (decisione esplicita: ogni avvio e' un setup esplicito).

---

### 3.7 Watchdog keepalive

#### 3.7.1 Protocollo

Ogni PC studente esegue uno script (`proxy_on.bat`) che:
1. Imposta il proxy di sistema verso il PC docente
2. Lancia uno script VBS in background che, ogni 5 secondi, fa un GET HTTP a `http://<IP_DOCENTE>:<PORTA_PROXY>/_alive`

Il GET viene fatto via `MSXML2.ServerXMLHTTP.6.0` con `setProxy 1` (no proxy locale, evita loop di se stesso).

#### 3.7.2 Lato server

Il proxy intercetta `GET /_alive` prima del normale forwarding. Aggiorna `aliveMap[ipClient] = Date.now()` e broadcasta SSE `{type: "alive", ip, ts}`.

`aliveMap` e' una `Map<string, int64>` in memoria. Sopravvive al toggle di sessione (il watchdog e' indipendente dalla sessione: anche durante lezioni il prof vuole vedere chi e' connesso).

#### 3.7.3 UI: dot colorato

Stato calcolato lato client da `state.aliveMap` ad ogni render:

| Stato | Colore | Tempo dall'ultimo ping | Significato |
|---|---|---|---|
| `verde` | 🟢 | < 15s | Attivo |
| `giallo` | 🟡 | 15-60s | Ritardo (transitorio) |
| `rosso` | 🔴 | > 60s (lampeggiante) | OFFLINE — possibile bypass o spegnimento |
| `grigio` | ⚫ | nessun ping mai ricevuto | Sconosciuto |

#### 3.7.4 Limitazioni note (gia' documentate in v1)

- Studente che killa `wscript.exe`: dot diventa rosso entro 60s (rilevabile)
- Studente che usa hotspot dal telefono: il browser non passa dal proxy ma il watchdog continua a vedere il PC dello studente connesso → non rileva. Limite strutturale, va gestito a vista.
- `proxy_on.bat` non eseguito su uno studente: dot grigio (mai pingato), il prof vede subito chi non e' connesso al proxy.

---

### 3.12 UI

#### 3.12.1 Tab principali (4 in v2)

| Tab | Scope | Default visibile? |
|---|---|---|
| `Live` | Monitor real-time + sessione + Veyon controls | ✓ default |
| `Report` | Riepilogo singola sessione (corrente o archivio) | ✓ |
| `Storico` (nuovo v2) | Cross-session: per studente, confronti | ✓ |
| `Impostazioni` | Config, mappe, preset, integrazione Veyon, regole auto | ✓ |

Tab attivo persistito in `localStorage`.

#### 3.12.2 Layout Live tab

```
┌─────────────────────────────────────────────────────────────┐
│ [Topbar: tabs] [pausa] [countdown] [🌙] [🔔]                  │
├─────────────────────────────────────────────────────────────┤
│ [Toolbar: avvia/ferma sessione] [pausa] [⋮ azioni blocklist] │
│                                              [Fine HH:MM] [x] │
├─────────────────────────────────────────────────────────────┤
│ [Toolbar Veyon "Azioni classe ▾"]    (nuovo v2)              │
├─────────────────────────────────────────────────────────────┤
│ [Stat row: 5 card]                                           │
├──────────┬──────────────────────────────────┬───────────────┤
│ Sidebar  │ Pannello IP (griglia o lista)    │ Ultime        │
│ Domini   │                                  │ richieste     │
│ (collap) │ [▦ griglia] [☰ lista]             │ (collap)      │
│          │                                  │               │
│ [filter] │ ┌─────┐ ┌─────┐ ┌─────┐           │ 10:23 [Mario] │
│ - AI     │ │card │ │card │ │card │           │   chatgpt.com │
│ - Siti   │ └─────┘ └─────┘ └─────┘           │ ...           │
│ - Sis    │ ...                              │               │
└──────────┴──────────────────────────────────┴───────────────┘
```

Pannelli laterali (sidebar e ultime richieste) sono **collassabili** indipendentemente.

#### 3.12.3 Vista IP: griglia vs lista

Toggle in panel header. Persistito in `localStorage.vistaIp` (default `griglia`).

**Griglia** (default): card per studente, layout responsive `repeat(auto-fill, minmax(200px, 1fr))`. Card mostra: dot watchdog, nome+IP, conteggio richieste reali, ultima attivita', primi 6 domini, **bottoni Veyon inline (lock, screenshot, run)** in v2.

**Lista**: tabella 5 colonne (WD / Studente / N / Ultima / Domini), domini come tag inline. In v2, una colonna "azioni" aggiuntiva con stessi bottoni Veyon.

#### 3.12.4 Filtro testuale

Input in sidebar. Filtra in tempo reale per nome studente / IP / dominio. Match case-insensitive substring. Le entry non-match hanno classe `filtro-hidden`.

#### 3.12.5 Focus IP

Click su una card/riga → toggle focus su quell'IP. Quando attivo:
- Stat row mostra solo numeri di quell'IP
- Pannello "Ultime richieste" filtrato sull'IP
- Header pannello mostra `Focus: NOME (IP) [X]`

⚠️ In v2 il single click resta focus, ma e' affiancato da Ctrl+click per multi-select Veyon (vedi 3.8.6).

#### 3.12.6 Tema chiaro / scuro

Toggle in topbar (icona ☀️/🌙). Stato persistito in `localStorage.darkmode`. Variabili CSS scambiate via classe `body.dark`.

#### 3.12.7 Notifiche e suoni

Toggle in topbar (🔔/🔕). Quando attive:
- Banner AI accompagnato da suono sinusoidale 880 Hz, 150ms
- Notifica desktop (HTML5 Notifications API) per detection AI e scadenza deadline

Permesso desktop notifications richiesto al primo enable. Default: disattivato.

#### 3.12.8 Deadline / countdown

Input `time` (HH:MM) in toolbar. Imposta una scadenza assoluta (oggi o domani se gia' passata). Display countdown live in topbar:
- Stato normale: `MM:SS rimanenti`
- Warning: < 5 min, sfondo arancione
- Critical: < 1 min, sfondo rosso lampeggiante
- Scaduto: banner "TEMPO SCADUTO" + tre beep + notifica desktop

Stato in-memory (perso al restart, come v1).

#### 3.12.9 Banner AI

Banner top-fixed, lampeggiante rosso/rosso-scuro. Trigger:
- Detection di un dominio AI (lista upstream o locale) **non gia' bloccato**
- Si auto-nasconde dopo 5s. Ogni nuovo trigger riapre/riallunga il banner.

Domini AI gia' bloccati (rispondono 403): non triggeano il banner, ma sono evidenziati nel log come `blocked: true`.

---

#### 3.8.1 Architettura

Planck v2 implementa il **client-side del protocollo Veyon** in Go, parlando TCP direttamente con i `veyon-server` sui PC studenti. Il PC docente non ha bisogno di Veyon Master installato — Planck e' la sua dashboard unificata.

Prerequisito: i PC studenti devono avere `veyon-server` installato (servizio Windows). Setup IT una tantum per laboratorio.

#### 3.8.2 Comandi supportati

**V2 MVP — alta priorita':**

| Comando | Cosa fa | Use case |
|---|---|---|
| **RunProgram** | Lancia un programma sul PC studente | Lancio di `proxy_on.bat` (push runtime, vedi sotto), app didattiche |
| **FileTransfer** | Invia file dal docente allo studente | Testo verifica, esercizio starter |
| **PowerOn** | Wake-on-LAN | Accendere i PC del laboratorio prima della lezione |
| **PowerDown** | Spegnimento ordinato | Spegnere a fine giornata |
| **Reboot** | Riavvio | Recovery di un PC bloccato |
| **LogOff** | Disconnetti utente | Fine sessione studente |

**V2 MVP — media priorita':**

| Comando | Cosa fa |
|---|---|
| **ScreenLock** | Blocca/sblocca schermo |
| **Screenshot** (one-shot) | Cattura un singolo frame del framebuffer dello studente |

**V2.x — deferred:**

- **TextMessage**: popup di testo. Implica anche evoluzione UC6 (auto-reazione con messaggio personalizzato).
- **RemoteAccess** (VNC live streaming): framebuffer continuo, complesso. La capacita' tecnica gia' la implementiamo per Screenshot one-shot, basta aggiungere lo streaming.
- **DemoServer/DemoClient** (broadcast schermo docente): stessa complessita' tecnica di RemoteAccess.

#### 3.8.3 Distribuzione di proxy_on.bat — push runtime

Il bottone "Distribuisci proxy_on.bat" funziona in **modalita' push runtime**: Planck esegue in sequenza:
1. `FileTransfer` di `proxy_on.bat` in `%TEMP%\` su ogni PC studente target
2. `RunProgram` di `%TEMP%\proxy_on.bat`

Vantaggio: zero setup IT preliminare, Planck e' autonomo. Svantaggio: piccolo overhead di rete a ogni distribuzione (file da pochi KB, accettabile).

#### 3.8.4 Granularita'

Ogni comando ha tre target:
- **Singolo studente** (click sulla card)
- **Selezione multipla** (vedi 3.8.6)
- **Tutti** (toolbar globale)

#### 3.8.5 UI delle azioni Veyon — tre layer

**1. Card studente (azioni rapide):**

Bottoni icona inline visibili sulla card della griglia (e come actions cell in vista lista):

- 🔒 ScreenLock
- 📷 Screenshot one-shot
- ▶️ Run program (dropdown dei "programmi rapidi" configurati in Impostazioni)

Le azioni meno frequenti (FileTransfer / PowerDown / Reboot / LogOff / PowerOn) **non** stanno sulla card — vivono nel context menu.

**2. Toolbar globale "Azioni classe" (su tutti):**

Toolbar separata da quella sessione/pausa/blocchi, con bottoni:
- 🔒 Lock all
- 📷 Screenshot all
- 📁 FileTransfer to all
- ⏻ PowerDown all
- 🔄 Reboot all
- ⚡ PowerOn all (Wake-on-LAN su tutti gli IP della mappa)

Per non saturare la UI: l'opzione preferita e' un dropdown "Azioni classe ▾" che apre il pannello con tutti i bottoni.

**3. Right-click context menu (catalogo completo):**

Right-click su una card → menu con tutti i comandi:
- Se la selezione attiva e' >= 2 card → agisce sulla selezione
- Altrimenti → agisce solo sulla card cliccata

#### 3.8.6 Selezione multipla

Convenzione standard:
- **Click semplice** sulla card: focus IP (filtra traffico, comportamento come v1)
- **Ctrl/Cmd + click**: aggiungi/rimuovi dalla selezione
- **Shift + click**: range select dall'ultima selezione
- **ESC**: deseleziona tutto

Visual: card selezionata = border accent + checkmark in alto a destra.

Quando selezione >= 1 appare una **selection bar** in cima alla griglia: `"N studenti selezionati"` + bottoni azioni di gruppo + `"Deseleziona tutti"`.

#### 3.8.7 Programmi rapidi configurabili

Lista in Impostazioni → Veyon → "Programmi rapidi". Ogni voce:
- **Nome** (es. `proxy_on`, `Apri VSCode`, `Browser su google.com`)
- **Comando** (path eseguibile + argomenti)
- **Modalita'**: `push runtime` (FileTransfer + Run) | `preinstallato` (Run di un path noto)

Il dropdown ▶️ sulla card mostra questa lista. Voce di default: `proxy_on` (preconfigurata, push runtime).

#### 3.8.8 Configurazione Veyon (auto-import + override)

Al boot Planck cerca `Veyon.conf`:
- Windows: `%PROGRAMDATA%\Veyon\Veyon.conf` + registry `HKLM\SOFTWARE\Veyon`
- Linux: `/etc/veyon/Veyon.conf`

Se trovata, parsing di:
- Auth method (KeyFile / LDAP / ACL)
- Path delle chiavi di autenticazione
- Lista NetworkObjects (host noti)

Se non trovata, fallback su config Planck-only.

**Tab Impostazioni → Veyon:**
- Status: "Veyon config trovata in [path]" / "Veyon non rilevato"
- Override: path custom a `Veyon.conf`, chiavi auth custom, mapping IP→keyfile specifico
- Bottone "Test connessione" per verificare le credenziali su un IP scelto

#### 3.8.9 Autenticazione

Metodi Veyon supportati:
- **KeyFile auth** (chiavi RSA/EC, formato PEM): **default Planck v2**
- LDAP: skip MVP (richiede infra LDAP)
- ACL semplice (user/pass): skip

Implementazione: chiave privata caricata al boot di Planck. Per ogni connessione TCP a un veyon-server: handshake HMAC firmato (formato definito in `core/src/AuthenticationManager.cpp` del sorgente Veyon).

#### 3.8.10 Edge cases

| Scenario | Comportamento |
|---|---|
| Veyon non installato sul PC docente | Warning UI, funzionalita' Veyon disabilitate, Monitor + Sessione restano attivi |
| `veyon-server` non in ascolto su uno studente | Errore "Irraggiungibile via Veyon"; icona warning sulla card |
| Chiave auth invalida | Errore "Authentication failed"; possibilita' di rigenerare/reimportare in Impostazioni |
| Studente in mappa ma mai pingato dal watchdog | Card con comandi Veyon disabled finche' non c'e' almeno un ping |
| FileTransfer di file grandi (>100 MB) | Avviso "File grande, invio lento"; barra avanzamento dettagliata; possibilita' di annullare |
| Comando broadcast su 30 studenti, 5 falliscono | Summary in UI: `"25/30 OK, 5 errori (clicca per dettagli)"` |
| `proxy_on.bat` push runtime fallisce su uno studente | Lo studente compare senza watchdog dot verde; il prof riceve l'errore in toolbar e puo' riprovare |

---

### 3.9 Auto-classification AI

#### 3.9.1 Architettura — tre fonti di verita'

La classificazione `dominio -> tipo` consulta tre fonti, in ordine:

1. **Lista upstream** (`data/domini-ai.json`): mantenuta nel repo `DoimoJr/planck-proxy`, sincronizzata periodicamente con Planck installati. Curated dal maintainer del progetto.
2. **Lista locale del docente** (`data/domini-ai-locali.json`): domini aggiunti manualmente dal prof tramite UI ("Si', e' AI"). Vive solo sul PC del prof, non viene rimandata upstream automaticamente.
3. **Heuristic flagging**: pattern-match runtime su domini sconosciuti. Non classifica come AI direttamente — propone solo al prof di valutare.

Un dominio e' "AI confermato" se compare in (1) OPPURE in (2). Un dominio e' "sospetto AI" se non compare in (1)/(2) ma matcha l'heuristic (3).

#### 3.9.2 Lista upstream

**Sorgente**: file JSON nel repo `DoimoJr/planck-proxy`, path `data/domini-ai.json`.

**Formato**:
```json
{
  "version": "2026-04-28",
  "descrizione": "Lista di domini AI noti, curata dal maintainer di Planck",
  "domini": [
    "openai.com",
    "chatgpt.com",
    "anthropic.com",
    ...
  ]
}
```

Versioning per data ISO (semplice, ordinabile, leggibile).

**Distribuzione**: Planck scarica via HTTPS da GitHub raw URL:
`https://raw.githubusercontent.com/DoimoJr/planck-proxy/main/data/domini-ai.json`

#### 3.9.3 Sync — boot + manuale

**All'avvio di Planck**:
1. Tenta il fetch della lista upstream
2. Se `version` upstream > `version` cache locale: aggiorna cache (`cache/domini-ai-upstream.json`)
3. Se fetch fallisce (no internet, GitHub irraggiungibile): usa l'ultima cache. Se nessuna cache: usa la lista bundled nel binario (snapshot al momento del build).

**Bottone "Aggiorna lista AI"** in Impostazioni: trigger manuale dello stesso flow. Mostra status: "Ultimo update: [data]" + spinner durante il fetch.

**Niente refresh automatico in background**. Motivo: i lab scuola spesso hanno reti interne con connettivita' Internet limitata, e fare GET random in background sarebbe rumore inutile. L'utente decide quando vuole aggiornare.

#### 3.9.4 Lista locale del docente

Domini aggiunti manualmente tramite UI (vedi 3.9.6). File JSON formato identico a quello upstream:

```json
{
  "domini": [
    "nuovissimoai.xyz",
    "modello-locale.it"
  ]
}
```

**Persistenza**: file salvato accanto a `config.json`, non sincronizzato con upstream.

**UI Impostazioni → "I miei domini AI locali"**: lista editabile, possibilita' di rimuovere voci aggiunte per errore.

#### 3.9.5 Heuristic flagging

Un dominio e' "sospetto AI" (badge 🤔) se non e' in (1)/(2) e matcha **almeno una** di queste regole:

**Boundary-aware substring** (token con confini `.`, `-`, inizio/fine):
- `ai`
- `gpt`
- `chat`
- `llm`
- `assistant`

Esempi: `chat.example.com` ✓, `myai-tool.io` ✓, `ai.something.com` ✓. Ma `domain.com` ✗ (no match), `chair.io` ✗ (no boundary), `paint.com` ✗.

**TLD sospetti**:
- `.ai` (es. `pippo.ai`)
- `.chat` (es. `pippo.chat`)

**Implementazione**: una regex composita per i token + un check `endsWith` per i TLD.

Le heuristic sono **non aggressive** by design: false positivi generano noise UI ma non bloccano nulla — il dominio passa comunque, il prof decide.

#### 3.9.6 UI del flag — discreto

Un dominio sospetto compare nelle stesse sezioni della sidebar dove sarebbe finito comunque (Siti / Sistema / Utente, in base alla sua altra eventuale classificazione), con un **badge 🤔 inline**.

Click sul badge → modal:

```
Dominio sospetto: pippo.ai
E' un servizio AI?

  [Si', classifica come AI]   [No, non e' AI]   [Annulla]
```

- "Si', classifica come AI" → aggiunge a `domini-ai-locali.json`, ri-classifica immediatamente, badge 🤔 sparisce, dominio passa nella sezione AI della sidebar
- "No, non e' AI" → aggiunge a una lista `domini-non-ai-locali.json` per non chiedere piu', badge 🤔 sparisce
- "Annulla" → nulla, badge resta, riproporra' alla prossima sessione

> Niente banner rosso lampeggiante per i sospetti. Quello e' riservato ai domini AI **confermati** (lista upstream o locale). Un dominio "sospetto" ma non confermato non e' un'emergenza — il prof ci mette mano quando ha tempo.

#### 3.9.7 Edge cases

| Scenario | Comportamento |
|---|---|
| Dominio in `dominiIgnorati` matcha un'euristica | `dominiIgnorati` ha precedenza assoluta (e' droppato a monte, prima della classificazione) |
| Stesso dominio in lista upstream E in lista locale | Idempotente: classificato come AI una sola volta. Niente duplicati |
| Prof aggiunge per errore un dominio non-AI | UI Impostazioni → "I miei domini AI locali" → rimuovi |
| Lista upstream cambia un dominio (e.g., rimosso) ma e' in lista locale | La lista locale prevale: il dominio resta classificato come AI |
| Internet down al boot | Usa cache; se nessuna cache, lista bundled del binario |
| Heuristic match su dominio benigno (`pair.com` → ai?) | Boundary-aware: `pair.com` non matcha `ai` perche' non c'e' confine. Falsi positivi minimi by design |
| Studente accede a un dominio sospetto durante una verifica | Passa (non e' bloccato), genera badge 🤔, prof valuta. Se conferma → aggiunto a lista locale + (separatamente) puo' bloccare con un click |

---

### 3.10 Reazioni automatiche

#### 3.10.1 Modello — preset hardcoded + rule engine

Modello misto in due livelli:

1. **Preset rapidi** (3-4 toggle in alto nelle Impostazioni): coprono i casi piu' frequenti, attivabili con un click. Sono le combinazioni "trigger + azione" gia' note utili.
2. **Regole personalizzate** (sezione collassabile, opt-in): editor che permette di costruire regole custom combinando trigger, condizioni, azioni, cooldown, scope.

Tutto il sistema e' **opt-in**: di default tutte le reazioni automatiche sono disabilitate. Niente azione si attiva mai senza che l'utente abbia esplicitamente flaggato un toggle o creato una regola.

#### 3.10.2 Trigger disponibili

| Trigger | Quando scatta |
|---|---|
| `ai_rilevato` | Detection di un dominio classificato AI (lista upstream o locale) e non gia' bloccato dalla blocklist |
| `accesso_bloccato` | Tentativo studente di accedere a un dominio in blocklist (proxy risponde 403) |
| `watchdog_rosso` | Watchdog di uno studente passa a stato `rosso` (>60s da ultimo ping) — possibile bypass |
| `inattivita` | Studente non genera traffico (utente+ai) da N minuti durante una sessione attiva. Default soglia: `inattivitaSogliaSec` di config |

#### 3.10.3 Azioni disponibili

| Azione | Cosa fa | Note |
|---|---|---|
| `screen_lock` | ScreenLock Veyon dello studente target | Richiede Veyon configurato |
| `screenshot` | Screenshot one-shot Veyon dello studente, salvato in `screenshots/` | Richiede Veyon configurato |
| `notifica_docente` | Banner UI lampeggiante + suono + (se abilitato) notifica desktop del browser | Sempre disponibile |
| `log_marcato` | L'entry traffico che ha scatenato il trigger viene marcata `flagged: true`, evidenziata in Report con badge alto-priorita' | Richiede sessione attiva |
| `annotazione` | Aggiunge una entry sintetica al log della sessione: `"[AUTO 10:23:15] ai_rilevato (chatgpt.com) → screen_lock Mario.Rossi"` | Richiede sessione attiva |

`log_marcato` e `annotazione` sono complementari: il primo evidenzia l'evento originale, il secondo crea un audit trail leggibile delle reazioni.

#### 3.10.4 Granularita' — solo per-studente

Le reazioni automatiche agiscono **esclusivamente sull'IP che ha generato il trigger**. Mai su gruppi o sull'intera classe.

Esempio: se Mario tenta `chatgpt.com`, scatta lock di Mario. Gli altri studenti non sono coinvolti.

> Le azioni "su tutta la classe" restano sempre disponibili manualmente nella toolbar globale (vedi 3.8.5), ma non sono trigger-able da regole automatiche. La motivazione e' evitare effetti a catena indesiderati.

#### 3.10.5 Cooldown

**Default: 60 secondi per coppia (regola, studente).**

Una volta che una regola scatta su uno studente, non si ripete su quello studente per la durata del cooldown. Motivo: evitare retry storm (es. browser che riprova 5 volte chatgpt in 2 secondi → 5 lock).

**Configurabile per regola**: il prof puo' impostare cooldown diverso per regole specifiche (es. cooldown 0 per `accesso_bloccato` se vuole una reazione ad ogni tentativo).

**Stato visibile**: durante il cooldown la card dello studente mostra un piccolo indicatore (`⏱ 30s`) cosi' il prof sa che la regola e' "armata in attesa" su quell'IP.

#### 3.10.6 Preset rapidi

Tre toggle in Impostazioni → "Reazioni automatiche" → "Preset":

```
[ ] Lock automatico su rilevamento AI
    Quando uno studente accede a un dominio AI: screen_lock + notifica_docente + annotazione

[ ] Screenshot su tentativo bloccato
    Quando uno studente prova un dominio bloccato: screenshot + log_marcato

[ ] Allarme watchdog rosso
    Quando uno studente sparisce dalla rete: notifica_docente + annotazione
```

Ogni toggle ha cooldown di default 60s e scope "solo sessione attiva". Stato persistito in `config.json`.

#### 3.10.7 Regole personalizzate

Sezione "Regole personalizzate" sotto i preset, collassabile (collapsed by default). Per ogni regola:

```
Nome:        [ Sospetto AI giallo ]
Trigger:     [ ai_rilevato ▾ ]
Azioni:      [✓] screen_lock
             [✓] screenshot
             [ ] notifica_docente
             [ ] log_marcato
             [✓] annotazione
Cooldown:    [ 60 ] secondi
Scope:       (•) solo sessione attiva
             ( ) sempre
Stato:       [✓] Abilitata
```

Bottoni: `Nuova regola` / `Modifica` / `Elimina` / `Duplica`.

Le regole sono salvate in un nuovo file `regole.json`.

> v2 MVP: rule engine senza condizioni avanzate (e.g. "se trigger E orario tra 8 e 13"). Solo trigger → azioni. Le condizioni complesse sono v2.x.

#### 3.10.8 Edge cases

| Scenario | Comportamento |
|---|---|
| Veyon non disponibile e regola usa `screen_lock` | L'azione viene saltata silenziosamente, le altre azioni della regola eseguono comunque (es. notifica). Log warning una volta per regola |
| Sessione ferma + regola con `log_marcato` o `annotazione` | Le azioni "session-required" no-op, le altre azioni eseguono normalmente |
| Stesso trigger scatena piu' regole simultaneamente | Ogni regola esegue indipendentemente con il proprio cooldown |
| Studente non in mappa (IP sconosciuto) | Le azioni Veyon usano l'IP raw; le annotazioni mostrano l'IP nel testo |
| Cooldown attivo + nuovo trigger arriva | Ignored: la card mostra `⏱ Ns` |
| Regola disabilitata mentre cooldown e' attivo | Cooldown si reset, prossima volta la regola puo' triggerare di nuovo (se riabilitata) |
| Inattivita' triggera mentre la sessione e' ferma | No fire (l'inattivita' implica tracking del traffico, che richiede sessione attiva) |

---

### 3.11 Reportistica

#### 3.11.1 Tre viste, due tab

La reportistica e' divisa in:

| Vista | Tab UI | Scope |
|---|---|---|
| **Report singola sessione** | Tab `Report` (esistente) | Una sessione alla volta (corrente o archiviata) |
| **Storico cross-session** | Tab `Storico` (nuovo) | Aggregazioni e confronti su piu' sessioni nel tempo |
| **Export** | Bottoni in entrambi i tab | JSON / CSV / PDF / JSON aggregato |

Il tab Report resta semplice e veloce: e' lo strumento di "controllo dopo la verifica appena finita". Il tab Storico e' lo strumento di "analisi nel tempo" — separato per non sovraccaricare il primo.

#### 3.11.2 Tab Report (singola sessione)

Funzionalita' come v1:
- Riepilogo (durata, totali, % bloccate, breakdown per tipo)
- Top 10 domini (tutti)
- Top 10 domini AI
- Top studenti per attivita' vera
- Selettore archivio: dropdown delle sessioni passate

**Nuovo in v2**: ogni entry "marcata da regola automatica" (vedi 3.10) viene mostrata con badge alto-priorita' + riga sintetica delle annotazioni automatiche generate durante la sessione.

#### 3.11.3 Tab Storico — vista 1: per studente nel tempo

Ricerca uno studente (input nome o IP) → la vista mostra:

- **Header studente**: nome, ultimo IP visto, classi/lab in cui e' apparso
- **Tabella sessioni**: tutte le sessioni in cui lo studente compare, ordinate per data desc:
  - Data, classe, lab, durata sessione
  - Richieste reali (utente+ai)
  - Detection AI (count e domini distinti)
  - Tentativi su domini bloccati
  - Eventi auto-reazione (count)
- **Mini-trend**: line chart della "attivita' AI" per sessione nel tempo
- **Click su una riga sessione** → apre quella sessione nel tab Report

#### 3.11.4 Tab Storico — vista 2: confronto fra due sessioni

Selettore di due sessioni → vista side-by-side:

- **Riepiloghi affiancati**: stessi numeri della singola sessione, ma in due colonne
- **Top studenti diff**: per ogni studente comune alle due sessioni, mostra il delta di richieste/AI tra le due
- **Top domini diff**: domini che compaiono solo in una delle due, o con grossa differenza di count
- **Grafico comparativo**: barre affiancate per i top 10 domini

Use case tipico: "verifica 1 in 4DII vs verifica 2 in 4DII per vedere se gli studenti hanno migliorato o peggiorato".

#### 3.11.5 Filtri

Filtri combinabili nel tab Storico:

- **Studente**: nome (autocomplete) o IP
- **Classe + lab**: dropdown (legge dalla mappa classi)
- **Periodo**: data inizio - data fine (date picker)
- **Tipo evento**: checkbox (solo AI / solo bloccati / solo eventi auto-reazione / tutto)

Tutti i filtri sono **combinabili**: es. "Tutti gli eventi AI di Mario in 4DII tra il 2026-04-01 e il 2026-06-30".

I filtri si applicano sia al rendering della UI sia agli export.

#### 3.11.6 Export

Quattro formati, accessibili dai due tab:

| Formato | Contenuto | Use case |
|---|---|---|
| **JSON sessione** | Una sessione completa (entries + metadata + bloccati + studenti) | Archivio lossless, import in altro Planck |
| **CSV sessione** | Una entry per riga (ora, ip, nome, dominio, tipo, blocked, flagged) | Excel / analisi manuale / LibreOffice |
| **PDF report** | Riepilogo + grafici della sessione, formato stampabile | Allegare a verbali disciplinari, archivio cartaceo |
| **JSON aggregato cross-session** | Tutte le sessioni nel filtro corrente, dati pieni | Backup, analisi esterna, import in altro Planck |

**Implementazione PDF**: generato lato browser (button "Stampa PDF" → CSS print stylesheet + `window.print()` → utente sceglie "Salva come PDF" nel dialog). Vantaggi: zero dipendenze backend, niente engine PDF da bundlare nel binario Go.

#### 3.11.7 Retention dati

**Default v2: nessuna retention automatica.** Le sessioni restano nel database SQLite finche' qualcuno non le cancella manualmente.

Comandi disponibili:
- Tab Storico → bottone `Elimina sessioni nel filtro` (conferma esplicita): cancella tutte le sessioni che matchano i filtri attivi
- Tab Storico → click su una sessione → bottone `Elimina sessione` (singola)

> ⚠️ **Nota di responsabilita'**: i dati raccolti (IP, nomi studenti, domini visitati) sono **dati personali ai sensi GDPR**. La scelta "nessuna retention automatica" e' una scelta di default permissiva: la responsabilita' di definire una policy di retention adeguata e' della scuola/del docente che adotta Planck. Si consiglia di valutare con il DPO della scuola e di cancellare manualmente i dati piu' vecchi quando non servono piu'.

**v2.x**: introduzione di una retention policy configurabile (auto-delete dopo N mesi, anonimizzazione dei nomi mantenendo aggregati anonimi). Deferred per ora.

#### 3.11.8 Edge cases

| Scenario | Comportamento |
|---|---|
| Studente cercato non e' mai apparso | "Nessuna sessione trovata per Mario.Rossi" + suggerimenti vicini (typo correction) |
| Studente con IP cambiato nel tempo (DHCP) | Match su nome studente (mappato), aggrega tutti gli IP che hanno quel nome |
| Lo stesso IP appare con nomi diversi in sessioni diverse | Mostra ogni occorrenza con il nome di quella sessione (lo studente ha cambiato banco) |
| Confronto fra due sessioni con classi diverse | Permesso (es. 4DII vs 4AII): la diff si concentra sui domini comuni, gli studenti sono mostrati in due gruppi separati |
| Filtro periodo con data fine prima della data inizio | Validazione UI, errore inline |
| Export JSON aggregato di 100+ sessioni | Warning "Export grande, attendere..." + spinner; possibilita' di ridurre con filtri |
| Studente eliminato dalla mappa attuale ma presente in sessioni passate | Le sessioni passate restano consultabili con il nome che aveva al momento dell'archivio (snapshot, non lookup live) |

---

*(Sezione 3 completa)*

---

## 4. Modello dati

### 4.1 Storage backend

Singolo file SQLite `planck.db` accanto all'eseguibile. Driver: **`modernc.org/sqlite`** (pure Go, niente cgo, semplifica la build cross-platform).

**Modalita' WAL** abilitata per concorrenza letture/scritture (`PRAGMA journal_mode=WAL`). Vantaggi:
- Una transazione di scrittura non blocca i lettori.
- Backup-friendly: `.db` + `.db-wal` + `.db-shm`, copiabili a freddo o tramite `VACUUM INTO`.

**Niente piu' file `config.json`**: tutta la config dinamica vive in SQLite, accessibile da UI. I default per il primo boot sono hardcoded nel binario.

### 4.2 Schema completo

```sql
-- Versione schema (per migrazioni future)
CREATE TABLE schema_version (
    version INTEGER NOT NULL
);

-- Config kv (settings non-relazionali, mutabili a caldo dalla UI)
CREATE TABLE kv (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,           -- JSON-encoded
    updated_at INTEGER NOT NULL    -- unix ms
);
-- Chiavi previste: 'proxy.port', 'web.port', 'web.auth.enabled',
-- 'web.auth.user', 'web.auth.password_hash', 'modo' ('blocklist'|'allowlist'),
-- 'titolo', 'classe', 'inattivita_soglia_sec'

-- Domini ignorati (drop pre-classificazione, sempre passano dal proxy)
CREATE TABLE domini_ignorati (
    dominio TEXT PRIMARY KEY
);

-- Blocklist (bloccati o passanti a seconda di modo)
CREATE TABLE bloccati (
    dominio TEXT PRIMARY KEY,
    added_at INTEGER NOT NULL
);

-- Preset blocklist (snapshot nominati)
CREATE TABLE presets (
    nome TEXT PRIMARY KEY,
    descrizione TEXT,
    domini TEXT NOT NULL,           -- JSON array di stringhe
    created_at INTEGER NOT NULL
);

-- Mappa studenti corrente (sostituita ad ogni 'Carica combo')
CREATE TABLE studenti_correnti (
    ip TEXT PRIMARY KEY,
    nome TEXT NOT NULL
);

-- Snapshot mappa per coppia (classe, lab)
CREATE TABLE combo (
    classe TEXT NOT NULL,
    lab TEXT NOT NULL,
    mappa TEXT NOT NULL,            -- JSON {"ip":"nome", ...}
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (classe, lab)
);

-- ============= SESSIONI E ENTRIES =============

-- Header sessione: una riga per sessione archiviata
CREATE TABLE sessioni (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sessione_inizio TEXT NOT NULL,      -- ISO 8601 UTC
    sessione_fine TEXT,                  -- ISO 8601 UTC, NULL solo per sessioni "in corso al momento del crash"
    durata_sec INTEGER,                  -- calcolato all'archive
    classe TEXT NOT NULL DEFAULT '',
    lab TEXT NOT NULL DEFAULT '',
    titolo TEXT,
    modo TEXT NOT NULL,                  -- 'blocklist' | 'allowlist' al momento dell'archive
    studenti_snapshot TEXT NOT NULL,     -- JSON {ip:nome} al momento dell'archive
    bloccati_snapshot TEXT NOT NULL,     -- JSON [domini]
    archiviata_at INTEGER NOT NULL       -- unix ms
);
CREATE INDEX idx_sessioni_classe_lab ON sessioni(classe, lab);
CREATE INDEX idx_sessioni_inizio ON sessioni(sessione_inizio);

-- Una entry per richiesta loggata. Tabella ad alto volume.
CREATE TABLE entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sessione_id INTEGER NOT NULL REFERENCES sessioni(id) ON DELETE CASCADE,
    ora TEXT NOT NULL,                    -- ISO 8601 UTC, formato display ("YYYY-MM-DD HH:MM:SS")
    ts INTEGER NOT NULL,                   -- unix ms (per query temporali rapide)
    ip TEXT NOT NULL,
    nome_studente TEXT,                    -- snapshot al momento (puo' essere NULL se IP non in mappa)
    metodo TEXT NOT NULL,                  -- 'GET'/'POST'/.../'HTTPS'
    dominio TEXT NOT NULL,
    tipo TEXT NOT NULL,                    -- 'ai' | 'utente' | 'sistema'
    blocked INTEGER NOT NULL CHECK (blocked IN (0, 1)),
    flagged INTEGER NOT NULL DEFAULT 0 CHECK (flagged IN (0, 1))  -- marcato da regola auto
);
CREATE INDEX idx_entries_sessione ON entries(sessione_id);
CREATE INDEX idx_entries_nome ON entries(nome_studente) WHERE nome_studente IS NOT NULL;
CREATE INDEX idx_entries_ts ON entries(ts);
CREATE INDEX idx_entries_dominio ON entries(dominio);

-- ============= REAZIONI AUTOMATICHE =============

-- Definizione regole (preset hardcoded + custom)
CREATE TABLE regole_auto (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nome TEXT NOT NULL,
    trigger TEXT NOT NULL,                 -- 'ai_rilevato' | 'accesso_bloccato' | 'watchdog_rosso' | 'inattivita'
    azioni TEXT NOT NULL,                   -- JSON ['screen_lock', 'screenshot', ...]
    cooldown_sec INTEGER NOT NULL DEFAULT 60,
    scope TEXT NOT NULL DEFAULT 'sessione', -- 'sessione' | 'sempre'
    abilitata INTEGER NOT NULL DEFAULT 1 CHECK (abilitata IN (0, 1)),
    is_preset INTEGER NOT NULL DEFAULT 0 CHECK (is_preset IN (0, 1)),
    preset_key TEXT,                         -- es. 'lock_su_ai' per i toggle predefiniti, NULL per custom
    parametri TEXT,                          -- JSON con parametri trigger-specifici (es. soglia inattivita')
    created_at INTEGER NOT NULL,
    UNIQUE (preset_key)
);

-- Audit dei trigger scattati (per Report singola sessione e cross-session)
CREATE TABLE eventi_auto (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sessione_id INTEGER NOT NULL REFERENCES sessioni(id) ON DELETE CASCADE,
    regola_id INTEGER REFERENCES regole_auto(id) ON DELETE SET NULL,
    ts INTEGER NOT NULL,
    trigger TEXT NOT NULL,
    target_ip TEXT NOT NULL,
    target_nome TEXT,
    azioni_eseguite TEXT NOT NULL,         -- JSON ['screen_lock', 'screenshot']
    azioni_fallite TEXT,                    -- JSON ['screen_lock'] se Veyon down
    dettagli TEXT                           -- JSON: dominio scatenante, ecc.
);
CREATE INDEX idx_eventi_auto_sessione ON eventi_auto(sessione_id);

-- ============= AUTO-CLASSIFICATION AI =============

-- Cache della lista upstream (singola riga)
CREATE TABLE upstream_cache (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version TEXT NOT NULL,
    descrizione TEXT,
    domini TEXT NOT NULL,                   -- JSON array
    fetched_at INTEGER NOT NULL
);

-- Domini classificati AI dal docente (locale, non sync upstream)
CREATE TABLE domini_ai_locali (
    dominio TEXT PRIMARY KEY,
    added_at INTEGER NOT NULL
);

-- Domini esclusi dall'heuristic flagging (il docente ha detto "no, non e' AI")
CREATE TABLE domini_non_ai_locali (
    dominio TEXT PRIMARY KEY,
    added_at INTEGER NOT NULL
);

-- ============= INTEGRAZIONE VEYON =============

-- Programmi rapidi per il dropdown ▶️ sulla card studente
CREATE TABLE veyon_programmi_rapidi (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nome TEXT NOT NULL,
    comando TEXT NOT NULL,                  -- path eseguibile + args
    modalita TEXT NOT NULL CHECK (modalita IN ('push_runtime', 'preinstallato')),
    posizione INTEGER NOT NULL DEFAULT 0    -- ordine nel dropdown
);

-- Override config Veyon (vedi 3.8.8)
-- Vive nel kv generale come chiavi 'veyon.*':
--   'veyon.config_path_override', 'veyon.auth_keyfile_override',
--   'veyon.distribuzione_default' ('push_runtime')
```

### 4.3 Note di design

#### 4.3.1 `entries.ts` E `entries.ora`

Doppio storage temporale per ragioni opposte:
- `ts` (unix ms intero): query temporali rapide, ordering, range filter
- `ora` (string ISO): leggibile a occhio nel db browser, retrocompatibile con il formato v1 dei log

Costo: 8 byte extra per entry. Trascurabile.

#### 4.3.2 Snapshot vs riferimento

Le sessioni archiviate **fotografano** mappa studenti, blocklist, modo. Modificare la mappa attiva DOPO l'archive non altera i report storici.

`entries.nome_studente` e' anch'esso un snapshot al momento della richiesta. Se domani lo studente cambia banco e il suo IP viene rimappato a un altro nome, le sue entries storiche restano col nome corretto.

#### 4.3.3 Volume dati e indici

Stima volumi (per sessione tipica):
- 30 studenti, ~50-100 richieste/sec di picco, sessione 60-90 min
- Stima: 10.000-50.000 entries per sessione
- 5-10 sessioni/giorno × 200 giorni scuola/anno = ~10M-100M entries/anno per docente

SQLite gestisce queste taglie senza problemi se gli indici sono corretti.

Indici scelti:
- `idx_entries_sessione`: query top-domini/top-studenti su singola sessione
- `idx_entries_nome` (parziale, escludendo NULL): cross-session per studente
- `idx_entries_ts`: filtri temporali del Storico
- `idx_entries_dominio`: storico di un dominio specifico

Skipped (non utili o low-cardinality):
- ~~`idx_entries_ip`~~: si usa nome_studente, gli IP cambiano
- ~~`idx_entries_tipo`~~: solo 3 valori, tablescan piu' veloce
- ~~`idx_entries_blocked`/`flagged`~~: bool, tablescan ok

#### 4.3.4 `studenti_correnti` come tabella separata

Alternativa: serializzare in `kv['studenti_correnti']` come JSON. Tabella vince per:
- Edit di una singola riga (UPDATE WHERE ip=X) senza riserializzare tutto
- Niente race condition se un'altra parte del codice sta leggendo
- SELECT con WHERE per render filtrato

Costo: 8KB DB extra per 30 righe. Non importa.

#### 4.3.5 Password storage

⚠️ **Improvement vs v1**: in v1 `web.auth.password` e' in chiaro in `config.json`. In v2 va in `kv['web.auth.password_hash']` come hash bcrypt o argon2 (libreria Go: `golang.org/x/crypto/bcrypt`). Cambia anche la `controllaAuth()` per fare hash-compare.

L'API `/api/settings/update` per la password riceve la nuova password in chiaro, il server la hasha e salva l'hash. La password non lascia mai il server in nessuna risposta (vedi sanitizzazione gia' presente in v1).

### 4.4 Migrazione da v1

Al primo boot di v2:
1. Se `planck.db` non esiste: crea schema, inserisci default in `kv`, niente preset, niente combo, niente domini.
2. Se sono presenti file legacy v1 (rilevati: `config.json`, `studenti.json`, `_blocked_domains.txt`, `presets/*.json`, `classi/*.json`, `sessioni/*.json`):
    - Mostra prompt al primo accesso UI: "Trovati dati v1. Importa nel database?"
    - Se accetta: tool di migrazione one-shot legge i file e popola SQLite. I file legacy restano sul disco come backup (rinominati `*.v1.bak`).
    - Se rifiuta: parte fresh, i file legacy restano intatti.

### 4.5 Backup

Strategie consigliate (documentazione utente):
- **Backup hot** (mentre Planck e' in esecuzione): `sqlite3 planck.db "VACUUM INTO 'backup.db'"` → snapshot consistente.
- **Backup cold** (Planck spento): `cp planck.db backup.db` (anche `.db-wal` e `.db-shm` se presenti).
- **Backup automatico**: non implementato in v2 MVP. v2.x potrebbe schedulare un VACUUM INTO settimanale.

### 4.6 Schema versioning e migrazioni

Tabella `schema_version` con singola riga `version INTEGER`.

Al boot:
1. Se `schema_version` assente: schema iniziale, inserisce `version = 1`.
2. Se `schema_version.version < V_BINARY`: applica migrazioni `Vn → Vn+1` in sequenza (script SQL embedded nel binario via `//go:embed`).
3. Se `schema_version.version > V_BINARY`: refuse to start, errore "Database creato da una versione piu' nuova di Planck".

Migrazioni forward-only. Niente rollback automatico (i rollback DB sono fragili; la strategia di recovery e' "ripristina backup").

---

## 5. API e protocolli

### 5.1 Convenzioni REST

**Cambio rispetto a v1**: tutte le mutazioni passano da `POST` con body JSON. v1 usava `GET` con query params per ogni cosa, comodo da testare con curl ma non RESTful e senza guardia contro CSRF. v2 standardizza:

- `GET` per letture (sicure, idempotenti)
- `POST` per mutazioni con `Content-Type: application/json`
- `Content-Type: application/json` per le response (eccetto export e static)

**Shape response standard**:
```json
{ "ok": true, "data": <payload> }
```
oppure
```json
{ "ok": false, "error": "messaggio leggibile", "code": "VEYON_UNREACHABLE" }
```

HTTP status:
- 200: ok
- 400: input invalido (es. JSON malformato, parametri mancanti)
- 401: auth richiesta o credenziali errate
- 404: risorsa non trovata (es. sessione id inesistente)
- 409: conflitto (es. preset con nome gia' esistente in modalita' "no-overwrite")
- 500: errore interno
- 503: dipendenza esterna down (Veyon non raggiungibile, ecc.)

**Auth**: HTTP Basic, applicata a tutti gli endpoint `/api/*` se `kv['web.auth.enabled'] = true`. L'endpoint `/_alive` sul proxy NON richiede auth (deve poter essere chiamato dagli studenti).

### 5.2 Boot data

| Metodo | Path | Risposta |
|---|---|---|
| `GET` | `/api/config` | Config base + lista AI consolidata + classi disponibili |
| `GET` | `/api/history` | Snapshot stato corrente (entries recenti, blocklist, sessione, alive map) per idratazione UI all'apertura |

### 5.3 Sessione

| Metodo | Path | Effetto |
|---|---|---|
| `POST` | `/api/session/start` | Apre nuova sessione (se gia' attiva, archivia la precedente come fallback difensivo) |
| `POST` | `/api/session/stop` | Archivia + chiude sessione corrente |
| `GET` | `/api/session/status` | `{attiva, inizio, fine, durata_sec, richieste, ...}` |
| `GET` | `/api/session/recover` | Status sessione interrotta da crash (NDJSON residuo); UI propone import |
| `POST` | `/api/session/recover/import` | Importa sessione interrotta nel DB |
| `POST` | `/api/session/recover/discard` | Scarta NDJSON residuo |

### 5.4 Blocchi

| Metodo | Path | Body | Effetto |
|---|---|---|---|
| `POST` | `/api/block` | `{dominio}` | Aggiunge a bloccati |
| `POST` | `/api/unblock` | `{dominio}` | Rimuove da bloccati |
| `POST` | `/api/block-all-ai` | — | Aggiunge tutti i domini AI consolidati |
| `POST` | `/api/unblock-all-ai` | — | Rimuove |
| `POST` | `/api/clear-blocklist` | — | Svuota blocklist |
| `POST` | `/api/pause/toggle` | — | Toggle pausa globale |
| `POST` | `/api/pause/on` / `/api/pause/off` | — | Imposta esplicitamente |

### 5.5 Preset

| Metodo | Path | Body | Effetto |
|---|---|---|---|
| `GET` | `/api/presets` | — | Lista nomi |
| `POST` | `/api/preset/save` | `{nome, descrizione?}` | Salva blocklist corrente come preset |
| `POST` | `/api/preset/load` | `{nome}` | Carica preset (sostituisce blocklist) |
| `POST` | `/api/preset/delete` | `{nome}` | Elimina |

### 5.6 Mappa studenti / Combo

| Metodo | Path | Body | Effetto |
|---|---|---|---|
| `POST` | `/api/students/set` | `{ip, nome}` | Upsert |
| `POST` | `/api/students/delete` | `{ip}` | Elimina |
| `POST` | `/api/students/clear` | — | Svuota mappa attiva |
| `GET` | `/api/classi` | — | Lista combo `[{classe, lab, file}, ...]` |
| `POST` | `/api/classi/load` | `{classe, lab}` | Carica snapshot in mappa attiva |
| `POST` | `/api/classi/save` | `{classe, lab}` | Salva mappa attiva come snapshot |
| `POST` | `/api/classi/delete` | `{classe, lab}` | Elimina snapshot |

### 5.7 Settings

| Metodo | Path | Body | Effetto |
|---|---|---|---|
| `GET` | `/api/settings` | — | Tutto il `kv` (con password mascherata) |
| `POST` | `/api/settings/update` | `{key: value, ...}` | Mass-update dotted-keys, validazione per chiave |
| `POST` | `/api/settings/ignorati/add` | `{dominio}` | Aggiunge a `domini_ignorati` |
| `POST` | `/api/settings/ignorati/remove` | `{dominio}` | Rimuove |

### 5.8 Archivio sessioni

| Metodo | Path | Body | Risposta |
|---|---|---|---|
| `GET` | `/api/sessioni` | — | Lista metadata `[{id, inizio, fine, classe, lab, totale_entries, ...}]` |
| `GET` | `/api/sessioni/:id` | — | Dump completo (header + entries + eventi_auto) |
| `POST` | `/api/sessioni/:id/delete` | — | Elimina sessione + entries (CASCADE) |
| `POST` | `/api/sessioni/archivia` | — | Forza archive senza chiudere sessione (checkpoint) |

### 5.9 Storico (cross-session)

| Metodo | Path | Query | Risposta |
|---|---|---|---|
| `GET` | `/api/storico/student` | `?nome=X` | Tutte le sessioni in cui lo studente compare + aggregati |
| `GET` | `/api/storico/compare` | `?a=:id&b=:id` | Diff side-by-side fra due sessioni |
| `GET` | `/api/storico/filter` | `?classe=&lab=&data_inizio=&data_fine=&tipo=` | Lista sessioni filtrate con riepiloghi |

### 5.10 Export

| Metodo | Path | Query | Output |
|---|---|---|---|
| `GET` | `/api/export/session/:id` | `?format=json\|csv` | File download (`Content-Disposition: attachment`) |
| `GET` | `/api/export/aggregato` | filtri come storico | File JSON aggregato con tutte le sessioni nel filtro |

PDF: generato lato browser (vedi 3.11.6), niente endpoint backend.

### 5.11 Auto-classification AI

| Metodo | Path | Body | Effetto |
|---|---|---|---|
| `POST` | `/api/ai/upstream/sync` | — | Trigger fetch manuale della lista upstream |
| `GET` | `/api/ai/upstream/status` | — | `{version, fetched_at, source_url, total_domini}` |
| `POST` | `/api/ai/local/add` | `{dominio}` | Aggiunge a `domini_ai_locali` |
| `POST` | `/api/ai/local/remove` | `{dominio}` | Rimuove |
| `POST` | `/api/ai/non-ai/add` | `{dominio}` | Aggiunge a `domini_non_ai_locali` (esclude da heuristic) |

### 5.12 Reazioni automatiche

| Metodo | Path | Body | Effetto |
|---|---|---|---|
| `GET` | `/api/regole` | — | Lista regole_auto |
| `POST` | `/api/regole/save` | `{id?, nome, trigger, azioni[], cooldown_sec, scope, abilitata, parametri}` | Crea/aggiorna |
| `POST` | `/api/regole/delete` | `{id}` | Elimina |
| `POST` | `/api/regole/preset-toggle` | `{preset_key, abilitata}` | Toggle dei 3 preset hardcoded (3.10.6) |

### 5.13 Veyon

| Metodo | Path | Body | Effetto |
|---|---|---|---|
| `POST` | `/api/veyon/feature` | `{ips[], feature, args?}` | Esegue feature su uno o piu' IP |
| `POST` | `/api/veyon/file-transfer` | `multipart: ips[] + file` | FileTransfer del file caricato a tutti gli IP |
| `GET` | `/api/veyon/screenshot/:ip` | — | Immagine PNG del framebuffer dello studente (one-shot) |
| `GET` | `/api/veyon/programmi-rapidi` | — | Lista programmi rapidi configurati |
| `POST` | `/api/veyon/programmi-rapidi/save` | `{id?, nome, comando, modalita, posizione}` | Crea/aggiorna |
| `POST` | `/api/veyon/programmi-rapidi/delete` | `{id}` | Elimina |
| `GET` | `/api/veyon/test-connection` | `?ip=X` | Verifica auth + raggiungibilita' di un singolo IP |
| `GET` | `/api/veyon/status` | — | Status di Veyon: `{config_path, auto_imported, auth_method, n_hosts}` |

`feature` accettati nel POST `/api/veyon/feature`:
`screen_lock`, `screenshot`, `run_program` (args: `{programma_id}` o `{comando_raw}`), `power_down`, `reboot`, `log_off`, `power_on`. (TextMessage e RemoteAccess: v2.x.)

Risposta multi-IP: array di esiti per ogni IP `[{ip, ok, error?}]` cosi' la UI puo' mostrare summary "25/30 OK, 5 errori".

### 5.14 SSE — `/api/stream`

Stream Server-Sent Events. La connessione si tiene aperta finche' il client non disconnette. Heartbeat `: hb` ogni 20s per evitare timeout proxy.

Ogni messaggio: `data: <JSON>\n\n` con campo `type` discriminator.

| `type` | Payload | Trigger |
|---|---|---|
| `traffic` | `{entry: {ora, ts, ip, nome, metodo, dominio, tipo, blocked, flagged}}` | Ogni richiesta loggata (sempre attivo, indipendente da sessione) |
| `blocklist` | `{list: [domini]}` | Mutazione blocklist |
| `studenti` | `{studenti: {ip:nome,...}}` | Mutazione mappa attiva |
| `classi` | `{classi: [{classe,lab,...}]}` | CRUD su combo |
| `settings` | `{settings: {...}}` | Update kv (con password mascherata) |
| `session-state` | `{attiva, inizio, fine, durata_sec}` | Avvia / Ferma / archive |
| `pause` | `{pausato}` | Toggle pausa |
| `deadline` | `{deadline_iso}` | Set/clear deadline |
| `deadline-reached` | `{deadline_iso}` | Scadenza raggiunta |
| `alive` | `{ip, ts}` | Ogni ping watchdog |
| `regola-fired` | `{regola_id, target_ip, azioni_eseguite, dettagli}` | Una reazione auto e' scattata (UI mostra notifica) |
| `flag-sospetto` | `{dominio, motivo}` | Heuristic flagging ha rilevato un nuovo dominio sospetto AI |
| `upstream-synced` | `{version, fetched_at, n_domini}` | Sync lista upstream completato |

### 5.15 Watchdog endpoint — `/_alive`

Servito sul **server proxy** (`config.proxy.port`), non sul server web. Senza auth.

```
GET /_alive HTTP/1.1
Host: <ip-docente>:9090

→
HTTP/1.1 200 OK
Content-Type: text/plain
Content-Length: 2
Connection: close
Cache-Control: no-store

ok
```

Il server estrae l'IP del client dal socket, registra `aliveMap[ip] = now()`, broadcasta SSE `{type:'alive', ip, ts}`. Latenza target: < 5ms.

### 5.16 Protocollo Veyon (client side)

Planck v2 implementa il **client del protocollo Veyon Master ↔ veyon-server**. Sorgente di verita' del protocollo: il codice C++ Qt del progetto Veyon (https://github.com/veyon/veyon, principalmente `core/src/`).

#### 5.16.1 Trasporto

- TCP, porta default 11100 sui PC studenti
- TLS opzionale (Veyon server di default accetta TLS via openssl, certificati self-signed firmati dalla chiave master)
- Framing: `[uint32 length BE][payload]`
- Payload: messaggi binari Qt-serialized (QVariantMap → QDataStream)

⚠️ Nota implementativa: Qt usa un proprio formato di serializzazione binaria non-standard. Reimplementarlo da zero in Go richiede leggere `qdatastream.h`/`qmetatype.h` e replicare. Esistono libs Go embrionali per questo (es. `github.com/dlclark/regexp2` non c'entra; vedi librerie per Qt deserialization Go — da valutare se usare o reimplementare il sottoinsieme che ci serve).

**Decisione pragmatica per MVP**: implementare manualmente il subset di QDataStream necessario (solo i tipi che usa Veyon: QString, QByteArray, QVariantMap, QVariantList, qint32, ecc.). Stima: ~300 righe Go, lavoro una tantum.

#### 5.16.2 Handshake autenticazione (KeyFile)

Sequenza:
1. Client (Planck): apre TCP+TLS a server:11100
2. Server: invia `AuthChallenge` (random nonce)
3. Client: firma il nonce con la chiave privata RSA/EC, invia `AuthResponse`
4. Server: verifica firma con la chiave pubblica registrata, risponde `AuthOk` o `AuthFail`
5. Da qui in poi, scambio messaggi feature

Riferimento source: `core/src/AuthenticationManager.cpp`, `core/src/AuthKeysManager.cpp`.

#### 5.16.3 Comandi feature

Ogni feature ha un UUID stabile (definito in `core/src/Feature.h` di Veyon). Comando feature = messaggio con:
```
{
    "feature_uid": "<uuid>",
    "command": "start" | "stop" | "query",
    "arguments": { ... feature-specific ... }
}
```

UUID delle feature usate (estratti dal sorgente Veyon):
| Feature | UUID |
|---|---|
| ScreenLock | `ccb535a2-1d24-4cc1-a709-8b47d2b2ac79` |
| RunProgram | `da9ca56a-b2ad-4fff-8f8a-929b2927b442` |
| PowerControl (su/giu/log off/reboot) | `f483be1d-ddf4-4df3-bd6f-23a7b6249f9d` |
| FileTransfer | `4a70bd5a-fab2-4a4b-a92a-a1660a6105b7` |
| ScreenshotManagement | `ea197ac6-0e06-46be-8f2c-b21a26b05b88` (deferred ma in roadmap MVP) |

⚠️ Verificare gli UUID al momento dell'implementazione contro il sorgente Veyon corrente — sono in `<feature>/<feature>FeaturePlugin.cpp`.

#### 5.16.4 Screenshot one-shot via VNC

Per Screenshot non vogliamo aprire un canale VNC streaming completo. Tre opzioni:

- **(a)** Usare la feature `ScreenshotManagement` di Veyon che gia' implementa "cattura uno screenshot e salvalo": il server cattura e ritorna un PNG come messaggio. **Preferito.**
- (b) Aprire una connessione RFB transient, fare un solo `FramebufferUpdateRequest`, leggere il primo frame, chiudere.
- (c) Snapshot via altra via (skip).

Decisione: **(a)** se la feature `ScreenshotManagement` di Veyon supporta la cattura on-demand (verificare al momento dell'implementazione). Altrimenti fallback a (b).

#### 5.16.5 Edge cases protocol

| Scenario | Comportamento |
|---|---|
| TLS handshake fallisce (cert untrusted, ecc.) | Errore `VEYON_TLS_FAILED`, log con dettagli, suggerimento di rigenerare keys |
| Server timeout (no response in 5s) | Errore `VEYON_TIMEOUT`, retry singolo, poi fallimento |
| Auth fail | Errore `VEYON_AUTH_FAILED` |
| Feature UID sconosciuto al server (versione vecchia) | Errore `VEYON_FEATURE_NOT_SUPPORTED`, UI mostra warning sulla card |
| File transfer interrotto | Errore con percentuale di completamento, possibilita' di retry |

---

## 6. UI map

### 6.1 Layout globale

```
┌─────────────────────────────────────────────────────────────┐
│ [Live] [Report] [Storico] [Impostazioni]    [PAUSA] [03:42] │
│                                              [🌙] [🔔]       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│              CONTENUTO DEL TAB ATTIVO                        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

- **Topbar fissa** in alto: tab nav (4 voci) a sinistra, indicatori live (pausa lampeggiante, countdown deadline, toggle tema, toggle notifiche) a destra
- Tab attivo persistito in `localStorage`
- Switch tab e' istantaneo (i tab inattivi sono `display: none`, non distrutti — preserva scroll e focus)
- Banner AI (rosso lampeggiante su detection) appare sopra la topbar, full-width, auto-dismiss 5s

### 6.2 Tab Live

```
┌────────────────────────────────────────────────────────────────┐
│ [Toolbar principale]                                            │
│ [Avvia sessione] [Pausa] [Blocca AI] [Sblocca AI] [⋮]           │
│                                              Fine: [HH:MM] [×]  │
├────────────────────────────────────────────────────────────────┤
│ [Toolbar Veyon] (collapsible/dropdown "Azioni classe ▾")        │
│ [🔒 Lock all] [📷 Shot all] [📁 File] [⏻ Off] [🔄 Reboot] [⚡ On] │
├────────────────────────────────────────────────────────────────┤
│ [Stat row: 5 card]                                              │
│ Richieste │ Domini │ IP attivi │ Durata │ Stato                 │
├──────────┬───────────────────────────────────┬──────────────────┤
│ Sidebar  │ Pannello IP                        │ Ultime richieste│
│ Domini   │                                    │                  │
│ [«]      │ [▦] [☰]   "Selezione: 3 [✕]"        │              [»] │
│          │ ┌─────┐ ┌─────┐ ┌─────┐             │ 10:23:45 [Mario] │
│ [filter] │ │card │ │card │ │card │             │   chatgpt.com    │
│ - AI(3)  │ │selez│ │     │ │selez│             │ 10:23:42 [Luca]  │
│ - Siti   │ └─────┘ └─────┘ └─────┘             │   booking.com    │
│ - Sis    │ ...                                │ ...              │
│ - Block  │                                    │                  │
└──────────┴───────────────────────────────────┴──────────────────┘
```

#### 6.2.1 Toolbar principale

Ordine bottoni: `[Avvia/Ferma sessione]` (primary, colore cambia con stato) → `[Pausa]` → `[Blocca AI]` (rosso) → `[Sblocca AI]` → `[⋮ menu overflow]`. A destra: `Fine: [time picker] [×]` per la deadline.

Menu `⋮` contiene: `Svuota blocklist`, sezione `Preset` (load + save), `Esporta JSON sessione corrente`.

#### 6.2.2 Toolbar Veyon "Azioni classe"

In una riga separata sotto la toolbar principale, oppure nascosta dietro un dropdown `Azioni classe ▾` per non saturare visivamente. Default: dropdown collassato.

Bottoni: `Lock all`, `Screenshot all`, `FileTransfer to all`, `PowerDown all`, `Reboot all`, `PowerOn all` (Wake-on-LAN).

#### 6.2.3 Selection bar (multi-select)

Compare in cima al pannello IP quando selezione >= 1 card:

```
[3 studenti selezionati]  [🔒] [📷] [💬] [📁] [...]  [Deseleziona tutti]
```

Mostra le stesse azioni della toolbar globale ma applicate alla selezione invece che a tutti.

#### 6.2.4 Pannello IP — vista griglia (default)

Ogni card studente:

```
┌───────────────────────────┐
│ 🟢 Mario Rossi  ✓         │  ← dot watchdog, nome, checkmark se selezionata
│    192.168.6.15           │  ← IP piccolo
│ ─────────────────────────  │
│ 47                 2m fa  │  ← N richieste (grande), ultima
│ ─────────────────────────  │
│ classroom.google.com       │
│ docs.google.com            │  ← top 6 domini (più recenti in cima)
│ mail.google.com            │
│ +12                        │  ← overflow indicator
│ ─────────────────────────  │
│ [🔒] [📷] [▶ ▾]            │  ← bottoni Veyon inline (v2)
└───────────────────────────┘
```

Stati visivi:
- Default: bordo grigio chiaro
- Hover: bordo accent
- **Inattivo**: sfondo arancione tenue + bordo arancione (>180s no traffic real)
- **Selezionata**: bordo accent spesso + checkmark
- **Focus** (single-click): outline accent (filtra traffico per quell'IP)

#### 6.2.5 Pannello IP — vista lista

Tabella 6 colonne: WD | Studente / IP | N | Ultima | Domini (tag inline) | Azioni

Stesse semantiche della griglia, layout piu' compatto. Le 3 icone Veyon vivono in colonna "Azioni".

#### 6.2.6 Ultime richieste (pannello destro)

Lista cronologica inversa, ultime 100 entry. Ogni riga: `<orario> [<nome o ip>] <dominio>`. Domini AI evidenziati in rosso. Filtro testuale (sidebar) si applica anche qui.

#### 6.2.7 Sidebar domini

5 sezioni: AI / Siti / Sistema / Bloccati / Nascosti. Sezioni vuote nascoste automaticamente.

Ogni voce dominio: `<nome> <count> [×]`. Click sul nome: nascondi (sposta in Nascosti). Click sulla `×`: blocca (sposta in Bloccati).

Filtro testuale globale in cima.

### 6.3 Tab Report

```
┌─────────────────────────────────────────────────────────────────┐
│ Report sessione corrente                  [Export ▾]            │
│ [Sessione: -- corrente -- ▾]                                    │
├──────────────────────────────────┬──────────────────────────────┤
│ Riepilogo                         │ Top domini AI                │
│ Inizio:    2026-04-22 09:15       │ chatgpt.com    ████████ 12   │
│ Durata:    1:23:45                │ claude.ai      ████ 6        │
│ Richieste: 1240                   │ ...                          │
│ ...                               │                              │
├──────────────────────────────────┼──────────────────────────────┤
│ Top 10 domini (tutti)             │ Top studenti                 │
│ ...                               │ Mario Rossi   ████████ 156   │
│                                   │ ...                          │
├──────────────────────────────────┴──────────────────────────────┤
│ Eventi auto-reazione (se presenti)                               │
│ 09:23:11  ai_rilevato (chatgpt.com) → screen_lock Mario          │
│ ...                                                              │
└─────────────────────────────────────────────────────────────────┘
```

Header con dropdown selettore sessione (corrente o archivi). Bottone `Export ▾`: dropdown JSON / CSV / Stampa PDF.

Sezione "Eventi auto-reazione" appare solo se la sessione ha generato eventi.

### 6.4 Tab Storico (nuovo v2)

Due viste switchable in alto:

```
┌─────────────────────────────────────────────────────────────────┐
│ [Per studente] [Confronto sessioni]                             │
├─────────────────────────────────────────────────────────────────┤
│ [Filtri: classe ▾, lab ▾, da [date], a [date], tipo evento ▾]   │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   contenuto della vista selezionata                              │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

#### 6.4.1 Vista "Per studente"

```
[Cerca studente: <input autocomplete>]

────────────────────────────────────────────────────────
Mario Rossi
Ultimo IP visto: 192.168.6.15 (lab2 / 4DII)
Apparso in 7 sessioni dal 2026-03-12 al 2026-04-22

[ Trend AI nel tempo: line chart compact ]

Sessioni:
┌──────────────┬────────┬─────┬────────┬─────────┬─────────┐
│ Data         │ Classe │ Lab │ Richi. │ AI tent.│ Bloccati │
├──────────────┼────────┼─────┼────────┼─────────┼─────────┤
│ 2026-04-22   │ 4DII   │ lab2│  156   │   3     │   1     │
│ 2026-04-15   │ 4DII   │ lab2│   42   │   0     │   0     │
│ ...                                                       │
└──────────────┴────────┴─────┴────────┴─────────┴─────────┘
(click su una riga → apre la sessione nel tab Report)
```

#### 6.4.2 Vista "Confronto sessioni"

```
[Sessione A: <select>]   [Sessione B: <select>]

┌──────────────────────────┬──────────────────────────┐
│ Sessione A               │ Sessione B               │
│ 2026-04-22 4DII / lab2   │ 2026-04-15 4DII / lab2   │
│ Durata: 1:30             │ Durata: 1:15             │
│ Richieste: 1240          │ Richieste: 980           │
│ AI: 12                   │ AI: 4                    │
│ ...                      │ ...                      │
├──────────────────────────┴──────────────────────────┤
│ Diff top domini                                      │
│  • chatgpt.com:    A=12  B=2   (+10)                 │
│  • booking.com:    A=8   B=15  (-7)                  │
│  • new-domain.io:  A=5   B=0   (solo A)              │
│  ...                                                 │
├──────────────────────────────────────────────────────┤
│ Diff studenti                                        │
│  • Mario Rossi:   A=156  B=42   (+114)               │
│  ...                                                 │
└──────────────────────────────────────────────────────┘
```

### 6.5 Tab Impostazioni

Layout: lista verticale di card (sezioni espandibili/collassabili). Ordine consigliato:

1. **Sessione** (riepilogo stato + bottone forza-archive)
2. **Generale**: titolo, classe (cosmetic), modo proxy, soglia inattivita'
3. **Mappa studenti**: tabella editabile + form add + combo manager (carica/salva/elimina)
4. **Domini ignorati**: lista + add/remove
5. **Reazioni automatiche**: 3 toggle preset + regole custom (collapsible)
6. **Veyon**: status, override config, test connessione, programmi rapidi (CRUD)
7. **Lista AI**: status upstream (versione, last sync), bottone sync manuale, "I miei domini AI locali" (CRUD), "Domini esclusi da heuristic" (CRUD)
8. **Porte** (boot-only, badge "richiede riavvio")
9. **Autenticazione** (boot-only, badge "richiede riavvio")
10. **Backup** (info path DB, comandi suggeriti)
11. **Info**: versione Planck, link al repo, license

⚠️ Banner orange in cima se `riavvio_richiesto = true` (modifica a chiave `SETTINGS_RESTART`): "Alcune modifiche richiedono il riavvio del server".

### 6.6 Modali e popup ricorrenti

| Modale | Trigger | Contenuto |
|---|---|---|
| **Conferma azione distruttiva** | Svuota blocklist, Elimina preset, Elimina sessione, Svuota mappa, Reset cache | Testo "Sei sicuro?" + bottoni `Annulla` / `Conferma` |
| **Conferma sessione** | Avvia / Ferma sessione | Testo contestuale (vedi UC2) + bottoni |
| **È AI?** (heuristic) | Click sul badge 🤔 | Dominio + 3 bottoni: `Si', AI` / `No, non AI` / `Annulla` |
| **Sessione interrotta** | Boot rileva NDJSON residuo | Info sessione (data, dimensione, durata) + `Importa nel database` / `Scarta` |
| **Nuova/edit regola auto** | Pulsante in Impostazioni | Form: nome, trigger, azioni (checkboxes), cooldown, scope, abilitata |
| **File picker FileTransfer** | Bottone "Invia file" | File picker browser nativo + dialog conferma "Inviare a N studenti?" |
| **Test connessione Veyon** | Bottone in Impostazioni | Spinner → risultato (success/error con dettagli) |

Convenzioni modali:
- Backdrop scuro semi-trasparente
- ESC chiude (eccetto se c'e' un form modificato → conferma)
- Click sul backdrop chiude (stesso comportamento di ESC)
- Focus auto-trapped dentro la modale

### 6.7 Shortcut da tastiera

| Shortcut | Azione |
|---|---|
| `Ctrl+1` / `Ctrl+2` / `Ctrl+3` / `Ctrl+4` | Switch tab (Live/Report/Storico/Impostazioni) |
| `Ctrl+S` | Toggle Avvia/Ferma sessione |
| `Ctrl+P` | Toggle pausa globale |
| `Ctrl+F` | Focus sul filtro testuale (Live tab) |
| `ESC` | Deseleziona tutti / chiude modale / clear focus IP |
| `Ctrl+A` (in vista IP) | Seleziona tutti gli IP visibili |

⚠️ v2 MVP: shortcuts implementati ma non discoverable. v2.x: tasto `?` mostra help overlay con tutti gli shortcut.

### 6.8 Stati visivi e feedback

| Stato | Quando | Visualizzazione |
|---|---|---|
| **Empty state — nessun IP** | Mappa vuota e nessun ping | Pannello IP: "Nessuno studente connesso ancora. Distribuisci proxy_on.bat con il bottone in toolbar." con CTA |
| **Loading** | Sync upstream, FileTransfer, Screenshot fetch | Spinner inline + testo "in corso..." + (quando applicabile) % progress |
| **Error toast** | Comando Veyon fallito, sync fallito | Toast in basso a destra, auto-dismiss 5s, icona errore + messaggio |
| **Success toast** | Preset salvato, regola creata, file inviato | Toast verde, auto-dismiss 3s |
| **Connection lost** | EventSource disconnesso | Stat card "Stato" mostra `OFF` rosso, banner discreto in cima "Riconnessione..." |
| **Veyon down** | Comando Veyon fallisce con auth/connection error | Card studente: bordo grigio + icona ⚠️, tooltip con dettaglio errore |

### 6.9 Internazionalizzazione

**Solo italiano per v2.** Niente switcher lingua, niente i18n infrastructure. Tutte le stringhe hardcoded in italiano.

⚠️ Decisione consapevole: il target sono prof italiani della scuola del maintainer; tradurre in inglese o altre lingue non aggiunge valore al primo deploy. v2.x potra' considerare i18n se ci sono richieste reali.

### 6.10 Responsiveness e dispositivi

**Desktop-first, esclusivamente.**

- Min width consigliata: **1280px** (le 3 colonne della Live tab necessitano questo spazio)
- Tablet (es. 1024px): degradazione graziosa, ma le card della griglia diventano un po' strette. Usabile ma non ottimale.
- Mobile (< 768px): non supportato. Il toolkit e' un dashboard da PC docente, non un'app mobile.

Niente media queries elaborate per mobile in MVP. La dashboard e' pensata per essere aperta su un monitor secondario del PC cattedra del laboratorio.

### 6.11 Tema

Variabili CSS custom properties con valori scambiati via classe `body.dark`. Toggle in topbar (icona ☀️/🌙).

Persistenza in `localStorage.darkmode`. Niente "auto" basato su preferenza OS in MVP (semplifica; v2.x: aggiungere `prefers-color-scheme: dark` come default).

Palette:
- Light: `#f0f2f5` background, `#fff` card, `#333` text, accent viola `#8e44ad`
- Dark: `#1a1d23` background, `#252a33` card, `#e0e0e0` text, accent viola chiaro `#b77dd4`

---

## 7. Non funzionali

### 7.1 Performance

**Target di throughput**:
- 30 studenti per istanza, picco ~50-100 req/s di traffico HTTP/HTTPS proxato
- Watchdog: 30 ping/s (uno ogni 5s × 30 studenti) — banale
- SSE: il server broadcasta a 1-N client UI (di solito 1, raramente 2-3)

**Target di latenza**:
- Overhead proxy (lookup blocklist + classificazione + log + broadcast): **< 5 ms** per richiesta. Trascurabile rispetto al RTT internet.
- SSE delivery (server fire → render UI): **< 100 ms** end-to-end
- Query Report singola sessione: **< 50 ms** (entries con indice `sessione_id`)
- Query Storico filter complesso: **< 200 ms** anche con 1M entries totali
- Render UI sotto burst (50 SSE messages/s): nessun frame drop visibile (60fps stabile grazie a batch SSE 250ms gia' presente in v1)

**Strategie**:
- Batch SSE traffic (250ms windows): porta render storm da ~60/s a ~4/s
- Render UI con `syncChildren` (gia' v1): riconciliazione DOM keyed, niente innerHTML wipe
- Indici SQLite mirati (vedi §4.3.3)
- WAL mode: scritture non bloccano letture
- NDJSON append durante sessione: write O(1) per entry, niente latency spikes

**Stress non testati ma sospettati come limiti**:
- 100+ studenti: SSE buffer potrebbe saturare; richiede misure
- 10M+ entries totali in DB: query analitiche cross-session iniziano a richiedere ottimizzazioni (indici composti, pre-aggregazioni)

### 7.2 Sicurezza

#### 7.2.1 Threat model

Planck e' un tool **trusted-LAN**: assume che la rete del laboratorio sia sotto il controllo della scuola. NON e' progettato per:
- Esposizione su Internet
- Multi-tenant
- Difesa contro attaccanti motivati con accesso fisico al PC docente

Threat in scope:
- Studente curioso/scaltro che cerca di bypassare il proxy con metodi banali (impostazioni Windows, reload, ecc.)
- Studente che tenta di intercettare il pannello docente sulla LAN
- Errori operatore (docente che lascia password debole)

#### 7.2.2 Auth

- HTTP Basic su tutti gli endpoint `/api/*` se `kv['web.auth.enabled'] = true`
- Password: hashata bcrypt (cost 10 default), MAI plaintext nel DB
- Salting automatico via bcrypt
- Default: auth **disabilitata** (LAN trusted assunto). L'utente la attiva esplicitamente in Impostazioni se vuole.

⚠️ **Improvement vs v1**: in v1 password in chiaro in `config.json`. In v2 hash bcrypt. La password originale non transita mai nel DB.

#### 7.2.3 TLS

| Endpoint | TLS |
|---|---|
| Web UI (porta 9999) | **NO** in MVP (LAN trusted). HTTP plain |
| Proxy (porta 9090) | **NO** sul lato docente (gli studenti vedono il proxy come HTTP) |
| Proxy → origin server | TLS opaco (CONNECT tunneling, non proxy MITM) |
| Veyon connection (verso student PCs) | **SI** (TLS standard di Veyon, certificati self-signed firmati con la chiave master) |
| Sync lista AI upstream | **SI** (HTTPS verso GitHub) |

Per esposizione futura del Web UI con TLS: deferred a v2.x. Nel mentre, suggerimento per chi vuole esporre: usare un reverse proxy esterno (nginx/Caddy) o tunnel TLS (Cloudflare/Tailscale).

#### 7.2.4 Secrets handling

- Password (plaintext) e chiave privata Veyon: **mai** loggate, **mai** in response API
- `sanitizeConfig()` strappa `web.auth.password` da ogni response, sostituisce con `passwordSet: bool`
- Chiave privata Veyon: caricata in RAM al boot, mai serializzata in API. Path della chiave e' configurabile in Impostazioni ma il contenuto della chiave non lo e'.

#### 7.2.5 Input validation

- Filename sanitization per preset/sessioni/combo: regex `[a-zA-Z0-9_-]` (gia' v1)
- Path traversal in `serveStatic`: check `..` nel path (gia' v1)
- Validatori per ogni chiave settings (vedi §4 e v1 `SETTINGS_VALIDATORI`)
- Body size limit POST: 1 MB (gia' v1, anti-flood)

#### 7.2.6 CSRF

Mitigato da:
- POST + body JSON `Content-Type: application/json`: i browser non permettono questo cross-origin senza CORS preflight
- Niente `Access-Control-Allow-Origin: *` impostato (default same-origin)
- Niente token CSRF dedicato in MVP (overkill per LAN trusted)

⚠️ Se in v2.x si esponesse il pannello su Internet, valutare token CSRF.

### 7.3 Deployment

#### 7.3.1 Distribuzione

- **Single binary**: `planck.exe` (Windows x64), `planck` (Linux x64), `planck-darwin` (macOS) — best-effort cross-platform
- Build via `go build`, niente cgo (modernc.org/sqlite e' pure Go)
- Frontend embedded in binario via `//go:embed public/*`
- Target dimensione binario: **< 25 MB**

#### 7.3.2 Setup iniziale

1. Scarica `planck.exe` dalla pagina releases GitHub
2. Mettilo in una cartella sul PC docente
3. Doppio click → si avvia
4. Apri browser su `http://localhost:9999`
5. (opzionale) Configura auth in Impostazioni

Niente installer, niente service registration. Vuoi che parta con Windows? Aggiungi un collegamento in `shell:startup`. (v2.x potrebbe offrire un installer Windows con scelta "avvio automatico").

#### 7.3.3 Cartella di lavoro

Il binario crea/usa file accanto a se':
- `planck.db` (SQLite)
- `planck.db-wal`, `planck.db-shm` (WAL files)
- `sessioni/_corrente.ndjson` (sessione in corso)
- `sessioni/<id>.json` (export snapshots, opzionale backup)
- `screenshots/<sessione>/<ip>-<ts>.png` (se feature Screenshot usata)

⚠️ Il binario richiede **scrivibilita'** della propria cartella. Non funziona se messo in `Program Files` senza override.

#### 7.3.4 Aggiornamento

1. Stop di Planck (chiusura console o interruzione)
2. Sostituisci `planck.exe` con la nuova versione
3. Riavvio
4. Migrazioni DB applicate automaticamente (vedi §4.6)

Il DB e i file dati restano intatti tra le versioni.

⚠️ **Decisione esplicita**: niente auto-update in MVP. v2.x potrebbe aggiungere check release GitHub al boot ("Nuova versione disponibile").

### 7.4 Affidabilita' e crash recovery

#### 7.4.1 Tipi di shutdown

| Tipo | Comportamento Planck |
|---|---|
| Graceful (Ctrl+C, SIGTERM) | Stop sessione corrente + archive automatico, chiusura DB pulita |
| Crash (panico Go, kill -9, BSOD) | NDJSON resta su disco, DB potrebbe avere transazioni incomplete (WAL le rolla back al boot) |
| Power loss | Come crash. WAL garantisce consistenza |

#### 7.4.2 Recovery automatico al boot

1. SQLite WAL replay: SQLite recupera transazioni committed, scarta quelle in flight
2. Schema migrations applicate se schema_version < V_BINARY (§4.6)
3. Detection NDJSON residuo (§3.3.4): UI propone "Importa sessione interrotta?"

#### 7.4.3 Backup

Già discusso in §4.5. Comandi documentati per l'utente, niente automation in MVP.

### 7.5 Compatibilita'

#### 7.5.1 OS supportati

| OS | Status |
|---|---|
| Windows 10/11 x64 | **Primario** — target principale, testato |
| Linux x64 (Ubuntu/Debian recenti) | Best-effort — dovrebbe funzionare ma non testato come scenario primario |
| macOS Apple Silicon / Intel | Best-effort — improbabile use case ma niente blocchi noti |
| Windows 7/8/Server | Non supportato |

#### 7.5.2 Browser per la dashboard

| Browser | Status |
|---|---|
| Chrome / Edge >= 100 | **Primario** |
| Firefox >= 100 | Supportato |
| Safari recente | Best-effort |
| Internet Explorer | Non supportato |

Tecnologie usate (no build step):
- ES modules nativi (`<script type="module">`)
- `EventSource` (SSE)
- `<details>` per menu overflow
- CSS custom properties per tema
- HTML5 Notifications API (opzionale)
- Web Audio API (per beep, opzionale)

Tutte supportate da molto tempo nei browser sopra.

### 7.6 Logging e observability

**stdout structured-ish**:
- Formato: `<HH:MM:SS> <livello> <messaggio>`
- Livelli: `INFO`, `WARN`, `ERROR`
- Default: solo INFO e sopra
- DEBUG verbose via env var `PLANCK_DEBUG=1` (logga ogni request proxy con dominio + classificazione)

**Niente file di log strutturato in MVP**: il "log" applicativo e' il DB stesso (tabella `entries`, `eventi_auto`).

**Niente metrics endpoint** (`/metrics`) in MVP. v2.x potrebbe aggiungere counters Prometheus-style se ci sono casi d'uso reali.

### 7.7 Privacy e GDPR

#### 7.7.1 Dati raccolti

Planck raccoglie e persiste:
- IP dei PC studenti
- Nomi degli studenti (mappati manualmente dal docente)
- Hostname dei siti visitati (NON i body delle richieste)
- Timestamp delle visite
- Eventi di reazione automatica (lock, screenshot, ecc.)

Tutti questi sono **dati personali ai sensi GDPR**.

#### 7.7.2 Architettura privacy-preserving

- **Zero telemetria**: Planck NON invia dati a server esterni. Niente analytics, niente "phone home".
- **Zero cloud**: tutti i dati restano sul PC del docente in `planck.db`.
- **Sync upstream lista AI**: il GET HTTP verso GitHub raw e' l'**unico** outbound network call. Niente dati studenti vi transitano (e' solo un fetch della lista). Mai POST verso esterni.
- **Screenshots**: salvati localmente in `screenshots/`, mai uploadati.

#### 7.7.3 Responsabilita' della scuola

Decisione di default: **nessuna retention automatica** (vedi §3.11.7). La scuola deve:
- Includere Planck nel **registro dei trattamenti** GDPR
- **Informare gli studenti** del trattamento dei dati durante le verifiche (informativa)
- Definire **policy di retention** appropriata e cancellare manualmente i dati piu' vecchi
- In caso di **richiesta di accesso/cancellazione** da parte di uno studente: il docente puo' esportare/cancellare le sessioni che riguardano quello studente (filtro per nome → eliminazione, vedi §3.11.5)

⚠️ Planck come strumento e' privacy-preserving by design (zero esfiltrazione). La compliance operativa e' della scuola.

### 7.8 Limiti strutturali noti

Riassunti per chiarezza (gia' presenti in altre sezioni):

| Limite | Mitigazione |
|---|---|
| **Hotspot bypass**: studente esce dalla LAN scuola, proxy non lo vede | Sorveglianza visiva, watchdog rosso eventualmente rileva |
| **No MITM HTTPS**: solo hostname visibile, niente body inspection | By design (vedi §1.4) |
| **DNS over HTTPS / VPN built-in browser**: studente puo' bypassare proxy se il browser usa DoH custom | Imporre browser senza DoH (es. Chrome enterprise policy) — fuori scope di Planck |
| **Watchdog = detection, non prevention**: il dot rosso ti dice "qualcosa non va" ma non blocca | Sorveglianza visiva |
| **Proxy soltanto sul PC**: dispositivi mobili in classe non sono coperti | Politica scuola: niente cellulari |
| **Performance non testata oltre 100 studenti / 10M entries** | Misurare e ottimizzare se serve |
| **Nessuna alta affidabilita'/replica**: singolo punto, se il PC docente crasha, fine sessione | Acceptable per il caso d'uso (1 lab = 1 verifica = niente HA) |

---

## 8. Roadmap a fasi

### 8.1 Strategia generale

Sviluppo **sequenziale** (singolo dev, in parallelo all'apprendimento Go): ogni fase ha un goal verificabile e un checkpoint.

**MVP cut line per v2.0**: Phase 0-4 + Phase 8 polish minimale = backend riscritto + Veyon base + UI multi-select.

**Deferred a v2.1+**: Auto-AI classification, Reazioni automatiche, Tab Storico cross-session. Sono feature di valore ma il core e' usabile senza.

**Stima totale per v2.0**: ~6-8 settimane part-time (compreso ramp-up Go). Stima per "tutto fino a v2.2 incluso": ~12-16 settimane.

⚠️ Le stime sono indicative e assumono lavoro continuo part-time. Tutto si dilata se intercede la vita.

---

### 8.2 Phase 0 — Prep e setup repo

**Goal**: ambiente di sviluppo pronto, scheletro Go funzionante.

**Tasks**:
- Tour of Go (~2-4 ore di studio iniziale)
- Lettura "Effective Go"
- Init Go module: `go mod init github.com/DoimoJr/planck-proxy`
- Struttura cartelle (vedi §3.8.1 della discussione precedente o ARCHITECTURE.md futuro)
- `go.mod` con dipendenze: `modernc.org/sqlite`, `golang.org/x/crypto/bcrypt`, niente altro per ora
- Build script (`build.sh` / `build.bat`) che produce `planck.exe`
- Embed.FS minimale: serve un `index.html` di placeholder
- Clone del sorgente Veyon in locale per consultazione: `git clone https://github.com/veyon/veyon.git`

**Checkpoint**: `go run cmd/planck/main.go` parte e mostra "Hello Planck v2" sulla console + serve una pagina HTML su `http://localhost:9999`.

**Effort**: ~1-2 giorni.

---

### 8.3 Phase 1 — Backend port + Monitor sempre attivo

**Goal**: Planck v2 con feature parity v1, **piu'** la separazione Monitor / Sessione (v2 design).

**Tasks**:
- Proxy HTTP forwarding (`net/http` reverse proxy)
- Proxy HTTPS CONNECT tunneling (`net.Dial` + `io.Copy` x2)
- Endpoint watchdog `/_alive` sul proxy server
- Web server REST API: porting di tutti gli endpoint in §5.2-5.10 (escluso Veyon, Storico, Auto-AI)
- SSE `/api/stream` con i message type di §5.14 (escluso `regola-fired`, `flag-sospetto`, `upstream-synced`)
- `domains.go`: porting di `DOMINI_AI` + `PATTERN_SISTEMA` + `classifica()`
- Persistenza file-based provvisoria: NDJSON per sessione corrente, JSON per archivio (come v1, niente SQLite ancora)
- Frontend porting: `public/` v2 con i moduli ES come v1, adattando le poche cose nuove (tab Storico placeholder, toolbar Veyon placeholder)
- **Monitor sempre attivo** (NEW): rimuovere `if (!sessioneAttiva) return` da `registraTraffico`, broadcast SSE sempre attivo
- Smoke test: avvio + proxy passthrough + UI funzionante con 1-2 client studenti

**Checkpoint**: tutti gli smoke test della v1 (in ARCHITECTURE.md `## Run / test`) passano sul nuovo binario Go. Monitor live funziona anche con sessione ferma.

**Effort**: ~1.5-2 settimane.

**Rischi**: la prima settimana e' anche "sto imparando Go". Mettere in conto rallentamenti, errori da neofita (nil pointer, goroutines confuse, mutex).

---

### 8.4 Phase 2 — Persistenza SQLite

**Goal**: tutto il dato persistente vive in `planck.db`. File legacy (config.json, ecc.) eliminati o solo come backup.

**Tasks**:
- Schema `planck.db` come da §4.2
- Migrations system (`schema_version` + `//go:embed migrations/*.sql`)
- Layer `internal/store` con CRUD per ogni tabella
- Migrazione one-shot dei dati v1 al primo boot v2 (vedi §4.4)
- Modifica endpoint API per leggere/scrivere su DB invece che file
- Test: avvio fresh + avvio con migrazione da fixtures v1

**Checkpoint**: una sessione completa (Avvia → traffico → Ferma) finisce in `entries` con tutti i campi compilati. Re-import di una sessione esportata funziona.

**Effort**: ~3-5 giorni.

**Rischi**: migrazione da v1 puo' avere edge case (file corrotti, encoding, formati legacy). Strategia: dare sempre la possibilita' di "Skip e riparti vuoto" come fallback.

---

### 8.5 Phase 3 — Veyon protocol foundation

**Goal**: distribuzione automatica di `proxy_on.bat` da Planck con un click. Tutto il resto (lock, screenshot, ecc.) ancora no.

**Tasks**:
- **QDataStream subset in Go** (~300 LoC): serializzazione tipi Qt che servono (QString, QByteArray, QVariantMap, qint32, ecc.). Reference: `qdatastream.h` Qt source
- Connection layer: TCP + TLS verso `veyon-server`
- Auth handshake KeyFile (vedi §5.16.2). Reference: `core/src/AuthenticationManager.cpp`
- Comando feature framework (invio messaggio + parse response)
- Implementazione **solo `RunProgram` + `FileTransfer`** in questa fase
- API endpoint: `/api/veyon/feature` + `/api/veyon/file-transfer` + `/api/veyon/test-connection` + `/api/veyon/status`
- Settings UI: tab Veyon con auto-import config + override + test connection
- Bottone toolbar "Distribuisci proxy_on.bat": esegue FileTransfer + RunProgram in coppia (push runtime, §3.8.3)

**Checkpoint**: dal pannello Planck, click "Distribuisci proxy_on" → tutti i PC studenti del lab ricevono il file e lo eseguono. Watchdog dot diventa verde su ogni studente entro 10s.

**Effort**: ~2-3 settimane. Settimana 1 buona parte va in QDataStream + auth (lavoro di studio del sorgente Veyon). Settimana 2-3: feature implementation + UI.

**Rischi**:
- Reverse engineering del framing Qt e' la parte piu' incerta. Avere sotto mano il sorgente Veyon e fare debugging con `wireshark` su una connessione reale (Veyon Master vs veyon-server) puo' aiutare a confermare il protocollo
- Auth keys: bisogna capire dove Veyon salva le chiavi su Windows (file system o registry?) e come farsele firmare

---

### 8.6 Phase 4 — Veyon features complete

**Goal**: tutte le feature MVP Veyon (§3.8.2) funzionanti dalla UI.

**Tasks**:
- Implementazione comandi: `PowerOn` (WoL), `PowerDown`, `Reboot`, `LogOff`, `ScreenLock`, `Screenshot` (tramite `ScreenshotManagement` o RFB transient)
- UI card studente: bottoni inline (🔒, 📷, ▶️ con dropdown programmi rapidi)
- UI toolbar globale "Azioni classe" (collapsible)
- UI selezione multipla (Ctrl/Shift+click + selection bar)
- UI right-click context menu
- API endpoint: `/api/veyon/screenshot/:ip`, `/api/veyon/programmi-rapidi/*`
- Settings UI: CRUD programmi rapidi
- Edge case handling: Veyon down → UI degraded ma usabile (vedi §3.8.10)

**Checkpoint**:
- Click sull'icona 🔒 sulla card di Mario → schermo di Mario si blocca entro 2-3s
- Selezione multipla di 3 studenti → click `📷` toolbar → arrivano 3 PNG in `screenshots/`
- "PowerOn all" wake-on-LAN risveglia tutti i PC del lab

**Effort**: ~1.5-2 settimane.

**Rischi**: Screenshot via VNC RFB e' quello piu' complesso se il fallback (b) e' necessario. Iniziare provando l'opzione (a) `ScreenshotManagement` direttamente.

---

### 8.7 Phase 8 — Polish + release v2.0 (MVP)

**Goal**: pronti per primo deploy reale.

**Tasks**:
- Visual polish (CSS finale, dark mode rifinito, transizioni)
- Empty state, loading state, error toast (vedi §6.8)
- Shortcut tastiera (§6.7) implementati
- README aggiornato per v2 (download release, setup, setup Veyon su PC studenti)
- ARCHITECTURE.md aggiornato (struttura Go, dove sta cosa)
- Build releases per Windows x64 (Linux best-effort)
- Smoke test dal vivo: una verifica reale in classe
- Bug fixing post-test
- Tag GitHub release `v2.0.0`

**Checkpoint**: una verifica reale conclusa con Planck v2, dati archiviati correttamente, nessun crash.

**Effort**: ~1 settimana (di cui meta' e' "smoke test dal vivo").

---

### 8.8 Phase 5 — Auto-classification AI (v2.1)

**Goal**: lista upstream auto-aggiornata + heuristic flagging.

**Tasks**:
- Repo path: `data/domini-ai.json` nel repo principale, snapshot iniziale dalla `DOMINI_AI` v1
- Backend: endpoint `/api/ai/upstream/sync`, `/api/ai/upstream/status`, `/api/ai/local/*`, `/api/ai/non-ai/*`
- Logica sync: fetch al boot + bottone manuale, cache in `upstream_cache`
- Heuristic regex composito (boundary-aware substring + TLD)
- UI badge 🤔 + modal "E' AI?" (§3.9.6)
- Settings UI: "I miei domini AI locali" + "Domini esclusi heuristic"

**Checkpoint**: aggiungo upstream un nuovo dominio → restart Planck → auto-classificato come AI senza mettere mano. Heuristic cattura un dominio nuovo simulato in lab.

**Effort**: ~5-7 giorni.

---

### 8.9 Phase 6 — Reazioni automatiche (v2.1)

**Goal**: auto-lock su detection AI funzionante.

**Tasks**:
- Trigger engine in backend: subscribe a eventi (`traffic`, `alive`, `inactivity_check_periodic`)
- Cooldown tracker (Map per `(regola_id, ip)`)
- Action dispatcher: invoca Veyon o emette eventi UI
- Tabella `regole_auto` + `eventi_auto`
- Preset hardcoded (3 toggle, §3.10.6) + UI in Impostazioni
- Rule engine custom: form + lista regole CRUD
- SSE `regola-fired` per notifica UI

**Checkpoint**: abilitato il preset "Lock automatico su rilevamento AI" → uno studente apre `chatgpt.com` (non bloccato) → lock dello studente entro 2s + audit `eventi_auto` registrato.

**Effort**: ~1-2 settimane.

**Rischi**: cooldown e race condition fra goroutines. Test mirati.

---

### 8.10 Phase 7 — Tab Storico (v2.2)

**Goal**: reportistica cross-session funzionante.

**Tasks**:
- API `/api/storico/student`, `/api/storico/compare`, `/api/storico/filter`
- Query SQLite ottimizzate (verifica indici §4.3.3 reggono il volume)
- UI tab Storico: due viste (per studente + confronto)
- Filtri composti (classe, lab, periodo, tipo evento)
- Export aggregato JSON
- Mini-trend chart compact (libreria leggera o vanilla SVG)

**Checkpoint**: cerco "Mario Rossi" → vedo le sue 7 sessioni con aggregati. Confronto due verifiche della stessa classe → vedo diff plausibile.

**Effort**: ~1-2 settimane.

---

### 8.11 v2.x — Deferred features

Da pianificare quando i casi d'uso reali emergono. Niente date fissate.

| Feature | Priorita' indicativa |
|---|---|
| **TextMessage** Veyon (popup studenti) | media |
| **RemoteAccess** Veyon (live VNC streaming schermo) | bassa |
| **DemoServer** Veyon (broadcast schermo docente) | bassa |
| **Auto-update Planck** (check release GitHub al boot) | bassa |
| **TLS** opzionale sul Web UI (cert self-signed o Let's Encrypt) | bassa |
| **Retention policy** automatica (auto-delete dopo N mesi) | media |
| **Anonimizzazione** sessioni vecchie (rimuovi nomi mantenendo aggregati) | media |
| **Centralized mode** (1 Planck per scuola, multi-lab) | bassa |
| **Modello dati: tabella classi/students normalizzata** (al posto degli snapshot JSON) | bassa |
| **i18n / multi-lingua** | bassa |
| **Mobile / tablet support** | "mai" |
| **Help overlay shortcut tastiera** (`?`) | bassa |
| **Per-classe auto-reazioni** (vs solo per-studente) | bassa |
| **Rule engine condizioni avanzate** (orario, contatori) | bassa |
| **Metrics endpoint Prometheus** | bassa |
| **Auto-backup schedulato** | bassa |
| **Detection bypass DNS-over-HTTPS** del browser studente | esplorativa |

### 8.12 Riepilogo cronologico stima

| Fase | Durata stima | Cumulativo |
|---|---|---|
| 0. Prep | 1-2 giorni | 1-2 g |
| 1. Backend port + Monitor v2 | 1.5-2 settimane | ~2-3 sett |
| 2. SQLite | 3-5 giorni | ~3-4 sett |
| 3. Veyon foundation | 2-3 settimane | ~5-7 sett |
| 4. Veyon features complete | 1.5-2 settimane | ~6.5-9 sett |
| 8. Polish + release v2.0 | 1 settimana | **~7.5-10 sett** |
| 5. Auto-AI (v2.1) | 5-7 giorni | ~9-11 sett |
| 6. Reazioni auto (v2.1) | 1-2 settimane | ~10-13 sett |
| 7. Storico (v2.2) | 1-2 settimane | ~12-15 sett |

> Stime in **calendar weeks** part-time. Lavoro intenso a tempo pieno: dimezzabile.

---

*Fine specifica.*

# Planck Proxy

Toolkit leggero per la **vigilanza durante le verifiche in aula di informatica**. Il PC del docente fa da proxy HTTP/HTTPS per tutti i PC degli studenti: una dashboard web mostra in tempo reale chi visita cosa, segnala eventuali domini AI, permette di bloccare siti al volo e archivia le sessioni.

**Pensato per rete LAN di fiducia**, non è una soluzione di sicurezza enterprise: rileva e disincentiva, non è impossibile da aggirare (vedi [Limiti strutturali](#limiti-strutturali)).

## Caratteristiche

- **Proxy HTTP + HTTPS** su singola porta (CONNECT tunneling, nessun MITM)
- **Dashboard web** a tre tab (Live / Report / Impostazioni) con aggiornamento realtime via SSE
- **Classificazione traffico** in tre categorie: AI / Utente / Sistema (~180 pattern di rumore esclusi dai conteggi per studente)
- **Blocklist o allowlist** con toggle globale di pausa
- **Watchdog keepalive**: ogni PC studente segnala la propria presenza ogni 5s → dot colorato nella dashboard (verde/giallo/rosso/grigio)
- **Gestione sessioni**: Avvia / Ferma espliciti, archivio automatico, esport JSON
- **Mappa studenti** IP→nome editabile dalla UI, con combinazioni classe+laboratorio swappabili
- **Countdown** con scadenza programmabile
- **Vista a griglia o lista**, colonne collassabili, tema chiaro/scuro
- **Tutto italiano**, tutto portable: niente `npm install`, niente build step

## Setup

### 1. Scarica Node.js

Il progetto usa il runtime `node.exe` **bundled** nella cartella, non tracciato su git (troppo grande). Scarica la versione Windows x64:

➡️ https://nodejs.org/en/download

Scegli **"Windows Installer (.msi)"** oppure **"Windows Binary (.zip)"**. Dallo zip estrai `node.exe` e **copialo nella radice del progetto** (accanto a `server.js`).

In alternativa, se Node.js è già installato globalmente sul PC docente, sostituisci `avvia.bat` con `node server.js` oppure modifica il `.bat` per invocare il `node` del PATH.

Versione consigliata: Node 20+ (LTS).

### 2. Avvia il monitor

Doppio click su `avvia.bat`.

Alla prima esecuzione vedrai nella console:
```
Proxy:     http://<tuo-ip>:9090  (modo: blocklist)
Monitor:   http://<tuo-ip>:9999
```

Apri nel browser `http://localhost:9999` (oppure `http://<tuo-ip>:9999` da un altro PC in LAN).

### 3. Configura i PC studenti

Apri `proxy_on.bat` con un editor di testo e imposta l'IP del PC docente:
```batch
set IP_PROF=192.168.1.100   REM <-- metti qui il tuo IP
set PORTA=9090
```

Distribuisci `proxy_on.bat` sui PC studenti tramite [Veyon](https://veyon.io/) (o software equivalente) e lancialo con un doppio click. Configura automaticamente il proxy di Windows + avvia il watchdog keepalive.

A fine verifica distribuisci `proxy_off.bat` per ripristinare.

## Uso tipico

1. Sul PC docente: `avvia.bat` → apri `http://localhost:9999`
2. Nella tab **Impostazioni** carica la mappa della classe (oppure aggiungila a mano)
3. Nella tab **Live** premi **Avvia sessione** per iniziare la registrazione
4. (Opzionale) imposta una **Fine** (HH:MM) per il countdown
5. Distribuisci `proxy_on.bat` agli studenti tramite Veyon
6. Osserva la griglia: dot verdi = connessi, nome studente, domini contattati, richieste
7. Se un dominio AI viene contattato, banner rosso in alto + suono (se abilitato)
8. Premi **Ferma sessione** a fine verifica → i dati vengono archiviati automaticamente in `sessioni/`
9. Nella tab **Report** vedi i top domini, le statistiche per studente, ecc. Esporta JSON se serve

## Struttura

```
planck-proxy/
├── server.js               Processo unico: proxy + web server + API + SSE
├── domains.js              Classificatore: DOMINI_AI (chatbot, assistenti) + PATTERN_SISTEMA (rumore)
├── config.json             Porte, auth, titolo, modo, domini ignorati
├── studenti.json           Mappa IP→nome
├── avvia.bat               Launcher per Windows
├── blocked.html            Pagina 403 servita agli studenti bloccati
├── proxy_on.bat            Script studente: imposta proxy + watchdog (distribuire via Veyon)
├── proxy_off.bat           Script studente: ripristina
├── public/
│   ├── index.html          Dashboard 3 tab (Live / Report / Impostazioni)
│   ├── monitor.css
│   └── js/                 ES modules (app / state / render / actions / sse / util)
├── presets/                Snapshot di blocklist salvate (base.json, programmazione.json)
├── classi/                 Mappe classe+laboratorio salvate (escluse da git: dati sensibili)
└── sessioni/               Archivio automatico sessioni (escluse da git)
```

Il file `CLAUDE.md` contiene la documentazione architetturale dettagliata (per contributori e per [Claude Code](https://claude.com/claude-code)).

## Modi: blocklist / allowlist / pausa

- **Blocklist** (default): tutto passa, tranne i domini nella lista bloccati
- **Allowlist**: niente passa, tranne i domini nella lista bloccati (il nome è invertito, è uno switch di modo) e quelli in `dominiIgnorati`
- **Pausa globale**: blocca tutto, tranne `dominiIgnorati`

Modificabili dalla UI (tab Impostazioni).

## Limiti strutturali

- **Hotspot bypass**: se uno studente si collega al suo hotspot mobile, esce dalla LAN e bypassa il proxy. Il watchdog rileva la disconnessione (dot rosso dopo 60s), ma non previene l'uso. La sorveglianza visiva del docente resta necessaria.
- **Auth HTTP Basic**: non TLS. Pensato per LAN di fiducia. Disabilitato di default.
- **Archivio salvato allo Stop e allo shutdown grazioso**. `kill -9` o power loss perdono il buffer in RAM.
- **No tests, no CI**: validare con [smoke tests in CLAUDE.md](./CLAUDE.md) e con test manuale nel browser.

## Licenza

[MIT](./LICENSE) — 2026 Francesco Doimo

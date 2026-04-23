/**
 * @file Server unificato: proxy HTTP/HTTPS + web UI + API REST + SSE.
 *
 * Architettura:
 *   - Unico processo Node, due HTTP server (proxy su `config.proxy.port`
 *     e web su `config.web.port`), entrambi su `0.0.0.0`.
 *   - Stato condiviso in RAM (`storia`, `bloccati`, `aliveMap`, ...).
 *   - Persistenza su file:
 *       - `_blocked_domains.txt` (blocklist, una riga per dominio)
 *       - `_traffico_log.txt` (audit append-only, non riletto)
 *       - `config.json` (ri-salvato da `/api/settings/update`)
 *       - `studenti.json` (mappa IP -> nome)
 *       - `presets/*.json` (snapshot blocklist)
 *       - `sessioni/*.json` (archivio sessioni concluse)
 *       - `classi/<classe>--<lab>.json` (mappe studenti combo classe+lab)
 *
 * Avvio: `node server.js` (o `avvia.bat` su Windows). Le porte sono lette
 * una sola volta al boot: modificarle dalla UI richiede restart manuale.
 * Auth HTTP Basic e `dominiIgnorati` sono letti runtime da `config.*`.
 */

const net = require('net');
const http = require('http');
const fs = require('fs');
const path = require('path');
const os = require('os');

const { DOMINI_AI, PATTERN_SISTEMA, classifica } = require('./domains');

// --- PATH ---
const CONFIG_PATH = path.join(__dirname, 'config.json');
const STUDENTI_PATH = path.join(__dirname, 'studenti.json');
const BLOCKED_HTML_PATH = path.join(__dirname, 'blocked.html');
const LOG_FILE = path.join(__dirname, '_traffico_log.txt');
const BLOCKED_FILE = path.join(__dirname, '_blocked_domains.txt');
const PUBLIC_DIR = path.join(__dirname, 'public');
const PRESETS_DIR = path.join(__dirname, 'presets');
const SESSIONI_DIR = path.join(__dirname, 'sessioni');
const CLASSI_DIR = path.join(__dirname, 'classi');

// --- CONFIG ---
const config = JSON.parse(fs.readFileSync(CONFIG_PATH, 'utf8'));
const PORTA_PROXY = config.proxy.port;     // usato solo al listen (richiede restart)
const PORTA_WEB = config.web.port;          // idem
// auth e dominiIgnorati sono letti dinamicamente da `config` (modificabili a caldo)

/** Scrive `config` su disco. Silenzia errori (ENOSPC, permessi). */
function salvaConfigFile() {
    try { fs.writeFileSync(CONFIG_PATH, JSON.stringify(config, null, 2)); } catch {}
}

/**
 * Restituisce una copia di `config` con la password rimossa e sostituita
 * da `passwordSet: boolean`. Usata per tutte le risposte a `/api/settings`
 * e per i broadcast SSE `settings` — la password reale non esce mai dal server.
 * @param {Object} c
 * @returns {Object}
 */
function sanitizeConfig(c) {
    const out = JSON.parse(JSON.stringify(c));
    if (out.web?.auth) {
        out.web.auth.passwordSet = !!out.web.auth.password;
        out.web.auth.password = '';
    }
    return out;
}

/**
 * Legge un valore da un oggetto via path dotted (es. "web.auth.enabled").
 * @param {Object} obj
 * @param {string} path
 */
function getDeep(obj, path) {
    return path.split('.').reduce((o, k) => (o == null ? o : o[k]), obj);
}

/**
 * Scrive un valore in un oggetto via path dotted, creando oggetti
 * intermedi mancanti. Mutazione in-place.
 * @param {Object} obj
 * @param {string} path
 * @param {*} value
 */
function setDeep(obj, path, value) {
    const parts = path.split('.');
    let cur = obj;
    for (let i = 0; i < parts.length - 1; i++) {
        if (typeof cur[parts[i]] !== 'object' || cur[parts[i]] === null) cur[parts[i]] = {};
        cur = cur[parts[i]];
    }
    cur[parts[parts.length - 1]] = value;
}

/**
 * Mappa key-dotted -> validator. Ogni valore in ingresso a
 * `/api/settings/update` deve passare il predicato corrispondente,
 * altrimenti finisce in `rejected`.
 */
const SETTINGS_VALIDATORI = {
    'titolo': v => typeof v === 'string' && v.length <= 200,
    'classe': v => typeof v === 'string' && v.length <= 100,
    'modo': v => v === 'blocklist' || v === 'allowlist',
    'inattivitaSogliaSec': v => typeof v === 'number' && v >= 10 && v <= 3600,
    'proxy.port': v => Number.isInteger(v) && v >= 1024 && v <= 65535,
    'web.port': v => Number.isInteger(v) && v >= 1024 && v <= 65535,
    'web.auth.enabled': v => typeof v === 'boolean',
    'web.auth.user': v => typeof v === 'string' && v.length > 0 && v.length < 100,
    'web.auth.password': v => typeof v === 'string' && v.length > 0 && v.length < 200,
    'dominiIgnorati': v => Array.isArray(v) && v.every(s => typeof s === 'string'),
};
/**
 * Chiavi che vengono scritte su disco ma NON applicate a caldo.
 * Il client latch-a `riavvioRichiesto` e mostra un banner persistente.
 */
const SETTINGS_RESTART = new Set(['proxy.port', 'web.port', 'web.auth.enabled', 'web.auth.user', 'web.auth.password']);

/**
 * Applica un set di coppie key-dotted/value a `config`, valida per chiave,
 * persiste su disco se almeno una e' stata accettata.
 *
 * @param {Object<string,*>} body
 * @returns {{updated: string[], rejected: string[], richiedeRiavvio: string[]}}
 */
function applicaSettings(body) {
    const updated = [], rejected = [], richiedeRiavvio = [];
    for (const [key, value] of Object.entries(body || {})) {
        const valid = SETTINGS_VALIDATORI[key];
        if (!valid || !valid(value)) { rejected.push(key); continue; }
        setDeep(config, key, value);
        updated.push(key);
        if (SETTINGS_RESTART.has(key)) richiedeRiavvio.push(key);
    }
    if (updated.length > 0) salvaConfigFile();
    return { updated, rejected, richiedeRiavvio };
}

/**
 * Legge un body HTTP come JSON, con guard anti-flood a 1 MB.
 * Ritorna `{}` su errore di parse o payload malformato (mai rigetta).
 * @param {import('http').IncomingMessage} req
 * @returns {Promise<Object>}
 */
function leggiBody(req) {
    return new Promise((resolve) => {
        let data = '';
        req.on('data', chunk => { data += chunk; if (data.length > 1e6) { data = ''; req.destroy(); } });
        req.on('end', () => { try { resolve(JSON.parse(data || '{}')); } catch { resolve({}); } });
        req.on('error', () => resolve({}));
    });
}

if (!fs.existsSync(SESSIONI_DIR)) fs.mkdirSync(SESSIONI_DIR);
if (!fs.existsSync(CLASSI_DIR)) fs.mkdirSync(CLASSI_DIR);

// ============================================================
// STUDENTI: mappa IP -> nome persistita in `studenti.json`
// ============================================================
/** @type {Object<string,string>} */
let studenti = {};

/**
 * (Ri)carica la mappa studenti da `studenti.json`. Le chiavi con prefisso
 * `_` sono ignorate: convenzione per inserire "commenti" nel JSON (che per
 * sua natura non supporta commenti nativi). Ogni mutazione via API rimuove
 * comunque questi commenti riscrivendo il file.
 */
function caricaStudenti() {
    try {
        const raw = JSON.parse(fs.readFileSync(STUDENTI_PATH, 'utf8'));
        studenti = {};
        for (const [k, v] of Object.entries(raw)) {
            if (!k.startsWith('_')) studenti[k] = v;
        }
    } catch { studenti = {}; }
}

/** Scrive la mappa corrente su `studenti.json`. Silenzia errori di I/O. */
function salvaStudentiFile() {
    try { fs.writeFileSync(STUDENTI_PATH, JSON.stringify(studenti, null, 2)); } catch {}
}
caricaStudenti();

// ============================================================
// CLASSI: mappe studenti salvate per coppia (classe, lab).
// File su disco: `classi/<classe>--<lab>.json` con contenuto
// `{classe, lab, mappa}`. I segmenti sono sanitizzati a [a-zA-Z0-9_-].
// ============================================================

/**
 * Sanitizza un segmento di nome classe/lab, tenendo solo alfanumerici,
 * underscore e trattino. Restituisce null se il risultato e' vuoto.
 */
function sanSegmento(s) {
    const safe = (s || '').replace(/[^a-zA-Z0-9_-]/g, '');
    return safe || null;
}

/**
 * Costruisce il nome file per una combinazione (classe, lab).
 * @returns {string|null} Nome file (es. `4dii--lab1.json`) o null se segmenti non validi.
 */
function fileClasse(classe, lab) {
    const sc = sanSegmento(classe);
    const sl = sanSegmento(lab);
    if (!sc || !sl) return null;
    return sc + '--' + sl + '.json';
}

/** Inverso di `fileClasse`: estrae `{classe, lab}` dal filename. */
function parseFileClasse(filename) {
    const stem = filename.replace(/\.json$/, '');
    const idx = stem.indexOf('--');
    if (idx < 0) return null;
    return { classe: stem.slice(0, idx), lab: stem.slice(idx + 2) };
}

/**
 * Elenca tutte le combinazioni salvate in `classi/`.
 * @returns {Array<{classe:string, lab:string, file:string}>}
 */
function listaClassi() {
    try {
        return fs.readdirSync(CLASSI_DIR)
            .filter(f => f.endsWith('.json'))
            .map(f => {
                const parsed = parseFileClasse(f);
                return parsed ? { classe: parsed.classe, lab: parsed.lab, file: f } : null;
            })
            .filter(Boolean)
            .sort((a, b) => a.classe.localeCompare(b.classe) || a.lab.localeCompare(b.lab));
    } catch { return []; }
}

/**
 * Legge una combo da disco.
 * @returns {{classe:string, lab:string, mappa:Object<string,string>}|null}
 */
function leggiClasse(classe, lab) {
    const file = fileClasse(classe, lab);
    if (!file) return null;
    try { return JSON.parse(fs.readFileSync(path.join(CLASSI_DIR, file), 'utf8')); }
    catch { return null; }
}
/**
 * Salva una combo su disco. @returns {boolean} true in caso di successo.
 * @param {string} classe
 * @param {string} lab
 * @param {Object<string,string>} mappa - IP -> nome.
 */
function salvaClasse(classe, lab, mappa) {
    const file = fileClasse(classe, lab);
    if (!file) return false;
    const contenuto = { classe, lab, mappa };
    try { fs.writeFileSync(path.join(CLASSI_DIR, file), JSON.stringify(contenuto, null, 2)); return true; }
    catch { return false; }
}

/** Elimina il file combo corrispondente. @returns {boolean} */
function eliminaClasse(classe, lab) {
    const file = fileClasse(classe, lab);
    if (!file) return false;
    try { fs.unlinkSync(path.join(CLASSI_DIR, file)); return true; }
    catch { return false; }
}

// ============================================================
// PAGINA BLOCCATA: HTML servito agli studenti su domini bloccati.
// Personalizzabile editando `blocked.html`; fallback inline se mancante.
// ============================================================
let paginaBloccata;
try { paginaBloccata = fs.readFileSync(BLOCKED_HTML_PATH, 'utf8'); }
catch {
    paginaBloccata = '<html><body style="font-family:sans-serif;text-align:center;padding:80px">'
        + '<h1 style="color:#e74c3c">Accesso bloccato dal docente</h1></body></html>';
}

/**
 * Restituisce il primo IPv4 non-loopback delle interfacce di rete locali.
 * Usato solo per stampare a console un URL comodo all'avvio ("Apri il
 * browser su http://<ipLocale>:9999"). Se niente e' disponibile, fallback
 * a 127.0.0.1.
 */
function ipLocale() {
    const interfaces = os.networkInterfaces();
    for (const name of Object.keys(interfaces)) {
        for (const iface of interfaces[name]) {
            if (iface.family === 'IPv4' && !iface.internal) return iface.address;
        }
    }
    return '127.0.0.1';
}
const IP_SERVER = ipLocale();

// ============================================================
// STATO IN MEMORIA
// ============================================================

/** Capienza del ring buffer traffico. Oltre, le entry piu' vecchie vengono scartate. */
const MAX_STORIA = 5000;

/** @type {Array<{ora:string,ip:string,metodo:string,dominio:string,tipo:string,blocked:boolean}>} */
const storia = [];

/** @type {Set<string>} Domini in blocklist (persistiti in `_blocked_domains.txt`). */
let bloccati = new Set();

/** @type {Set<import('http').ServerResponse>} Response degli EventSource attualmente connessi. */
const sseClients = new Set();

// Lifecycle sessione esplicito: al boot la sessione NON e' attiva.
// Il proxy instrada e blocca comunque, ma niente viene registrato finche'
// l'utente non preme "Avvia sessione".
let sessioneAttiva = false;
/** @type {string|null} ISO timestamp dell'inizio della sessione corrente. */
let sessioneInizio = null;
/** @type {string|null} ISO timestamp del "Ferma" — usato per congelare la durata in UI. */
let sessioneFineISO = null;
let pausato = false;
/** @type {string|null} ISO della scadenza programmata. */
let deadlineISO = null;
/** @type {NodeJS.Timeout|null} Handle del setTimeout che notifica lo scadere. */
let deadlineTimer = null;
/** @type {Map<string, number>} IP -> ms dell'ultimo ping watchdog ricevuto. */
const aliveMap = new Map();

/**
 * Registra un ping keepalive dallo script `proxy_on.bat` di uno studente.
 * Aggiorna `aliveMap` e broadcastta ai client `{type:'alive', ip, ts}` per
 * far colorare il dot in UI.
 */
function registraAlive(ip) {
    const ts = Date.now();
    aliveMap.set(ip, ts);
    broadcast({ type: 'alive', ip, ts });
}

// ============================================================
// BLOCKLIST: persistita su `_blocked_domains.txt`, una riga per dominio.
// ============================================================

/** Carica la blocklist da disco. Se il file non esiste, parte vuota. */
function caricaLista() {
    try {
        const txt = fs.readFileSync(BLOCKED_FILE, 'utf8');
        bloccati = new Set(txt.split('\n').map(s => s.trim()).filter(Boolean));
    } catch { bloccati = new Set(); }
}

/** Scrive la blocklist corrente su disco. */
function salvaLista() {
    fs.writeFileSync(BLOCKED_FILE, [...bloccati].join('\n') + '\n');
}
caricaLista();

/**
 * True se il dominio matcha un elemento di `config.dominiIgnorati` (per
 * sostringa). I domini ignorati vengono droppati pre-log e passano
 * sempre dal proxy, anche in pausa/allowlist.
 */
function isIgnorato(dominio) {
    const d = dominio.toLowerCase();
    const lista = config.dominiIgnorati || [];
    return lista.some(di => d.includes(di.toLowerCase()));
}

/**
 * Decide se il proxy deve rispondere 403 per il dominio richiesto.
 * Combina tre check nell'ordine:
 *   1. Pausa globale: blocca tutto tranne `dominiIgnorati`.
 *   2. Allowlist mode: blocca tutto tranne i domini in `bloccati` (lista
 *      usata "al contrario") e quelli ignorati.
 *   3. Blocklist mode (default): blocca solo se matcha `bloccati`.
 *
 * @param {string} dominio
 * @returns {boolean}
 */
function dominioBloccato(dominio) {
    const d = dominio.toLowerCase();
    const ignorato = isIgnorato(dominio);

    if (pausato && !ignorato) return true;

    const matchInLista = [...bloccati].some(bl => d.includes(bl.toLowerCase()));
    if (config.modo === 'allowlist') {
        if (ignorato) return false;
        return !matchInLista;
    }
    return matchInLista;
}

// ============================================================
// TRAFFICO: log + ring buffer + broadcast SSE
// ============================================================

/**
 * Registra una richiesta proxata: append al log file + stampa a console
 * sempre; ring buffer e broadcast SSE solo a sessione attiva (il monitor
 * UI resta "pulito" finche' non si avvia una sessione). Domini in
 * `dominiIgnorati` vengono loggati ma NON entrano nel buffer / SSE.
 *
 * @param {string} ip - IP client del PC studente.
 * @param {string} metodo - "GET"/"POST"/... per HTTP, "HTTPS" per CONNECT.
 * @param {string} urlStr - URL completa (HTTP) o hostname (HTTPS).
 * @param {boolean} blocked - True se la richiesta e' stata respinta con 403.
 */
function registraTraffico(ip, metodo, urlStr, blocked) {
    const ora = new Date().toISOString().replace('T', ' ').substring(0, 19);
    const rigaLog = `${ora} | ${ip} | ${metodo} | ${urlStr}${blocked ? ' | BLOCKED' : ''}\n`;
    fs.appendFile(LOG_FILE, rigaLog, () => {});

    let dominio;
    try { dominio = new URL(urlStr).hostname; } catch { dominio = urlStr; }

    const oraCorta = ora.substring(11);

    if (isIgnorato(dominio)) {
        console.log(`${oraCorta} [${ip}] ${metodo} ${dominio}${blocked ? ' [BLOCKED]' : ''} (ignorato)`);
        return;
    }

    console.log(`${oraCorta} [${ip}] ${metodo} ${dominio}${blocked ? ' [BLOCKED]' : ''}`);

    if (!sessioneAttiva) return;

    const tipo = classifica(dominio);
    const entry = { ora, ip, metodo, dominio, tipo, blocked };

    storia.push(entry);
    if (storia.length > MAX_STORIA) storia.shift();

    broadcast({ type: 'traffic', entry });
}

/**
 * Invia un payload JSON a tutti gli EventSource connessi (formato SSE).
 * Errori di write silenziati (client disconnesso a meta' flush).
 * @param {Object} payload
 */
function broadcast(payload) {
    const data = `data: ${JSON.stringify(payload)}\n\n`;
    for (const res of sseClients) {
        try { res.write(data); } catch {}
    }
}

/** Risponde 403 con la pagina HTML di blocco. */
function rispondiBloccato(res) {
    res.writeHead(403, {
        'Content-Type': 'text/html; charset=UTF-8',
        'Content-Length': Buffer.byteLength(paginaBloccata),
        'Connection': 'close'
    });
    res.end(paginaBloccata);
}

/**
 * Estrae l'IPv4 dal socket, rimuovendo il prefisso IPv6-mapped `::ffff:`
 * che Node aggiunge quando il socket e' dual-stack.
 */
function getClientIp(socket) {
    return socket.remoteAddress?.replace('::ffff:', '') || 'unknown';
}

// ============================================================
// DEADLINE: conto alla rovescia programmato
// ============================================================

/**
 * Imposta (o annulla con `null`) la scadenza. Cancella l'eventuale timer
 * precedente. Schedula un `setTimeout` che broadcastta
 * `{type: 'deadline-reached'}` allo scadere. Broadcast immediato del nuovo
 * valore `{type: 'deadline'}` per sincronizzare i client.
 * Nota: la deadline e' in-memory, si perde al restart.
 * @param {string|null} iso
 */
function impostaDeadline(iso) {
    clearTimeout(deadlineTimer);
    deadlineISO = iso;
    if (iso) {
        const ms = new Date(iso).getTime() - Date.now();
        if (ms > 0) {
            deadlineTimer = setTimeout(() => {
                broadcast({ type: 'deadline-reached', deadlineISO });
            }, ms);
        }
    }
    broadcast({ type: 'deadline', deadlineISO });
}

// ============================================================
// ARCHIVIO SESSIONI
// ============================================================

/**
 * Serializza il buffer corrente in `sessioni/<sessioneInizio>.json`.
 * Chiamata da: `/api/session/stop`, `/api/session/start` (caso difensivo),
 * `/api/sessioni/archivia`, graceful shutdown.
 *
 * Idempotente: chiamate ripetute scrivono lo stesso filename, solo
 * `esportatoAlle` cambia. No-op se il buffer e' vuoto o se la sessione
 * non e' mai stata avviata.
 *
 * @returns {string|null} Nome del file archiviato o null.
 */
function archiviaSessioneCorrente() {
    if (storia.length === 0 || !sessioneInizio) return null;
    const data = {
        sessioneInizio,
        esportatoAlle: new Date().toISOString(),
        titolo: config.titolo,
        classe: config.classe,
        modo: config.modo,
        studenti,
        bloccati: [...bloccati],
        entries: storia.slice(),
    };
    const nome = sessioneInizio.replace(/[:T.]/g, '-').substring(0, 19) + '.json';
    const dest = path.join(SESSIONI_DIR, nome);
    try { fs.writeFileSync(dest, JSON.stringify(data, null, 2)); return nome; }
    catch { return null; }
}

/** Invia ai client SSE `{type:'session-state', ...}` con lo stato corrente. */
function broadcastStatoSessione() {
    broadcast({
        type: 'session-state',
        sessioneAttiva,
        sessioneInizio,
        sessioneFineISO,
    });
}

/**
 * Elenca i file archivio in ordine cronologico inverso (piu' recenti prima).
 * I nomi file sono timestamp-sorted lessicograficamente grazie al formato
 * ISO-like (YYYY-MM-DD-HH-MM-SS.json).
 */
function listaSessioni() {
    try {
        return fs.readdirSync(SESSIONI_DIR)
            .filter(f => f.endsWith('.json'))
            .sort().reverse();
    } catch { return []; }
}

/**
 * Sanitizza un nome file archivio evitando path traversal (`../`).
 * Accetta solo `[a-zA-Z0-9_.-]` e il suffisso `.json` obbligatorio.
 * @returns {string|null} Nome safe oppure null se invalido.
 */
function nomeSessioneSafe(nome) {
    const safe = (nome || '').replace(/[^a-zA-Z0-9_.\-]/g, '');
    return (safe && safe.endsWith('.json')) ? safe : null;
}

// ============================================================
// PROXY HTTP/HTTPS
// ============================================================

/**
 * Proxy HTTP: inoltra le richieste non-CONNECT verso l'origin server.
 *
 * Percorso richiesta normale:
 *   1. Check keepalive `/_alive` (risposta diretta, non proxata).
 *   2. Parse URL (con fallback su `Host` header per client che mandano solo path).
 *   3. Check `dominioBloccato()` -> 403 + log.
 *   4. Forward con `http.request` (timeout 15s), stream bidirezionale.
 *
 * Il server NON fa MITM per HTTPS: vedi `.on('connect')` sotto per il
 * tunneling CONNECT. Il blocco HTTPS e' basato solo sull'hostname target.
 */
const proxyServer = http.createServer((req, res) => {
    const ipClient = getClientIp(req.socket);
    const targetUrl = req.url;

    if (targetUrl === '/_alive' || targetUrl.startsWith('/_alive?')) {
        registraAlive(ipClient);
        res.writeHead(200, {
            'Content-Type': 'text/plain',
            'Content-Length': 2,
            'Connection': 'close',
            'Cache-Control': 'no-store',
        });
        res.end('ok');
        return;
    }

    let parsed;
    try { parsed = new URL(targetUrl); }
    catch {
        const host = req.headers.host;
        if (host) {
            try { parsed = new URL(`http://${host}${targetUrl}`); }
            catch { res.writeHead(400); res.end('URL non valido'); return; }
        } else { res.writeHead(400); res.end('URL non valido'); return; }
    }

    const dominio = parsed.hostname;

    if (dominioBloccato(dominio)) {
        registraTraffico(ipClient, req.method, targetUrl, true);
        rispondiBloccato(res);
        return;
    }

    registraTraffico(ipClient, req.method, targetUrl, false);

    const porta = parseInt(parsed.port) || 80;
    const percorso = parsed.pathname + (parsed.search || '');
    const headers = {};
    for (const [key, val] of Object.entries(req.headers)) {
        if (!key.startsWith('proxy-')) headers[key] = val;
    }
    headers['connection'] = 'close';

    const proxyReq = http.request({
        hostname: dominio, port: porta, path: percorso,
        method: req.method, headers, timeout: 15000
    }, (proxyRes) => {
        res.writeHead(proxyRes.statusCode, proxyRes.headers);
        proxyRes.pipe(res);
    });

    proxyReq.on('error', (err) => {
        if (!res.headersSent) {
            res.writeHead(502);
            res.end(`Impossibile raggiungere ${dominio}: ${err.message}`);
        }
    });
    proxyReq.setTimeout(15000, () => proxyReq.destroy());
    req.pipe(proxyReq);
});

/**
 * Proxy HTTPS tramite tunneling CONNECT. Il browser apre un tunnel TCP
 * attraverso il proxy; noi vediamo solo `CONNECT hostname:443` e il blocco
 * viene deciso esclusivamente sulla base dell'hostname (niente MITM,
 * niente certificati finti). Dopo il 200 Connection Established, i byte
 * sono TLS opachi.
 */
proxyServer.on('connect', (req, clientSocket, head) => {
    const ipClient = getClientIp(clientSocket);
    const [host, portStr] = req.url.split(':');
    const port = parseInt(portStr) || 443;

    if (dominioBloccato(host)) {
        registraTraffico(ipClient, 'HTTPS', host, true);
        clientSocket.write(
            'HTTP/1.1 403 Forbidden\r\n'
            + 'Content-Type: text/html; charset=UTF-8\r\n'
            + `Content-Length: ${Buffer.byteLength(paginaBloccata)}\r\n`
            + 'Connection: close\r\n\r\n'
            + paginaBloccata
        );
        clientSocket.end();
        return;
    }

    registraTraffico(ipClient, 'HTTPS', host, false);

    const targetSocket = net.connect(port, host, () => {
        clientSocket.write('HTTP/1.1 200 Connection Established\r\n\r\n');
        if (head && head.length > 0) targetSocket.write(head);
        targetSocket.pipe(clientSocket);
        clientSocket.pipe(targetSocket);
    });

    targetSocket.on('error', () => clientSocket.end());
    clientSocket.on('error', () => targetSocket.destroy());
    targetSocket.setTimeout(30000, () => targetSocket.destroy());
    clientSocket.setTimeout(30000, () => clientSocket.destroy());
});

proxyServer.on('error', (err) => console.error(`Errore proxy: ${err.message}`));
proxyServer.listen(PORTA_PROXY, '0.0.0.0');

// ============================================================
// WEB SERVER
// ============================================================

/** Content-Type per estensione. I file senza match prendono `application/octet-stream`. */
const MIME = {
    '.html': 'text/html; charset=UTF-8',
    '.css': 'text/css; charset=UTF-8',
    '.js': 'application/javascript; charset=UTF-8',
    '.json': 'application/json; charset=UTF-8',
};

/**
 * Serve un file statico da `public/`. Blocca path traversal (`..`) con un
 * check rudimentale (adeguato perche' non accettiamo upload o path
 * parametrici). Fallback 404 se il file non esiste.
 */
function serveStatic(req, res) {
    let rel = req.url === '/' ? '/index.html' : req.url.split('?')[0];
    if (rel.includes('..')) { res.writeHead(400); res.end(); return; }
    const filePath = path.join(PUBLIC_DIR, rel);
    fs.readFile(filePath, (err, data) => {
        if (err) { res.writeHead(404); res.end('Not found'); return; }
        const ext = path.extname(filePath).toLowerCase();
        res.writeHead(200, { 'Content-Type': MIME[ext] || 'application/octet-stream' });
        res.end(data);
    });
}

/**
 * Helper per rispondere con JSON + status.
 * @param {import('http').ServerResponse} res
 * @param {Object} obj
 * @param {number} [status=200]
 */
function jsonReply(res, obj, status = 200) {
    const body = JSON.stringify(obj);
    res.writeHead(status, { 'Content-Type': 'application/json; charset=UTF-8' });
    res.end(body);
}

/**
 * Guard HTTP Basic: se `config.web.auth.enabled`, richiede credenziali.
 * Letto ogni request (modifiche via UI applicate a caldo). Restituisce
 * false se ha gia' scritto la response 401 (il caller deve semplicemente
 * fare `return`).
 * @returns {boolean} true se la richiesta puo' procedere.
 */
function controllaAuth(req, res) {
    const auth = config.web?.auth || { enabled: false };
    if (!auth.enabled) return true;
    const h = req.headers.authorization || '';
    if (!h.startsWith('Basic ')) {
        res.writeHead(401, { 'WWW-Authenticate': 'Basic realm="Monitor"' });
        res.end('Autenticazione richiesta');
        return false;
    }
    const decoded = Buffer.from(h.slice(6), 'base64').toString();
    const idx = decoded.indexOf(':');
    const user = decoded.slice(0, idx);
    const pass = decoded.slice(idx + 1);
    if (user !== auth.user || pass !== auth.password) {
        res.writeHead(401, { 'WWW-Authenticate': 'Basic realm="Monitor"' });
        res.end('Credenziali errate');
        return false;
    }
    return true;
}

/** Restituisce i nomi dei preset disponibili (senza suffisso `.json`). */
function listaPresets() {
    try {
        return fs.readdirSync(PRESETS_DIR)
            .filter(f => f.endsWith('.json'))
            .map(f => f.replace(/\.json$/, ''));
    } catch { return []; }
}

/**
 * Legge un preset da disco. Sanitizza il nome a `[a-zA-Z0-9_-]`.
 * @returns {{nome:string,descrizione:string,domini:string[]}|null}
 */
function leggiPreset(nome) {
    const safe = (nome || '').replace(/[^a-zA-Z0-9_-]/g, '');
    if (!safe) return null;
    try { return JSON.parse(fs.readFileSync(path.join(PRESETS_DIR, safe + '.json'), 'utf8')); }
    catch { return null; }
}

/**
 * Salva un preset (overwrites silenziosamente se esiste).
 * @param {string} nome
 * @param {string[]} domini
 * @returns {boolean}
 */
function salvaPreset(nome, domini) {
    const safe = (nome || '').replace(/[^a-zA-Z0-9_-]/g, '');
    if (!safe) return false;
    if (!fs.existsSync(PRESETS_DIR)) fs.mkdirSync(PRESETS_DIR);
    const contenuto = { nome, descrizione: '', domini };
    fs.writeFileSync(path.join(PRESETS_DIR, safe + '.json'), JSON.stringify(contenuto, null, 2));
    return true;
}

/**
 * Web server: serve la UI (`public/`), le API REST e lo stream SSE.
 * Tutte le richieste passano da `controllaAuth`. Il router e' una sequenza
 * di if/return su `p` (pathname). L'elenco completo degli endpoint e' in
 * ARCHITECTURE.md sezione "API surface".
 */
const webServer = http.createServer(async (req, res) => {
    if (!controllaAuth(req, res)) return;

    const u = new URL(req.url, `http://${req.headers.host}`);
    const p = u.pathname;

    // --- Config ---
    if (p === '/api/config') {
        jsonReply(res, {
            titolo: config.titolo,
            classe: config.classe,
            modo: config.modo,
            inattivitaSogliaSec: config.inattivitaSogliaSec || 180,
            dominiAI: DOMINI_AI,
            patternSistema: PATTERN_SISTEMA,
            studenti,
            presets: listaPresets(),
            classi: listaClassi(),
        });
        return;
    }

    // --- Storia (snapshot completa) ---
    if (p === '/api/history') {
        jsonReply(res, {
            entries: storia,
            bloccati: [...bloccati],
            sessioneAttiva,
            sessioneInizio,
            sessioneFineISO,
            pausato,
            deadlineISO,
            alive: Object.fromEntries(aliveMap),
        });
        return;
    }

    // --- Blocklist ---
    if (p === '/api/block') {
        const d = u.searchParams.get('domain');
        if (!d) { jsonReply(res, { ok: false, error: 'Dominio mancante' }, 400); return; }
        bloccati.add(d);
        salvaLista();
        broadcast({ type: 'blocklist', list: [...bloccati] });
        jsonReply(res, { ok: true, blocked: [...bloccati] });
        return;
    }
    if (p === '/api/unblock') {
        const d = u.searchParams.get('domain');
        if (!d) { jsonReply(res, { ok: false, error: 'Dominio mancante' }, 400); return; }
        bloccati.delete(d);
        salvaLista();
        broadcast({ type: 'blocklist', list: [...bloccati] });
        jsonReply(res, { ok: true, blocked: [...bloccati] });
        return;
    }
    if (p === '/api/block-all-ai') {
        for (const d of DOMINI_AI) bloccati.add(d);
        salvaLista();
        broadcast({ type: 'blocklist', list: [...bloccati] });
        jsonReply(res, { ok: true, blocked: [...bloccati] });
        return;
    }
    if (p === '/api/unblock-all-ai') {
        for (const d of DOMINI_AI) bloccati.delete(d);
        salvaLista();
        broadcast({ type: 'blocklist', list: [...bloccati] });
        jsonReply(res, { ok: true, blocked: [...bloccati] });
        return;
    }
    if (p === '/api/clear-blocklist') {
        bloccati.clear();
        salvaLista();
        broadcast({ type: 'blocklist', list: [] });
        jsonReply(res, { ok: true });
        return;
    }

    // --- Presets ---
    if (p === '/api/presets') {
        jsonReply(res, { presets: listaPresets() });
        return;
    }
    if (p === '/api/preset/load') {
        const nome = u.searchParams.get('nome');
        const pr = leggiPreset(nome);
        if (!pr) { jsonReply(res, { ok: false, error: 'Preset non trovato' }, 404); return; }
        bloccati = new Set(pr.domini || []);
        salvaLista();
        broadcast({ type: 'blocklist', list: [...bloccati] });
        jsonReply(res, { ok: true, preset: pr, blocked: [...bloccati] });
        return;
    }
    if (p === '/api/preset/save') {
        const nome = u.searchParams.get('nome');
        if (!nome) { jsonReply(res, { ok: false, error: 'Nome mancante' }, 400); return; }
        const ok = salvaPreset(nome, [...bloccati]);
        jsonReply(res, { ok, presets: listaPresets() });
        return;
    }

    // --- Sessione corrente ---
    if (p === '/api/session/start') {
        // Flusso normale: Stop ha gia' archiviato, Start azzera e riparte.
        // Caso difensivo: se la sessione era ancora attiva (API call diretta senza passare
        // da Stop) archivia prima di azzerare, altrimenti perdiamo i dati.
        let archiviata = null;
        if (sessioneAttiva) archiviata = archiviaSessioneCorrente();
        storia.length = 0;
        try { fs.writeFileSync(LOG_FILE, ''); } catch {}
        sessioneInizio = new Date().toISOString();
        sessioneFineISO = null;
        sessioneAttiva = true;
        broadcast({ type: 'reset', sessioneInizio });
        broadcastStatoSessione();
        jsonReply(res, { ok: true, sessioneInizio, sessioneAttiva, archiviata });
        return;
    }
    if (p === '/api/session/stop') {
        // Ferma la registrazione + archivia subito (se ha dati).
        // I dati restano visibili nel buffer: l'archivio e' per sicurezza (crash / spegnimento).
        // Il successivo /api/session/start azzera il buffer (e ri-archivia, idempotente).
        let archiviata = null;
        if (sessioneAttiva) {
            sessioneAttiva = false;
            sessioneFineISO = new Date().toISOString();
            archiviata = archiviaSessioneCorrente();
            broadcastStatoSessione();
        }
        jsonReply(res, { ok: true, sessioneAttiva, sessioneFineISO, archiviata });
        return;
    }
    if (p === '/api/session/status') {
        const fine = sessioneAttiva ? Date.now() : (sessioneFineISO ? new Date(sessioneFineISO).getTime() : Date.now());
        const inizio = sessioneInizio ? new Date(sessioneInizio).getTime() : fine;
        jsonReply(res, {
            sessioneAttiva,
            sessioneInizio,
            sessioneFineISO,
            durataSec: Math.floor((fine - inizio) / 1000),
            richieste: storia.length,
            bloccati: [...bloccati],
            pausato,
            deadlineISO,
        });
        return;
    }
    if (p === '/api/export') {
        const data = {
            sessioneInizio,
            esportatoAlle: new Date().toISOString(),
            titolo: config.titolo,
            classe: config.classe,
            modo: config.modo,
            studenti,
            bloccati: [...bloccati],
            entries: storia,
        };
        const filename = `sessione_${sessioneInizio.replace(/[:T]/g, '-').substring(0, 19)}.json`;
        res.writeHead(200, {
            'Content-Type': 'application/json; charset=UTF-8',
            'Content-Disposition': `attachment; filename="${filename}"`,
        });
        res.end(JSON.stringify(data, null, 2));
        return;
    }
    if (p === '/api/reset') {
        storia.length = 0;
        try { fs.writeFileSync(LOG_FILE, ''); } catch {}
        broadcast({ type: 'reset', sessioneInizio });
        jsonReply(res, { ok: true });
        return;
    }

    // --- Archivio sessioni ---
    if (p === '/api/sessioni') {
        jsonReply(res, { sessioni: listaSessioni() });
        return;
    }
    if (p === '/api/sessioni/load') {
        const safe = nomeSessioneSafe(u.searchParams.get('nome'));
        if (!safe) { jsonReply(res, { ok: false, error: 'Nome non valido' }, 400); return; }
        try {
            const contenuto = JSON.parse(fs.readFileSync(path.join(SESSIONI_DIR, safe), 'utf8'));
            jsonReply(res, { ok: true, sessione: contenuto });
        } catch { jsonReply(res, { ok: false, error: 'Non trovata' }, 404); }
        return;
    }
    if (p === '/api/sessioni/delete') {
        const safe = nomeSessioneSafe(u.searchParams.get('nome'));
        if (!safe) { jsonReply(res, { ok: false, error: 'Nome non valido' }, 400); return; }
        try {
            fs.unlinkSync(path.join(SESSIONI_DIR, safe));
            jsonReply(res, { ok: true, sessioni: listaSessioni() });
        } catch { jsonReply(res, { ok: false, error: 'Errore eliminazione' }, 404); }
        return;
    }
    if (p === '/api/sessioni/archivia') {
        const nome = archiviaSessioneCorrente();
        jsonReply(res, { ok: !!nome, nome, sessioni: listaSessioni() });
        return;
    }

    // --- Pausa globale ---
    if (p === '/api/pausa/toggle') {
        pausato = !pausato;
        broadcast({ type: 'pausa', pausato });
        jsonReply(res, { ok: true, pausato });
        return;
    }
    if (p === '/api/pausa/on') {
        pausato = true;
        broadcast({ type: 'pausa', pausato });
        jsonReply(res, { ok: true, pausato });
        return;
    }
    if (p === '/api/pausa/off') {
        pausato = false;
        broadcast({ type: 'pausa', pausato });
        jsonReply(res, { ok: true, pausato });
        return;
    }

    // --- Deadline ---
    if (p === '/api/deadline/set') {
        const time = u.searchParams.get('time');  // "HH:MM" locale
        if (!time || !/^\d{1,2}:\d{2}$/.test(time)) {
            jsonReply(res, { ok: false, error: 'Formato HH:MM' }, 400);
            return;
        }
        const [h, m] = time.split(':').map(Number);
        const now = new Date();
        const d = new Date(now.getFullYear(), now.getMonth(), now.getDate(), h, m, 0);
        if (d <= now) d.setDate(d.getDate() + 1);
        impostaDeadline(d.toISOString());
        jsonReply(res, { ok: true, deadlineISO });
        return;
    }
    if (p === '/api/deadline/clear') {
        impostaDeadline(null);
        jsonReply(res, { ok: true });
        return;
    }

    // --- Settings (config modificabile da UI) ---
    if (p === '/api/settings') {
        jsonReply(res, { settings: sanitizeConfig(config) });
        return;
    }
    if (p === '/api/settings/update' && req.method === 'POST') {
        const body = await leggiBody(req);
        const result = applicaSettings(body);
        if (result.updated.length > 0) {
            broadcast({ type: 'settings', settings: sanitizeConfig(config) });
        }
        jsonReply(res, { ok: true, ...result, settings: sanitizeConfig(config) });
        return;
    }
    if (p === '/api/settings/ignorati/add') {
        const dominio = (u.searchParams.get('dominio') || '').trim();
        if (!dominio) { jsonReply(res, { ok: false, error: 'Dominio mancante' }, 400); return; }
        const lista = config.dominiIgnorati || [];
        if (!lista.includes(dominio)) lista.push(dominio);
        config.dominiIgnorati = lista;
        salvaConfigFile();
        broadcast({ type: 'settings', settings: sanitizeConfig(config) });
        jsonReply(res, { ok: true, dominiIgnorati: lista });
        return;
    }
    if (p === '/api/settings/ignorati/remove') {
        const dominio = (u.searchParams.get('dominio') || '').trim();
        if (!dominio) { jsonReply(res, { ok: false }, 400); return; }
        config.dominiIgnorati = (config.dominiIgnorati || []).filter(d => d !== dominio);
        salvaConfigFile();
        broadcast({ type: 'settings', settings: sanitizeConfig(config) });
        jsonReply(res, { ok: true, dominiIgnorati: config.dominiIgnorati });
        return;
    }

    // --- Studenti ---
    if (p === '/api/reload-studenti') {
        caricaStudenti();
        broadcast({ type: 'studenti', studenti });
        jsonReply(res, { ok: true, studenti });
        return;
    }
    if (p === '/api/studenti/set') {
        const ip = (u.searchParams.get('ip') || '').trim();
        const nome = (u.searchParams.get('nome') || '').trim();
        if (!ip || !nome) { jsonReply(res, { ok: false, error: 'IP e nome richiesti' }, 400); return; }
        studenti[ip] = nome;
        salvaStudentiFile();
        broadcast({ type: 'studenti', studenti });
        jsonReply(res, { ok: true, studenti });
        return;
    }
    if (p === '/api/studenti/delete') {
        const ip = (u.searchParams.get('ip') || '').trim();
        if (!ip) { jsonReply(res, { ok: false, error: 'IP richiesto' }, 400); return; }
        delete studenti[ip];
        salvaStudentiFile();
        broadcast({ type: 'studenti', studenti });
        jsonReply(res, { ok: true, studenti });
        return;
    }
    if (p === '/api/studenti/clear') {
        studenti = {};
        salvaStudentiFile();
        broadcast({ type: 'studenti', studenti });
        jsonReply(res, { ok: true, studenti });
        return;
    }

    // --- Classi (mappe studenti salvate) ---
    if (p === '/api/classi') {
        jsonReply(res, { classi: listaClassi() });
        return;
    }
    if (p === '/api/classi/load') {
        const classe = u.searchParams.get('classe');
        const lab = u.searchParams.get('lab');
        const cl = leggiClasse(classe, lab);
        if (!cl) { jsonReply(res, { ok: false, error: 'Combinazione non trovata' }, 404); return; }
        const mappa = cl.mappa || {};
        studenti = {};
        for (const [k, v] of Object.entries(mappa)) {
            if (!k.startsWith('_')) studenti[k] = v;
        }
        salvaStudentiFile();
        broadcast({ type: 'studenti', studenti });
        jsonReply(res, { ok: true, studenti, classe, lab });
        return;
    }
    if (p === '/api/classi/save') {
        const classe = u.searchParams.get('classe');
        const lab = u.searchParams.get('lab');
        if (!classe || !lab) { jsonReply(res, { ok: false, error: 'classe e lab richiesti' }, 400); return; }
        const ok = salvaClasse(classe, lab, studenti);
        broadcast({ type: 'classi', classi: listaClassi() });
        jsonReply(res, { ok, classi: listaClassi() });
        return;
    }
    if (p === '/api/classi/delete') {
        const classe = u.searchParams.get('classe');
        const lab = u.searchParams.get('lab');
        const ok = eliminaClasse(classe, lab);
        broadcast({ type: 'classi', classi: listaClassi() });
        jsonReply(res, { ok, classi: listaClassi() });
        return;
    }

    // --- SSE ---
    if (p === '/api/stream') {
        res.writeHead(200, {
            'Content-Type': 'text/event-stream',
            'Cache-Control': 'no-cache',
            'Connection': 'keep-alive',
            'X-Accel-Buffering': 'no',
        });
        res.write(': connected\n\n');
        sseClients.add(res);
        const hb = setInterval(() => {
            try { res.write(': hb\n\n'); } catch {}
        }, 20000);
        req.on('close', () => {
            sseClients.delete(res);
            clearInterval(hb);
        });
        return;
    }

    serveStatic(req, res);
});

// ============================================================
// GRACEFUL SHUTDOWN
// ============================================================

/**
 * Chiusura ordinata: notifica EventSource, archivia il buffer corrente se
 * ha dati (salva anche quando la sessione e' "ferma"), chiude entrambi i
 * server HTTP. Timeout di 500ms come safety net — poi `process.exit(0)`.
 *
 * Agganciato a SIGINT (Ctrl+C nella cmd) e SIGTERM.
 * Casi NON coperti: `kill -9`, power loss — il buffer in RAM si perde.
 */
function spegni() {
    console.log('\nChiusura in corso...');
    for (const c of sseClients) { try { c.end(); } catch {} }
    archiviaSessioneCorrente();
    try { proxyServer.close(); } catch {}
    try { webServer.close(); } catch {}
    setTimeout(() => process.exit(0), 500);
}
process.on('SIGINT', spegni);
process.on('SIGTERM', spegni);

webServer.listen(PORTA_WEB, '0.0.0.0', () => {
    console.log('===========================================');
    console.log(`  ${config.titolo}${config.classe ? ' - ' + config.classe : ''}`);
    console.log('===========================================');
    console.log(`Proxy:     http://${IP_SERVER}:${PORTA_PROXY}  (modo: ${config.modo})`);
    console.log(`Monitor:   http://${IP_SERVER}:${PORTA_WEB}${config.web?.auth?.enabled ? '  [auth]' : ''}`);
    console.log(`Studenti:  ${Object.keys(studenti).length} mappati`);
    console.log(`Classi:    ${listaClassi().length} salvate`);
    console.log(`Presets:   ${listaPresets().length} disponibili`);
    console.log(`Sessioni:  ${listaSessioni().length} archiviate`);
    console.log('-------------------------------------------');
    console.log('Configura i PC studenti con:');
    console.log(`  Proxy: ${IP_SERVER}:${PORTA_PROXY}`);
    console.log('-------------------------------------------');
    console.log('In ascolto...\n');
});

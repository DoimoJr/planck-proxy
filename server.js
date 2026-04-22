// ============================================================
// Server unificato: proxy HTTP/HTTPS + UI monitor + API
// Uso: node server.js  (oppure avvia.bat)
// ============================================================

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

function salvaConfigFile() {
    try { fs.writeFileSync(CONFIG_PATH, JSON.stringify(config, null, 2)); } catch {}
}

function sanitizeConfig(c) {
    const out = JSON.parse(JSON.stringify(c));
    if (out.web?.auth) {
        out.web.auth.passwordSet = !!out.web.auth.password;
        out.web.auth.password = '';
    }
    return out;
}

function getDeep(obj, path) {
    return path.split('.').reduce((o, k) => (o == null ? o : o[k]), obj);
}
function setDeep(obj, path, value) {
    const parts = path.split('.');
    let cur = obj;
    for (let i = 0; i < parts.length - 1; i++) {
        if (typeof cur[parts[i]] !== 'object' || cur[parts[i]] === null) cur[parts[i]] = {};
        cur = cur[parts[i]];
    }
    cur[parts[parts.length - 1]] = value;
}

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
const SETTINGS_RESTART = new Set(['proxy.port', 'web.port', 'web.auth.enabled', 'web.auth.user', 'web.auth.password']);

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

// --- STUDENTI ---
let studenti = {};
function caricaStudenti() {
    try {
        const raw = JSON.parse(fs.readFileSync(STUDENTI_PATH, 'utf8'));
        studenti = {};
        for (const [k, v] of Object.entries(raw)) {
            if (!k.startsWith('_')) studenti[k] = v;
        }
    } catch { studenti = {}; }
}
function salvaStudentiFile() {
    try { fs.writeFileSync(STUDENTI_PATH, JSON.stringify(studenti, null, 2)); } catch {}
}
caricaStudenti();

// --- CLASSI (mappe studenti salvate: coppia classe+laboratorio) ---
function sanSegmento(s) {
    const safe = (s || '').replace(/[^a-zA-Z0-9_-]/g, '');
    return safe || null;
}
function fileClasse(classe, lab) {
    const sc = sanSegmento(classe);
    const sl = sanSegmento(lab);
    if (!sc || !sl) return null;
    return sc + '--' + sl + '.json';
}
function parseFileClasse(filename) {
    const stem = filename.replace(/\.json$/, '');
    const idx = stem.indexOf('--');
    if (idx < 0) return null;
    return { classe: stem.slice(0, idx), lab: stem.slice(idx + 2) };
}
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
function leggiClasse(classe, lab) {
    const file = fileClasse(classe, lab);
    if (!file) return null;
    try { return JSON.parse(fs.readFileSync(path.join(CLASSI_DIR, file), 'utf8')); }
    catch { return null; }
}
function salvaClasse(classe, lab, mappa) {
    const file = fileClasse(classe, lab);
    if (!file) return false;
    const contenuto = { classe, lab, mappa };
    try { fs.writeFileSync(path.join(CLASSI_DIR, file), JSON.stringify(contenuto, null, 2)); return true; }
    catch { return false; }
}
function eliminaClasse(classe, lab) {
    const file = fileClasse(classe, lab);
    if (!file) return false;
    try { fs.unlinkSync(path.join(CLASSI_DIR, file)); return true; }
    catch { return false; }
}

// --- PAGINA BLOCCATA ---
let paginaBloccata;
try { paginaBloccata = fs.readFileSync(BLOCKED_HTML_PATH, 'utf8'); }
catch {
    paginaBloccata = '<html><body style="font-family:sans-serif;text-align:center;padding:80px">'
        + '<h1 style="color:#e74c3c">Accesso bloccato dal docente</h1></body></html>';
}

// --- IP LOCALE ---
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

// --- STATO IN MEMORIA ---
const MAX_STORIA = 5000;
const storia = [];
let bloccati = new Set();
const sseClients = new Set();
// Lifecycle sessione esplicito: al boot la sessione NON e' attiva.
// Il proxy instrada e blocca comunque, ma niente viene registrato finche'
// l'utente non preme "Avvia sessione".
let sessioneAttiva = false;
let sessioneInizio = null;
let sessioneFineISO = null;  // timestamp del momento di "Ferma" (per durata congelata)
let pausato = false;
let deadlineISO = null;
let deadlineTimer = null;
const aliveMap = new Map(); // ip -> ultimo ping timestamp (ms)

function registraAlive(ip) {
    const ts = Date.now();
    aliveMap.set(ip, ts);
    broadcast({ type: 'alive', ip, ts });
}

// --- BLOCKLIST ---
function caricaLista() {
    try {
        const txt = fs.readFileSync(BLOCKED_FILE, 'utf8');
        bloccati = new Set(txt.split('\n').map(s => s.trim()).filter(Boolean));
    } catch { bloccati = new Set(); }
}
function salvaLista() {
    fs.writeFileSync(BLOCKED_FILE, [...bloccati].join('\n') + '\n');
}
caricaLista();

function isIgnorato(dominio) {
    const d = dominio.toLowerCase();
    const lista = config.dominiIgnorati || [];
    return lista.some(di => d.includes(di.toLowerCase()));
}

function dominioBloccato(dominio) {
    const d = dominio.toLowerCase();
    const ignorato = isIgnorato(dominio);

    // Pausa globale: blocca tutto tranne ignorati
    if (pausato && !ignorato) return true;

    const matchInLista = [...bloccati].some(bl => d.includes(bl.toLowerCase()));
    if (config.modo === 'allowlist') {
        if (ignorato) return false;
        return !matchInLista;
    }
    return matchInLista;
}

// --- TRAFFICO ---
function registraTraffico(ip, metodo, urlStr, blocked) {
    // Quando la sessione e' ferma: proxy funzionante (blocchi applicati a monte)
    // ma niente log, niente buffer, niente SSE. Il traffico e' "fuori sessione".
    if (!sessioneAttiva) return;

    const ora = new Date().toISOString().replace('T', ' ').substring(0, 19);
    const rigaLog = `${ora} | ${ip} | ${metodo} | ${urlStr}${blocked ? ' | BLOCKED' : ''}\n`;
    fs.appendFile(LOG_FILE, rigaLog, () => {});

    let dominio;
    try { dominio = new URL(urlStr).hostname; } catch { dominio = urlStr; }

    if (isIgnorato(dominio)) {
        const oraCorta = ora.substring(11);
        console.log(`${oraCorta} [${ip}] ${metodo} ${dominio}${blocked ? ' [BLOCKED]' : ''} (ignorato)`);
        return;
    }

    const tipo = classifica(dominio);
    const entry = { ora, ip, metodo, dominio, tipo, blocked };

    storia.push(entry);
    if (storia.length > MAX_STORIA) storia.shift();

    const oraCorta = ora.substring(11);
    console.log(`${oraCorta} [${ip}] ${metodo} ${dominio}${blocked ? ' [BLOCKED]' : ''}`);

    broadcast({ type: 'traffic', entry });
}

function broadcast(payload) {
    const data = `data: ${JSON.stringify(payload)}\n\n`;
    for (const res of sseClients) {
        try { res.write(data); } catch {}
    }
}

function rispondiBloccato(res) {
    res.writeHead(403, {
        'Content-Type': 'text/html; charset=UTF-8',
        'Content-Length': Buffer.byteLength(paginaBloccata),
        'Connection': 'close'
    });
    res.end(paginaBloccata);
}

function getClientIp(socket) {
    return socket.remoteAddress?.replace('::ffff:', '') || 'unknown';
}

// --- DEADLINE ---
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

// --- ARCHIVIO SESSIONI ---
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

function broadcastStatoSessione() {
    broadcast({
        type: 'session-state',
        sessioneAttiva,
        sessioneInizio,
        sessioneFineISO,
    });
}
function listaSessioni() {
    try {
        return fs.readdirSync(SESSIONI_DIR)
            .filter(f => f.endsWith('.json'))
            .sort().reverse();
    } catch { return []; }
}
function nomeSessioneSafe(nome) {
    const safe = (nome || '').replace(/[^a-zA-Z0-9_.\-]/g, '');
    return (safe && safe.endsWith('.json')) ? safe : null;
}

// ============================================================
// PROXY HTTP/HTTPS
// ============================================================

const proxyServer = http.createServer((req, res) => {
    const ipClient = getClientIp(req.socket);
    const targetUrl = req.url;

    // Endpoint keepalive: richiesta diretta al proxy (non proxata)
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

const MIME = {
    '.html': 'text/html; charset=UTF-8',
    '.css': 'text/css; charset=UTF-8',
    '.js': 'application/javascript; charset=UTF-8',
    '.json': 'application/json; charset=UTF-8',
};

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

function jsonReply(res, obj, status = 200) {
    const body = JSON.stringify(obj);
    res.writeHead(status, { 'Content-Type': 'application/json; charset=UTF-8' });
    res.end(body);
}

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

function listaPresets() {
    try {
        return fs.readdirSync(PRESETS_DIR)
            .filter(f => f.endsWith('.json'))
            .map(f => f.replace(/\.json$/, ''));
    } catch { return []; }
}
function leggiPreset(nome) {
    const safe = (nome || '').replace(/[^a-zA-Z0-9_-]/g, '');
    if (!safe) return null;
    try { return JSON.parse(fs.readFileSync(path.join(PRESETS_DIR, safe + '.json'), 'utf8')); }
    catch { return null; }
}
function salvaPreset(nome, domini) {
    const safe = (nome || '').replace(/[^a-zA-Z0-9_-]/g, '');
    if (!safe) return false;
    if (!fs.existsSync(PRESETS_DIR)) fs.mkdirSync(PRESETS_DIR);
    const contenuto = { nome, descrizione: '', domini };
    fs.writeFileSync(path.join(PRESETS_DIR, safe + '.json'), JSON.stringify(contenuto, null, 2));
    return true;
}

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

// --- GRACEFUL SHUTDOWN ---
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

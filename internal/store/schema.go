package store

// migration descrive una singola SQL migration applicata al boot.
type migration struct {
	Version int
	Name    string
	SQL     string
}

// allMigrations e' la lista ordinata di migration. Aggiungi nuove versioni in
// fondo, mai modificare quelle esistenti dopo che sono state rilasciate.
//
// La schema iniziale (v1) include solo le tabelle necessarie per le feature
// di Phase 1 (config, blocklist, presets, classi, sessioni con entries).
// Le tabelle per Veyon (v3-4), Auto-AI (v5), Reazioni (v6) verranno
// aggiunte come migration v2, v3, ... quando le rispettive fasi le
// richiederanno.
var allMigrations = []migration{
	{
		Version: 1,
		Name:    "init",
		SQL: `
-- ==========================================================
-- Config (key/value generico)
-- ==========================================================
CREATE TABLE kv (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

-- ==========================================================
-- Liste
-- ==========================================================
CREATE TABLE domini_ignorati (
    dominio TEXT PRIMARY KEY
);

CREATE TABLE bloccati (
    dominio  TEXT PRIMARY KEY,
    added_at INTEGER NOT NULL
);

CREATE TABLE presets (
    nome        TEXT PRIMARY KEY,
    descrizione TEXT,
    domini      TEXT NOT NULL,        -- JSON array
    created_at  INTEGER NOT NULL
);

CREATE TABLE studenti_correnti (
    ip   TEXT PRIMARY KEY,
    nome TEXT NOT NULL
);

CREATE TABLE combo (
    classe     TEXT NOT NULL,
    lab        TEXT NOT NULL,
    mappa      TEXT NOT NULL,         -- JSON {"ip":"nome",...}
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (classe, lab)
);

-- ==========================================================
-- Sessioni e entries
-- ==========================================================
CREATE TABLE sessioni (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    sessione_inizio   TEXT NOT NULL,
    sessione_fine     TEXT,             -- NULL = sessione attiva (in corso)
    durata_sec        INTEGER,
    classe            TEXT NOT NULL DEFAULT '',
    lab               TEXT NOT NULL DEFAULT '',
    titolo            TEXT,
    modo              TEXT NOT NULL,
    studenti_snapshot TEXT NOT NULL,    -- JSON {ip:nome,...}
    bloccati_snapshot TEXT NOT NULL,    -- JSON [domini]
    archiviata_at     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_sessioni_classe_lab ON sessioni(classe, lab);
CREATE INDEX idx_sessioni_inizio     ON sessioni(sessione_inizio);

CREATE TABLE entries (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    sessione_id   INTEGER NOT NULL REFERENCES sessioni(id) ON DELETE CASCADE,
    ora           TEXT NOT NULL,
    ts            INTEGER NOT NULL,
    ip            TEXT NOT NULL,
    nome_studente TEXT,
    metodo        TEXT NOT NULL,
    dominio       TEXT NOT NULL,
    tipo          TEXT NOT NULL,
    blocked       INTEGER NOT NULL CHECK (blocked IN (0, 1)),
    flagged       INTEGER NOT NULL DEFAULT 0 CHECK (flagged IN (0, 1))
);
CREATE INDEX idx_entries_sessione ON entries(sessione_id);
CREATE INDEX idx_entries_nome     ON entries(nome_studente) WHERE nome_studente IS NOT NULL;
CREATE INDEX idx_entries_ts       ON entries(ts);
CREATE INDEX idx_entries_dominio  ON entries(dominio);
`,
	},
	{
		Version: 2,
		Name:    "watchdog_plugins",
		SQL: `
-- ==========================================================
-- Watchdog plugins (Phase 5): eventi e config per-plugin
-- ==========================================================
CREATE TABLE watchdog_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    sessione_id   INTEGER REFERENCES sessioni(id) ON DELETE SET NULL,
    plugin        TEXT NOT NULL,
    ip            TEXT NOT NULL,
    nome_studente TEXT,
    ts            INTEGER NOT NULL,
    severity      TEXT NOT NULL,             -- info | warning | critical
    payload_json  TEXT NOT NULL              -- JSON plugin-specific
);
CREATE INDEX idx_watchdog_sessione ON watchdog_events(sessione_id);
CREATE INDEX idx_watchdog_ip       ON watchdog_events(ip);
CREATE INDEX idx_watchdog_ts       ON watchdog_events(ts);
CREATE INDEX idx_watchdog_plugin   ON watchdog_events(plugin);

CREATE TABLE watchdog_config (
    plugin       TEXT PRIMARY KEY,
    enabled      INTEGER NOT NULL DEFAULT 0,
    config_json  TEXT NOT NULL DEFAULT '{}',
    updated_at   INTEGER NOT NULL
);
`,
	},
	{
		Version: 3,
		Name:    "bloccati_per_ip",
		SQL: `
-- ==========================================================
-- Blocchi per-IP: domini bloccati solo per uno studente specifico.
-- Additivi rispetto alla blocklist globale (proxy.DominioBloccato
-- controlla entrambi: globale OR per-IP).
-- ==========================================================
CREATE TABLE bloccati_per_ip (
    ip       TEXT NOT NULL,
    dominio  TEXT NOT NULL,
    added_at INTEGER NOT NULL,
    PRIMARY KEY (ip, dominio)
);
CREATE INDEX idx_bloccati_per_ip_ip ON bloccati_per_ip(ip);
`,
	},
	{
		Version: 4,
		Name:    "ignorati_infrastruttura_scolastica",
		SQL: `
-- ==========================================================
-- Rumore di infrastruttura scolastica, dalla sessione reale del
-- 2026-09-28 (5BII, 89 minuti, 2135 richieste): questi domini pesavano
-- 817 richieste, il 38% del traffico, e finivano tutti nei conteggi
-- per studente perche' nessun PATTERN_SISTEMA li intercettava.
--
-- INSERT OR IGNORE + migration versionata: gira una volta sola, quindi
-- se in futuro rimuovi uno di questi dalla UI non te lo ritrovi al
-- riavvio successivo.
--
-- Nota: firewall.maxplanck.* e' specifico di questo istituto. Resta qui
-- perche' il binario nasce per quella scuola; su un'altra installazione
-- e' semplicemente un dominio che non comparira' mai.
-- ==========================================================
INSERT OR IGNORE INTO domini_ignorati (dominio) VALUES
    ('firewall.maxplanck.edu.it'),      -- firewall/captive portal d'istituto (199 richieste)
    ('firewall.maxplanck.it'),          -- stesso, secondo hostname (44)
    ('vo.msecnd.net'),                  -- CDN Azure, match per sottostringa (132)
    ('api.faronics.com'),               -- Deep Freeze, gestione laboratorio (75)
    ('upd.faronicslabs.com'),           -- Deep Freeze, canale aggiornamenti (75)
    ('dc.services.visualstudio.com'),   -- telemetria Application Insights di VS Code (108)
    ('aka.ms'),                         -- short link Microsoft, usati da VS Code (87)
    ('targetednotifications-tm.trafficmanager.net'), -- notifiche Microsoft (46)
    ('firefox-portal-detection.com'),   -- captive portal detection di Firefox (23)
    ('vscode-unpkg.net'),               -- CDN estensioni VS Code (18)
    ('dl.google.com');                  -- aggiornamenti Chrome/Google (10)
`,
	},
	{
		Version: 5,
		Name:    "usb_classi_rumorose",
		SQL: `
-- ==========================================================
-- Stessa sessione: il plugin USB ha prodotto 277 eventi e rilevato zero
-- chiavette. Erano stampanti di rete, volumi, copie shadow, miniport WAN
-- e code di stampa — classi PnP che non erano nella lista di esclusione.
--
-- Serve una migration e non basta cambiare DefaultConfig: la config
-- salvata in watchdog_config vince sui default, quindi sulle
-- installazioni gia' avviate i nuovi valori non arriverebbero mai.
--
-- Cancelliamo la riga invece di riscriverne il JSON: al prossimo
-- LoadWatchdogConfig il plugin riparte dal DefaultConfig aggiornato, che
-- include anche l'eccezione USBSTOR per le chiavette vere. Si perde una
-- eventuale allowlist VID:PID personalizzata, che nella pratica e' vuota.
-- ==========================================================
DELETE FROM watchdog_config WHERE plugin = 'usb';
`,
	},
}

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
}

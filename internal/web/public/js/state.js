/**
 * @file Stato globale condiviso tra i moduli frontend.
 *
 * Unica "source of truth" lato client: `app.js` idrata da `/api/history`, le
 * mutazioni avvengono in `actions.js` e nelle callback SSE di `sse.js`,
 * `render.js` legge e basta (funzioni pure).
 *
 * La persistenza su `localStorage` copre solo lo stato UI (tema, tab, vista,
 * collassi, filtri nascosti): tutto il resto e' ricostruito a ogni boot del
 * server via `/api/history`.
 */

/**
 * @typedef {'ai'|'utente'|'sistema'} TipoDominio
 * Categoria di un dominio secondo `domains.js::classifica()`.
 * - `ai`: dominio di servizio AI (chatbot, assistenti, code AI, ...).
 * - `sistema`: traffico di background (telemetria, ad tech, update, CMP) escluso dai conteggi per-studente.
 * - `utente`: attivita' reale dello studente (tutto il resto).
 */

/**
 * @typedef {Object} Entry
 * Singola richiesta loggata dal proxy, inviata via SSE e storicizzata in `entries`.
 * @property {string} ora - Timestamp "YYYY-MM-DD HH:MM:SS" UTC.
 * @property {string} ip - IP del PC studente.
 * @property {string} metodo - Metodo HTTP oppure "HTTPS" per le CONNECT.
 * @property {string} dominio - Hostname target.
 * @property {TipoDominio} tipo - Classificazione.
 * @property {boolean} blocked - True se il proxy ha risposto 403.
 */

/**
 * Stato globale mutabile. Non sostituire l'oggetto: mutare le proprieta'
 * al suo interno (i moduli importano `state` per riferimento).
 */
export const state = {
    /**
     * Configurazione arrivata da `/api/config` al primo load.
     * Alcuni campi possono essere sovrascritti da messaggi SSE `settings`.
     */
    cfg: { titolo: 'Monitor', classe: '', modo: 'blocklist', inattivitaSogliaSec: 180, dominiAI: [], patternSistema: [], studenti: {}, presets: [] },

    /** @type {Entry[]} Ring buffer client (replica di `storia` server). */
    entries: [],
    /** @type {Map<string, Entry[]>} IP -> tutte le sue entry (per la tabella Live). */
    perIp: new Map(),
    /** @type {Map<string, {count:number, tipo:TipoDominio, ultima?:string}>} Aggregato per dominio. */
    perDominio: new Map(),
    /** @type {Map<string, string>} IP -> ora (stringa) dell'ultima entry vista. */
    ultimaPerIp: new Map(),
    /** @type {Map<string, number>} IP -> timestamp ms dell'ultimo ping watchdog. NON resettata da `resetDatiTraffico`. */
    aliveMap: new Map(),

    /** @type {Set<string>} Domini in blocklist (rispecchia lo stato server). */
    bloccati: new Set(),
    /**
     * Blocchi per-IP (additivi rispetto a `bloccati`): un dominio bloccato
     * SOLO per uno specifico studente. Mappa ip → Set<dominio>. Persistito
     * lato server in tabella `bloccati_per_ip`. Aggiornato via SSE
     * `blocchi-per-ip` o all'idratazione da /api/history.
     * @type {Map<string, Set<string>>}
     */
    blocchiPerIp: new Map(),
    /** @type {Set<string>} Domini nascosti dall'UI (persistito in localStorage). */
    nascosti: new Set(JSON.parse(localStorage.getItem('nascosti') || '[]')),

    /** Filtro testuale attivo nella Live tab. */
    filtro: '',
    /** @type {string|null} IP su cui il traffico e' filtrato (click su riga/card). */
    focusIp: null,
    /**
     * @type {string|null} IP per cui il detail pane (a destra) e' aperto.
     * Quando non null, il pannello stream si nasconde e a destra appare il
     * detail pane 280px (azioni, watchdog, domini recenti, sessione).
     * Click sulla X o sulla stessa card chiude (torna stream).
     */
    detailIp: null,

    // ============================================================
    // Banner alert + Log eventi (Phase 7)
    // ============================================================
    /** True se l'utente ha cliccato la X del banner. Auto-reset al prossimo evento. */
    bannerDismissed: false,
    /** Chiave evento corrente (aiCount-wdCount-lastTs): cambia → bannerDismissed = false. */
    bannerLastEventKey: '',
    /** 'pulse' (default) | 'sticky' | 'slide' — variante visiva del banner. */
    bannerKind: 'pulse',
    /** True quando il pannello "Log eventi" e' aperto a destra (mutex con stream/detail). */
    logPanelOpen: false,
    /** Filtro lista log: 'all' | 'ai' | 'wd'. */
    logFilter: 'all',
    /** @type {Set<string>} Eventi marcati come "ignora" dall'utente. */
    eventiIgnoredIds: new Set(),
    /**
     * Set di IP per i quali abbiamo inviato un comando Veyon screenLock
     * andato a buon fine (e nessun unlock successivo). Tracking lato
     * client only — Veyon non espone uno "stato locked" consultabile,
     * quindi se l'utente blocca/sblocca da un altro Veyon Master non
     * lo vediamo. Per il use case "uso solo Planck" e' sufficiente.
     * Resettato a ogni boot (Set in-memory).
     * @type {Set<string>}
     */
    lockedIps: new Set(),
    /**
     * Multi-selezione (Phase 4 polish). Set di IP selezionati con Ctrl/
     * Shift+click sulle card. Quando non vuoto, le azioni Veyon "classe"
     * agiscono sulla selezione invece che su tutti gli IP attivi.
     * @type {Set<string>}
     */
    selectedIps: new Set(),
    /** @type {string|null} Ultimo IP cliccato — anchor per Shift+click range selection. */
    selectionAnchor: null,
    // Default dark mode (Claude Designer): se l'utente non ha mai espresso
    // una preferenza, parte in dark. localStorage memorizza '0' per light
    // esplicito o '1' per dark esplicito.
    darkmode: localStorage.getItem('darkmode') !== '0',
    notifiche: localStorage.getItem('notifiche') === '1',
    /** @type {'live'|'report'|'impostazioni'} */
    tabAttivo: localStorage.getItem('tab') || 'live',
    /** @type {'griglia'|'lista'} Modo di visualizzazione degli IP nella Live. */
    vistaIp: localStorage.getItem('vistaIp') || 'griglia',
    sidebarCollassata: localStorage.getItem('sidebarCollassata') === '1',
    richiesteCollassate: localStorage.getItem('richiesteCollassate') === '1',

    // Sessione (lifecycle esplicito: Avvia/Ferma)
    sessioneAttiva: false,
    /** @type {string|null} ISO timestamp dell'inizio sessione corrente (null se mai avviata). */
    sessioneInizio: null,
    /** @type {string|null} ISO timestamp del "Ferma" — usato per congelare la durata. */
    sessioneFineISO: null,
    pausato: false,
    /** @type {string|null} ISO timestamp della scadenza programmata. */
    deadlineISO: null,

    // Archivio sessioni (tab Report / Impostazioni)
    /** @type {string[]} Lista filename in `sessioni/` ordinati per data desc. */
    sessioniArchivio: [],
    /** @type {string|null} Nome file dell'archivio attualmente visualizzato (null = sessione corrente). */
    sessioneVisualizzata: null,
    /** @type {Object|null} Contenuto JSON caricato da `/api/sessioni/load`. */
    datiSessioneVisualizzata: null,

    /** True quando EventSource e' connesso (badge LIVE/OFF). */
    connesso: false,

    /** @type {Object|null} Config ricevuta da `/api/settings` (usata dal form Impostazioni). */
    settings: null,
    /** Diventa true quando si modifica un settings key in `SETTINGS_RESTART` (banner orange). */
    riavvioRichiesto: false,

    /**
     * Stato della configurazione Veyon. True quando una master key e' stata
     * importata: i bottoni Veyon (lock, msg, distribuisci) diventano cliccabili
     * e visibili. Aggiornato da `veyonAggiornaStato()` (boot + cambia tab).
     */
    veyonConfigured: false,

    // ============================================================
    // Watchdog plugins (Phase 5)
    // ============================================================

    /**
     * Lista dei plugin watchdog registrati lato server, ognuno con
     * stato enabled/disabled e config corrente. Caricato a boot da
     * /api/watchdog/plugins, aggiornato post toggle/save.
     * @type {Array<{id:string,name:string,description:string,enabled:boolean,config:any}>}
     */
    watchdogPlugins: [],

    /**
     * Eventi watchdog recenti per IP (ultimi 5 min). Renderizzati come
     * badge sulla card studente + nel pannello eventi. Map IP→array di
     * eventi {plugin, ts, severity, format, payload}.
     * @type {Map<string, Array<object>>}
     */
    watchdogEventsPerIp: new Map(),

    /** Coda globale degli ultimi N eventi watchdog (per il pannello "Eventi"). */
    watchdogEvents: [],

    /**
     * Stato corrente della lista AI (Phase 6). Aggiornato a boot via
     * /api/ai/status, poi su SSE 'ai-list' o dopo refresh manuale.
     * @type {{count:number, source:string, updatedAt:string, url?:string}}
     */
    aiList: { count: 0, source: '', updatedAt: '' },
};

/** Persiste il Set dei domini nascosti. */
export function salvaNascosti() { localStorage.setItem('nascosti', JSON.stringify([...state.nascosti])); }
/** Persiste la preferenza darkmode. */
export function salvaDarkmode() { localStorage.setItem('darkmode', state.darkmode ? '1' : '0'); }
/** Persiste la preferenza notifiche desktop+beep. */
export function salvaNotifiche() { localStorage.setItem('notifiche', state.notifiche ? '1' : '0'); }
/** Persiste il tab attivo (al reload si torna su quello). */
export function salvaTab() { localStorage.setItem('tab', state.tabAttivo); }
/** Persiste la vista IP scelta (griglia/lista). */
export function salvaVistaIp() { localStorage.setItem('vistaIp', state.vistaIp); }
/** Persiste lo stato collasso di sidebar e pannello ultime richieste. */
export function salvaCollassi() {
    localStorage.setItem('sidebarCollassata', state.sidebarCollassata ? '1' : '0');
    localStorage.setItem('richiesteCollassate', state.richiesteCollassate ? '1' : '0');
}

/**
 * Incorpora una nuova entry traffico in tutti gli aggregati derivati.
 * Chiamata sia al primo load (idratazione) sia a ogni messaggio SSE `traffic`.
 * @param {Entry} e
 */
export function assorbiEntry(e) {
    state.entries.push(e);
    if (!state.perIp.has(e.ip)) state.perIp.set(e.ip, []);
    state.perIp.get(e.ip).push(e);

    if (!state.perDominio.has(e.dominio)) state.perDominio.set(e.dominio, { count: 0, tipo: e.tipo });
    const info = state.perDominio.get(e.dominio);
    info.count++;
    info.ultima = e.ora;

    state.ultimaPerIp.set(e.ip, e.ora);
}

/**
 * Azzera gli aggregati client quando arriva un SSE `reset` (cambio sessione).
 * `aliveMap` e' preservata perche' il watchdog e' indipendente dalla sessione.
 */
export function resetDatiTraffico() {
    state.entries.length = 0;
    state.perIp.clear();
    state.perDominio.clear();
    state.ultimaPerIp.clear();
    state.focusIp = null;
}

/**
 * Restituisce il nome dello studente mappato all'IP, oppure `null` se non mappato.
 * @param {string} ip
 * @returns {string|null}
 */
export function nomeStudente(ip) {
    return state.cfg.studenti && state.cfg.studenti[ip] ? state.cfg.studenti[ip] : null;
}

/**
 * Calcola aggregati on-demand per il tab Report. Usato sia sulla sessione
 * corrente (`state.entries`) sia sul contenuto di un archivio caricato.
 *
 * Due mappe per-IP separate:
 * - `perIp`: conteggio totale (include sistema) — per il totale "Richieste totali".
 * - `perIpAttive`: solo utente+ai — per il ranking "Top studenti" (il rumore
 *   di sistema falserebbe la classifica).
 *
 * @param {Entry[]} entries
 * @returns {{
 *   perDominio: Map<string, {count:number, tipo:TipoDominio}>,
 *   perIp: Map<string, number>,
 *   perIpAttive: Map<string, number>,
 *   perTipo: {ai:number, utente:number, sistema:number},
 *   bloccate: number,
 *   totale: number
 * }}
 */
export function aggregaPerReport(entries) {
    const perDominio = new Map();
    const perIp = new Map();
    const perIpAttive = new Map();
    const perTipo = { ai: 0, utente: 0, sistema: 0 };
    let bloccate = 0;
    for (const e of entries) {
        if (!perDominio.has(e.dominio)) perDominio.set(e.dominio, { count: 0, tipo: e.tipo });
        perDominio.get(e.dominio).count++;
        perIp.set(e.ip, (perIp.get(e.ip) || 0) + 1);
        if (e.tipo !== 'sistema') {
            perIpAttive.set(e.ip, (perIpAttive.get(e.ip) || 0) + 1);
        }
        perTipo[e.tipo] = (perTipo[e.tipo] || 0) + 1;
        if (e.blocked) bloccate++;
    }
    return { perDominio, perIp, perIpAttive, perTipo, bloccate, totale: entries.length };
}

// Stato globale condiviso fra moduli

export const state = {
    cfg: { titolo: 'Monitor', classe: '', modo: 'blocklist', inattivitaSogliaSec: 180, dominiAI: [], patternSistema: [], studenti: {}, presets: [] },

    entries: [],
    perIp: new Map(),
    perDominio: new Map(),
    ultimaPerIp: new Map(),
    aliveMap: new Map(), // ip -> timestamp ms dell'ultimo ping watchdog

    bloccati: new Set(),
    nascosti: new Set(JSON.parse(localStorage.getItem('nascosti') || '[]')),

    filtro: '',
    focusIp: null,
    darkmode: localStorage.getItem('darkmode') === '1',
    notifiche: localStorage.getItem('notifiche') === '1',
    tabAttivo: localStorage.getItem('tab') || 'live',
    vistaIp: localStorage.getItem('vistaIp') || 'griglia',  // 'griglia' | 'lista'
    sidebarCollassata: localStorage.getItem('sidebarCollassata') === '1',
    richiesteCollassate: localStorage.getItem('richiesteCollassate') === '1',

    // Sessione
    sessioneAttiva: false,
    sessioneInizio: null,
    sessioneFineISO: null,
    pausato: false,
    deadlineISO: null,

    // Archivio sessioni
    sessioniArchivio: [],
    sessioneVisualizzata: null, // null = sessione corrente, altrimenti nome file archivio
    datiSessioneVisualizzata: null, // contenuto caricato dall'archivio (se sessioneVisualizzata != null)

    connesso: false,

    // Settings editabili dalla UI
    settings: null,
    riavvioRichiesto: false,
};

export function salvaNascosti() { localStorage.setItem('nascosti', JSON.stringify([...state.nascosti])); }
export function salvaDarkmode() { localStorage.setItem('darkmode', state.darkmode ? '1' : '0'); }
export function salvaNotifiche() { localStorage.setItem('notifiche', state.notifiche ? '1' : '0'); }
export function salvaTab() { localStorage.setItem('tab', state.tabAttivo); }
export function salvaVistaIp() { localStorage.setItem('vistaIp', state.vistaIp); }
export function salvaCollassi() {
    localStorage.setItem('sidebarCollassata', state.sidebarCollassata ? '1' : '0');
    localStorage.setItem('richiesteCollassate', state.richiesteCollassate ? '1' : '0');
}

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

export function resetDatiTraffico() {
    state.entries.length = 0;
    state.perIp.clear();
    state.perDominio.clear();
    state.ultimaPerIp.clear();
    // aliveMap NON viene resettata: il watchdog continua a pingare indipendentemente
    state.focusIp = null;
}

export function nomeStudente(ip) {
    return state.cfg.studenti && state.cfg.studenti[ip] ? state.cfg.studenti[ip] : null;
}

// Aggregazioni derivate per il report (ricalcolate ogni render)
// perIp = conteggio totale (include sistema)
// perIpAttive = conteggio attivita' vera (utente + ai) - usato per classifiche studenti
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

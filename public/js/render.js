/**
 * @file Funzioni di rendering della UI.
 *
 * Contratto: le funzioni esportate sono **pure rispetto al DOM**: leggono
 * `state` e scrivono `innerHTML` / `textContent` sui nodi di riferimento.
 * Nessuna mutazione di `state` avviene qui (vedi `actions.js` per le
 * mutazioni e `sse.js` per quelle reattive).
 *
 * Throttling: `renderAll()` e' guardato da un flag + `requestAnimationFrame`
 * — chiamate multiple nello stesso frame coalescono in un unico paint.
 * Questo e' cruciale durante burst SSE (20+ studenti attivi): ogni entry
 * invoca `renderAll()` ma si ridipinge una volta sola per frame.
 *
 * Eccezione: `renderCountdown()` ha il suo `setInterval(1000)` separato —
 * aggiorna solo la cella countdown, no-alloc per il ticker a 1Hz.
 */

import { state, nomeStudente, aggregaPerReport } from './state.js';
import { $, escapeHtml, attrEscape, ip2long, parseOra, formatDurata, formatRelativo } from './util.js';

/** Numero massimo di entry mostrate nella card "Ultime richieste". */
const MAX_RICHIESTE_UI = 100;

/**
 * @param {string} testo - Testo da confrontare col filtro corrente.
 * @returns {boolean} true se il filtro e' vuoto o se `testo` lo contiene (case-insensitive).
 */
function matchFiltro(testo) {
    if (!state.filtro) return true;
    return String(testo).toLowerCase().includes(state.filtro.toLowerCase());
}

// ========================================================================
// Sidebar domini (Live tab, colonna sinistra)
// ========================================================================

/**
 * Rigenera l'intera sidebar: suddivide i domini visti in 5 sezioni
 * (AI / Siti / Sistema / Bloccati / Nascosti), aggiorna i contatori,
 * nasconde le sezioni vuote.
 */
export function renderSidebar() {
    const ai = [], utente = [], sistema = [], bloccatiList = [], nascostiList = [];
    const domOrdinati = [...state.perDominio.entries()].sort(([a], [b]) => a.localeCompare(b));

    for (const [dominio, info] of domOrdinati) {
        if (state.bloccati.has(dominio)) { bloccatiList.push([dominio, info]); continue; }
        if (state.nascosti.has(dominio)) { nascostiList.push([dominio, info]); continue; }
        if (info.tipo === 'ai') ai.push([dominio, info]);
        else if (info.tipo === 'sistema') sistema.push([dominio, info]);
        else utente.push([dominio, info]);
    }

    renderListaDomini('domini-ai-list', ai, 'ai', false);
    renderListaDomini('domini-siti-list', utente, 'utente', false);
    renderListaDomini('domini-sistema-list', sistema, 'sistema', false);
    renderListaBloccati('domini-bloccati-list', bloccatiList);
    renderListaDomini('domini-nascosti-list', nascostiList, 'nascosto', true);

    $('count-ai').textContent = ai.length;
    $('count-siti').textContent = utente.length;
    $('count-sistema').textContent = sistema.length;
    $('count-bloccati').textContent = state.bloccati.size;
    $('count-nascosti').textContent = nascostiList.length;
    $('count-domini').textContent = state.perDominio.size;

    $('sezione-ai').style.display = ai.length > 0 ? '' : 'none';
    $('sezione-sistema').style.display = sistema.length > 0 ? '' : 'none';
    $('sezione-bloccati').style.display = state.bloccati.size > 0 ? '' : 'none';
    $('sezione-nascosti').style.display = nascostiList.length > 0 ? '' : 'none';
}

/**
 * Rigenera il contenuto di una lista domini dentro la sidebar.
 * @param {string} elId - ID del contenitore.
 * @param {Array<[string, {count:number, tipo:string}]>} items
 * @param {string} tipoClass - Classe CSS del tipo ("ai" | "utente" | "sistema" | "nascosto").
 * @param {boolean} isNascosto - Se true, l'azione principale del click e' "mostra" invece di "nascondi".
 */
function renderListaDomini(elId, items, tipoClass, isNascosto) {
    const el = $(elId);
    if (!el) return;
    el.innerHTML = items.map(([dominio, info]) => {
        const d = escapeHtml(dominio);
        const da = attrEscape(dominio);
        const extraClass = isNascosto ? ' nascosto-item' : '';
        const hidden = matchFiltro(dominio) ? '' : ' filtro-hidden';
        const azionePrincipale = isNascosto ? 'mostra-dominio' : 'nascondi-dominio';
        return `<div class="dominio-item ${tipoClass}${extraClass}${hidden}" data-action="${azionePrincipale}" data-dominio="${da}">
            <span class="nome">${d}</span>
            <span class="count">${info.count}</span>
            <button class="btn-block" data-action="blocca" data-dominio="${da}" title="Blocca">X</button>
        </div>`;
    }).join('');
}

/**
 * Variante di `renderListaDomini` per la sezione "Bloccati": mostra anche
 * i domini in blocklist ma non ancora visti nel traffico (count=0).
 * @param {string} elId
 * @param {Array<[string, {count:number, tipo:string}]>} items
 */
function renderListaBloccati(elId, items) {
    const el = $(elId);
    const renderRiga = (dominio, count) => {
        const d = escapeHtml(dominio);
        const da = attrEscape(dominio);
        const hidden = matchFiltro(dominio) ? '' : ' filtro-hidden';
        return `<div class="dominio-item blocked-item${hidden}">
            <span class="nome">${d}</span>
            <span class="count">${count}</span>
            <button class="btn-unblock" data-action="sblocca" data-dominio="${da}" title="Sblocca">OK</button>
        </div>`;
    };
    let html = items.map(([d, info]) => renderRiga(d, info.count)).join('');
    const visti = new Set(items.map(([d]) => d));
    for (const b of state.bloccati) {
        if (!visti.has(b)) html += renderRiga(b, 0);
    }
    el.innerHTML = html;
}

// ========================================================================
// Stat row + stato sessione
// ========================================================================

/**
 * Conta le entry "attive" (utente + ai), escludendo il traffico di sistema
 * (telemetria, ad tech, ecc.). Rationale: il rumore di background falserebbe
 * il numero di richieste per studente mostrato in UI.
 * @param {import('./state.js').Entry[]} entries
 * @returns {number}
 */
function contaAttive(entries) {
    let n = 0;
    for (const e of entries) if (e.tipo !== 'sistema') n++;
    return n;
}

/**
 * Aggiorna la stat row (5 card) e l'etichetta "MODO / SESSIONE FERMA / IN PAUSA".
 * In focus mode conta solo le entry dell'IP selezionato. La durata si
 * congela quando la sessione e' ferma (usa `sessioneFineISO` al posto di now).
 */
export function renderStats() {
    const ips = state.focusIp ? 1 : state.perIp.size;
    const fonte = state.focusIp ? (state.perIp.get(state.focusIp) || []) : state.entries;
    $('stat-richieste').textContent = contaAttive(fonte);
    $('stat-domini').textContent = state.perDominio.size;
    $('stat-ip').textContent = ips;

    if (state.sessioneInizio) {
        const fine = state.sessioneAttiva
            ? Date.now()
            : (state.sessioneFineISO ? new Date(state.sessioneFineISO).getTime() : Date.now());
        const sec = Math.max(0, Math.floor((fine - new Date(state.sessioneInizio).getTime()) / 1000));
        $('stat-durata').textContent = formatDurata(sec);
    } else {
        $('stat-durata').textContent = '0:00';
    }

    const modoEl = $('stat-modo');
    let label = state.cfg.modo === 'allowlist' ? 'MODO: ALLOW' : 'MODO: BLOCK';
    if (state.pausato) label = 'IN PAUSA';
    if (!state.sessioneAttiva) label = 'SESSIONE FERMA';
    modoEl.textContent = label;
}

/**
 * Aggiorna label/classe dei bottoni Pausa e Avvia/Ferma sessione in base
 * allo stato, e mostra l'indicatore PAUSA in topbar se attivo.
 */
export function renderPausaEBottoni() {
    const btn = $('btn-pausa');
    const ind = $('pausa-indicator');
    if (state.pausato) {
        btn.textContent = 'Riprendi';
        btn.classList.add('attivo');
        ind.classList.remove('hidden');
    } else {
        btn.textContent = 'Pausa';
        btn.classList.remove('attivo');
        ind.classList.add('hidden');
    }

    const btnSes = $('btn-sessione');
    if (btnSes) {
        if (state.sessioneAttiva) {
            btnSes.textContent = 'Ferma sessione';
            btnSes.classList.remove('btn-primary');
            btnSes.classList.add('btn-danger');
        } else {
            btnSes.textContent = 'Avvia sessione';
            btnSes.classList.remove('btn-danger');
            btnSes.classList.add('btn-primary');
        }
    }
}

// ========================================================================
// Countdown deadline
// ========================================================================

/**
 * Aggiorna la cella countdown in topbar. Invocata sia dentro `renderAll`
 * sia dal suo `setInterval(1000)` indipendente.
 * Classi applicate:
 * - `warning` se < 5 min
 * - `critical` se < 1 min (aggiunge anche blink)
 * - `scaduto` se tempo esaurito
 */
export function renderCountdown() {
    const el = $('countdown-display');
    if (!state.deadlineISO) {
        el.textContent = '';
        el.className = 'countdown';
        return;
    }
    const msLeft = new Date(state.deadlineISO).getTime() - Date.now();
    if (msLeft <= 0) {
        el.textContent = 'SCADUTO';
        el.className = 'countdown scaduto';
        return;
    }
    const secLeft = Math.floor(msLeft / 1000);
    const h = Math.floor(secLeft / 3600);
    const m = Math.floor((secLeft % 3600) / 60);
    const s = secLeft % 60;
    const tempo = h > 0
        ? `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
        : `${m}:${String(s).padStart(2, '0')}`;
    el.textContent = tempo + ' rimanenti';
    el.className = 'countdown' + (secLeft < 300 ? ' warning' : '') + (secLeft < 60 ? ' critical' : '');
}

// ========================================================================
// Tabella/Griglia IP (Live tab, pannello principale)
// ========================================================================

/**
 * Determina stato + label del dot watchdog per un IP.
 * @param {string} ip
 * @returns {{classe:'verde'|'giallo'|'rosso'|'grigio', titolo:string}}
 */
function statoWatchdog(ip) {
    const ts = state.aliveMap.get(ip);
    if (!ts) return { classe: 'grigio', titolo: 'Watchdog mai visto' };
    const age = Date.now() - ts;
    if (age < 15000) return { classe: 'verde', titolo: `Attivo (${Math.round(age/1000)}s fa)` };
    if (age < 60000) return { classe: 'giallo', titolo: `Ritardo (${Math.round(age/1000)}s fa)` };
    return { classe: 'rosso', titolo: `OFFLINE da ${Math.round(age/1000)}s - possibile bypass` };
}

/**
 * Calcola i dati derivati per una singola riga/card IP.
 * Condiviso tra le due modalita' di rendering (lista e griglia).
 *
 * @param {string} ip
 * @param {number} ora - `Date.now()` al momento del render (passato per coerenza tra righe).
 * @param {number} soglia - Millisecondi di inattivita' per considerare l'IP inattivo.
 * @returns {{
 *   lista: import('./state.js').Entry[],
 *   listaAttive: import('./state.js').Entry[],
 *   dominiMap: Map<string, string>,
 *   diffSec: number,
 *   inattivo: boolean,
 *   nome: string|null,
 *   wd: {classe:string, titolo:string}
 * }}
 */
function calcolaStatoIp(ip, ora, soglia) {
    const lista = state.perIp.get(ip) || [];
    const listaAttive = lista.filter(e => e.tipo !== 'sistema');
    const dominiMap = new Map();
    for (const e of listaAttive) dominiMap.set(e.dominio, e.tipo);
    const ultimaStr = listaAttive.length > 0 ? listaAttive[listaAttive.length - 1].ora : null;
    const ultimaDate = ultimaStr ? parseOra(ultimaStr) : null;
    const diffSec = ultimaDate ? Math.floor((ora - ultimaDate.getTime()) / 1000) : 0;
    const inattivo = ultimaDate && (ora - ultimaDate.getTime()) > soglia;
    const nome = nomeStudente(ip);
    const wd = statoWatchdog(ip);
    return { lista, listaAttive, dominiMap, diffSec, inattivo, nome, wd };
}

/**
 * Genera l'HTML di un tag dominio cliccabile. Applica le classi
 * `blocked` o `nascosto` se presenti nel rispettivo set.
 * @param {string} d
 * @param {string} tipo
 * @returns {string}
 */
function tagDominioHtml(d, tipo) {
    const classi = ['dominio-tag', tipo];
    if (state.bloccati.has(d)) classi.push('blocked');
    else if (state.nascosti.has(d)) classi.push('nascosto');
    return `<span class="${classi.join(' ')}" data-action="blocca" data-dominio="${attrEscape(d)}" title="Click per bloccare">${escapeHtml(d)}</span>`;
}

/**
 * Dispatcher del pannello IP: sceglie tra vista lista e vista griglia
 * in base a `state.vistaIp` e aggiorna lo stato dei due bottoni toggle.
 *
 * L'unione `perIp.keys() + aliveMap.keys()` assicura che anche gli IP che
 * pingano ma non hanno ancora generato traffico compaiano nella lista
 * (riga/card con 0 richieste + dot watchdog).
 */
export function renderTabellaIp() {
    const container = $('ip-container');
    if (!container) return;
    const tuttiIps = new Set([...state.perIp.keys(), ...state.aliveMap.keys()]);
    const ips = [...tuttiIps].sort((a, b) => ip2long(a) - ip2long(b));
    const ora = Date.now();
    const soglia = state.cfg.inattivitaSogliaSec * 1000;

    const btnG = $('btn-vista-griglia');
    const btnL = $('btn-vista-lista');
    if (btnG) btnG.classList.toggle('attivo', state.vistaIp === 'griglia');
    if (btnL) btnL.classList.toggle('attivo', state.vistaIp === 'lista');

    if (state.vistaIp === 'lista') {
        container.innerHTML = renderListaIp(ips, ora, soglia);
    } else {
        container.innerHTML = renderGrigliaIp(ips, ora, soglia);
    }
}

/**
 * Costruisce l'HTML della vista a tabella (5 colonne: WD / Studente / N / Ultima / Domini).
 * @param {string[]} ips
 * @param {number} ora
 * @param {number} soglia
 * @returns {string}
 */
function renderListaIp(ips, ora, soglia) {
    const righe = ips.map(ip => {
        const s = calcolaStatoIp(ip, ora, soglia);
        const tagsHtml = [...s.dominiMap.entries()].map(([d, t]) => tagDominioHtml(d, t)).join('');
        const label = s.nome
            ? `<span class="nome-studente">${escapeHtml(s.nome)}</span> <span class="ip-sub">${escapeHtml(ip)}</span>`
            : `<span class="ip-label">${escapeHtml(ip)}</span>`;
        const wdDot = `<span class="watchdog-dot ${s.wd.classe}" title="${escapeHtml(s.wd.titolo)}"></span>`;
        const ultimaCella = s.listaAttive.length > 0 ? formatRelativo(s.diffSec) : '<span class="ip-sub">-</span>';
        const rowClass = [];
        if (s.inattivo) rowClass.push('inattivo');
        if (state.focusIp === ip) rowClass.push('focus');
        if (state.filtro && !matchFiltro(`${s.nome || ''} ${ip}`)) rowClass.push('filtro-hidden');
        return `<tr class="${rowClass.join(' ')}" data-action="focus-ip" data-ip="${attrEscape(ip)}">
            <td>${wdDot}</td>
            <td>${label}</td>
            <td>${s.listaAttive.length}</td>
            <td><span class="ultima-attivita${s.inattivo ? ' inattivo' : ''}">${ultimaCella}</span></td>
            <td>${tagsHtml}</td>
        </tr>`;
    }).join('');
    return `<table>
        <thead><tr><th title="Watchdog">WD</th><th>Studente / IP</th><th>N</th><th>Ultima</th><th>Domini</th></tr></thead>
        <tbody>${righe}</tbody>
    </table>`;
}

/** Numero massimo di tag dominio mostrati dentro una singola card della griglia. */
const DOMINI_CARD_MAX = 6;

/**
 * Costruisce l'HTML della vista a griglia (card per IP).
 * @param {string[]} ips
 * @param {number} ora
 * @param {number} soglia
 * @returns {string}
 */
function renderGrigliaIp(ips, ora, soglia) {
    if (ips.length === 0) return '<div class="ip-grid-vuota">Nessun IP ancora rilevato.</div>';
    const card = ips.map(ip => {
        const s = calcolaStatoIp(ip, ora, soglia);
        const dominiOrdinati = [...s.dominiMap.entries()].reverse();
        const dominiVisibili = dominiOrdinati.slice(0, DOMINI_CARD_MAX);
        const extra = dominiOrdinati.length - dominiVisibili.length;
        const tagsHtml = dominiVisibili.map(([d, t]) => tagDominioHtml(d, t)).join('');
        const extraHtml = extra > 0 ? `<span class="ip-card-extra">+${extra}</span>` : '';
        const ultimaTxt = s.listaAttive.length > 0 ? formatRelativo(s.diffSec) : '-';

        const classi = ['ip-card'];
        if (s.inattivo) classi.push('inattivo');
        if (state.focusIp === ip) classi.push('focus');
        if (state.filtro && !matchFiltro(`${s.nome || ''} ${ip}`)) classi.push('filtro-hidden');

        const nomeHtml = s.nome
            ? `<div class="ip-card-nome">${escapeHtml(s.nome)}</div><div class="ip-card-ip">${escapeHtml(ip)}</div>`
            : `<div class="ip-card-nome ip-card-nome-solo">${escapeHtml(ip)}</div>`;

        return `<div class="${classi.join(' ')}" data-action="focus-ip" data-ip="${attrEscape(ip)}">
            <div class="ip-card-head">
                <span class="watchdog-dot ${s.wd.classe}" title="${escapeHtml(s.wd.titolo)}"></span>
                ${nomeHtml}
            </div>
            <div class="ip-card-metriche">
                <div class="ip-card-num">${s.listaAttive.length}</div>
                <div class="ip-card-ultima${s.inattivo ? ' inattivo' : ''}">${ultimaTxt}</div>
            </div>
            <div class="ip-card-tags">${tagsHtml}${extraHtml}</div>
        </div>`;
    }).join('');
    return `<div class="ip-grid">${card}</div>`;
}

/**
 * Riempie il pannello "Ultime richieste" con le ultime N entry in ordine
 * temporale inverso. In focus mode mostra solo le entry dell'IP selezionato.
 */
export function renderUltimeRichieste() {
    const el = $('lista-richieste');
    let fonte = state.entries;
    if (state.focusIp) fonte = state.perIp.get(state.focusIp) || [];
    const ultime = fonte.slice(-MAX_RICHIESTE_UI).reverse();

    el.innerHTML = ultime.map(e => {
        const aiClass = e.tipo === 'ai' ? 'ai-alert' : '';
        const nome = nomeStudente(e.ip);
        const ipLabel = nome ? `${nome} .${e.ip.split('.').pop()}` : e.ip;
        const match = matchFiltro(e.dominio) || matchFiltro(e.ip) || (nome && matchFiltro(nome));
        const hidden = match ? '' : ' filtro-hidden';
        return `<div class="traffico-entry${hidden}">
            <span class="orario">${e.ora.substring(11)}</span>
            <span class="ip-label">[${escapeHtml(ipLabel)}]</span>
            <span class="dominio-txt ${aiClass}">${escapeHtml(e.dominio)}</span>
        </div>`;
    }).join('');
}

/**
 * Aggiorna il titolo del pannello IP: "Traffico per IP" o "Focus: NOME (ip)"
 * con bottone di clear quando un IP e' in focus.
 */
export function renderFocus() {
    const titolo = $('panel-ip-titolo');
    if (state.focusIp) {
        const nome = nomeStudente(state.focusIp);
        const label = nome ? `${nome} (${state.focusIp})` : state.focusIp;
        titolo.innerHTML = `Focus: ${escapeHtml(label)} <span class="focus-bar">filtrato <button data-action="focus-clear">X</button></span>`;
    } else {
        titolo.textContent = 'Traffico per IP';
    }
}

// ========================================================================
// Tab management + controlli minori
// ========================================================================

/** Attiva il tab e il pannello corrispondenti a `state.tabAttivo`. */
export function renderTabs() {
    document.querySelectorAll('.tab-btn').forEach(b => {
        b.classList.toggle('active', b.dataset.tab === state.tabAttivo);
    });
    document.querySelectorAll('.tab-panel').forEach(p => {
        p.classList.toggle('active', p.id === 'tab-' + state.tabAttivo);
    });
}

/**
 * Rigenera le opzioni del select preset preservando l'opzione selezionata.
 * Chiamata esplicitamente dopo salvaPreset (non parte di renderAll perche'
 * e' usato anche prima del primo render in init).
 */
export function aggiornaSelectPresets() {
    const sel = $('preset-select');
    const val = sel.value;
    sel.innerHTML = '<option value="">-- Preset --</option>'
        + state.cfg.presets.map(p => `<option value="${attrEscape(p)}">${escapeHtml(p)}</option>`).join('');
    sel.value = val;
}

/**
 * Aggiorna i due bottoni toggle (tema + notifiche) in topbar:
 * - Icona riflette lo stato attuale: ☀️/🌙, 🔔/🔕.
 * - Background verde `.attivo` solo sul bottone notifiche (il cambio
 *   campana/campana-barrata e' sottile, aiuta evidenziarlo).
 * - Classe `dark` applicata/rimossa da `<body>`.
 */
export function aggiornaToggleButtons() {
    const btnT = $('btn-darkmode');
    const btnN = $('btn-notifiche');
    btnT.textContent = state.darkmode ? '☀️' : '🌙';
    btnN.textContent = state.notifiche ? '🔔' : '🔕';
    btnN.classList.toggle('attivo', state.notifiche);
    document.body.classList.toggle('dark', state.darkmode);
}

/** Sincronizza il valore dell'input time con `state.deadlineISO`. */
export function aggiornaInputDeadline() {
    const input = $('input-deadline');
    if (!state.deadlineISO) { input.value = ''; return; }
    const d = new Date(state.deadlineISO);
    const hh = String(d.getHours()).padStart(2, '0');
    const mm = String(d.getMinutes()).padStart(2, '0');
    input.value = `${hh}:${mm}`;
}

// ========================================================================
// Tab Report (riepiloghi + grafici a barre)
// ========================================================================

/**
 * Aggiorna il tab Report. Opera sulla sessione corrente OPPURE su un
 * archivio caricato (`state.datiSessioneVisualizzata`). La durata viene
 * calcolata differentemente nei tre casi:
 * - archivio: esportatoAlle - sessioneInizio
 * - corrente attiva: now - sessioneInizio
 * - corrente ferma: sessioneFineISO - sessioneInizio (congelata)
 *
 * "Top studenti" usa `perIpAttive` (escluso sistema) per riflettere
 * l'attivita' reale, non il rumore di telemetria.
 */
export function renderReport() {
    const tab = $('tab-report');
    if (!tab.classList.contains('active') && state.tabAttivo !== 'report') return;

    const usaArchivio = !!state.datiSessioneVisualizzata;
    const entries = usaArchivio ? state.datiSessioneVisualizzata.entries : state.entries;
    const sessioneInizio = usaArchivio ? state.datiSessioneVisualizzata.sessioneInizio : state.sessioneInizio;
    const bloccatiList = usaArchivio ? (state.datiSessioneVisualizzata.bloccati || []) : [...state.bloccati];
    const studentiMap = usaArchivio ? (state.datiSessioneVisualizzata.studenti || {}) : state.cfg.studenti;

    const titoloEl = $('report-titolo');
    if (usaArchivio) {
        const d = new Date(sessioneInizio);
        titoloEl.textContent = `Archivio: ${d.toLocaleString('it-IT')}`;
        $('btn-elimina-sessione').disabled = false;
    } else {
        titoloEl.textContent = 'Report sessione corrente';
        $('btn-elimina-sessione').disabled = true;
    }

    const agg = aggregaPerReport(entries);
    const fine = usaArchivio
        ? new Date(state.datiSessioneVisualizzata.esportatoAlle || Date.now()).getTime()
        : (state.sessioneAttiva ? Date.now() : (state.sessioneFineISO ? new Date(state.sessioneFineISO).getTime() : Date.now()));
    const durataSec = sessioneInizio
        ? Math.max(0, Math.floor((fine - new Date(sessioneInizio).getTime()) / 1000))
        : 0;

    const riepilogo = $('report-riepilogo');
    const pctBloccate = agg.totale > 0 ? Math.round((agg.bloccate / agg.totale) * 100) : 0;
    riepilogo.innerHTML = `
        <dt>Inizio</dt><dd>${escapeHtml(new Date(sessioneInizio || Date.now()).toLocaleString('it-IT'))}</dd>
        <dt>Durata</dt><dd>${formatDurata(durataSec)}</dd>
        <dt>Richieste totali</dt><dd>${agg.totale}</dd>
        <dt>Richieste bloccate</dt><dd>${agg.bloccate} (${pctBloccate}%)</dd>
        <dt>Domini contattati</dt><dd>${agg.perDominio.size}</dd>
        <dt>IP attivi</dt><dd>${agg.perIp.size}</dd>
        <dt>Richieste AI</dt><dd>${agg.perTipo.ai || 0}</dd>
        <dt>Richieste utente</dt><dd>${agg.perTipo.utente || 0}</dd>
        <dt>Richieste sistema</dt><dd>${agg.perTipo.sistema || 0}</dd>
        <dt>In blocklist</dt><dd>${bloccatiList.length}</dd>
    `;

    const dominiOrdinati = [...agg.perDominio.entries()].sort((a, b) => b[1].count - a[1].count);
    const soloAI = dominiOrdinati.filter(([, info]) => info.tipo === 'ai').slice(0, 10);
    $('report-top-ai').innerHTML = soloAI.length > 0
        ? renderBarre(soloAI.map(([d, i]) => [d, i.count]), true)
        : '<p class="hint">Nessuna richiesta AI in questa sessione.</p>';

    const top10 = dominiOrdinati.slice(0, 10).map(([d, i]) => [d, i.count]);
    $('report-top-domini').innerHTML = top10.length > 0
        ? renderBarre(top10, false)
        : '<p class="hint">Nessuna richiesta.</p>';

    const ipOrdinati = [...agg.perIpAttive.entries()].sort((a, b) => b[1] - a[1]).slice(0, 20);
    const barreStudenti = ipOrdinati.map(([ip, n]) => {
        const nome = studentiMap[ip];
        const label = nome ? `${nome} (${ip})` : ip;
        return [label, n];
    });
    $('report-top-studenti').innerHTML = barreStudenti.length > 0
        ? renderBarre(barreStudenti, false)
        : '<p class="hint">Nessuna attivita\'.</p>';
}

/**
 * Genera un grafico a barre orizzontali. La barra piu' lunga e' al 100%,
 * le altre in proporzione.
 * @param {Array<[string, number]>} items - Coppie [label, count].
 * @param {boolean} isAi - Se true, applica uno styling rosso distintivo.
 * @returns {string}
 */
function renderBarre(items, isAi) {
    if (items.length === 0) return '';
    const max = Math.max(...items.map(([, n]) => n));
    return items.map(([label, n]) => {
        const pct = Math.round((n / max) * 100);
        const onDark = pct > 55;
        return `<div class="report-bar${isAi ? ' ai' : ''}">
            <div class="barra-wrap">
                <div class="barra" style="width:${pct}%"></div>
                <span class="label${onDark ? ' on-dark' : ''}">${escapeHtml(label)}</span>
            </div>
            <span class="count">${n}</span>
        </div>`;
    }).join('');
}

// ========================================================================
// Tab Impostazioni
// ========================================================================

/**
 * Naviga un oggetto tramite un path dotted (es. "web.auth.enabled").
 * Safe rispetto a segmenti mancanti.
 * @param {Object} obj
 * @param {string} path
 */
function getDeep(obj, path) {
    return path.split('.').reduce((o, k) => (o == null ? o : o[k]), obj);
}

/**
 * Aggiorna il valore di un singolo campo settings se l'utente NON lo sta
 * attualmente editando (altrimenti perderebbe il typing).
 * Caso speciale password: non la inietta mai; imposta solo il placeholder
 * a seconda di `passwordSet`.
 */
function aggiornaSettingsInput(el, val) {
    if (document.activeElement === el) return;
    if (el.type === 'checkbox') el.checked = !!val;
    else if (el.type === 'password') {
        el.placeholder = val === '' && state.settings?.web?.auth?.passwordSet ? '(impostata — scrivi per cambiare)' : '(non impostata)';
    }
    else el.value = val ?? '';
}

/**
 * Rigenera il tab Impostazioni: sincronizza il form settings, banner
 * "riavvio richiesto", lista domini ignorati, mappa studenti, dropdown
 * combo classe+lab, lista sessioni archiviate.
 */
export function renderImpostazioni() {
    if (state.tabAttivo !== 'impostazioni') return;

    if (state.settings) {
        document.querySelectorAll('[data-action="settings-field"]').forEach(el => {
            const key = el.dataset.key;
            const val = getDeep(state.settings, key);
            aggiornaSettingsInput(el, val);
        });
    }
    const banner = $('riavvio-banner');
    if (banner) banner.classList.toggle('hidden', !state.riavvioRichiesto);

    renderIgnorati();
    renderMappaStudenti();

    const sessioniEl = $('sessioni-list');
    const select = $('report-sessione-select');
    const valSel = select.value;
    select.innerHTML = '<option value="">-- Sessione corrente --</option>'
        + state.sessioniArchivio.map(s => `<option value="${attrEscape(s)}">${escapeHtml(s.replace(/\.json$/, ''))}</option>`).join('');
    select.value = valSel;

    sessioniEl.innerHTML = state.sessioniArchivio.length > 0
        ? state.sessioniArchivio.map(s => `<li data-action="sessione-apri" data-nome="${attrEscape(s)}">
            <span class="nome">${escapeHtml(s.replace(/\.json$/, ''))}</span>
            <button class="btn btn-danger" data-action="sessione-elimina" data-nome="${attrEscape(s)}">Elimina</button>
        </li>`).join('')
        : '<li class="hint">Archivio vuoto. Ogni "Nuova sessione" archivia la precedente.</li>';
}

/** Rigenera la lista dei domini ignorati nel tab Impostazioni. */
function renderIgnorati() {
    const el = $('ignorati-list');
    if (!el) return;
    const lista = state.settings?.dominiIgnorati || [];
    el.innerHTML = lista.length > 0
        ? lista.map(d => `<li>
            <span class="dominio">${escapeHtml(d)}</span>
            <button class="btn btn-danger" data-action="rimuovi-ignorato" data-dominio="${attrEscape(d)}">X</button>
        </li>`).join('')
        : '<li class="hint">Nessun dominio ignorato.</li>';
}

/**
 * Rigenera la tabella mappa studenti preservando il focus/selezione se
 * l'utente sta attualmente editando un input. Senza questa preservazione,
 * ogni SSE `studenti` broadcast (inclusi quelli triggerati dal typing
 * dell'utente stesso) ruberebbe il focus a meta' parola.
 */
function renderMappaStudenti() {
    const tbody = $('studenti-tbody');
    if (!tbody) return;

    const active = document.activeElement;
    const activeIp = (active && active.classList.contains('edit-studente')) ? active.dataset.ip : null;
    const activeSel = activeIp ? [active.selectionStart, active.selectionEnd] : null;

    const entries = Object.entries(state.cfg.studenti || {}).sort(([a], [b]) => ip2long(a) - ip2long(b));
    $('count-studenti').textContent = entries.length;

    tbody.innerHTML = entries.length > 0
        ? entries.map(([ip, nome]) => `<tr>
            <td class="col-ip">${escapeHtml(ip)}</td>
            <td class="col-nome"><input type="text" class="edit-studente" data-action="edit-studente" data-ip="${attrEscape(ip)}" value="${attrEscape(nome)}"></td>
            <td class="col-azioni"><button class="btn-block" data-action="elimina-studente" data-ip="${attrEscape(ip)}" title="Elimina">X</button></td>
        </tr>`).join('')
        : '<tr><td colspan="3" class="hint-cell">Nessuno studente mappato. Aggiungi una riga sotto o carica una classe.</td></tr>';

    if (activeIp) {
        const nuovoInput = tbody.querySelector(`.edit-studente[data-ip="${CSS.escape(activeIp)}"]`);
        if (nuovoInput) {
            nuovoInput.focus();
            if (activeSel) nuovoInput.setSelectionRange(activeSel[0], activeSel[1]);
        }
    }

    renderSelectCombo();
}

/**
 * Rigenera i due dropdown classe/lab dalla lista `state.cfg.classi`.
 * Abilita Load/Delete solo se la combinazione selezionata esiste in archivio.
 */
function renderSelectCombo() {
    const tutte = state.cfg.classi || [];
    const classi = [...new Set(tutte.map(c => c.classe))].sort();
    const lab = [...new Set(tutte.map(c => c.lab))].sort();

    const selClasse = $('sel-classe');
    const selLab = $('sel-lab');
    if (!selClasse || !selLab) return;

    const valClasse = selClasse.value;
    const valLab = selLab.value;

    selClasse.innerHTML = '<option value="">-- Classe --</option>'
        + classi.map(c => `<option value="${attrEscape(c)}">${escapeHtml(c)}</option>`).join('');
    selClasse.value = classi.includes(valClasse) ? valClasse : '';

    selLab.innerHTML = '<option value="">-- Laboratorio --</option>'
        + lab.map(l => `<option value="${attrEscape(l)}">${escapeHtml(l)}</option>`).join('');
    selLab.value = lab.includes(valLab) ? valLab : '';

    const esiste = selClasse.value && selLab.value
        && tutte.some(c => c.classe === selClasse.value && c.lab === selLab.value);
    $('btn-combo-load').disabled = !esiste;
    $('btn-combo-delete').disabled = !esiste;
}

// ========================================================================
// Render completo (throttled)
// ========================================================================

/** Esegue tutti i renderer sincronamente. Chiamato da `renderAll` dentro RAF. */
function _renderAllSync() {
    renderSidebar();
    renderStats();
    renderPausaEBottoni();
    renderTabellaIp();
    renderUltimeRichieste();
    renderFocus();
    renderReport();
    renderImpostazioni();
    renderCountdown();
}

let rafPending = false;

/**
 * Richiede un ri-render completo coalescente per frame. Chiamate multiple
 * nello stesso frame diventano un singolo render — essenziale durante burst
 * SSE (es. 20 studenti che generano traffico simultaneamente).
 */
export function renderAll() {
    if (rafPending) return;
    rafPending = true;
    requestAnimationFrame(() => {
        rafPending = false;
        _renderAllSync();
    });
}

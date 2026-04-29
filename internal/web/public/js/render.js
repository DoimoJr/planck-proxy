/**
 * @file Funzioni di rendering della UI.
 *
 * Contratto: le funzioni esportate sono pure rispetto allo `state`
 * (leggono, non mutano — vedi `actions.js` / `sse.js` per le mutazioni).
 *
 * Strategia anti-flicker: le liste frequentemente aggiornate (sidebar
 * domini, ultime richieste, tabella IP in entrambe le viste) usano
 * `syncChildren` — riconciliazione keyed dei figli DOM. I nodi vengono
 * riutilizzati tra un render e l'altro: cambia solo cio' che e' davvero
 * cambiato (textContent di un counter, classi per stato), preservando
 * scroll, hover e focus anche sotto raffiche di aggiornamenti.
 *
 * Coalescing: `renderAll()` e' guardato da `requestAnimationFrame` — piu'
 * chiamate nello stesso frame diventano un singolo paint. In aggiunta,
 * `sse.js` batcha le entries `traffic` ogni ~250ms, abbattendo i render
 * da decine al secondo a ~4/s durante burst (20+ studenti attivi).
 *
 * Eccezione: `renderCountdown()` ha il suo `setInterval(1000)` separato —
 * aggiorna solo la cella countdown, no-alloc per il ticker a 1Hz.
 */

import { state, nomeStudente, aggregaPerReport } from './state.js';
import { $, escapeHtml, attrEscape, ip2long, parseOra, formatDurata, formatRelativo, syncChildren } from './util.js';

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
 * Rigenera il contenuto di una lista domini dentro la sidebar usando
 * `syncChildren`: nodi riutilizzati per dominio, solo `count` e classi
 * cambiano al volo. Evita il flicker da innerHTML-wipe.
 *
 * @param {string} elId - ID del contenitore.
 * @param {Array<[string, {count:number, tipo:string}]>} items
 * @param {string} tipoClass - Classe CSS del tipo ("ai" | "utente" | "sistema" | "nascosto").
 * @param {boolean} isNascosto - Se true, l'azione principale del click e' "mostra" invece di "nascondi".
 */
function renderListaDomini(elId, items, tipoClass, isNascosto) {
    const el = $(elId);
    if (!el) return;
    const azionePrincipale = isNascosto ? 'mostra-dominio' : 'nascondi-dominio';
    syncChildren(el, items,
        ([d]) => d,
        ([dominio]) => {
            const div = document.createElement('div');
            div.dataset.action = azionePrincipale;
            div.dataset.dominio = dominio;
            const nome = document.createElement('span');
            nome.className = 'nome';
            nome.textContent = dominio;
            const count = document.createElement('span');
            count.className = 'count';
            const btn = document.createElement('button');
            btn.className = 'btn-block';
            btn.dataset.action = 'blocca';
            btn.dataset.dominio = dominio;
            btn.title = 'Blocca';
            btn.textContent = 'X';
            div.append(nome, count, btn);
            return div;
        },
        (div, [dominio, info]) => {
            const extra = isNascosto ? ' nascosto-item' : '';
            const hidden = matchFiltro(dominio) ? '' : ' filtro-hidden';
            div.className = `dominio-item ${tipoClass}${extra}${hidden}`;
            const countEl = div.querySelector('.count');
            const nuovo = String(info.count);
            if (countEl.textContent !== nuovo) countEl.textContent = nuovo;
        }
    );
}

/**
 * Variante di `renderListaDomini` per la sezione "Bloccati": mostra anche
 * i domini in blocklist ma non ancora visti nel traffico (count=0).
 * @param {string} elId
 * @param {Array<[string, {count:number, tipo:string}]>} items
 */
function renderListaBloccati(elId, items) {
    const el = $(elId);
    if (!el) return;
    const rows = items.map(([d, info]) => ({ dominio: d, count: info.count }));
    const visti = new Set(items.map(([d]) => d));
    for (const b of state.bloccati) {
        if (!visti.has(b)) rows.push({ dominio: b, count: 0 });
    }
    syncChildren(el, rows,
        r => r.dominio,
        r => {
            const div = document.createElement('div');
            const nome = document.createElement('span');
            nome.className = 'nome';
            nome.textContent = r.dominio;
            const count = document.createElement('span');
            count.className = 'count';
            const btn = document.createElement('button');
            btn.className = 'btn-unblock';
            btn.dataset.action = 'sblocca';
            btn.dataset.dominio = r.dominio;
            btn.title = 'Sblocca';
            btn.textContent = 'OK';
            div.append(nome, count, btn);
            return div;
        },
        (div, r) => {
            const hidden = matchFiltro(r.dominio) ? '' : ' filtro-hidden';
            div.className = `dominio-item blocked-item${hidden}`;
            const countEl = div.querySelector('.count');
            const nuovo = String(r.count);
            if (countEl.textContent !== nuovo) countEl.textContent = nuovo;
        }
    );
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
    // Card visibili = unione di:
    //   - IP della mappa studenti (anche se nessun traffico ne' watchdog ping)
    //     → permette di inviare comandi Veyon prima della distribuzione del proxy
    //   - IP che hanno generato traffico (perIp)
    //   - IP che hanno pingato il watchdog (aliveMap)
    const tuttiIps = new Set([
        ...Object.keys(state.cfg.studenti || {}),
        ...state.perIp.keys(),
        ...state.aliveMap.keys(),
    ]);
    const ips = [...tuttiIps].sort((a, b) => ip2long(a) - ip2long(b));
    const ora = Date.now();
    const soglia = state.cfg.inattivitaSogliaSec * 1000;

    const btnG = $('btn-vista-griglia');
    const btnL = $('btn-vista-lista');
    if (btnG) btnG.classList.toggle('attivo', state.vistaIp === 'griglia');
    if (btnL) btnL.classList.toggle('attivo', state.vistaIp === 'lista');

    if (state.vistaIp === 'lista') {
        renderListaIp(container, ips, ora, soglia);
    } else {
        renderGrigliaIp(container, ips, ora, soglia);
    }
}

/**
 * Garantisce lo "scheletro" del contenitore per una certa vista. Se la
 * vista cambia (o il contenitore e' vuoto), ricostruisce l'involucro e
 * ritorna il nodo in cui inserire le righe/card. Per vista uguale,
 * restituisce l'involucro gia' presente (senza toccare il DOM).
 *
 * @param {HTMLElement} container
 * @param {string} vista
 * @returns {HTMLElement} target su cui chiamare syncChildren
 */
function scheletroVistaIp(container, vista) {
    if (container.dataset.vista === vista && container.firstElementChild) {
        if (vista === 'lista') return container.firstElementChild.querySelector('tbody');
        return container.firstElementChild;
    }
    container.textContent = '';
    container.dataset.vista = vista;
    if (vista === 'lista') {
        const table = document.createElement('table');
        table.innerHTML = '<thead><tr><th title="Watchdog">WD</th><th>Studente / IP</th><th>N</th><th>Ultima</th><th>Domini</th></tr></thead><tbody></tbody>';
        container.appendChild(table);
        return table.querySelector('tbody');
    }
    const grid = document.createElement('div');
    grid.className = 'ip-grid';
    container.appendChild(grid);
    return grid;
}

/**
 * Riempie un nodo con i tag dominio per un IP, riutilizzando gli span
 * esistenti (`syncChildren` per chiave = dominio). Un eventuale
 * `+N` finale viene gestito come nodo statico appeso dopo il sync.
 * @param {HTMLElement} container
 * @param {Array<[string,string]>} domini - Entries dominio -> tipo.
 * @param {number} extra - Quantita' "+N" da mostrare in coda, 0 per niente.
 */
function syncTagsDominio(container, domini, extra) {
    syncChildren(container, domini,
        ([d]) => d,
        ([d]) => {
            const span = document.createElement('span');
            span.dataset.action = 'blocca';
            span.dataset.dominio = d;
            span.title = 'Click per bloccare';
            span.textContent = d;
            return span;
        },
        (span, [d, tipo]) => {
            const classi = ['dominio-tag', tipo];
            if (state.bloccati.has(d)) classi.push('blocked');
            else if (state.nascosti.has(d)) classi.push('nascosto');
            const nuova = classi.join(' ');
            if (span.className !== nuova) span.className = nuova;
        }
    );
    // Gestione pastiglia "+N": ultimo child se presente, rimossa/aggiornata qui.
    let extraEl = container.querySelector(':scope > .ip-card-extra');
    if (extra > 0) {
        if (!extraEl) {
            extraEl = document.createElement('span');
            extraEl.className = 'ip-card-extra';
            container.appendChild(extraEl);
        } else {
            container.appendChild(extraEl);
        }
        const txt = `+${extra}`;
        if (extraEl.textContent !== txt) extraEl.textContent = txt;
    } else if (extraEl) {
        extraEl.remove();
    }
}

/**
 * Costruisce/aggiorna la vista a tabella nel tbody: una riga per IP,
 * riusa le `<tr>` esistenti tramite syncChildren.
 */
function renderListaIp(container, ips, ora, soglia) {
    const body = scheletroVistaIp(container, 'lista');
    syncChildren(body, ips,
        ip => ip,
        ip => {
            const tr = document.createElement('tr');
            tr.dataset.action = 'focus-ip';
            tr.dataset.ip = ip;
            tr.innerHTML = '<td><span class="watchdog-dot"></span></td>'
                + '<td></td>'
                + '<td class="col-n"></td>'
                + '<td><span class="ultima-attivita"></span></td>'
                + '<td class="col-tags"></td>';
            return tr;
        },
        (tr, ip) => {
            const s = calcolaStatoIp(ip, ora, soglia);
            const rowClass = [];
            if (s.inattivo) rowClass.push('inattivo');
            if (state.focusIp === ip) rowClass.push('focus');
            if (state.selectedIps.has(ip)) rowClass.push('selected');
            if (state.filtro && !matchFiltro(`${s.nome || ''} ${ip}`)) rowClass.push('filtro-hidden');
            const nuova = rowClass.join(' ');
            if (tr.className !== nuova) tr.className = nuova;

            const tds = tr.children;
            const wd = tds[0].firstElementChild;
            const wdClass = `watchdog-dot ${s.wd.classe}`;
            if (wd.className !== wdClass) wd.className = wdClass;
            if (wd.title !== s.wd.titolo) wd.title = s.wd.titolo;

            const labelTd = tds[1];
            // Ricostruisci solo se cambia la presenza del nome (cambio raro).
            const hasNome = labelTd.firstElementChild?.classList?.contains('nome-studente');
            if (!!s.nome !== !!hasNome || (s.nome && labelTd.firstElementChild.textContent !== s.nome)) {
                labelTd.textContent = '';
                if (s.nome) {
                    const a = document.createElement('span'); a.className = 'nome-studente'; a.textContent = s.nome;
                    const b = document.createElement('span'); b.className = 'ip-sub'; b.textContent = ip;
                    labelTd.append(a, ' ', b);
                } else {
                    const a = document.createElement('span'); a.className = 'ip-label'; a.textContent = ip;
                    labelTd.append(a);
                }
            }

            const nStr = String(s.listaAttive.length);
            if (tds[2].textContent !== nStr) tds[2].textContent = nStr;

            const ultimaSpan = tds[3].firstElementChild;
            const ultimaTxt = s.listaAttive.length > 0 ? formatRelativo(s.diffSec) : '-';
            if (ultimaSpan.textContent !== ultimaTxt) ultimaSpan.textContent = ultimaTxt;
            const ultimaCls = `ultima-attivita${s.inattivo ? ' inattivo' : ''}`;
            if (ultimaSpan.className !== ultimaCls) ultimaSpan.className = ultimaCls;

            syncTagsDominio(tds[4], [...s.dominiMap.entries()], 0);
        }
    );
}

/** Numero massimo di tag dominio mostrati dentro una singola card della griglia. */
const DOMINI_CARD_MAX = 6;

/**
 * Costruisce/aggiorna la vista a griglia (card per IP), con riuso nodi.
 */
function renderGrigliaIp(container, ips, ora, soglia) {
    const grid = scheletroVistaIp(container, 'griglia');
    if (ips.length === 0) {
        grid.textContent = '';
        if (!grid.querySelector(':scope > .ip-grid-vuota')) {
            const v = document.createElement('div');
            v.className = 'ip-grid-vuota';
            v.textContent = 'Nessun IP ancora rilevato.';
            grid.appendChild(v);
        }
        return;
    }
    // Rimuovi eventuale placeholder "vuoto" ereditato da un render precedente.
    const vuoto = grid.querySelector(':scope > .ip-grid-vuota');
    if (vuoto) vuoto.remove();

    syncChildren(grid, ips,
        ip => ip,
        ip => {
            const card = document.createElement('div');
            card.dataset.action = 'focus-ip';
            card.dataset.ip = ip;
            card.innerHTML = '<div class="ip-card-head">'
                + '<span class="watchdog-dot"></span>'
                + '<div class="nome-wrap"></div>'
                + '</div>'
                + '<div class="ip-card-metriche">'
                + '<div class="ip-card-num"></div>'
                + '<div class="ip-card-ultima"></div>'
                + '</div>'
                + '<div class="ip-card-tags"></div>'
                + '<div class="ip-card-veyon">'
                + '<button type="button" data-action="veyon-card-lock" data-ip="' + ip + '" title="Blocca schermo">🔒</button>'
                + '<button type="button" data-action="veyon-card-unlock" data-ip="' + ip + '" title="Sblocca schermo">🔓</button>'
                + '<button type="button" data-action="veyon-card-msg" data-ip="' + ip + '" title="Messaggio">💬</button>'
                + '</div>';
            return card;
        },
        (card, ip) => {
            const s = calcolaStatoIp(ip, ora, soglia);
            const classi = ['ip-card'];
            if (s.inattivo) classi.push('inattivo');
            if (state.focusIp === ip) classi.push('focus');
            if (state.selectedIps.has(ip)) classi.push('selected');
            if (state.filtro && !matchFiltro(`${s.nome || ''} ${ip}`)) classi.push('filtro-hidden');
            const nuova = classi.join(' ');
            if (card.className !== nuova) card.className = nuova;

            const head = card.firstElementChild;
            const wd = head.firstElementChild;
            const wdClass = `watchdog-dot ${s.wd.classe}`;
            if (wd.className !== wdClass) wd.className = wdClass;
            if (wd.title !== s.wd.titolo) wd.title = s.wd.titolo;

            const nomeWrap = head.children[1];
            const hasNome = nomeWrap.firstElementChild?.classList?.contains('ip-card-nome')
                && !nomeWrap.firstElementChild.classList.contains('ip-card-nome-solo');
            const needNome = !!s.nome;
            if (needNome !== hasNome || (needNome && nomeWrap.firstElementChild.textContent !== s.nome)) {
                nomeWrap.textContent = '';
                if (s.nome) {
                    const a = document.createElement('div'); a.className = 'ip-card-nome'; a.textContent = s.nome;
                    const b = document.createElement('div'); b.className = 'ip-card-ip'; b.textContent = ip;
                    nomeWrap.append(a, b);
                } else {
                    const a = document.createElement('div'); a.className = 'ip-card-nome ip-card-nome-solo'; a.textContent = ip;
                    nomeWrap.append(a);
                }
            }

            const metriche = card.children[1];
            const numEl = metriche.firstElementChild;
            const numStr = String(s.listaAttive.length);
            if (numEl.textContent !== numStr) numEl.textContent = numStr;
            const ultimaEl = metriche.children[1];
            const ultimaTxt = s.listaAttive.length > 0 ? formatRelativo(s.diffSec) : '-';
            if (ultimaEl.textContent !== ultimaTxt) ultimaEl.textContent = ultimaTxt;
            const ultimaCls = `ip-card-ultima${s.inattivo ? ' inattivo' : ''}`;
            if (ultimaEl.className !== ultimaCls) ultimaEl.className = ultimaCls;

            const tags = card.children[2];
            const dominiOrd = [...s.dominiMap.entries()].reverse();
            const visibili = dominiOrd.slice(0, DOMINI_CARD_MAX);
            const extra = dominiOrd.length - visibili.length;
            syncTagsDominio(tags, visibili, extra);
        }
    );
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

    // Chiave stabile: indice origine nel buffer (combinato con ora+ip+dominio
    // per robustezza in caso di reset). Le entries non mutano una volta
    // inserite, quindi `update` si limita a rivalutare il filtro.
    syncChildren(el, ultime,
        e => `${e.ora}|${e.ip}|${e.dominio}|${e.metodo}`,
        e => {
            const div = document.createElement('div');
            const nome = nomeStudente(e.ip);
            const ipLabel = nome ? `${nome} .${e.ip.split('.').pop()}` : e.ip;
            const aiClass = e.tipo === 'ai' ? ' ai-alert' : '';
            const oraSpan = document.createElement('span');
            oraSpan.className = 'orario';
            oraSpan.textContent = e.ora.substring(11);
            const ipSpan = document.createElement('span');
            ipSpan.className = 'ip-label';
            ipSpan.textContent = `[${ipLabel}]`;
            const domSpan = document.createElement('span');
            domSpan.className = `dominio-txt${aiClass}`;
            domSpan.textContent = e.dominio;
            div.append(oraSpan, ipSpan, domSpan);
            return div;
        },
        (div, e) => {
            const nome = nomeStudente(e.ip);
            const match = matchFiltro(e.dominio) || matchFiltro(e.ip) || (nome && matchFiltro(nome));
            const hidden = match ? '' : ' filtro-hidden';
            const nuova = `traffico-entry${hidden}`;
            if (div.className !== nuova) div.className = nuova;
        }
    );
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

/**
 * Aggiorna la "selection bar" sopra il pannello IP. Visibile solo se
 * c'e' una multi-selezione attiva. Mostra count + bottoni per le azioni
 * Veyon piu' comuni (lock/messaggio) e un "Deseleziona tutti".
 */
export function renderSelectionBar() {
    const bar = $('selection-bar');
    if (!bar) return;
    const n = state.selectedIps.size;
    if (n === 0) {
        bar.classList.add('hidden');
        bar.textContent = '';
        return;
    }
    bar.classList.remove('hidden');
    // Bottoni Veyon nella bar appaiono solo se Veyon e' configurato.
    const veyonOn = !!state.veyonConfigured;
    bar.innerHTML =
        '<span class="selection-count">' + n + ' selezionat' + (n === 1 ? 'o' : 'i') + '</span>'
        + (veyonOn
            ? '<button class="btn" data-action="veyon-classe-lock" title="Blocca schermo">🔒</button>'
            + '<button class="btn" data-action="veyon-classe-unlock" title="Sblocca schermo">🔓</button>'
            + '<button class="btn" data-action="veyon-classe-msg" title="Messaggio">💬</button>'
            + '<button class="btn btn-warning" data-action="veyon-classe-reboot" title="Riavvia">🔄</button>'
            + '<button class="btn btn-danger" data-action="veyon-classe-poweroff" title="Spegni">⏻</button>'
            : '')
        + '<button class="btn" data-action="clear-selection">Deseleziona tutti</button>';
}

/** Esegue tutti i renderer sincronamente. Chiamato da `renderAll` dentro RAF. */
function _renderAllSync() {
    renderSidebar();
    renderStats();
    renderPausaEBottoni();
    renderTabellaIp();
    renderSelectionBar();
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

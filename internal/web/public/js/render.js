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
    const cntDom = $('count-domini'); if (cntDom) cntDom.textContent = state.perDominio.size;
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
            btn.innerHTML = '<svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true"><path d="M2 2L8 8M8 2L2 8"/></svg>';
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
            btn.innerHTML = '<svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2 5L4.2 7.2L8 3"/></svg>';
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
    const fonte = state.focusIp ? (state.perIp.get(state.focusIp) || []) : state.entries;

    // 1) Richieste totali (escluse "sistema") + sub "+N ultimi 60s".
    const tot = contaAttive(fonte);
    $('stat-richieste').textContent = tot.toLocaleString('it');
    const ora = Date.now();
    let last60 = 0;
    for (const e of state.entries) {
        if (e.tipo === 'sistema') continue;
        if (e.ts && (ora - e.ts) < 60000) last60++;
    }
    const subRich = $('stat-richieste-sub');
    if (subRich) subRich.textContent = `+${last60} ultimi 60s`;
    const rateEl = $('stream-rate');
    if (rateEl) rateEl.textContent = `~${(last60 / 60).toFixed(1)}/s`;

    // 2) AI rilevate: count IP unici con almeno una richiesta tipo='ai'.
    //    Numero rosso (.alert) e sub colorata se >0.
    const aiIps = new Set();
    for (const e of state.entries) if (e.tipo === 'ai') aiIps.add(e.ip);
    const aiCount = aiIps.size;
    const aiEl = $('stat-ai');
    if (aiEl) {
        aiEl.textContent = aiCount;
        // applica .alert sulla CARD (toggle), così .stat-card.alert .numero { color: alert }
        const aiCard = aiEl.closest('.stat-card');
        if (aiCard) aiCard.classList.toggle('alert', aiCount > 0);
    }
    const aiSub = $('stat-ai-sub');
    if (aiSub) {
        aiSub.textContent = aiCount === 0 ? 'nessuna' :
            (aiCount === 1 ? '1 studente' : `${aiCount} studenti`);
        aiSub.classList.toggle('alert', aiCount > 0);
    }

    // 3) Bloccate: count entries con blocked=true.
    const blkEl = $('stat-bloccate');
    if (blkEl) {
        let blk = 0;
        for (const e of state.entries) if (e.blocked) blk++;
        blkEl.textContent = blk;
    }

    // 4) Studenti attivi: IP che hanno avuto traffico negli ultimi
    //    `inattivitaSogliaSec` secondi. Sub "X idle" (totale - attivi).
    const sogliaMs = (state.cfg.inattivitaSogliaSec || 180) * 1000;
    let attivi = 0;
    for (const [, oraStr] of state.ultimaPerIp) {
        const t = Date.parse(oraStr.replace(' ', 'T') + 'Z');
        if (!isNaN(t) && (ora - t) < sogliaMs) attivi++;
    }
    const tot30 = Object.keys(state.cfg.studenti || {}).length || 30;
    const attEl = $('stat-attivi');
    if (attEl) attEl.textContent = attivi;
    const attSub = $('stat-attivi-sub');
    if (attSub) attSub.textContent = `${Math.max(0, tot30 - attivi)} idle`;

    // 5) Status: sub line proxy/web ports (la pill LIVE e' statica nell'HTML).
    const modoEl = $('stat-modo');
    if (modoEl) {
        const proxyPort = state.cfg?.proxy?.port || state.settings?.proxy?.port || 9090;
        const webPort = state.cfg?.web?.port || state.settings?.web?.port || 9999;
        if (state.pausato) modoEl.textContent = 'IN PAUSA';
        else if (!state.sessioneAttiva) modoEl.textContent = 'sessione ferma';
        else modoEl.textContent = `proxy :${proxyPort} · web :${webPort}`;
    }
}

/**
 * Aggiorna label/classe dei bottoni Pausa e Avvia/Ferma sessione in base
 * allo stato, e mostra l'indicatore PAUSA in topbar se attivo.
 */
export function renderPausaEBottoni() {
    // "Blocca tutto" toggle (era "Pausa"): off = tinted (.btn.block),
    // on = filled rosso + dot bianco pulsante (.btn.block.active).
    const btn = $('btn-pausa');
    if (btn) {
        if (state.pausato) {
            btn.textContent = 'Sblocca tutto';
            btn.classList.add('active');
            btn.title = 'Tutti i domini bloccati — click per riattivare';
        } else {
            btn.textContent = 'Blocca tutto';
            btn.classList.remove('active');
            btn.title = 'Blocca tutti i domini';
        }
    }

    // "Blocca AI" toggle: stesso pattern. Active quando TUTTI i domini AI
    // noti (state.cfg.dominiAI) sono nel set bloccati.
    const btnAi = $('btn-block-ai');
    if (btnAi) {
        const dominiAi = state.cfg.dominiAI || [];
        const tuttiBloccati = dominiAi.length > 0 && dominiAi.every(d => state.bloccati.has(d));
        if (tuttiBloccati) {
            btnAi.textContent = 'Sblocca AI';
            btnAi.classList.add('active');
            btnAi.title = 'Tutti i domini AI bloccati — click per sbloccare';
        } else {
            btnAi.textContent = 'Blocca AI';
            btnAi.classList.remove('active');
            btnAi.title = 'Blocca tutti i domini AI noti';
        }
    }

    // Bottone Rec sessione come toggle (stile blocca-tutto/blocca-ai):
    //   idle       → label "Rec sessione", primary rosso pieno
    //   recording  → label "Sta registrando", classe .recording (dot pulse + alone)
    // Click → toggleSessione (start o stop con confirm durata+eventi).
    const btnRec = $('btn-rec');
    if (btnRec) {
        btnRec.classList.toggle('recording', state.sessioneAttiva);
        // Sostituisce solo il TEXT NODE finale, preservando il <span class="rec-dot">.
        const last = btnRec.lastChild;
        const label = state.sessioneAttiva ? ' Sta registrando' : ' Rec sessione';
        if (last && last.nodeType === Node.TEXT_NODE) {
            if (last.textContent !== label) last.textContent = label;
        } else {
            btnRec.appendChild(document.createTextNode(label));
        }
        btnRec.title = state.sessioneAttiva
            ? 'Registrazione in corso — clicca per fermare e archiviare'
            : 'Avvia registrazione sessione';
    }
}

/**
 * Sincronizza le frecce SVG dei toggle sidebar/stream con lo stato.
 * Aperto: freccia che "spinge via" il pannello (chiude). Chiuso:
 * freccia che "tira fuori" il pannello (apre). Direzioni opposte
 * per il pulsante sx vs dx.
 */
export function aggiornaToggleArrows() {
    const sidebarArrow = document.getElementById('sidebar-arrow');
    if (sidebarArrow) {
        // Sidebar SX: aperta=>comprimi (freccia `>`), chiusa=>espandi (freccia `<`)
        sidebarArrow.setAttribute('d', state.sidebarCollassata
            ? 'M8.5 4.5L7 6l1.5 1.5'
            : 'M7 4.5L8.5 6L7 7.5');
    }
    const streamArrow = document.getElementById('stream-arrow');
    if (streamArrow) {
        // Stream DX: aperto=>comprimi (freccia `<`), chiuso=>espandi (freccia `>`)
        streamArrow.setAttribute('d', state.richiesteCollassate
            ? 'M3.5 4.5L5 6L3.5 7.5'
            : 'M5 4.5L3.5 6l1.5 1.5');
    }
}

/**
 * Avvia il rec timer mono in toolbar (HH:MM:SS, refresh ogni 1s).
 * Visibile solo a sessione attiva.
 */
export function avviaRecTimer() {
    const tick = () => {
        const el = document.getElementById('rec-timer');
        if (!el) return;
        if (!state.sessioneAttiva || !state.sessioneInizio) {
            el.classList.add('hidden');
            return;
        }
        el.classList.remove('hidden');
        const start = Date.parse(state.sessioneInizio);
        if (isNaN(start)) return;
        const sec = Math.max(0, Math.floor((Date.now() - start) / 1000));
        const h = String(Math.floor(sec / 3600)).padStart(2, '0');
        const m = String(Math.floor((sec % 3600) / 60)).padStart(2, '0');
        const s = String(sec % 60).padStart(2, '0');
        el.textContent = `● rec ${h}:${m}:${s}`;
    };
    tick();
    setInterval(tick, 1000);
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
    if (!el) return; // countdown rimosso dalla topbar nel redesign Claude Designer
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

// Soglia "alive" per proxy/plugin heartbeats: oltre questa eta' il
// dato e' considerato stale. 15s = 3 ping mancati a 5s di intervallo
// (allineata a HeartbeatTimeout server-side).
const ALIVE_FRESH_MS = 15 * 1000;

/**
 * Flash visivo sulla card studente quando arriva una nuova entry traffic.
 * Aggiunge la classe `.pulse-traffic` per ~500ms (animazione CSS via
 * pseudo-elemento ::after — niente conflitto con bordo / box-shadow
 * di stato della card). Riavviabile su eventi consecutivi via reflow.
 * @param {string} ip
 */
export function flashCardTraffic(ip) {
    if (!ip) return;
    const cards = document.querySelectorAll(`.ip-card[data-ip="${CSS.escape(ip)}"]`);
    if (cards.length === 0) return;
    cards.forEach(card => {
        card.classList.remove('pulse-traffic');
        // Force reflow per riavviare l'animazione anche su ticks consecutivi.
        void card.offsetWidth;
        card.classList.add('pulse-traffic');
    });
}

/**
 * Stato del proxy per un IP (pallino top-left della card).
 * - verde: heartbeat proxy recente (<15s)
 * - rosso: heartbeat in passato ma silente ora (>15s) → bypass
 * - grigio: mai visto un heartbeat (PC scoperto via LAN, proxy non
 *   installato/avviato)
 * @param {string} ip
 * @returns {{classe:'verde'|'rosso'|'grigio', titolo:string}}
 */
function statoProxy(ip) {
    const ts = state.aliveMap.get(ip);
    if (!ts) return { classe: 'grigio', titolo: 'Proxy mai visto' };
    const age = Date.now() - ts;
    if (age < ALIVE_FRESH_MS) return { classe: 'verde', titolo: `Proxy attivo (${Math.round(age/1000)}s fa)` };
    return { classe: 'rosso', titolo: `Proxy silente da ${Math.round(age/1000)}s — possibile bypass` };
}

/**
 * Stato dei plugin watchdog abilitati per un IP (pallino bottom-left).
 * Conta quanti plugin abilitati nelle Impostazioni stanno pingando
 * recentemente:
 * - verde: tutti i plugin abilitati sono vivi
 * - giallo: alcuni mancanti (1+, ma non tutti)
 * - rosso: TUTTI mancanti
 * - grigio: nessun plugin abilitato (oppure mai ricevuto heartbeat)
 * @param {string} ip
 * @returns {{classe:'verde'|'giallo'|'rosso'|'grigio', titolo:string}}
 */
function statoPlugins(ip) {
    const enabledPlugins = (state.watchdogPlugins || []).filter(p => p.enabled);
    if (enabledPlugins.length === 0) {
        return { classe: 'grigio', titolo: 'Nessun watchdog abilitato' };
    }
    const inner = state.alivePluginMap.get(ip);
    const now = Date.now();
    let alive = 0, missingNames = [];
    for (const p of enabledPlugins) {
        const ts = inner ? inner.get(p.id) : 0;
        if (ts && (now - ts) < ALIVE_FRESH_MS) alive++;
        else missingNames.push(p.id);
    }
    const total = enabledPlugins.length;

    // Aliveness "rotta" (alcuni o tutti i plugin silenti) ha priorita'
    // sul filtro eventi: e' un segnale di kill manuale piu' forte di
    // un evento singolo. Se invece tutti pingano, controlliamo se ci
    // sono eventi recenti (es. "USB inserita" — il plugin USB e' vivo
    // ma sta segnalando) per portare il pallino a giallo/rosso anziche'
    // verde "tutto ok".
    if (alive === 0) return { classe: 'rosso', titolo: `Tutti i ${total} watchdog mancanti (${missingNames.join(', ')})` };
    if (alive < total) return { classe: 'giallo', titolo: `${total - alive}/${total} watchdog mancanti: ${missingNames.join(', ')}` };

    // Tutti vivi: per ciascun plugin valuta lo stato considerando
    // ignora-utente + risoluzione (info successivo). Aggrega al peggio.
    const evts = state.watchdogEventsPerIp.get(ip) || [];
    const cutoff = now - ALERT_WD_CUTOFF_MS;
    let hasCritical = false, hasWarning = false, lastFmt = '';
    for (const p of enabledPlugins) {
        const v = valutaWdPlugin(ip, p.id, evts, cutoff, state.eventiIgnoredIds);
        if (v.severity === 'critical') { hasCritical = true; lastFmt = v.topEv.format || lastFmt; }
        else if (v.severity === 'warning') { hasWarning = true; if (!lastFmt) lastFmt = v.topEv.format || ''; }
    }
    if (hasCritical) return { classe: 'rosso', titolo: 'Evento watchdog CRITICAL recente' + (lastFmt ? ' · ' + lastFmt : '') };
    if (hasWarning)  return { classe: 'giallo', titolo: 'Evento watchdog warning recente' + (lastFmt ? ' · ' + lastFmt : '') };
    return { classe: 'verde', titolo: `Tutti i ${total} watchdog attivi` };
}

// Ordinamento gravita' per "peggior colore" del bordo: verde < giallo
// < grigio < rosso. Grigio e' "sconosciuto/offline" e' meno grave del
// rosso ("attivo ma silente = bypass").
const STATO_RANK = { verde: 0, giallo: 1, grigio: 2, rosso: 3 };
function peggiorStato(a, b) {
    return (STATO_RANK[a] || 0) >= (STATO_RANK[b] || 0) ? a : b;
}

/**
 * Valuta lo stato di un singolo plugin watchdog per un IP, considerando:
 *   - eventi nei recentMs piu' recenti
 *   - severity warning/critical (info ignorati)
 *   - "risoluzione": se l'utente ha cliccato Ignora sull'evento e dopo e'
 *     arrivato un evento info (es. USB removed, processo stopped),
 *     l'evento e' considerato risolto → torna OK.
 *
 * Senza Ignora, il warning resta visibile per i 5 min anche se l'evento
 * e' stato "annullato" dal sistema (USB tolta).
 *
 * @returns {{topEv: object|null, severity: 'ok'|'warning'|'critical', resolved: boolean}}
 */
function valutaWdPlugin(ip, pluginId, evts, cutoff, ignoredIds) {
    const metaName = 'watchdog-' + pluginId;
    let topEv = null;
    let topRank = 0;
    for (const ev of evts) {
        if (ev.plugin !== pluginId && ev.plugin !== metaName) continue;
        if ((ev.ts || 0) < cutoff) continue;
        const rank = ev.severity === 'critical' ? 3 : ev.severity === 'warning' ? 2 : 0;
        if (rank === 0) continue; // info skipped per topEv
        if (rank > topRank || (rank === topRank && (ev.ts || 0) > (topEv?.ts || 0))) {
            topEv = ev;
            topRank = rank;
        }
    }
    if (!topEv) return { topEv: null, severity: 'ok', resolved: false };

    // Risoluzione = l'utente ha esplicitamente ignorato l'evento E dopo
    // di esso e' arrivato un info (USB removed, processo stopped, ...)
    // che indica che la condizione che l'aveva generato non c'e' piu'.
    const evId = 'wd:' + ip + ':' + topEv.plugin + ':' + topEv.ts;
    if (ignoredIds.has(evId)) {
        for (const ev of evts) {
            if (ev.plugin !== pluginId && ev.plugin !== metaName) continue;
            if ((ev.ts || 0) <= (topEv.ts || 0)) continue;
            if (ev.severity === 'info') {
                return { topEv, severity: 'ok', resolved: true };
            }
        }
    }
    return { topEv, severity: topEv.severity, resolved: false };
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
    const proxy = statoProxy(ip);
    const plugins = statoPlugins(ip);
    const bordo = peggiorStato(proxy.classe, plugins.classe);
    // wd e' mantenuto per compat con la lista (vista lista usa s.wd):
    // li' rappresentava lo stato watchdog generale, ora lo mappiamo sui
    // plugin (semantica piu' utile dello "alive proxy" gia' nella status).
    return { lista, listaAttive, dominiMap, diffSec, inattivo, nome, wd: plugins, proxy, plugins, bordo };
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

    // View segmented control nella action toolbar (Claude Designer).
    const segG = document.getElementById('btn-vseg-griglia');
    const segL = document.getElementById('btn-vseg-lista');
    if (segG) segG.classList.toggle('active', state.vistaIp === 'griglia');
    if (segL) segL.classList.toggle('active', state.vistaIp === 'lista');

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
        // 8 colonne come da Claude Designer: status dot | studente | IP |
        // REQ | ULTIMA | DOMINI RECENTI | WATCHDOG | STATO.
        const table = document.createElement('table');
        table.className = 'ip-list-table';
        table.innerHTML = '<thead><tr>'
            + '<th class="col-status"></th>'
            + '<th class="col-studente">STUDENTE</th>'
            + '<th class="col-ip">IP</th>'
            + '<th class="col-req">REQ</th>'
            + '<th class="col-ultima">ULTIMA</th>'
            + '<th class="col-domini">DOMINI RECENTI</th>'
            + '<th class="col-wd">WATCHDOG</th>'
            + '<th class="col-stato">STATO</th>'
            + '</tr></thead><tbody></tbody>';
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
            // Niente data-action: i chip sono puramente decorativi.
            // Click sulla card (incluso sui chip) apre il detail pane via
            // bubbling al data-action="focus-ip" del parent.
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
/** Numero massimo di chip dominio mostrati nella riga lista (overflow → +N). */
const DOMINI_LISTA_MAX = 4;

/**
 * Cutoff temporali per gli stati visivi delle card e del detail pane.
 * Coerenti con quelli del banner alert: una card resta colorata solo
 * finche' l'evento e' "recente". Senza, eventi vecchi resterebbero
 * visualmente "appiccicati" alle card per tutta la sessione (e anche
 * dopo riavvio di Planck, perche' /api/watchdog/events ritorna fino a
 * 200 eventi storici dal DB).
 */
const ALERT_AI_CUTOFF_MS = 10 * 60 * 1000; // 10 min
const ALERT_WD_CUTOFF_MS = 5 * 60 * 1000;  // 5 min

/**
 * Cutoff a 3 step per lo stato base della card.
 *
 * Logica:
 *   ping watchdog assente da > OFFLINE_PING_MS  →  offline (proxy non attivo)
 *   ping ok + traffico assente da > IDLE_TRAFFIC_MS →  idle (online ma non naviga)
 *   ping ok + traffico recente                  →  active (sta navigando)
 *
 * "Mai navigato" rientra in idle: la card e' grigio chiaro (proxy ok)
 * fino al primo traffico utente.
 *
 * Il watchdog VBS pinga ogni 5s. 15s = 3 ping mancati = solido segnale
 * di proxy killato (no glitch transitori). Coerente col timeout dei
 * plugin watchdog (anch'esso 15s in v2.9.9).
 */
const OFFLINE_PING_MS = 15 * 1000;        // 15s
const IDLE_TRAFFIC_MS = 15 * 1000;        // 15s

/** Ritorna true se l'IP ha una entry AI nelle ultime ALERT_AI_CUTOFF_MS. */
function hasAIRecente(ip, ora) {
    const cutoff = ora - ALERT_AI_CUTOFF_MS;
    const lista = state.perIp.get(ip) || [];
    for (let i = lista.length - 1; i >= 0; i--) {
        const e = lista[i];
        if (e.tipo !== 'ai') continue;
        const ts = e.ts || (e.ora ? Date.parse(e.ora.replace(' ', 'T') + 'Z') : 0);
        if (ts >= cutoff) return true;
        if (ts && ts < cutoff) return false; // sorted, possiamo uscire
    }
    return false;
}

/** Ritorna true se l'IP ha un evento watchdog warning/critical nelle ultime ALERT_WD_CUTOFF_MS. */
function hasWDRecente(ip, ora) {
    const cutoff = ora - ALERT_WD_CUTOFF_MS;
    const evts = state.watchdogEventsPerIp.get(ip) || [];
    for (let i = evts.length - 1; i >= 0; i--) {
        const ev = evts[i];
        if (ev.severity !== 'warning' && ev.severity !== 'critical') continue;
        if ((ev.ts || 0) >= cutoff) return true;
    }
    return false;
}

/**
 * Ritorna {aliveAgo, trafficoAgo} in ms per l'IP:
 *   aliveAgo: tempo dal ping watchdog piu' recente (Infinity se mai pingato)
 *   trafficoAgo: tempo dall'ultima entry traffico (Infinity se mai navigato)
 *
 * I due valori vengono usati separatamente per derivare offline/idle/active.
 */
function ipSignals(ip, ora) {
    const aliveTs = state.aliveMap.get(ip) || 0;
    const aliveAgo = aliveTs > 0 ? (ora - aliveTs) : Infinity;
    const lista = state.perIp.get(ip) || [];
    // Idle = lo studente non sta navigando attivamente. Il traffico
    // 'sistema' (OCSP, telemetry Microsoft, captive portal, ecc.) arriva
    // anche a finestra Edge chiusa: includerlo nel calcolo significava
    // tenere la card sempre "active". Filtriamo sull'ultima entry NON
    // sistema (web/ai/blocked).
    let trafficoAgo = Infinity;
    for (let i = lista.length - 1; i >= 0; i--) {
        const e = lista[i];
        if (e.tipo === 'sistema') continue;
        const ts = e.ts || (e.ora ? Date.parse(e.ora.replace(' ', 'T') + 'Z') : 0);
        if (ts) { trafficoAgo = ora - ts; break; }
    }
    return { aliveAgo, trafficoAgo };
}

/** Calcola lo stato base (offline/idle/active) dall'IP. */
function statoBase(ip, ora) {
    const { aliveAgo, trafficoAgo } = ipSignals(ip, ora);
    if (aliveAgo > OFFLINE_PING_MS) return 'offline';
    if (trafficoAgo > IDLE_TRAFFIC_MS) return 'idle';
    return 'active';
}

function renderListaIp(container, ips, ora, soglia) {
    const body = scheletroVistaIp(container, 'lista');
    syncChildren(body, ips,
        ip => ip,
        ip => {
            const tr = document.createElement('tr');
            tr.dataset.action = 'focus-ip';
            tr.dataset.ip = ip;
            tr.innerHTML = '<td class="col-status"><span class="dot"></span></td>'
                + '<td class="col-studente"></td>'
                + '<td class="col-ip"><span class="ip-text"></span><span class="ip-row-blocks hidden"></span></td>'
                + '<td class="col-req"></td>'
                + '<td class="col-ultima"></td>'
                + '<td class="col-domini"><span class="chips"></span></td>'
                + '<td class="col-wd"><span class="dot"></span></td>'
                + '<td class="col-stato"></td>';
            return tr;
        },
        (tr, ip) => {
            const s = calcolaStatoIp(ip, ora, soglia);
            const hasAI = hasAIRecente(ip, ora);
            const hasWD = hasWDRecente(ip, ora);

            // Stato uniforme alle card: offline > idle > active > watchdog > ai > selected
            let stato = statoBase(ip, ora);
            if (hasWD) stato = 'watchdog';
            if (hasAI) stato = 'ai';
            if (state.focusIp === ip) stato = 'selected';
            if (tr.dataset.state !== stato) tr.dataset.state = stato;

            const rowClass = [];
            if (state.selectedIps.has(ip)) rowClass.push('multi');
            if (state.lockedIps.has(ip)) rowClass.push('locked');
            if (state.filtro && !matchFiltro(`${s.nome || ''} ${ip}`)) rowClass.push('filtro-hidden');
            const nuova = rowClass.join(' ');
            if (tr.className !== nuova) tr.className = nuova;

            const tds = tr.children;

            // Col 0: status dot = stato proxy (verde/rosso/grigio).
            // NON cambia colore per selected/ai/watchdog: quelli vivono
            // su altri segnali visivi (bordo card, banner, focus row).
            const statusDot = tds[0].firstElementChild;
            const dotClass = ({
                verde:  'dot ok',
                rosso:  'dot alert',
                grigio: 'dot muted',
            })[s.proxy.classe] || 'dot';
            if (statusDot.className !== dotClass) statusDot.className = dotClass;

            // Col 1: nome studente (fallback IP last octet se senza nome).
            const nomeTxt = s.nome || ('.' + ip.split('.').pop());
            if (tds[1].textContent !== nomeTxt) tds[1].textContent = nomeTxt;

            // Col 2: IP completo mono + badge blocchi per-IP (⊘N) se >0.
            const ipTextEl = tds[2].querySelector('.ip-text');
            if (ipTextEl && ipTextEl.textContent !== ip) ipTextEl.textContent = ip;
            const rowBlocksEl = tds[2].querySelector('.ip-row-blocks');
            const perIpSet = state.blocchiPerIp.get(ip);
            if (rowBlocksEl) {
                if (perIpSet && perIpSet.size > 0) {
                    const txt = '⊘' + perIpSet.size;
                    if (rowBlocksEl.textContent !== txt) rowBlocksEl.textContent = txt;
                    rowBlocksEl.classList.remove('hidden');
                } else {
                    rowBlocksEl.classList.add('hidden');
                }
            }

            // Col 3: REQ (numero richieste utente+ai, no sistema).
            const nStr = String(s.listaAttive.length);
            if (tds[3].textContent !== nStr) tds[3].textContent = nStr;

            // Col 4: Ultima attività relativa.
            const ultimaTxt = s.listaAttive.length > 0 ? formatRelativo(s.diffSec) : '-';
            if (tds[4].textContent !== ultimaTxt) tds[4].textContent = ultimaTxt;

            // Col 5: chips dei domini recenti, max DOMINI_LISTA_MAX, +N overflow.
            const tuttiDom = [...s.dominiMap.entries()];
            const visibili = tuttiDom.slice(-DOMINI_LISTA_MAX);
            const extra = Math.max(0, tuttiDom.length - visibili.length);
            syncTagsDominio(tds[5].firstElementChild, visibili, extra);

            // Col 6: watchdog dot (s.wd.classe = ok/warn/muted).
            const wdDot = tds[6].firstElementChild;
            const wdClass = `dot ${s.wd.classe || 'muted'}`;
            if (wdDot.className !== wdClass) wdDot.className = wdClass;
            if (wdDot.title !== s.wd.titolo) wdDot.title = s.wd.titolo;

            // Col 7: stato testuale (attivo/idle/offline/ai/watchdog).
            const statoTxt = ({
                active: 'attivo',
                idle: 'idle',
                offline: 'offline',
                ai: 'ai',
                watchdog: 'watchdog',
                selected: 'attivo',
            })[stato] || 'attivo';
            if (tds[7].textContent !== statoTxt) tds[7].textContent = statoTxt;
            const statoCls = 'col-stato ' + stato;
            if (tds[7].className !== statoCls) tds[7].className = statoCls;
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
        const v = document.createElement('div');
        v.className = 'ip-grid-vuota';

        const titolo = document.createElement('div');
        titolo.className = 'empty-title';
        titolo.textContent = 'Nessuno studente connesso ancora.';
        v.appendChild(titolo);

        const hint = document.createElement('div');
        hint.className = 'empty-hint';
        const haStudenti = Object.keys(state.cfg.studenti || {}).length > 0;
        const haVeyon = !!state.veyonConfigured;
        if (haVeyon && haStudenti) {
            hint.textContent = 'Clicca "Distribuisci proxy" per attivare il monitoraggio sui PC studente.';
        } else if (haStudenti) {
            hint.textContent = 'Configura Veyon nelle Impostazioni per distribuire automaticamente il proxy_on.bat agli studenti.';
        } else {
            hint.textContent = 'Aggiungi studenti alla mappa nelle Impostazioni, oppure attendi che il proxy venga lanciato sui PC studente.';
        }
        v.appendChild(hint);

        if (haVeyon && haStudenti) {
            const cta = document.createElement('button');
            cta.type = 'button';
            cta.className = 'empty-cta';
            cta.dataset.action = 'veyon-distribuisci-proxy';
            cta.textContent = '📁 Distribuisci proxy ora';
            v.appendChild(cta);
        }
        grid.appendChild(v);
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
            // Template design Claude: status dot esterno + head (nome+ip)
            // + meta inline + chips + foot watchdog. Niente bottoni Veyon
            // visibili (le azioni vivono nel detail pane al click).
            card.innerHTML = '<span class="status"></span>'
                + '<div class="ip-card-head">'
                + '<span class="ip-card-nome"></span>'
                + '<span class="ip-card-blocks hidden"></span>'
                + '<span class="ip-card-ip"></span>'
                + '</div>'
                + '<div class="ip-card-metriche">'
                + '<span><span class="ip-card-num">0</span> req</span>'
                + '<span class="card-meta-sep">·</span>'
                + '<span class="ip-card-ultima">-</span>'
                + '</div>'
                + '<div class="ip-card-tags"></div>'
                + '<div class="ip-card-foot">'
                + '<span class="watchdog-dot"></span>'
                + '<span class="wd-label">watchdog</span>'
                + '<span class="wd-octet"></span>'
                + '</div>'
                + '<div class="ip-card-lock-overlay" aria-hidden="true">'
                + '<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">'
                + '<rect x="5" y="11" width="14" height="10" rx="2"/>'
                + '<path d="M8 11V7a4 4 0 0 1 8 0v4"/>'
                + '</svg>'
                + '</div>';
            return card;
        },
        (card, ip) => {
            const s = calcolaStatoIp(ip, ora, soglia);

            // Stato derivato:
            //   - .status (top-left)   = stato proxy: statoProxy(ip)
            //   - .watchdog-dot (foot) = stato plugin abilitati: statoPlugins(ip)
            //   - data-border          = peggior colore tra i due → bordo card
            //   - data-state (legacy) = ai/selected/idle per override visivi
            //     speciali (banner AI, focus modal, opacity idle).
            const hasAI = hasAIRecente(ip, ora);
            let stato = statoBase(ip, ora); // active/idle/offline (per opacity)
            if (hasAI) stato = 'ai';
            if (state.focusIp === ip) stato = 'selected';
            card.dataset.state = stato;
            card.dataset.border = s.bordo;
            card.dataset.proxy = s.proxy.classe;

            const classi = ['ip-card'];
            if (state.selectedIps.has(ip)) classi.push('multi');
            if (state.lockedIps.has(ip)) classi.push('locked');
            if (state.filtro && !matchFiltro(`${s.nome || ''} ${ip}`)) classi.push('filtro-hidden');
            const nuova = classi.join(' ');
            if (card.className !== nuova) card.className = nuova;

            // Head: nome (o IP se nome vuoto) + badge blocchi per-IP + IP mono.
            const head = card.children[1];
            const nomeEl = head.children[0];
            const blocksEl = head.children[1];
            const ipEl = head.children[2];
            const nomeText = s.nome || ip;
            if (nomeEl.textContent !== nomeText) nomeEl.textContent = nomeText;
            // Badge blocchi per-IP: visibile solo se >0.
            const perIpSet = state.blocchiPerIp.get(ip);
            const blocksCount = perIpSet ? perIpSet.size : 0;
            if (blocksCount > 0) {
                const txt = '⊘' + blocksCount;
                if (blocksEl.textContent !== txt) blocksEl.textContent = txt;
                blocksEl.classList.remove('hidden');
            } else {
                blocksEl.classList.add('hidden');
            }
            // Mostra IP mono solo se c'e' nome (sennò sarebbe duplicato).
            const ipText = s.nome ? ip : '';
            if (ipEl.textContent !== ipText) ipEl.textContent = ipText;

            // Meta inline: <num> req · ora|Xm fa
            const metriche = card.children[2];
            const numEl = metriche.querySelector('.ip-card-num');
            const numStr = String(s.listaAttive.length);
            if (numEl && numEl.textContent !== numStr) numEl.textContent = numStr;
            const ultimaEl = metriche.querySelector('.ip-card-ultima');
            const ultimaTxt = s.listaAttive.length > 0 ? formatRelativo(s.diffSec) : '-';
            if (ultimaEl && ultimaEl.textContent !== ultimaTxt) ultimaEl.textContent = ultimaTxt;

            // Chips dominio (max DOMINI_CARD_MAX).
            const tags = card.children[3];
            const dominiOrd = [...s.dominiMap.entries()].reverse();
            const visibili = dominiOrd.slice(0, DOMINI_CARD_MAX);
            const extra = dominiOrd.length - visibili.length;
            syncTagsDominio(tags, visibili, extra);

            // Foot: watchdog dot + label + .NN ottetto IP.
            const foot = card.children[4];
            if (foot) {
                const wd = foot.firstElementChild;
                const wdClass = `watchdog-dot ${s.wd.classe}`;
                if (wd.className !== wdClass) wd.className = wdClass;
                if (wd.title !== s.wd.titolo) wd.title = s.wd.titolo;
                const oct = foot.querySelector('.wd-octet');
                if (oct) {
                    const lastOct = '.' + ip.split('.').pop();
                    if (oct.textContent !== lastOct) oct.textContent = lastOct;
                }
            }
        }
    );
}

/**
 * Helper: true se `d` matcha uno dei pattern AI in `state.cfg.dominiAI`.
 */
function isAIDomainNome(d) {
    if (!d) return false;
    const list = (state.cfg && state.cfg.dominiAI) || [];
    const lower = d.toLowerCase();
    for (const ai of list) {
        if (lower.includes(ai.toLowerCase())) return true;
    }
    return false;
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
            const tSpan = document.createElement('span');
            tSpan.className = 't';
            tSpan.textContent = e.ora.substring(11);
            const bodySpan = document.createElement('span');
            bodySpan.className = 'body';
            const ipSpan = document.createElement('span');
            ipSpan.className = 'ip';
            ipSpan.textContent = e.ip + ' →';
            const hostSpan = document.createElement('span');
            hostSpan.className = 'host';
            hostSpan.textContent = ' ' + e.dominio;
            bodySpan.append(ipSpan, hostSpan);
            div.append(tSpan, bodySpan);
            return div;
        },
        (div, e) => {
            const match = matchFiltro(e.dominio) || matchFiltro(e.ip);
            const aiCls = e.tipo === 'ai' ? ' ai' : '';
            const hidden = match ? '' : ' filtro-hidden';
            const nuova = `stream-row${aiCls}${hidden}`;
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
    if (!titolo) return; // header rimosso nel redesign Claude Designer
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
    if (!sel) return; // select rimosso nel redesign Claude Designer
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
    // Theme toggle: swap icone SVG sole/luna (entrambe in DOM, mostriamo
    // quella che corrisponde al tema OPPOSTO — click la commuta).
    const sun = document.getElementById('icon-sun');
    const moon = document.getElementById('icon-moon');
    if (sun && moon) {
        sun.style.display = state.darkmode ? '' : 'none';
        moon.style.display = state.darkmode ? 'none' : '';
    }
    // Notifiche: il toggle vive in Impostazioni in v2.7.x+.
    const btnNotifSet = document.getElementById('btn-notifiche-settings');
    if (btnNotifSet) btnNotifSet.classList.toggle('attivo', state.notifiche);
    document.body.classList.toggle('dark', state.darkmode);
}

/**
 * Avvia clock topbar (HH:MM, refresh ogni 30s). Chiamato una volta a init.
 */
export function avviaTopbarClock() {
    const tick = () => {
        const el = document.getElementById('topbar-clock');
        if (!el) return;
        const d = new Date();
        const hh = String(d.getHours()).padStart(2, '0');
        const mm = String(d.getMinutes()).padStart(2, '0');
        el.textContent = `${hh}:${mm}`;
    };
    tick();
    setInterval(tick, 30 * 1000);
}

/**
 * Aggiorna il contatore "N attivi" in topbar — IP che hanno avuto traffico
 * negli ultimi `inattivitaSogliaSec` secondi.
 */
export function aggiornaTopbarCount() {
    const el = document.getElementById('topbar-active-count');
    if (!el) return;
    const ora = Date.now();
    const sogliaMs = (state.cfg.inattivitaSogliaSec || 180) * 1000;
    let attivi = 0;
    for (const [, oraStr] of state.ultimaPerIp) {
        const t = Date.parse(oraStr.replace(' ', 'T') + 'Z');
        if (!isNaN(t) && (ora - t) < sogliaMs) attivi++;
    }
    el.textContent = attivi;
}

/** Sincronizza il valore dell'input time con `state.deadlineISO`. */
export function aggiornaInputDeadline() {
    const input = $('input-deadline');
    if (!input) return; // input rimosso nel redesign Claude Designer
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

    // Popola il dropdown delle sessioni archiviate anche qui (oltre che
    // in renderImpostazioni): se l'utente apre Report come prima tab,
    // senza passare da Impostazioni, il select restava vuoto.
    aggiornaSelectSessioniArchivio();

    const usaArchivio = !!state.datiSessioneVisualizzata;
    const entries = usaArchivio ? state.datiSessioneVisualizzata.entries : state.entries;
    const sessioneInizio = usaArchivio ? state.datiSessioneVisualizzata.sessioneInizio : state.sessioneInizio;
    const bloccatiList = usaArchivio ? (state.datiSessioneVisualizzata.bloccati || []) : [...state.bloccati];
    const studentiMap = usaArchivio ? (state.datiSessioneVisualizzata.studenti || {}) : state.cfg.studenti;

    const titoloEl = $('report-titolo');
    if (usaArchivio) {
        const d = new Date(sessioneInizio);
        titoloEl.textContent = `Archivio · ${d.toLocaleString('it-IT', { dateStyle: 'short', timeStyle: 'short' })}`;
        $('btn-elimina-sessione').disabled = false;
    } else {
        titoloEl.textContent = state.sessioneAttiva ? 'Sessione corrente · in registrazione' : 'Sessione corrente';
        $('btn-elimina-sessione').disabled = true;
    }

    const agg = aggregaPerReport(entries);
    // Calcolo durata: per archivio usa sessioneFineISO/durataSec dal payload
    // (NON Date.now() — altrimenti il timer continua a salire mentre guardi
    // un report passato). Per sessione corrente: now se attiva, sessioneFineISO
    // se ferma.
    let durataSec = 0;
    if (usaArchivio) {
        const dv = state.datiSessioneVisualizzata;
        if (typeof dv.durataSec === 'number' && dv.durataSec > 0) {
            durataSec = dv.durataSec;
        } else if (dv.sessioneFineISO && sessioneInizio) {
            durataSec = Math.max(0, Math.floor(
                (new Date(dv.sessioneFineISO).getTime() - new Date(sessioneInizio).getTime()) / 1000
            ));
        }
    } else if (sessioneInizio) {
        const fine = state.sessioneAttiva
            ? Date.now()
            : (state.sessioneFineISO ? new Date(state.sessioneFineISO).getTime() : Date.now());
        durataSec = Math.max(0, Math.floor((fine - new Date(sessioneInizio).getTime()) / 1000));
    }

    // Stat strip 5 colonne (stile coerente con Live tab).
    $('report-stat-durata').textContent = formatDurata(durataSec);
    $('report-stat-inizio').textContent = sessioneInizio
        ? new Date(sessioneInizio).toLocaleString('it-IT', { dateStyle: 'short', timeStyle: 'short' })
        : '—';
    $('report-stat-richieste').textContent = (agg.totale || 0).toLocaleString('it');
    $('report-stat-mix').textContent = `${agg.perTipo.utente || 0} utente · ${agg.perTipo.ai || 0} AI`;
    $('report-stat-domini').textContent = agg.perDominio.size;
    $('report-stat-ip-sub').textContent = `${agg.perIp.size} studenti`;
    const pctBloccate = agg.totale > 0 ? Math.round((agg.bloccate / agg.totale) * 100) : 0;
    $('report-stat-bloccate').textContent = agg.bloccate;
    $('report-stat-bloccate-pct').textContent = `${pctBloccate}% del totale`;
    $('report-stat-blocklist').textContent = bloccatiList.length;

    // Tabelle dense (stile lista IP della Live).
    const dominiOrdinati = [...agg.perDominio.entries()].sort((a, b) => b[1].count - a[1].count);
    const soloAI = dominiOrdinati.filter(([, info]) => info.tipo === 'ai').slice(0, 10);
    $('report-top-ai').innerHTML = renderReportTable(
        soloAI.map(([d, i]) => ({ label: d, n: i.count, kind: 'ai' })),
        'Nessuna richiesta AI in questa sessione.'
    );

    const top10 = dominiOrdinati.slice(0, 10).map(([d, i]) => ({ label: d, n: i.count, kind: i.tipo }));
    $('report-top-domini').innerHTML = renderReportTable(top10, 'Nessuna richiesta.');

    const ipOrdinati = [...agg.perIpAttive.entries()].sort((a, b) => b[1] - a[1]).slice(0, 30);
    const studenti = ipOrdinati.map(([ip, n]) => {
        const nome = studentiMap[ip];
        const label = nome ? `${nome}` : ip;
        const sub = nome ? ip : '';
        return { label, sub, n, kind: 'std' };
    });
    $('report-top-studenti').innerHTML = renderReportTable(studenti, 'Nessuna attività.');

    // Eventi watchdog (USB / process / network) della sessione.
    // Per archivio: presi dal payload `watchdogEvents` salvato a Stop.
    // Per sessione corrente: presi da state.watchdogEvents (live).
    let eventi;
    if (usaArchivio) {
        eventi = state.datiSessioneVisualizzata.watchdogEvents || [];
    } else {
        // Per la corrente filtro per timestamp >= sessioneInizio (se disponibile).
        const inizioMs = sessioneInizio ? new Date(sessioneInizio).getTime() : 0;
        eventi = (state.watchdogEvents || []).filter(e => !inizioMs || (e.ts || 0) >= inizioMs);
    }
    const cntEl = $('report-eventi-count');
    if (cntEl) cntEl.textContent = eventi.length > 0 ? `${eventi.length} totali` : '';
    $('report-eventi').innerHTML = renderReportEventi(eventi, studentiMap);
}

/** Render della tabella "Eventi watchdog" del Report. */
function renderReportEventi(eventi, studentiMap) {
    if (!eventi || eventi.length === 0) {
        return '<div class="report-empty">Nessun evento durante questa sessione.</div>';
    }
    // Ordine cronologico inverso (recenti prima).
    const ordinati = [...eventi].sort((a, b) => (b.ts || 0) - (a.ts || 0));
    const PLUGIN_LABEL = { usb: 'USB', process: 'Processi', network: 'Network' };
    const SEVERITY_DOT = { warning: 'warn', critical: 'alert', info: 'muted' };
    const fmtTs = (ts) => ts
        ? new Date(ts).toLocaleString('it-IT', { dateStyle: 'short', timeStyle: 'medium' })
        : '—';
    return ordinati.map(ev => {
        const plugin = PLUGIN_LABEL[ev.plugin] || ev.plugin || '?';
        const dotCls = SEVERITY_DOT[ev.severity] || 'muted';
        const nome = ev.nome || (studentiMap || {})[ev.ip] || '';
        const studLabel = nome ? `${nome} · ${ev.ip}` : (ev.ip || '');
        let detail = '';
        if (ev.payload && typeof ev.payload === 'object') {
            const parts = Object.entries(ev.payload)
                .filter(([k]) => k !== 'event')
                .map(([k, v]) => `${k}: ${typeof v === 'object' ? JSON.stringify(v) : v}`);
            detail = parts.join(' · ');
        }
        return `<div class="report-evento">
            <span class="dot ${dotCls}"></span>
            <span class="re-ts">${escapeHtml(fmtTs(ev.ts))}</span>
            <span class="re-plugin">${escapeHtml(plugin)}</span>
            <span class="re-stud">${escapeHtml(studLabel)}</span>
            <span class="re-detail">${escapeHtml(detail)}</span>
        </div>`;
    }).join('');
}

/**
 * Rende una "tabella" densa con riga per ogni elemento: nome (mono per IP),
 * count tabular-right + barra proporzionale (max → 100%).
 * Stile coerente con la vista lista IP della Live.
 */
function renderReportTable(items, emptyMsg) {
    if (!items || items.length === 0) {
        return `<div class="report-empty">${escapeHtml(emptyMsg)}</div>`;
    }
    const max = Math.max(...items.map(i => i.n));
    return items.map(it => {
        const pct = max > 0 ? Math.round((it.n / max) * 100) : 0;
        const kindCls = it.kind === 'ai' ? ' ai' : '';
        const labelHtml = it.sub
            ? `<span class="rt-label">${escapeHtml(it.label)}</span><span class="rt-sub">${escapeHtml(it.sub)}</span>`
            : `<span class="rt-label">${escapeHtml(it.label)}</span>`;
        return `<div class="report-row${kindCls}">
            <div class="rt-name">${labelHtml}</div>
            <div class="rt-bar"><div class="rt-bar-fill" style="width:${pct}%"></div></div>
            <div class="rt-count">${it.n.toLocaleString('it')}</div>
        </div>`;
    }).join('');
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
 * "riavvio richiesto", lista domini ignorati, lista sessioni archiviate.
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

    aggiornaSelectSessioniArchivio();
    const sessioniEl = $('sessioni-list');
    sessioniEl.innerHTML = state.sessioniArchivio.length > 0
        ? state.sessioniArchivio.map(s => {
            const i = sessInfo(s);
            return `<li data-action="sessione-apri" data-nome="${attrEscape(i.filename)}">
            <span class="nome">${escapeHtml(i.label)}</span>
            <button class="btn btn-danger" data-action="sessione-elimina" data-nome="${attrEscape(i.filename)}">Elimina</button>
        </li>`;
        }).join('')
        : '<li class="hint">Archivio vuoto. Ogni "Nuova sessione" archivia la precedente.</li>';
}

/**
 * Helper: ritorna {filename, label} per una sessione archiviata,
 * supporta sia il nuovo array di oggetti `{filename, titolo, inizio}`
 * sia il vecchio array di stringhe (back-compat).
 */
function sessInfo(s) {
    if (typeof s === 'string') {
        return { filename: s, label: s.replace(/\.json$/, '') };
    }
    const filename = s.filename || '';
    const inizio = (s.inizio || '').replace('T', ' ').replace(/\..*$/, '').slice(0, 16);
    const lbl = s.titolo
        ? `${s.titolo} · ${inizio}`
        : (filename.replace(/\.json$/, ''));
    return { filename, label: lbl };
}

/**
 * Popola il <select id="report-sessione-select"> con tutte le sessioni
 * archiviate. Chiamato sia da renderImpostazioni sia da renderReport
 * cosi' il dropdown del Report e' sempre aggiornato anche se l'utente
 * non e' mai passato per il tab Impostazioni.
 */
function aggiornaSelectSessioniArchivio() {
    const select = $('report-sessione-select');
    if (!select) return;
    const valSel = select.value;
    select.innerHTML = '<option value="">-- Sessione corrente --</option>'
        + state.sessioniArchivio.map(s => {
            const i = sessInfo(s);
            return `<option value="${attrEscape(i.filename)}">${escapeHtml(i.label)}</option>`;
        }).join('');
    select.value = valSel;
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

// renderMappaStudenti / renderSelectCombo: rimossi in v2.6.0. La mappa
// IP→nome non e' piu' editabile (gli IP del /24 corrente sono generati
// server-side al boot e mostrati come label IP raw).

// ========================================================================
// Render completo (throttled)
// ========================================================================

/**
 * Aggiorna la "selection bar" sopra il pannello IP. Visibile solo se
 * c'e' una multi-selezione attiva. Mostra count + bottoni per le azioni
 * Veyon piu' comuni (lock/messaggio) e un "Deseleziona tutti".
 */
/**
 * Card "Lista AI auto-aggiornata" nelle Impostazioni: mostra count +
 * source (embedded/cache/remote) + timestamp ultimo update.
 */
export function renderAIListStatus() {
    const el = $('ai-list-status-value');
    if (!el) return;
    const a = state.aiList || {};
    if (!a.count) {
        el.textContent = 'caricamento...';
        return;
    }
    const sourceLabel = {
        embedded: 'integrata nel binario',
        cache:    'cache locale',
        remote:   'aggiornata da GitHub',
    }[a.source] || a.source || '?';
    let updated = a.updatedAt || '';
    if (updated) {
        try { updated = new Date(updated).toLocaleString('it-IT'); } catch {}
    }
    el.innerHTML = '<strong>' + a.count + '</strong> domini &mdash; sorgente: <code>' + sourceLabel + '</code><br>'
                 + '<span class="hint" style="font-size:0.85em">Ultimo update: ' + (updated || 'n/a') + '</span>';
}

/**
 * Renderizza la card "Watchdog plugins" nelle Impostazioni: per ogni
 * plugin un toggle abilita/disabilita + descrizione. La config raw
 * (JSON) e' nascosta in <details> per non sovraccaricare l'UI.
 */
export function renderWatchdogPluginsList() {
    const root = $('watchdog-plugins-list');
    if (!root) return;
    root.textContent = '';
    if (!state.watchdogPlugins.length) {
        const p = document.createElement('p'); p.className = 'hint';
        p.textContent = 'Nessun plugin registrato.';
        root.appendChild(p);
        return;
    }
    for (const plugin of state.watchdogPlugins) {
        const wrap = document.createElement('div');
        wrap.className = 'watchdog-plugin' + (plugin.enabled ? ' enabled' : '');
        const head = document.createElement('div');
        head.className = 'watchdog-plugin-head';
        const toggle = document.createElement('label');
        toggle.className = 'watchdog-toggle';
        const cb = document.createElement('input');
        cb.type = 'checkbox';
        cb.checked = !!plugin.enabled;
        cb.dataset.action = 'watchdog-toggle';
        cb.dataset.plugin = plugin.id;
        const span = document.createElement('span');
        span.textContent = plugin.name;
        toggle.appendChild(cb);
        toggle.appendChild(span);
        head.appendChild(toggle);
        const status = document.createElement('span');
        status.className = 'watchdog-status';
        status.textContent = plugin.enabled ? 'attivo' : 'inattivo';
        head.appendChild(status);
        wrap.appendChild(head);
        const desc = document.createElement('p');
        desc.className = 'hint';
        desc.textContent = plugin.description;
        wrap.appendChild(desc);

        // Config editor (collapsable). Le modifiche entrano in vigore
        // alla prossima Distribuisci proxy_on (gli studenti riscaricano
        // lo script con la nuova config).
        const det = document.createElement('details');
        det.className = 'watchdog-config-editor';
        const sum = document.createElement('summary');
        sum.textContent = 'Modifica configurazione (JSON)';
        det.appendChild(sum);
        const ta = document.createElement('textarea');
        ta.className = 'watchdog-config-json';
        ta.dataset.plugin = plugin.id;
        ta.spellcheck = false;
        ta.rows = 6;
        ta.value = JSON.stringify(plugin.config || {}, null, 2);
        det.appendChild(ta);
        const btnRow = document.createElement('div');
        btnRow.className = 'toolbar-group';
        const btnSave = document.createElement('button');
        btnSave.className = 'btn btn-primary';
        btnSave.dataset.action = 'watchdog-save-config';
        btnSave.dataset.plugin = plugin.id;
        btnSave.textContent = 'Salva configurazione';
        const btnReset = document.createElement('button');
        btnReset.className = 'btn';
        btnReset.dataset.action = 'watchdog-reset-config';
        btnReset.dataset.plugin = plugin.id;
        btnReset.textContent = 'Ripristina default';
        btnRow.appendChild(btnSave);
        btnRow.appendChild(btnReset);
        det.appendChild(btnRow);
        wrap.appendChild(det);

        root.appendChild(wrap);
    }
}

/**
 * Pannello eventi watchdog sopra la griglia IP. Mostra gli ultimi 5
 * eventi con severity warning/critical degli ultimi 5 minuti, con tag
 * per IP/plugin. Nascosto se non ci sono eventi rilevanti.
 */
/**
 * Aggrega gli eventi attivi (AI + Watchdog) per banner e log.
 * Ritorna { eventi, aiCount, wdCount, total, lastTs }.
 *
 * AI: per ogni IP che ha generato traffico tipo='ai' negli ultimi 10 min,
 *     una entry con l'ULTIMA richiesta AI di quell'IP.
 * WD: ultimi N eventi watchdog warning/critical (5 min cutoff).
 *
 * Eventi marcati come "ignorati" da `state.eventiIgnoredIds` vengono
 * filtrati e non contano per banner ne' log feed.
 */
function aggregaEventiAlert() {
    // v2.9.13: niente cutoff temporale. Gli eventi restano in lista (e nel
    // banner) finche' l'utente non li gestisce esplicitamente — click
    // "Ignora" nel log oppure "Ignora tutto" — oppure finche' un Reset
    // non ripulisce la coda. Cap implicito: state.entries (5000),
    // state.watchdogEvents (200) → naturale roll-off.
    const aiByIp = new Map(); // ip -> ultima entry AI
    for (const e of state.entries) {
        if (e.tipo !== 'ai') continue;
        const ts = e.ts || (e.ora ? Date.parse(e.ora.replace(' ', 'T') + 'Z') : 0);
        const prev = aiByIp.get(e.ip);
        if (!prev || (prev.ts || 0) < ts) {
            aiByIp.set(e.ip, { ...e, ts });
        }
    }
    const eventi = [];
    for (const [ip, e] of aiByIp.entries()) {
        const id = 'ai:' + ip + ':' + e.dominio;
        if (state.eventiIgnoredIds.has(id)) continue;
        eventi.push({
            id, type: 'ai', ip, ts: e.ts || 0,
            who: nomeStudente(ip) || ip,
            what: e.dominio,
            detail: 'richiesta AI rilevata',
        });
    }
    for (const ev of state.watchdogEvents || []) {
        if (ev.severity !== 'warning' && ev.severity !== 'critical') continue;
        const id = 'wd:' + ev.ip + ':' + ev.plugin + ':' + ev.ts;
        if (state.eventiIgnoredIds.has(id)) continue;
        const fmt = ev.format || (ev.plugin + ' ' + JSON.stringify(ev.payload || {}));
        eventi.push({
            id, type: 'wd', ip: ev.ip, ts: ev.ts,
            who: ev.nomeStudente || nomeStudente(ev.ip) || ev.ip,
            what: fmt.length > 50 ? fmt.slice(0, 47) + '…' : fmt,
            detail: ev.plugin || '',
            plugin: ev.plugin,
        });
    }
    eventi.sort((a, b) => (b.ts || 0) - (a.ts || 0));
    const aiIps = new Set(eventi.filter(x => x.type === 'ai').map(x => x.ip));
    const wdIps = new Set(eventi.filter(x => x.type === 'wd').map(x => x.ip));
    return {
        eventi,
        aiCount: aiIps.size,
        wdCount: wdIps.size,
        total: aiIps.size + wdIps.size,
        lastTs: eventi.length > 0 ? eventi[0].ts : 0,
    };
}

/** Banner alert unificato (sotto topbar). Visibile se total > 0 e !dismissed. */
export function renderAlertBanner() {
    const el = $('alert-banner');
    if (!el) return;
    const agg = aggregaEventiAlert();

    // Auto-reset dismissed quando arriva un nuovo evento (key cambia).
    const key = `${agg.aiCount}-${agg.wdCount}-${agg.lastTs}`;
    if (key !== state.bannerLastEventKey) {
        state.bannerLastEventKey = key;
        if (agg.total > 0) state.bannerDismissed = false;
    }

    if (agg.total === 0 || state.bannerDismissed) {
        el.classList.add('hidden');
        el.textContent = '';
        return;
    }
    el.classList.remove('hidden');

    const dominant = agg.aiCount >= agg.wdCount ? 'ai' : 'wd';
    const kind = state.bannerKind || 'pulse';
    el.className = `banner alert-unified ${dominant} ${kind}`;

    const parts = [];
    if (agg.aiCount > 0) parts.push(`${agg.aiCount} AI`);
    if (agg.wdCount > 0) parts.push(`${agg.wdCount} watchdog`);
    const headline = `${agg.total} event${agg.total === 1 ? 'o' : 'i'} · ${parts.join(' · ')}`;

    const sample0 = agg.eventi[0];
    const sampleTxt = sample0
        ? `· ${escapeHtml(sample0.who)} → ${escapeHtml(sample0.what)}`
        : '';

    const pulseCls = (kind === 'pulse') ? 'pulse' : '';
    const pillAi = agg.aiCount > 0 ? `<span class="pill ${pulseCls}">AI</span>` : '';
    const pillWd = agg.wdCount > 0 ? `<span class="pill warn ${pulseCls}">WD</span>` : '';

    const slideIcon = (kind === 'slide')
        ? '<svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M7 1.5L13 12H1L7 1.5z"/><path d="M7 6v3M7 10.5v.01"/></svg>'
        : '';

    el.innerHTML = `
        ${slideIcon}
        ${pillAi}${pillWd}
        <strong>${escapeHtml(headline)}</strong>
        <span class="banner-sample">${sampleTxt}</span>
        <span class="banner-spacer"></span>
        <button class="banner-btn" data-action="log-open" title="Apri log eventi">
            <svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><path d="M2 2h8v8H2z"/><path d="M4 4.5h4M4 6h4M4 7.5h2.5"/></svg>
            Apri log
        </button>
        <span class="x" data-action="banner-dismiss" title="Chiudi banner">&times;</span>
    `;
}

/** Pannello Log eventi (mutex con stream/detail). */
export function renderLogPanel() {
    const pane = document.getElementById('log-pane');
    if (!pane) return;
    const stream = document.getElementById('panel-richieste');
    const detail = document.getElementById('detail-pane');
    const btnToggle = document.getElementById('btn-toggle-log');

    if (!state.logPanelOpen) {
        pane.classList.add('hidden');
        if (pane.dataset.lastKey) {
            pane.innerHTML = '';
            delete pane.dataset.lastKey;
        }
        if (stream && !state.detailIp) stream.classList.remove('hidden-by-detail');
        if (btnToggle) btnToggle.classList.remove('attivo');
        return;
    }
    pane.classList.remove('hidden');
    if (stream) stream.classList.add('hidden-by-detail');
    if (detail) detail.classList.add('hidden'); // detail mutex con log
    if (btnToggle) btnToggle.classList.add('attivo');

    const agg = aggregaEventiAlert();
    const filtro = state.logFilter || 'all';

    // Skip rebuild se filtri/eventi non cambiati (no flicker click).
    const eventiKey = agg.eventi.map(e => e.id).join(',');
    const key = filtro + '|' + eventiKey + '|' + state.eventiIgnoredIds.size;
    if (pane.dataset.lastKey === key) return;
    pane.dataset.lastKey = key;
    const eventiFiltrati = agg.eventi.filter(e => {
        if (filtro === 'all') return true;
        return e.type === filtro;
    });

    const fmtTs = (ts) => ts ? new Date(ts).toLocaleTimeString('it-IT', { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : '—';

    pane.innerHTML = `
        <div class="detail-head">
            <div class="detail-title">
                <svg width="13" height="13" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><path d="M2 2h8v8H2z"/><path d="M4 4.5h4M4 6h4M4 7.5h2.5"/></svg>
                <div class="detail-title-text">
                    <div class="detail-nome">Log eventi</div>
                    <div class="detail-ip">${agg.eventi.length} totali</div>
                </div>
            </div>
            <button class="detail-x" data-action="log-close" title="Chiudi">&times;</button>
        </div>
        <div class="log-filtri">
            <button class="log-filtro-btn ${filtro === 'all' ? 'attivo' : ''}" data-action="log-filter" data-filter="all">Tutti · ${agg.eventi.length}</button>
            <button class="log-filtro-btn ai ${filtro === 'ai' ? 'attivo' : ''}" data-action="log-filter" data-filter="ai">AI · ${agg.aiCount}</button>
            <button class="log-filtro-btn wd ${filtro === 'wd' ? 'attivo' : ''}" data-action="log-filter" data-filter="wd">WD · ${agg.wdCount}</button>
            ${agg.eventi.length > 0 ? '<button class="log-filtro-btn ghost" data-action="ignora-tutti" title="Ignora tutti gli eventi correnti">Ignora tutto</button>' : ''}
        </div>
        <div class="detail-body">
            ${eventiFiltrati.length === 0 ? '<div class="detail-empty" style="padding: 20px; text-align: center;">Nessun evento.</div>' :
                eventiFiltrati.map(ev => {
                    const dotCls = ev.type === 'ai' ? 'alert' : 'warn';
                    const tipoLabel = ev.type === 'ai' ? 'AI' : 'WD';
                    return `
                        <div class="log-evento">
                            <span class="dot ${dotCls}"></span>
                            <span class="log-ts">${escapeHtml(fmtTs(ev.ts))}</span>
                            <div class="log-evento-body">
                                <div class="log-evento-head">
                                    <span class="log-tipo ${ev.type}">${tipoLabel}</span>${escapeHtml(ev.who)}
                                </div>
                                <div class="log-evento-detail">${escapeHtml(ev.what)} &middot; ${escapeHtml(ev.detail)}</div>
                                <div class="log-evento-actions">
                                    <button class="log-action-btn" data-action="evento-apri-studente" data-ip="${attrEscape(ev.ip)}">Apri studente</button>
                                    ${ev.type === 'ai' ? `<button class="log-action-btn danger" data-action="evento-blocca-dominio" data-ip="${attrEscape(ev.ip)}" data-dominio="${attrEscape(ev.what)}">Blocca dominio</button>` : ''}
                                    <button class="log-action-btn ghost" data-action="evento-ignora" data-id="${attrEscape(ev.id)}">Ignora</button>
                                </div>
                            </div>
                        </div>
                    `;
                }).join('')}
        </div>
    `;
}

export function renderWatchdogEventsPanel() {
    const panel = $('watchdog-events-panel');
    if (!panel) return;
    const cutoff = Date.now() - 5 * 60 * 1000;
    const recenti = state.watchdogEvents
        .filter(e => e.ts >= cutoff && e.severity !== 'info')
        .slice(-5)
        .reverse();
    if (recenti.length === 0) {
        panel.classList.add('hidden');
        panel.textContent = '';
        return;
    }
    panel.classList.remove('hidden');
    panel.innerHTML = '<strong>⚠️ Watchdog (ultimi 5 min):</strong>';
    for (const ev of recenti) {
        const row = document.createElement('div');
        row.className = 'watchdog-event watchdog-' + ev.severity;
        const ipLabel = ev.nomeStudente || ev.ip;
        const text = ev.format || (ev.plugin + ' ' + JSON.stringify(ev.payload || {}));
        row.textContent = `[${ipLabel}] ${text}`;
        panel.appendChild(row);
    }
}

/**
 * Conta gli eventi watchdog "rilevanti" (severity warning/critical)
 * negli ultimi 5 minuti per un IP. Usato per il badge sulla card.
 */
function watchdogBadgeCount(ip) {
    const arr = state.watchdogEventsPerIp.get(ip);
    if (!arr) return 0;
    const cutoff = Date.now() - 5 * 60 * 1000;
    let n = 0;
    for (const e of arr) {
        if (e.ts >= cutoff && e.severity !== 'info') n++;
    }
    return n;
}

/**
 * Floating selection bar: pillola sovrapposta sulla griglia, centrata
 * orizzontalmente, ancorata al bottom 24px. Visibile solo quando
 * `state.selectedIps.size > 0`. Le azioni agiscono SOLO sul subset
 * selezionato (gli endpoint Veyon usano `targetIps()` che ritorna
 * `selectedIps` quando non vuota; la blocca-dominio fa per-IP).
 */
export function renderSelectionBar() {
    const bar = $('selection-bar');
    if (!bar) return;
    const n = state.selectedIps.size;
    const veyonOn = !!state.veyonConfigured;

    // Skip rebuild se nulla di significativo e' cambiato. Senza, ogni
    // renderAll distruggeva e ricostruiva i bottoni → click "perso" durante
    // il flicker. Stesso fix applicato a detail-pane e log-pane.
    const key = n === 0 ? 'empty' : `n=${n}|veyon=${veyonOn}`;
    if (bar.dataset.lastKey === key) return;
    bar.dataset.lastKey = key;

    if (n === 0) {
        bar.classList.add('hidden');
        bar.textContent = '';
        return;
    }
    bar.classList.remove('hidden');

    // Icone SVG inline (Linear style, stroke 1.5, viewBox 12).
    const icoLock   = '<svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><rect x="2.5" y="5.5" width="7" height="5" rx="1"/><path d="M4 5.5V3.5a2 2 0 1 1 4 0v2"/></svg>';
    const icoUnlock = '<svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><rect x="2.5" y="5.5" width="7" height="5" rx="1"/><path d="M4 5.5V3.5a2 2 0 0 1 3.8-.9"/></svg>';
    const icoMsg    = '<svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"><path d="M1.5 2.5h9v6h-5L3.5 10.5v-2H1.5v-6z"/></svg>';
    const icoPlugOn = '<svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M4 1v3M8 1v3M3 4h6v3a3 3 0 0 1-6 0V4zM6 10v1.5"/></svg>';
    const icoPlugOff= '<svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M4 1v3M8 1v3M3 4h6v3a3 3 0 0 1-6 0V4zM6 10v1.5M1 1l10 10"/></svg>';

    bar.innerHTML =
        '<span class="sel-count">' + n + ' selezionat' + (n === 1 ? 'o' : 'i') + '</span>'
        + '<span class="sel-sep"></span>'
        + (veyonOn
            ? '<button class="sel-btn" data-action="veyon-classe-lock" title="Blocca schermo">' + icoLock + ' Blocca schermo</button>'
            + '<button class="sel-btn" data-action="veyon-classe-unlock" title="Sblocca schermo">' + icoUnlock + ' Sblocca</button>'
            + '<button class="sel-btn" data-action="veyon-classe-msg" title="Messaggia">' + icoMsg + ' Messaggia</button>'
            + '<button class="sel-btn" data-action="veyon-distribuisci-proxy" title="Distribuisci proxy">' + icoPlugOn + ' Proxy on</button>'
            + '<button class="sel-btn" data-action="veyon-disinstalla-proxy" title="Rimuovi proxy">' + icoPlugOff + ' Proxy off</button>'
            : '')
        + '<button class="sel-btn danger" data-action="multi-blocca-dominio" title="Blocca dominio per i selezionati">Blocca dominio</button>'
        + '<span class="sel-sep"></span>'
        + '<button class="sel-btn ghost" data-action="clear-selection" title="Pulisci selezione">&times;</button>';
}

/**
 * Renderizza il detail pane laterale destro per state.detailIp.
 * Quando detailIp e' null, nasconde il pannello e ripristina lo stream;
 * altrimenti popola header (status dot + nome + ip + X), banner AI
 * condizionale, e 4 sezioni: azioni rapide / watchdog / domini recenti
 * / sessione. Mutex con stream: .panel.narrow viene nascosto via classe
 * .layout-with-detail su .main-panels.
 */
export function renderDetailPane() {
    const pane = document.getElementById('detail-pane');
    if (!pane) return;
    const stream = document.getElementById('panel-richieste');
    const ip = state.detailIp;

    if (!ip) {
        pane.classList.add('hidden');
        if (pane.dataset.lastKey) {
            pane.innerHTML = '';
            delete pane.dataset.lastKey;
        }
        if (stream) stream.classList.remove('hidden-by-detail');
        return;
    }
    pane.classList.remove('hidden');
    if (stream) stream.classList.add('hidden-by-detail');

    const ora = Date.now();
    const soglia = (state.cfg.inattivitaSogliaSec || 180) * 1000;
    const s = calcolaStatoIp(ip, ora, soglia);
    const wdEvts = (state.watchdogEventsPerIp && state.watchdogEventsPerIp.get(ip)) || [];
    const hasAI = hasAIRecente(ip, ora);
    const hasWD = hasWDRecente(ip, ora);
    let stato = statoBase(ip, ora);
    if (hasWD) stato = 'watchdog';
    if (hasAI) stato = 'ai';

    // Skip innerHTML rebuild se nulla di significativo e' cambiato. Senza
    // questo, ogni renderAll (~ogni evento SSE + setInterval 5s) distrugge
    // e ricostruisce il DOM del pane: i bottoni venivano rimpiazzati durante
    // un click → click "perso" → flicker.
    const perIpCnt = (state.blocchiPerIp.get(ip) || new Set()).size;
    const key = [
        ip, stato, s.listaAttive.length, s.dominiMap.size,
        wdEvts.length, perIpCnt,
        (state.watchdogPlugins || []).length,
    ].join('|');
    if (pane.dataset.lastKey === key) return;
    pane.dataset.lastKey = key;

    const dotCls = ({ active: 'ok', idle: 'muted', offline: 'muted', ai: 'alert', watchdog: 'warn' })[stato] || 'muted';
    const nome = s.nome || ('.' + ip.split('.').pop());

    // Domini aggregati per IP: count + ultima ora.
    const lista = state.perIp.get(ip) || [];
    const dominiAgg = new Map(); // dominio -> {count, ultimaTs, tipo}
    for (const e of lista) {
        if (e.tipo === 'sistema') continue;
        const r = dominiAgg.get(e.dominio) || { count: 0, ultimaTs: 0, tipo: e.tipo };
        r.count++;
        const t = parseOra(e.ora)?.getTime() || 0;
        if (t > r.ultimaTs) r.ultimaTs = t;
        dominiAgg.set(e.dominio, r);
    }
    const dominiOrdered = [...dominiAgg.entries()]
        .sort(([, a], [, b]) => b.ultimaTs - a.ultimaTs)
        .slice(0, 12);

    // Sessione: connesso = primo evento, durata = now - connesso, ultima.
    const primoTs = lista.length > 0 ? (parseOra(lista[0].ora)?.getTime() || 0) : 0;
    const ultimaTs = lista.length > 0 ? (parseOra(lista[lista.length - 1].ora)?.getTime() || 0) : 0;
    const fmtTime = (ts) => ts ? new Date(ts).toLocaleTimeString('it-IT', { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : '—';
    const fmtDur = (msFrom) => {
        if (!msFrom) return '—';
        const sec = Math.floor((ora - msFrom) / 1000);
        const m = Math.floor(sec / 60), s2 = sec % 60;
        const h = Math.floor(m / 60);
        if (h > 0) return `${h}h ${m % 60}m`;
        if (m > 0) return `${m}m ${String(s2).padStart(2, '0')}s`;
        return `${s2}s`;
    };

    // Watchdog plugin status: label corte e messaggio "ok" specifico per
    // plugin (Claude Designer). Quando arriva un evento warning/critical,
    // mostro il testo formattato del payload, troncato.
    const PLUGIN_META = {
        usb:     { label: 'USB',      okDetail: 'nessun dispositivo' },
        process: { label: 'Processi', okDetail: 'nessun processo sospetto' },
        network: { label: 'Network',  okDetail: 'instradato via proxy' },
    };
    const plugins = state.watchdogPlugins || [];
    const wdRecentCutoff = ora - ALERT_WD_CUTOFF_MS;
    const pluginRows = plugins.map(p => {
        const meta = PLUGIN_META[p.id] || { label: p.name || p.id, okDetail: 'ok' };
        // Eventi recenti: sia eventi originati dal plugin (es. "USB inserita")
        // sia meta-eventi "watchdog-<plugin>" (stopped) emessi dal server
        // quando il plugin va silente. Cosi' il pallino riflette anche il
        // caso "il plugin USB e' stato killato dallo studente".
        // Coerente con statoPlugins (card grid): l'evento warning/critical
        // piu' grave nei 5 min determina il colore. Un info successivo
        // NON azzera il warning, MA se l'utente l'ha esplicitamente
        // ignorato e poi e' arrivato un info ("USB removed"), il warning
        // viene considerato "risolto" → torna verde subito.
        const v = valutaWdPlugin(ip, p.id, wdEvts, wdRecentCutoff, state.eventiIgnoredIds);
        let st = 'ok';
        let detail = meta.okDetail;
        if (v.topEv && !v.resolved) {
            if (v.severity === 'critical') st = 'alert';
            else if (v.severity === 'warning') st = 'warn';
            const formatted = v.topEv.format || JSON.stringify(v.topEv.payload || {});
            detail = formatted;
            if (typeof detail === 'string' && detail.length > 60) detail = detail.slice(0, 57) + '…';
        }
        return { name: meta.label, state: st, detail };
    });

    const aiDom = dominiOrdered.find(([d]) => isAIDomainNome(d))?.[0];
    const aiUltima = aiDom ? fmtTime(dominiAgg.get(aiDom)?.ultimaTs || 0) : '';

    // Render via innerHTML (rebuild ad ogni renderAll: il detail pane e'
    // statico e poco costoso, no need per syncChildren qui).
    pane.innerHTML = `
        <div class="detail-head">
            <div class="detail-title">
                <span class="dot ${dotCls}"></span>
                <div class="detail-title-text">
                    <div class="detail-nome">${escapeHtml(nome)}</div>
                    <div class="detail-ip">${escapeHtml(ip)}</div>
                </div>
            </div>
            <button class="detail-x" data-action="detail-close" title="Chiudi">&times;</button>
        </div>
        ${stato === 'ai' && aiDom ? `
            <div class="detail-banner alert">
                <strong>&#9888; AI rilevata</strong>
                <div class="detail-banner-sub">${escapeHtml(aiDom)} &middot; ultima richiesta ${escapeHtml(aiUltima)}</div>
            </div>` : ''}
        <div class="detail-body">
            <div class="detail-section">
                <div class="detail-section-label">Azioni rapide</div>
                <div class="detail-actions">
                    <button class="btn" data-action="veyon-card-lock" data-ip="${attrEscape(ip)}" title="Blocca schermo">Blocca schermo</button>
                    <button class="btn" data-action="veyon-card-unlock" data-ip="${attrEscape(ip)}" title="Sblocca schermo">Sblocca schermo</button>
                    <button class="btn" data-action="veyon-card-msg" data-ip="${attrEscape(ip)}" title="Messaggia">Messaggia</button>
                    <button class="btn" data-action="veyon-card-distribuisci-proxy" data-ip="${attrEscape(ip)}" title="Invia proxy_on.vbs">Invia proxy</button>
                    <button class="btn" data-action="veyon-card-disinstalla-proxy" data-ip="${attrEscape(ip)}" title="Rimuovi proxy">Rimuovi proxy</button>
                    <button class="btn danger detail-action-wide" data-action="detail-blocca-dominio" title="Blocca un dominio">Blocca dominio</button>
                </div>
            </div>
            <div class="detail-section">
                <div class="detail-section-label">Blocchi attivi${(() => {
                    const set = state.blocchiPerIp.get(ip);
                    return set && set.size > 0 ? ` &middot; ${set.size}` : '';
                })()}</div>
                <div class="detail-blocchi">
                    ${(() => {
                        const set = state.blocchiPerIp.get(ip);
                        if (!set || set.size === 0) {
                            return '<div class="detail-empty">nessun blocco specifico</div>';
                        }
                        return [...set].sort().map(d => `
                            <div class="detail-blocco-row">
                                <span class="detail-blocco-name">${escapeHtml(d)}</span>
                                <button class="detail-blocco-x" data-action="unblock-per-ip" data-ip="${attrEscape(ip)}" data-dominio="${attrEscape(d)}" title="Rimuovi blocco">&times;</button>
                            </div>
                        `).join('');
                    })()}
                </div>
            </div>
            <div class="detail-section">
                <div class="detail-section-label">Watchdog</div>
                <div class="detail-watchdog">
                    ${pluginRows.length === 0 ? '<div class="detail-empty">nessun plugin watchdog</div>' :
                        pluginRows.map(w => `
                            <div class="detail-wd-row">
                                <span class="dot ${w.state}"></span>
                                <span class="detail-wd-name">${escapeHtml(w.name)}</span>
                                <span class="detail-wd-detail">${escapeHtml(w.detail)}</span>
                            </div>
                        `).join('')}
                </div>
            </div>
            <div class="detail-section">
                <div class="detail-section-label">Domini recenti &middot; ${s.listaAttive.length} richieste</div>
                <div class="detail-domini">
                    ${dominiOrdered.length === 0 ? '<div class="detail-empty">nessun dominio</div>' :
                        dominiOrdered.map(([d, r]) => {
                            const isAI = r.tipo === 'ai';
                            const cnt = r.count;
                            const diff = ora - r.ultimaTs;
                            const ago = formatRelativo(Math.floor(diff / 1000));
                            return `
                                <div class="detail-dom-row${isAI ? ' ai' : ''}">
                                    <span class="detail-dom-name">${escapeHtml(d)}</span>
                                    <span class="detail-dom-count">${cnt}</span>
                                    <span class="detail-dom-ago">${escapeHtml(ago)}</span>
                                </div>
                            `;
                        }).join('')}
                </div>
            </div>
            <div class="detail-section">
                <div class="detail-section-label">Sessione</div>
                <div class="detail-sessione">
                    <span>connesso</span><span>${fmtTime(primoTs)}</span>
                    <span>durata</span><span>${fmtDur(primoTs)}</span>
                    <span>ultima</span><span>${fmtTime(ultimaTs)}</span>
                    <span>OS</span><span class="muted">&mdash;</span>
                    <span>browser</span><span class="muted">&mdash;</span>
                    <span>MAC</span><span class="muted">&mdash;</span>
                </div>
            </div>
        </div>
    `;
}

/** Esegue tutti i renderer sincronamente. Chiamato da `renderAll` dentro RAF. */
export function renderAllSync() { _renderAllSync(); }
function _renderAllSync() {
    // Ogni renderer in try/catch isolato: se un renderer crasha (tipico:
    // elemento DOM rimosso nel redesign UI ma reference ancora viva),
    // gli altri continuano. Un singolo throw NON deve piu' interrompere
    // la cascata e impedire `avviaSSE()` di partire (sintomo: bottoni
    // fanno il POST ma il broadcast non si applica perche' SSE mai aperto).
    const safe = (name, fn) => {
        try { fn(); }
        catch (e) { console.error('[planck] renderer crash:', name, e); }
    };
    safe('renderSidebar', renderSidebar);
    safe('renderStats', renderStats);
    safe('renderPausaEBottoni', renderPausaEBottoni);
    safe('renderTabellaIp', renderTabellaIp);
    safe('renderSelectionBar', renderSelectionBar);
    // renderWatchdogEventsPanel rimosso: gli eventi watchdog appaiono nel
    // banner alert + log eventi (a destra), non piu' duplicati in griglia.
    safe('renderUltimeRichieste', renderUltimeRichieste);
    safe('renderDetailPane', renderDetailPane);
    safe('renderLogPanel', renderLogPanel);
    safe('renderAlertBanner', renderAlertBanner);
    safe('renderFocus', renderFocus);
    safe('renderReport', renderReport);
    safe('renderImpostazioni', renderImpostazioni);
    safe('renderWatchdogPluginsList', renderWatchdogPluginsList);
    safe('renderAIListStatus', renderAIListStatus);
    safe('renderCountdown', renderCountdown);
    safe('aggiornaTopbarCount', aggiornaTopbarCount);
    safe('aggiornaToggleArrows', aggiornaToggleArrows);
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

/**
 * @file Azioni utente.
 *
 * Ogni funzione esportata e' un "command" invocato dall'event delegation di
 * `app.js`. Chiama le API backend e aggiorna `state` (eventualmente
 * triggerando un `renderAll()`).
 *
 * Convenzioni v2 (cambiato rispetto a v1):
 *   - POST + body JSON per tutte le mutazioni (era GET + query params)
 *   - Risposte mutazioni: {ok:true, ...} | {ok:false, error, code}
 *   - Auth Basic browser-side: il browser gestisce il challenge 401
 *
 * I `confirm`/`prompt`/`alert` sono quelli nativi del browser: volutamente
 * sincroni e brutti, per distinguere bene le azioni distruttive.
 */

import { state, salvaNascosti, salvaDarkmode, salvaNotifiche, salvaTab, salvaVistaIp, salvaCollassi } from './state.js';
import { renderAll, aggiornaToggleButtons, aggiornaSelectPresets, aggiornaInputDeadline, renderTabs } from './render.js';
import { toast } from './toast.js';
import { showPromptModal } from './modal.js';

/** Wrapper GET → JSON. */
async function apiGet(path) {
    const r = await fetch(path);
    return r.json();
}

/** Wrapper POST con body JSON. */
async function apiPost(path, body) {
    console.log('[planck] apiPost ->', path, body || '');
    try {
        const r = await fetch(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: body !== undefined ? JSON.stringify(body) : '',
        });
        const j = await r.json();
        console.log('[planck] apiPost <-', path, 'status=', r.status, 'body=', j);
        return j;
    } catch (err) {
        console.error('[planck] apiPost ERROR', path, err);
        throw err;
    }
}

// ========================================================================
// Blocklist
// ========================================================================

export async function bloccaDominio(d) { await apiPost('/api/block', { dominio: d }); }
export async function sbloccaDominio(d) { await apiPost('/api/unblock', { dominio: d }); }
export async function bloccaAI() { await apiPost('/api/block-all-ai'); }
export async function sbloccaAI() { await apiPost('/api/unblock-all-ai'); }
/**
 * Toggle "Blocca AI": leggiamo lo stato DAL BOTTONE (.active = AI bloccate).
 * Piu' robusto che ricalcolare da state.cfg.dominiAI vs state.bloccati,
 * che possono divergere se la lista AI lato server include domini non
 * presenti nel snapshot client. Feedback ottimistico immediato + il render
 * normale via SSE conferma/corregge.
 */
export async function toggleBloccaAI() {
    const btn = document.getElementById('btn-block-ai');
    const isActive = btn && btn.classList.contains('active');
    if (btn) {
        if (isActive) {
            btn.classList.remove('active');
            btn.textContent = 'Blocca AI';
        } else {
            btn.classList.add('active');
            btn.textContent = 'Sblocca AI';
        }
    }
    if (isActive) await apiPost('/api/unblock-all-ai');
    else await apiPost('/api/block-all-ai');
}

export async function svuotaBlocklist() {
    if (!confirm('Svuotare completamente la blocklist?')) return;
    await apiPost('/api/clear-blocklist');
}

/**
 * Reset completo: rimuove tutti i blocchi domini (generali + per-IP, oggi
 * tutti convergono nella stessa blocklist), disattiva la pausa globale se
 * attiva, e sblocca tutti gli schermi via Veyon (se configurato). Una sola
 * conferma utente, niente prompt nested.
 */
export async function resetTutto() {
    if (!confirm('Reset: svuoto eventi e richieste, rimuovo tutti i blocchi e sblocco i PC. Continuare?')) return;
    // 1. Svuota la blocklist (cattura sia "Blocca tutto AI" che blocchi puntuali).
    await apiPost('/api/clear-blocklist');
    // 2. Disattiva la pausa globale se attiva.
    if (state.pausato) await apiPost('/api/pause/toggle');
    // 3. Sblocca schermi su tutti i target Veyon (silenziosamente, no confirm).
    if (state.veyonConfigured) {
        const { ips } = targetIps();
        await Promise.all(ips.map(ip => veyonSendFeature(ip, 'screenUnlock').catch(() => false)));
        for (const ip of ips) state.lockedIps.delete(ip);
    }
    // 4. Svuota runtime server (storia traffic + tracking alert watchdog).
    //    Il server broadcasta 'reset-runtime' che sull'SSE pulisce le mappe
    //    aggregate client (entries, perIp, perDominio, watchdogEvents,
    //    eventiIgnoredIds). Il banner alert si svuota di conseguenza.
    await apiPost('/api/reset-runtime');
    toast.success('Reset completato');
}

// ========================================================================
// Sessione
// ========================================================================

/** Avvia una nuova sessione (no-op se gia' attiva). L'archiviazione
    avviene SOLO premendo Stop, mai automaticamente in Start. */
export async function startSessione() {
    if (state.sessioneAttiva) {
        toast.info('Una sessione e\' gia\' in corso. Premi Stop per archiviarla prima di avviarne un\'altra.');
        return;
    }
    const r = await apiPost('/api/session/start');
    state.sessioneInizio = r.sessioneInizio;
    state.sessioneAttiva = true;
    state.sessioneFineISO = null;
    toast.success('Registrazione avviata');
}

/** Ferma e archivia la sessione corrente (no-op se nessuna attiva). */
export async function stopSessione() {
    if (!state.sessioneAttiva) return;
    // Confirm arricchito con durata HH:MM:SS + count eventi non-sistema —
    // l'utente sa cosa sta archiviando prima di accettare.
    let durTxt = '00:00:00';
    if (state.sessioneInizio) {
        const start = Date.parse(state.sessioneInizio);
        if (!isNaN(start)) {
            const sec = Math.max(0, Math.floor((Date.now() - start) / 1000));
            const h = String(Math.floor(sec / 3600)).padStart(2, '0');
            const m = String(Math.floor((sec % 3600) / 60)).padStart(2, '0');
            const s = String(sec % 60).padStart(2, '0');
            durTxt = `${h}:${m}:${s}`;
        }
    }
    let nEventi = 0;
    for (const e of state.entries) if (e.tipo !== 'sistema') nEventi++;
    const r = await apiPost('/api/session/stop');
    // Dopo lo stop, chiedi all'utente un nome custom (modal in stile app).
    // Esc / Annulla → mantiene il default. r.archiviata e' "<id>-<inizio>.json".
    if (r.archiviata) {
        const m = r.archiviata.match(/^(\d+)-/);
        const sessionId = m ? parseInt(m[1], 10) : 0;
        if (sessionId > 0) {
            const nome = await showPromptModal({
                title: 'Sessione archiviata',
                message: `Durata ${durTxt} · ${nEventi} eventi. Dai un nome alla sessione (opzionale, oltre a data e ora).`,
                placeholder: 'Es. "Verifica Storia 5B"',
                okLabel: 'Salva',
                cancelLabel: 'Salta',
            });
            if (nome !== null && nome.trim()) {
                await apiPost('/api/session/rename', { id: sessionId, titolo: nome.trim() });
            }
        }
        await ricaricaSessioni();
    }
}

/** Toggle della sessione: delega a stopSessione (con confirm durata+eventi)
    o startSessione (con toast di conferma). Sostituisce i 2 bottoni
    Rec/Stop separati con un solo bottone in stile blocca-tutto/blocca-ai. */
export async function toggleSessione() {
    if (state.sessioneAttiva) {
        await stopSessione();
    } else {
        await startSessione();
    }
}

export async function esportaSessione() {
    const r = await fetch('/api/export');
    const blob = await r.blob();
    const disposizione = r.headers.get('Content-Disposition') || '';
    const match = disposizione.match(/filename="([^"]+)"/);
    const nome = match ? match[1] : 'sessione.json';
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url; a.download = nome; document.body.appendChild(a); a.click();
    setTimeout(() => { URL.revokeObjectURL(url); a.remove(); }, 500);
}

// ========================================================================
// Preset (snapshot blocklist)
// ========================================================================

export async function caricaPreset(nome) {
    if (!nome) return;
    if (!confirm(`Caricare il preset "${nome}"? Sostituira\' la blocklist corrente.`)) return;
    await apiPost('/api/preset/load', { nome });
}

export async function salvaPreset() {
    const nome = prompt('Nome del preset (lettere, numeri, _, -):');
    if (!nome) return;
    const r = await apiPost('/api/preset/save', { nome });
    if (r.ok) {
        // Refresh elenco preset
        const lista = await apiGet('/api/presets');
        state.cfg.presets = lista.presets || [];
        aggiornaSelectPresets();
        toast.success(`Preset "${nome}" salvato.`);
    } else {
        toast.error('Errore salvataggio preset: ' + (r.error || ''));
    }
}

// ========================================================================
// Pausa / Deadline
// ========================================================================

export async function togglePausa() { await apiPost('/api/pause/toggle'); }

export async function impostaDeadline(time) {
    if (!time) return;
    await apiPost('/api/deadline/set', { time });
}

export async function annullaDeadline() {
    await apiPost('/api/deadline/clear');
}

// ========================================================================
// UI: nascosti / filtro / focus / sezioni / vista / collassi
// ========================================================================

export function nascondiDominio(d) { state.nascosti.add(d); salvaNascosti(); renderAll(); }
export function mostraDominio(d) { state.nascosti.delete(d); salvaNascosti(); renderAll(); }
export function resetNascosti() { state.nascosti.clear(); salvaNascosti(); renderAll(); }

export function setFocus(ip) { state.focusIp = state.focusIp === ip ? null : ip; renderAll(); }

/** Apre il detail pane su `ip`: filtra anche il traffico (focusIp).
    Mutex: chiude eventuale log panel aperto. */
export function apriDetail(ip) {
    state.detailIp = ip;
    state.focusIp = ip;
    state.logPanelOpen = false;
    renderAll();
}
/** Chiude il detail pane (torna lo stream a destra) e libera il focus. */
export function chiudiDetail() {
    state.detailIp = null;
    state.focusIp = null;
    renderAll();
}
/** Toggle: stesso ip → chiudi, altrimenti → apri/sposta. */
export function toggleDetail(ip) {
    if (state.detailIp === ip) chiudiDetail();
    else apriDetail(ip);
}
/**
 * Blocca un dominio SOLO per lo studente del detail pane corrente
 * (blocco per-IP, additivo rispetto alla blocklist globale). Prompt()
 * pre-popolato con l'ultimo dominio AI rilevato per quell'IP se presente.
 */
export async function detailBloccaDominio() {
    const ip = state.detailIp;
    if (!ip) return;
    let suggerito = '';
    const lista = state.perIp.get(ip) || [];
    const aiEntry = lista.slice().reverse().find(e => e.tipo === 'ai');
    if (aiEntry) suggerito = aiEntry.dominio;
    const d = prompt('Blocca per ' + ip + ':', suggerito);
    if (d === null) return;
    const dominio = d.trim().toLowerCase();
    if (!dominio) return;
    await apiPost('/api/block-per-ip', { ip, dominio });
}

/** Blocca un dominio per uno specifico IP (additivo). */
export async function bloccaPerIp(ip, dominio) {
    if (!ip || !dominio) return;
    await apiPost('/api/block-per-ip', { ip, dominio });
}

/** Sblocca un dominio per uno specifico IP. */
export async function sbloccaPerIp(ip, dominio) {
    if (!ip || !dominio) return;
    await apiPost('/api/unblock-per-ip', { ip, dominio });
}

/** Rimuove TUTTI i blocchi per-IP per uno specifico IP. */
export async function clearBlocchiPerIp(ip) {
    if (!ip) return;
    await apiPost('/api/clear-blocks-for-ip', { ip });
}

// ========================================================================
// Banner alert + Log eventi (Phase 7)
// ========================================================================

/** Chiude il banner per la sessione UI corrente. Riappare al prossimo evento. */
export function dismissBanner() {
    state.bannerDismissed = true;
    renderAll();
}

/** Apre il pannello "Log eventi" a destra, mutex con stream/detail. */
export function apriLogEventi() {
    state.logPanelOpen = true;
    state.detailIp = null;
    state.focusIp = null;
    renderAll();
}

/** Chiude il pannello "Log eventi" → torna lo stream. */
export function chiudiLogEventi() {
    state.logPanelOpen = false;
    renderAll();
}

/** Toggle "Log eventi": apre se chiuso, chiude se aperto. */
export function toggleLogEventi() {
    if (state.logPanelOpen) chiudiLogEventi();
    else apriLogEventi();
}

/** Imposta il filtro lista log: 'all' | 'ai' | 'wd'. */
export function setLogFilter(filtro) {
    if (filtro === 'all' || filtro === 'ai' || filtro === 'wd') {
        state.logFilter = filtro;
        renderAll();
    }
}

/** Marca un evento come ignorato (sparisce dal feed log + banner). */
export function ignoraEvento(id) {
    if (!id) return;
    state.eventiIgnoredIds.add(id);
    renderAll();
}

/** Marca TUTTI gli eventi correntemente in lista come ignorati.
 *  Equivalente a cliccare "Ignora" su ognuno: spariscono da banner+log
 *  ma non vengono cancellati dal DB (history). */
export function ignoraTuttiEventi() {
    // Itera state.entries (per AI) + state.watchdogEvents (per WD)
    // costruendo gli stessi id di aggregaEventiAlert.
    let n = 0;
    const aiSeen = new Set();
    for (const e of state.entries) {
        if (e.tipo !== 'ai') continue;
        const id = 'ai:' + e.ip + ':' + e.dominio;
        if (aiSeen.has(id)) continue;
        aiSeen.add(id);
        if (!state.eventiIgnoredIds.has(id)) { state.eventiIgnoredIds.add(id); n++; }
    }
    for (const ev of state.watchdogEvents || []) {
        if (ev.severity !== 'warning' && ev.severity !== 'critical') continue;
        const id = 'wd:' + ev.ip + ':' + ev.plugin + ':' + ev.ts;
        if (!state.eventiIgnoredIds.has(id)) { state.eventiIgnoredIds.add(id); n++; }
    }
    renderAll();
    if (n > 0) toast.info(`${n} ${n === 1 ? 'evento ignorato' : 'eventi ignorati'}`);
}

/** Apre lo studente dell'evento e chiude il log. */
export function eventoApriStudente(ip) {
    if (!ip) return;
    state.logPanelOpen = false;
    apriDetail(ip);
}

/**
 * Blocca un dominio per ogni IP attualmente in selezione multipla
 * (NON globale). Prompt() per chiedere il dominio una sola volta.
 */
export async function bloccaDominioSelezione() {
    const ips = [...state.selectedIps];
    if (ips.length === 0) return;
    const d = prompt('Blocca dominio per ' + ips.length + ' studenti:', '');
    if (d === null) return;
    const dominio = d.trim().toLowerCase();
    if (!dominio) return;
    await Promise.all(ips.map(ip => apiPost('/api/block-per-ip', { ip, dominio })));
    toast.success('Bloccato per ' + ips.length + ' studenti: ' + dominio);
}
export function clearFocus() { state.focusIp = null; renderAll(); }

/** Lista IP nello stesso ordine usato dal render (sortati per IP numerico). */
function ipsInOrder() {
    const ips = new Set([...state.perIp.keys(), ...state.aliveMap.keys()]);
    return [...ips].sort((a, b) => {
        const A = a.split('.').reduce((n, p) => n * 256 + parseInt(p, 10), 0);
        const B = b.split('.').reduce((n, p) => n * 256 + parseInt(p, 10), 0);
        return A - B;
    });
}

/**
 * Gestisce il click su una card / riga studente. Tre comportamenti:
 *   - plain click       : toggle detail pane sull'IP, azzera selezione multi
 *   - Ctrl/Cmd + click  : toggle dell'IP nella selezione multi.
 *                         Se c'era un detail aperto, l'IP del detail viene
 *                         INCORPORATO nella selezione (non perso) prima del
 *                         toggle. Il detail viene chiuso.
 *   - Shift + click     : range da anchor a IP (estremi inclusi).
 *                         Se c'era un detail aperto, il suo IP viene aggiunto
 *                         al range. Il detail viene chiuso.
 *
 * Anchor viene aggiornato sempre all'IP cliccato.
 */
export function handleCardClick(ip, ev) {
    if (ev && ev.shiftKey && state.selectionAnchor) {
        const list = ipsInOrder();
        const i1 = list.indexOf(state.selectionAnchor);
        const i2 = list.indexOf(ip);
        if (i1 >= 0 && i2 >= 0) {
            const [lo, hi] = i1 < i2 ? [i1, i2] : [i2, i1];
            const range = new Set();
            for (let k = lo; k <= hi; k++) range.add(list[k]);
            // Conserva l'IP del detail pane se aperto: entra nel range.
            if (state.detailIp) range.add(state.detailIp);
            state.selectedIps = range;
            state.detailIp = null;
            state.focusIp = null;
            state.selectionAnchor = ip;
            renderAll();
            return;
        }
    }
    if (ev && (ev.ctrlKey || ev.metaKey)) {
        // Se c'era un detail aperto e niente in selezione, includilo come
        // primo elemento prima del toggle dell'IP cliccato.
        if (state.detailIp && state.selectedIps.size === 0) {
            state.selectedIps.add(state.detailIp);
        }
        if (state.selectedIps.has(ip)) state.selectedIps.delete(ip);
        else state.selectedIps.add(ip);
        state.detailIp = null;
        state.focusIp = null;
        state.selectionAnchor = ip;
        renderAll();
        return;
    }
    // plain click: apre/chiude il detail pane, azzera selezione multi.
    state.selectedIps.clear();
    state.selectionAnchor = ip;
    toggleDetail(ip);
}

/** Svuota la selezione multipla. */
export function clearSelection() {
    state.selectedIps.clear();
    state.selectionAnchor = null;
    renderAll();
}

export function setFiltro(val) { state.filtro = val; renderAll(); }

export function toggleSezione(nome) {
    const sez = document.getElementById('sezione-' + nome);
    if (sez) sez.classList.toggle('collapsed');
}

export function cambiaVistaIp(vista) {
    if (vista !== 'griglia' && vista !== 'lista') return;
    if (state.vistaIp === vista) return;
    state.vistaIp = vista;
    salvaVistaIp();
    renderAll();
}

export function toggleSidebar() {
    state.sidebarCollassata = !state.sidebarCollassata;
    salvaCollassi();
    applicaCollassi();
    renderAll(); // sync frecce SVG dei toggle in toolbar
}

export function toggleRichieste() {
    state.richiesteCollassate = !state.richiesteCollassate;
    salvaCollassi();
    applicaCollassi();
    renderAll();
}

export function applicaCollassi() {
    const sb = document.getElementById('sidebar-domini');
    const pr = document.getElementById('panel-richieste');
    if (sb) sb.classList.toggle('collassata', state.sidebarCollassata);
    if (pr) pr.classList.toggle('collassata', state.richiesteCollassate);
    // Feedback visivo "attivo" sui bottoni quando le rispettive sidebar
    // sono APERTE (non collassate). Coerente con btn-toggle-log/EVENTI.
    const btnSb = document.getElementById('btn-toggle-sidebar');
    const btnSt = document.getElementById('btn-toggle-stream');
    if (btnSb) btnSb.classList.toggle('attivo', !state.sidebarCollassata);
    if (btnSt) btnSt.classList.toggle('attivo', !state.richiesteCollassate);
}

// ========================================================================
// Tema / notifiche
// ========================================================================

export function toggleDarkmode() {
    state.darkmode = !state.darkmode;
    salvaDarkmode();
    aggiornaToggleButtons();
}

export async function toggleNotifiche() {
    if (!state.notifiche) {
        if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
            await Notification.requestPermission();
        }
        state.notifiche = true;
    } else {
        state.notifiche = false;
    }
    salvaNotifiche();
    aggiornaToggleButtons();
}

// ========================================================================
// Tabs
// ========================================================================

export function cambiaTab(nome) {
    state.tabAttivo = nome;
    salvaTab();
    if (nome !== 'report' && state.datiSessioneVisualizzata) {
        state.datiSessioneVisualizzata = null;
        state.sessioneVisualizzata = null;
    }
    renderTabs();
    renderAll();
    if (nome === 'impostazioni' || nome === 'report') ricaricaSessioni();
    if (nome === 'impostazioni') veyonAggiornaStato();
}

/** Switch tra i sub-tab di Impostazioni (Generale / Rete / Domini / etc.). */
export function cambiaSubtabImpostazioni(nome) {
    if (!nome) return;
    state.settingsSubtab = nome;
    localStorage.setItem('settingsSubtab', nome);
    document.querySelectorAll('.settings-tab-btn').forEach(b => {
        b.classList.toggle('active', b.dataset.subtab === nome);
    });
    document.querySelectorAll('.settings-tab-panel').forEach(p => {
        p.classList.toggle('active', p.dataset.subtab === nome);
    });
}

// (Rimosso in v2.6.0: edit nome studente, classi/lab salvate.)
// La mappa "studenti" e' ora una semplice lista di IP del /24 corrente,
// generata server-side al boot. Niente CRUD, niente persistence.

// ========================================================================
// Settings (config modificabile da UI)
// ========================================================================

export async function settingsCampoModificato(el) {
    const key = el.dataset.key;
    if (!key) return;
    let value;
    if (el.type === 'checkbox') value = el.checked;
    else if (el.type === 'number') {
        const n = Number(el.value);
        if (!Number.isFinite(n)) return;
        value = n;
    } else if (el.type === 'password') {
        if (!el.value) return;
        value = el.value;
    } else {
        value = el.value;
    }

    const r = await fetch('/api/settings/update', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ [key]: value }),
    }).then(r => r.json()).catch(() => null);

    if (!r) { toast.error('Errore rete'); return; }
    if (r.rejected && r.rejected.length > 0) {
        toast.error(`Valore rifiutato per: ${r.rejected.join(', ')}`);
    }
    state.settings = r.settings;
    if (r.richiedeRiavvio && r.richiedeRiavvio.length > 0) {
        state.riavvioRichiesto = true;
    }
    if (el.type === 'password') el.value = '';
    renderAll();
}

export async function aggiungiIgnorato() {
    const el = document.getElementById('nuovo-ignorato');
    const dominio = (el.value || '').trim();
    if (!dominio) return;
    const r = await apiPost('/api/settings/ignorati/add', { dominio });
    if (r.ok) { el.value = ''; el.focus(); }
    else toast.error('Errore: ' + (r.error || ''));
}

export async function rimuoviIgnorato(dominio) {
    await apiPost('/api/settings/ignorati/remove', { dominio });
}

// ========================================================================
// Archivio sessioni (cartella `sessioni/`)
// ========================================================================

export async function ricaricaSessioni() {
    try {
        const r = await apiGet('/api/sessioni');
        state.sessioniArchivio = r.sessioni || [];
        renderAll();
    } catch {}
}

export async function archiviaOra() {
    const r = await apiPost('/api/sessioni/archivia');
    if (r.ok) {
        // Refresh elenco
        const lista = await apiGet('/api/sessioni');
        state.sessioniArchivio = lista.sessioni || [];
        toast.success(`Sessione archiviata: ${r.archiviata}`);
        renderAll();
    } else {
        toast.error('Errore archiviazione: ' + (r.error || 'sessione vuota'));
    }
}

export async function apriSessioneArchiviata(filename) {
    if (!filename) {
        state.datiSessioneVisualizzata = null;
        state.sessioneVisualizzata = null;
        renderAll();
        return;
    }
    const r = await apiPost('/api/sessioni/load', { filename });
    if (r && !r.ok && r.error) {
        toast.error('Errore caricamento: ' + r.error);
        return;
    }
    // Il payload e' direttamente l'ArchiveFile (no wrap {ok})
    state.datiSessioneVisualizzata = r;
    state.sessioneVisualizzata = filename;
    if (state.tabAttivo !== 'report') {
        state.tabAttivo = 'report';
        salvaTab();
        renderTabs();
    }
    renderAll();
}

export async function eliminaSessioneArchiviata(filename) {
    if (!filename) {
        if (!state.sessioneVisualizzata) return;
        filename = state.sessioneVisualizzata;
    }
    if (!confirm(`Eliminare la sessione archiviata "${filename}"?`)) return;
    const r = await apiPost('/api/sessioni/delete', { filename });
    if (r.ok) {
        const lista = await apiGet('/api/sessioni');
        state.sessioniArchivio = lista.sessioni || [];
        if (state.sessioneVisualizzata === filename) {
            state.datiSessioneVisualizzata = null;
            state.sessioneVisualizzata = null;
        }
        renderAll();
    } else {
        toast.error('Errore eliminazione: ' + (r.error || ''));
    }
}

// ========================================================================
// Shutdown server (Phase 1.7+)
// ========================================================================

/**
 * Spegne il server Planck dopo conferma. La risposta HTTP arriva prima
 * che il processo esca; quando il binario muore la finestra app perde
 * la connessione (vedi banner "OFF" della stat row).
 */
export async function spegniServer() {
    if (!confirm('Spegnere il server Planck?\n\nLa registrazione corrente verra\' interrotta. Per riavviare, doppio click su planck.exe.')) return;
    try {
        await fetch('/api/shutdown', { method: 'POST' });
    } catch {
        // La fetch puo' fallire se il server muore prima di risponderci. OK.
    }
    // Mostra un overlay di "spento" per chiarire lo stato all'utente.
    const overlay = document.createElement('div');
    overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.85);color:#e0e0e0;display:flex;align-items:center;justify-content:center;flex-direction:column;font-family:system-ui;z-index:99999';
    overlay.innerHTML = '<h1 style="color:#b77dd4">Planck spento.</h1><p style="opacity:0.7">Puoi chiudere questa finestra.</p>';
    document.body.appendChild(overlay);
}

// ========================================================================
// Veyon (Phase 3e)
// ========================================================================

/**
 * Aggiorna il pannello "Stato" del card Veyon (se aperto) e il flag
 * globale `state.veyonConfigured` che pilota la visibilita' dei
 * bottoni Veyon nelle card studente. Chiamato al boot e ogni volta che
 * si entra nel tab impostazioni o si configura/clear la chiave.
 */
export async function veyonAggiornaStato() {
    let r;
    try {
        r = await apiGet('/api/veyon/status');
    } catch (e) {
        const elStato = document.getElementById('veyon-status');
        if (elStato) elStato.textContent = 'errore: ' + e;
        return;
    }

    state.veyonConfigured = !!r.configured;
    document.body.classList.toggle('veyon-on', state.veyonConfigured);

    const elStato = document.getElementById('veyon-status');
    const elKeyname = document.getElementById('veyon-keyname');
    const elPort = document.getElementById('veyon-port');
    if (elStato) {
        elStato.innerHTML = r.configured
            ? '<span style="color:#4ade80">configurato</span> &mdash; chiave: <code>' + r.keyName + '</code>'
            : '<span style="color:#999">non configurato</span>';
    }
    if (elKeyname) elKeyname.value = r.keyName || '';
    if (elPort) elPort.value = r.port || 11100;
}

/** Salva master key + keyName via /api/veyon/configure. */
export async function veyonConfigura() {
    const keyName = document.getElementById('veyon-keyname').value.trim();
    const pem = document.getElementById('veyon-pem').value.trim();
    if (!keyName || !pem) {
        toast.error('Inserisci nome chiave e contenuto PEM.');
        return;
    }
    const r = await apiPost('/api/veyon/configure', { keyName, privateKeyPEM: pem });
    if (r.ok) {
        document.getElementById('veyon-pem').value = '';
        await veyonAggiornaStato();
        toast.success('Veyon configurato.');
    } else {
        toast.error('Errore configurazione Veyon: ' + (r.error || 'sconosciuto'));
    }
}

/** Rimuove la configurazione Veyon. Chiede conferma. */
export async function veyonRimuovi() {
    if (!confirm('Rimuovere la configurazione Veyon? La chiave verra\' eliminata dal disco.')) return;
    await apiPost('/api/veyon/clear');
    await veyonAggiornaStato();
}

/** Test connessione verso un IP specifico. */
export async function veyonTest() {
    const ip = document.getElementById('veyon-test-ip').value.trim();
    const elResult = document.getElementById('veyon-test-result');
    if (!ip) { toast.error('Inserisci un IP.'); return; }
    elResult.textContent = 'connessione in corso...';
    elResult.style.color = '';
    const r = await apiPost('/api/veyon/test', { ip });
    if (r.ok) {
        elResult.innerHTML = '<span style="color:#4ade80">OK: connessione + autenticazione riuscite verso ' + ip + '</span>';
    } else {
        elResult.innerHTML = '<span style="color:#f87171">FAIL: ' + (r.error || 'errore sconosciuto') + '</span>';
    }
}

/** Invia un comando feature (UUID o nome simbolico) a uno studente. */
export async function veyonSendFeature(ip, feature, command, args) {
    const r = await apiPost('/api/veyon/feature', {
        ip,
        feature,
        command: command || 0,
        arguments: args || {},
    });
    if (!r.ok) {
        toast.error('Veyon ' + feature + ' fallito: ' + (r.error || 'errore'));
    }
    return r.ok;
}

/** ScreenLock su un singolo studente. */
export async function veyonCardLock(ip) {
    if (!state.veyonConfigured) return;
    const ok = await veyonSendFeature(ip, 'screenLock');
    if (ok) {
        state.lockedIps.add(ip);
        renderAll();
    }
}

/** Sblocca lo schermo di un singolo studente. */
export async function veyonCardUnlock(ip) {
    if (!state.veyonConfigured) return;
    const ok = await veyonSendFeature(ip, 'screenUnlock');
    if (ok) {
        state.lockedIps.delete(ip);
        renderAll();
    }
}

/** Disinstalla il proxy dal singolo studente (chiama proxy_off.vbs). */
export async function veyonCardDisinstallaProxy(ip) {
    if (!state.veyonConfigured) return;
    if (!confirm('Rimuovere il proxy da ' + ip + '?')) return;
    const r = await apiPost('/api/veyon/disinstalla-proxy', { ips: [ip] });
    if (r.ok) toast.success('Proxy rimosso da ' + ip);
    else toast.error('Disinstallazione fallita: ' + (r.error || ''));
}

/** Apre una prompt e invia un TextMessage modale. */
export async function veyonCardMsg(ip) {
    if (!state.veyonConfigured) return;
    const text = prompt('Messaggio da mostrare a ' + ip + ':', '');
    if (!text) return;
    // FeatureMessage args usano integer-stringa come chiave (vedi
    // FeatureMessage::argument in core/src/FeatureMessage.h).
    // TextMessage Argument enum: Text=0, Icon=1.
    await veyonSendFeature(ip, 'textMsg', 0, { '0': text, '1': 1 });
}

/** Lista IP visti almeno una volta come `alive` (ping watchdog). */
function ipAttivi() {
    return [...state.aliveMap.keys()];
}

/** Lista IP definiti nella mappa studenti (Impostazioni → Mappa studenti). */
function ipStudenti() {
    return Object.keys(state.cfg.studenti || {});
}

/**
 * Determina su quali IP applicare un'azione di classe.
 *
 * Priorita':
 *   1. Multi-selezione esplicita (Ctrl/Shift+click sulle card)
 *   2. Mappa studenti configurata (la "directory" del docente)
 *   3. Fallback: solo IP attivi (visti come alive)
 *
 * Veyon Master tipicamente usa una directory (LDAP/AD) per sapere quali
 * PC ci sono. Nel nostro setup il docente compila la mappa studenti
 * (IP→nome) — quella e' la nostra directory. Solo se non e' compilata
 * cadiamo sul watchdog (che pero' richiede gia' proxy_on attivo, quindi
 * non funziona per "distribuisci proxy_on").
 */
function targetIps() {
    if (state.selectedIps.size > 0) {
        return { ips: [...state.selectedIps], desc: state.selectedIps.size + ' studenti selezionati' };
    }
    const studenti = ipStudenti();
    if (studenti.length > 0) {
        return { ips: studenti, desc: studenti.length + ' studenti dalla mappa' };
    }
    const a = ipAttivi();
    return { ips: a, desc: a.length + ' studenti attivi' };
}

/**
 * Esegue una callback async per ogni IP target, raccoglie ok/fail.
 * `skipConfirm=true` salta il confirm() nativo (usato per azioni
 * non distruttive: lock/unlock/messaggio/proxy on/off). Le azioni
 * distruttive (reboot, poweroff) lo lasciano a false di default.
 */
async function veyonForEachTarget(label, fn, skipConfirm = false) {
    const { ips, desc } = targetIps();
    if (!ips.length) {
        toast.info('Nessuno studente nel target. Compila la mappa studenti o aspetta che pinghino il watchdog.');
        return;
    }
    if (!skipConfirm && !confirm(label + ' su ' + desc + '?')) return;
    let ok = 0, fail = 0;
    await Promise.all(ips.map(async ip => {
        try { (await fn(ip)) ? ok++ : fail++; }
        catch { fail++; }
    }));
    if (fail === 0) {
        toast.success(`${label}: ${ok}/${ips.length} OK.`);
    } else {
        toast.error(`${label}: ${ok}/${ips.length} OK, ${fail} falliti.`);
    }
}

/** ScreenLock su tutti i target (selezione/mappa studenti/attivi). */
export async function veyonClasseLock() {
    if (!state.veyonConfigured) return;
    await veyonForEachTarget('ScreenLock', async (ip) => {
        const ok = await veyonSendFeature(ip, 'screenLock');
        if (ok) state.lockedIps.add(ip);
        return ok;
    }, /*skipConfirm*/ true);
    renderAll();
}

/** ScreenUnlock su tutti i target. */
export async function veyonClasseUnlock() {
    if (!state.veyonConfigured) return;
    await veyonForEachTarget('ScreenUnlock', async (ip) => {
        const ok = await veyonSendFeature(ip, 'screenUnlock');
        if (ok) state.lockedIps.delete(ip);
        return ok;
    }, /*skipConfirm*/ true);
    renderAll();
}

/** TextMessage su tutti i target — modal in stile app invece di prompt(). */
export async function veyonClasseMsg() {
    if (!state.veyonConfigured) return;
    const { ips, desc } = targetIps();
    if (!ips.length) {
        toast.info('Nessuno studente nel target. Compila la mappa studenti o aspetta che pinghino il watchdog.');
        return;
    }
    const text = await showPromptModal({
        title: 'Invia messaggio a ' + desc,
        placeholder: 'Scrivi il messaggio da mostrare agli studenti...',
        okLabel: 'Invia',
        cancelLabel: 'Annulla',
    });
    if (text === null || !text.trim()) return;
    // TextMessage Argument enum: Text=0, Icon=1.
    await veyonForEachTarget('TextMessage',
        ip => veyonSendFeature(ip, 'textMsg', 0, { '0': text.trim(), '1': 1 }),
        /*skipConfirm*/ true);
}

/** Reboot su tutti i target. */
export async function veyonClasseReboot() {
    if (!state.veyonConfigured) return;
    if (!confirm('Riavviare i PC? L\'azione e\' immediata: gli studenti perdono il lavoro non salvato.')) return;
    await veyonForEachTarget('Reboot', ip => veyonSendFeature(ip, 'reboot'));
}

/** PowerDown su tutti i target. */
export async function veyonClassePowerDown() {
    if (!state.veyonConfigured) return;
    if (!confirm('Spegnere i PC? L\'azione e\' immediata: gli studenti perdono il lavoro non salvato.')) return;
    await veyonForEachTarget('PowerDown', ip => veyonSendFeature(ip, 'powerDown'));
}

/**
 * "Distribuisci proxy_on.bat": su ogni studente target lancia una shell
 * che scarica lo script da Planck stesso e lo esegue. Niente FileTransfer
 * (non esposto via RFB) — usiamo StartApp + powershell + iwr.
 *
 * I target di default sono gli IP della mappa studenti (la "directory"
 * del docente) perche' a questo punto gli studenti probabilmente NON
 * hanno ancora pingato il watchdog di Planck (proxy_on non e' stato
 * ancora distribuito).
 */
/**
 * Distribuisce e attiva proxy_on.vbs sui target via Veyon FileTransfer
 * + OpenFileInApplication=true. Il server legge il vbs dal data dir,
 * lo manda chunked via il protocollo Veyon nativo, e lo studente lo
 * apre col programma associato (wscript.exe per i .vbs → esegue
 * SILENZIOSAMENTE, nessuna console o finestra visibile).
 */
export async function veyonDistribuisciProxy() {
    if (!state.veyonConfigured) return;
    await veyonDistribuisciHelper('/api/veyon/distribuisci-proxy', 'Distribuzione proxy_on');
}

/**
 * Distribuisce proxy_off.vbs sui target: disabilita il proxy
 * registrato in HKCU e killa i watchdog locali.
 */
export async function veyonDisinstallaProxy() {
    if (!state.veyonConfigured) return;
    await veyonDistribuisciHelper('/api/veyon/disinstalla-proxy', 'Disinstallazione proxy');
}

/** Helper interno: chiama un endpoint distribuzione bat con i target attuali. */
async function veyonDistribuisciHelper(endpoint, label) {
    const { ips, desc } = targetIps();
    if (!ips.length) {
        toast.info('Nessuno studente nel target. Compila la mappa studenti.');
        return;
    }

    const r = await apiPost(endpoint, { ips });
    if (r.ok) {
        toast.success(`${label}: ${r.success}/${r.total} OK.`);
    } else {
        // Su parziale, mostra il primo errore nel toast e logga il resto in console
        const failed = (r.results || []).filter(x => !x.ok);
        const firstErr = failed.length ? failed[0].error : '';
        toast.error(`${label}: ${r.success}/${r.total} OK, ${r.failed} falliti. Es: ${firstErr}`);
        if (failed.length > 1) {
            console.warn('Distribuisci errori:', failed);
        }
    }
}

// ========================================================================
// Watchdog plugins (Phase 5)
// ========================================================================

/** Carica la lista plugin + stato dal server in `state.watchdogPlugins`. */
export async function watchdogAggiornaPlugins() {
    try {
        const r = await apiGet('/api/watchdog/plugins');
        state.watchdogPlugins = r.plugins || [];
        renderAll();
    } catch (e) {
        console.warn('watchdog: aggiornaPlugins fallito', e);
    }
}

/** Carica gli ultimi N eventi watchdog (idratazione boot). */
export async function watchdogAggiornaEventi() {
    try {
        const r = await apiGet('/api/watchdog/events?limit=200');
        const events = (r.events || []).map(e => ({
            type: 'watchdog',
            id: e.id,
            plugin: e.plugin,
            ip: e.ip,
            nomeStudente: e.nomeStudente,
            ts: e.ts,
            severity: e.severity,
            payload: typeof e.payload === 'string' ? JSON.parse(e.payload) : e.payload,
            format: '', // verra' sovrascritto dai SSE futuri
        }));
        // Server li manda DESC (recenti prima); per consistenza con la
        // coda live (push in coda), li ribaltiamo cosi' i push successivi
        // restano in fondo.
        state.watchdogEvents = events.reverse();
        state.watchdogEventsPerIp = new Map();
        for (const e of state.watchdogEvents) {
            if (!e.ip) continue;
            let arr = state.watchdogEventsPerIp.get(e.ip);
            if (!arr) { arr = []; state.watchdogEventsPerIp.set(e.ip, arr); }
            arr.push(e);
        }
        renderAll();
    } catch (e) {
        console.warn('watchdog: aggiornaEventi fallito', e);
    }
}

/** Toggle enable/disable di un plugin. */
export async function watchdogTogglePlugin(pluginId) {
    const plugin = state.watchdogPlugins.find(p => p.id === pluginId);
    if (!plugin) return;
    const newEnabled = !plugin.enabled;
    const r = await apiPost('/api/watchdog/config', {
        plugin: pluginId,
        enabled: newEnabled,
        config: plugin.config || {},
    });
    if (r.ok) {
        plugin.enabled = newEnabled;
        renderAll();
    } else {
        toast.error('Toggle ' + pluginId + ' fallito: ' + (r.error || 'errore'));
    }
}

/**
 * Salva la config (JSON) editata nel textarea. Valida che sia JSON
 * parseable, poi POST. Reminder all'utente: serve un Distribuisci
 * proxy per propagare la config agli studenti gia' attivi.
 */
export async function watchdogSaveConfig(pluginId) {
    const plugin = state.watchdogPlugins.find(p => p.id === pluginId);
    if (!plugin) return;
    const ta = document.querySelector(`.watchdog-config-json[data-plugin="${pluginId}"]`);
    if (!ta) return;
    let cfg;
    try {
        cfg = JSON.parse(ta.value);
    } catch (e) {
        toast.error('JSON non valido: ' + e.message);
        return;
    }
    const r = await apiPost('/api/watchdog/config', {
        plugin: pluginId,
        enabled: plugin.enabled,
        config: cfg,
    });
    if (r.ok) {
        plugin.config = cfg;
        toast.success('Config salvata. Per propagarla agli studenti gia\' attivi, ridistribuisci il proxy.');
        renderAll();
    } else {
        toast.error('Salvataggio fallito: ' + (r.error || 'errore'));
    }
}

/**
 * Ripristina i valori default del plugin (riapplicando DefaultConfig
 * lato server). Nel DB resta una riga ma con config = quella che il
 * server considera default — facciamo SaveWatchdogPluginConfig con
 * config={} e il prossimo LoadWatchdogConfig usera' DefaultConfig
 * per quei campi non specificati. Pero' meglio: ricarico tutti i
 * plugin per avere DefaultConfig fresco e setto come current.
 */
export async function watchdogResetConfig(pluginId) {
    if (!confirm('Ripristinare la configurazione default di ' + pluginId + '?')) return;
    // Re-fetch plugins per avere il default JSON canonico.
    // Server: se cancello la riga, LoadWatchdogConfig torna a
    // DefaultConfig. Endpoint /clear non esiste; uso config:{} che
    // pero' verrebbe persistito come {}. Soluzione semplice: usa la
    // DefaultConfig che la UI gia' ha (era nel /plugins response al
    // primo boot, prima di ogni save).
    //
    // In assenza di un campo "default" lato API, faccio un GET su
    // /api/watchdog/plugins forzato senza side effects: se il
    // plugin non e' in watchdog_config, ritorna DefaultConfig.
    // Ma se l'ho gia' salvato, ritorna il salvato.
    // Workaround: chiedo al server di azzerare via config={}+enabled
    // = stato corrente; il server riapplichera' DefaultConfig al
    // prossimo LoadWatchdogConfig perche' marshalled defaults.
    //
    // Per ora: setta il textarea con l'ultimo "current default" che
    // abbiamo, l'utente clicca Salva manualmente.
    toast.info('Click "Salva configurazione" per applicare i default mostrati nel textarea.');
    const ta = document.querySelector(`.watchdog-config-json[data-plugin="${pluginId}"]`);
    if (ta) {
        // Reset al default che il server ci ha mandato all'ultimo /plugins.
        // Recuperiamo da una nuova chiamata se non disponibile.
        try {
            const r = await apiGet('/api/watchdog/plugins');
            const fresh = (r.plugins || []).find(p => p.id === pluginId);
            if (fresh && fresh.config) {
                ta.value = JSON.stringify(fresh.config, null, 2);
            }
        } catch (e) {
            console.warn('reset config fetch:', e);
        }
    }
}

// ========================================================================
// Lista AI (Phase 6)
// ========================================================================

/** Carica lo status della lista AI (count + source + timestamp). */
export async function aiAggiornaStato() {
    try {
        state.aiList = await apiGet('/api/ai/status');
        renderAll();
    } catch (e) {
        console.warn('ai status:', e);
    }
}

/** Forza un fetch dalla URL remota. Toast con esito. */
export async function aiRefresh() {
    const r = await apiPost('/api/ai/refresh');
    if (r && r.ok) {
        state.aiList = { count: r.count, source: r.source, updatedAt: r.updatedAt };
        toast.success(`Lista AI aggiornata: ${r.count} domini.`);
        renderAll();
    } else {
        const msg = (r && r.error) || 'errore sconosciuto';
        toast.error('Aggiornamento fallito: ' + msg + ' (lista corrente intatta)');
        // aggiorna comunque lo stato visualizzato (potrebbe essere
        // tornato il count corrente con source='cache' o 'embedded')
        if (r && r.count !== undefined) {
            state.aiList = { count: r.count, source: r.source || '', updatedAt: r.updatedAt || '' };
            renderAll();
        }
    }
}

export { aggiornaInputDeadline };

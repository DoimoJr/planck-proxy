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

/** Wrapper GET → JSON. */
async function apiGet(path) {
    const r = await fetch(path);
    return r.json();
}

/** Wrapper POST con body JSON. */
async function apiPost(path, body) {
    const r = await fetch(path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: body !== undefined ? JSON.stringify(body) : '',
    });
    return r.json();
}

// ========================================================================
// Blocklist
// ========================================================================

export async function bloccaDominio(d) { await apiPost('/api/block', { dominio: d }); }
export async function sbloccaDominio(d) { await apiPost('/api/unblock', { dominio: d }); }
export async function bloccaAI() { await apiPost('/api/block-all-ai'); }
export async function sbloccaAI() { await apiPost('/api/unblock-all-ai'); }

export async function svuotaBlocklist() {
    if (!confirm('Svuotare completamente la blocklist?')) return;
    await apiPost('/api/clear-blocklist');
}

// ========================================================================
// Sessione
// ========================================================================

export async function toggleSessione() {
    if (state.sessioneAttiva) {
        if (!confirm('Fermare la sessione?\n\nLa registrazione si interrompe e la sessione viene archiviata.\nI dati restano visibili finche\' non avvii una nuova sessione.')) return;
        const r = await apiPost('/api/session/stop');
        if (r.archiviata) await ricaricaSessioni();
        return;
    }
    const hadData = state.entries.length > 0 && state.sessioneInizio;
    const messaggio = hadData
        ? 'Avviare una nuova sessione?\n\nIl buffer corrente verra\' azzerato (la sessione precedente e\' gia\' archiviata).'
        : 'Avviare la sessione?\n\nIl monitor inizia a registrare il traffico.';
    if (!confirm(messaggio)) return;
    const r = await apiPost('/api/session/start');
    state.sessioneInizio = r.sessioneInizio;
    state.sessioneAttiva = true;
    state.sessioneFineISO = null;
    if (r.archiviata) await ricaricaSessioni();
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
        alert(`Preset "${nome}" salvato.`);
    } else {
        alert('Errore salvataggio preset: ' + (r.error || ''));
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
 * Gestisce il click su una card studente con i modificatori della tastiera:
 *   - plain click       : focus singolo (toggle), clear selezione multipla
 *   - Ctrl/Cmd + click  : aggiunge/rimuove dalla selezione
 *   - Shift + click     : range da selectionAnchor (escluso) fino a `ip` (incluso)
 */
export function handleCardClick(ip, ev) {
    if (ev && ev.shiftKey && state.selectionAnchor) {
        const list = ipsInOrder();
        const i1 = list.indexOf(state.selectionAnchor);
        const i2 = list.indexOf(ip);
        if (i1 >= 0 && i2 >= 0) {
            const [lo, hi] = i1 < i2 ? [i1, i2] : [i2, i1];
            for (let k = lo; k <= hi; k++) state.selectedIps.add(list[k]);
            renderAll();
            return;
        }
    }
    if (ev && (ev.ctrlKey || ev.metaKey)) {
        if (state.selectedIps.has(ip)) state.selectedIps.delete(ip);
        else state.selectedIps.add(ip);
        state.selectionAnchor = ip;
        renderAll();
        return;
    }
    // plain click: comportamento legacy (toggle focus) + clear selezione
    state.selectedIps.clear();
    state.selectionAnchor = ip;
    setFocus(ip);
}

/** Svuota la selezione multipla. */
export function clearSelection() {
    state.selectedIps.clear();
    state.selectionAnchor = null;
    renderAll();
}

export function setFiltro(val) { state.filtro = val; renderAll(); }

export function toggleSezione(nome) {
    const lista = document.getElementById('domini-' + nome + '-list');
    if (lista) lista.classList.toggle('hidden');
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
}

export function toggleRichieste() {
    state.richiesteCollassate = !state.richiesteCollassate;
    salvaCollassi();
    applicaCollassi();
}

export function applicaCollassi() {
    const sb = document.getElementById('sidebar-domini');
    const pr = document.getElementById('panel-richieste');
    if (sb) sb.classList.toggle('collassata', state.sidebarCollassata);
    if (pr) pr.classList.toggle('collassata', state.richiesteCollassate);
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

// ========================================================================
// Studenti (mappa IP -> nome) — endpoint /api/students/* in v2
// ========================================================================

export async function modificaStudente(ip, nome) {
    const val = (nome || '').trim();
    if (!val) { await eliminaStudente(ip); return; }
    await apiPost('/api/students/set', { ip, nome: val });
}

export async function eliminaStudente(ip) {
    await apiPost('/api/students/delete', { ip });
}

export async function aggiungiStudente() {
    const ipEl = document.getElementById('nuovo-ip');
    const nomeEl = document.getElementById('nuovo-nome');
    const ip = (ipEl.value || '').trim();
    const nome = (nomeEl.value || '').trim();
    if (!ip || !nome) { alert('Inserisci sia IP che nome.'); return; }
    const r = await apiPost('/api/students/set', { ip, nome });
    if (r.ok) { ipEl.value = ''; nomeEl.value = ''; ipEl.focus(); }
    else alert('Errore: ' + (r.error || ''));
}

export async function svuotaMappaStudenti() {
    if (!confirm('Svuotare completamente la mappa studenti?')) return;
    await apiPost('/api/students/clear');
}

/**
 * Legacy: in v1 c'era un endpoint /api/reload-studenti per ri-leggere
 * il file da disco. In v2 la mappa e' caricata al boot e ogni mutazione
 * UI viene persistita; la rilettura non serve in pratica, ma se qualcuno
 * edita studenti.json a mano serve un riavvio. Per ora questa funzione
 * fa solo un GET /api/config che ricarica gli studenti dal server.
 */
export async function ricaricaStudenti() {
    const cfg = await apiGet('/api/config');
    if (cfg && cfg.studenti) {
        state.cfg.studenti = cfg.studenti;
        renderAll();
    }
}

// ========================================================================
// Classi (coppia classe + laboratorio)
// ========================================================================

function leggiSelCombo() {
    return {
        classe: (document.getElementById('sel-classe')?.value || '').trim(),
        lab: (document.getElementById('sel-lab')?.value || '').trim(),
    };
}

export async function caricaCombo() {
    const { classe, lab } = leggiSelCombo();
    if (!classe || !lab) return;
    if (!confirm(`Caricare la mappa "${classe}" in "${lab}"?\n\nLa mappa attuale verra' sostituita.`)) return;
    const r = await apiPost('/api/classi/load', { classe, lab });
    if (!r.ok) alert('Errore: ' + (r.error || ''));
}

export async function salvaCombo() {
    const { classe: precClasse, lab: precLab } = leggiSelCombo();
    const classe = prompt('Nome classe (lettere, numeri, _ -):\nEsempio: 4dii', precClasse || '');
    if (!classe) return;
    const lab = prompt('Nome laboratorio (lettere, numeri, _ -):\nEsempio: lab1', precLab || '');
    if (!lab) return;
    const r = await apiPost('/api/classi/save', { classe, lab });
    if (r.ok) {
        // Refresh elenco classi
        const lista = await apiGet('/api/classi');
        state.cfg.classi = lista.classi || [];
        alert(`Salvata: ${classe} in ${lab}`);
        const selC = document.getElementById('sel-classe');
        const selL = document.getElementById('sel-lab');
        if (selC) selC.value = classe;
        if (selL) selL.value = lab;
        renderAll();
    } else alert('Errore salvataggio: ' + (r.error || ''));
}

export async function eliminaCombo() {
    const { classe, lab } = leggiSelCombo();
    if (!classe || !lab) return;
    if (!confirm(`Eliminare la combinazione "${classe} / ${lab}"?`)) return;
    const r = await apiPost('/api/classi/delete', { classe, lab });
    if (r.ok) {
        const lista = await apiGet('/api/classi');
        state.cfg.classi = lista.classi || [];
        renderAll();
    }
}

export function aggiornaStatoCombo() {
    renderAll();
}

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

    if (!r) { alert('Errore rete'); return; }
    if (r.rejected && r.rejected.length > 0) {
        alert(`Valore rifiutato per: ${r.rejected.join(', ')}`);
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
    else alert('Errore: ' + (r.error || ''));
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
        alert(`Sessione archiviata: ${r.archiviata}`);
        renderAll();
    } else {
        alert('Errore archiviazione: ' + (r.error || 'sessione vuota'));
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
        alert('Errore caricamento: ' + r.error);
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
        alert('Errore eliminazione: ' + (r.error || ''));
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
        alert('Inserisci nome chiave e contenuto PEM.');
        return;
    }
    const r = await apiPost('/api/veyon/configure', { keyName, privateKeyPEM: pem });
    if (r.ok) {
        document.getElementById('veyon-pem').value = '';
        await veyonAggiornaStato();
    } else {
        alert('Errore: ' + (r.error || 'sconosciuto'));
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
    if (!ip) { alert('Inserisci un IP.'); return; }
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
        alert('Veyon ' + feature + ' fallito: ' + (r.error || 'errore'));
    }
    return r.ok;
}

/** ScreenLock su un singolo studente. */
export async function veyonCardLock(ip) {
    if (!state.veyonConfigured) return;
    await veyonSendFeature(ip, 'screenLock');
}

/** Apre una prompt e invia un TextMessage modale. */
export async function veyonCardMsg(ip) {
    if (!state.veyonConfigured) return;
    const text = prompt('Messaggio da mostrare a ' + ip + ':', '');
    if (!text) return;
    await veyonSendFeature(ip, 'textMsg', 0, { text });
}

/** Lista IP attivi correnti (visti almeno una volta come `alive`). */
function ipAttivi() {
    return [...state.aliveMap.keys()];
}

/**
 * Determina su quali IP applicare un'azione di classe:
 * - se c'e' una multi-selezione, usa quella
 * - altrimenti usa tutti gli IP attivi (watchdog)
 *
 * Ritorna anche un'etichetta descrittiva da mettere nel confirm().
 */
function targetIps() {
    if (state.selectedIps.size > 0) {
        return { ips: [...state.selectedIps], desc: state.selectedIps.size + ' studenti selezionati' };
    }
    const a = ipAttivi();
    return { ips: a, desc: a.length + ' studenti attivi' };
}

/** Esegue una callback async per ogni IP target, raccoglie ok/fail. */
async function veyonForEachTarget(label, fn) {
    const { ips, desc } = targetIps();
    if (!ips.length) {
        alert('Nessuno studente attivo nel monitor (nessun ping watchdog ricevuto).');
        return;
    }
    if (!confirm(label + ' su ' + desc + '?')) return;
    let ok = 0, fail = 0;
    await Promise.all(ips.map(async ip => {
        try { (await fn(ip)) ? ok++ : fail++; }
        catch { fail++; }
    }));
    alert(label + ' completato: ' + ok + ' OK, ' + fail + ' falliti su ' + ips.length + '.');
}

/** ScreenLock su tutti gli studenti attivi. */
export async function veyonClasseLock() {
    if (!state.veyonConfigured) return;
    await veyonForEachTarget('ScreenLock', ip => veyonSendFeature(ip, 'screenLock'));
}

/** TextMessage su tutti gli studenti attivi. */
export async function veyonClasseMsg() {
    if (!state.veyonConfigured) return;
    const text = prompt('Messaggio da mostrare a tutti gli studenti attivi:', '');
    if (!text) return;
    await veyonForEachTarget('TextMessage', ip => veyonSendFeature(ip, 'textMsg', 0, { text }));
}

/**
 * "Distribuisci proxy_on.bat": su ogni studente attivo lancia una shell
 * che scarica lo script da Planck stesso e lo esegue. Niente FileTransfer
 * (non esposto via WebAPI/RFB) — usiamo StartApp + powershell + iwr.
 */
export async function veyonDistribuisciProxy() {
    if (!state.veyonConfigured) return;
    const lanIp = prompt(
        'IP/host di Planck visibile dagli studenti (per scaricare proxy_on.bat):',
        location.hostname || '');
    if (!lanIp) return;
    const port = location.port || '9999';
    const url = `http://${lanIp}:${port}/api/scripts/proxy_on.bat`;
    const ps = `powershell -NoProfile -Command "iwr ${url} -OutFile $env:TEMP\\proxy_on.bat; & $env:TEMP\\proxy_on.bat"`;
    await veyonForEachTarget('Distribuzione proxy_on.bat', ip =>
        veyonSendFeature(ip, 'startApp', 0, { applications: [ps] })
    );
}

export { aggiornaInputDeadline };

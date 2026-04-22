/**
 * @file Azioni utente.
 *
 * Ogni funzione esportata e' un "command" invocato dall'event delegation di
 * `app.js`. La responsabilita' e' chiamare le API backend e aggiornare
 * `state` di conseguenza (eventualmente chiedendo un `renderAll()`).
 *
 * Convenzioni:
 * - Nessuna manipolazione diretta del DOM se non per leggere input utente
 *   (`document.getElementById` sui campi di form, mai innerHTML).
 * - Le mutazioni visibili sullo stato broadcast-ato via SSE spesso arrivano
 *   anche come messaggio SSE: si puo' aggiornare `state` localmente per
 *   reattivita' immediata OPPURE aspettare l'SSE — entrambi gli approcci
 *   sono presenti nel file.
 * - I `confirm`/`prompt`/`alert` sono quelli nativi del browser: volutamente
 *   sincroni e brutti, per distinguere bene le azioni distruttive.
 */

import { state, salvaNascosti, salvaDarkmode, salvaNotifiche, salvaTab, salvaVistaIp, salvaCollassi } from './state.js';
import { renderAll, aggiornaToggleButtons, aggiornaSelectPresets, aggiornaInputDeadline, renderTabs } from './render.js';

/**
 * Wrapper minimale su `fetch` che decodifica JSON. Usato per tutti gli
 * endpoint GET. Non gestisce errori di rete — chi chiama li propaga
 * all'utente con `alert` dove serve.
 * @param {string} path
 * @returns {Promise<Object>}
 */
async function api(path) {
    const r = await fetch(path);
    return r.json();
}

// ========================================================================
// Blocklist
// ========================================================================

/** Aggiunge un dominio alla blocklist. @param {string} d */
export async function bloccaDominio(d) { await api('/api/block?domain=' + encodeURIComponent(d)); }

/** Rimuove un dominio dalla blocklist. @param {string} d */
export async function sbloccaDominio(d) { await api('/api/unblock?domain=' + encodeURIComponent(d)); }

/** Blocca in massa tutti i domini di `DOMINI_AI`. */
export async function bloccaAI() { await api('/api/block-all-ai'); }

/** Sblocca in massa tutti i domini di `DOMINI_AI`. */
export async function sbloccaAI() { await api('/api/unblock-all-ai'); }

/** Svuota completamente la blocklist dopo conferma utente. */
export async function svuotaBlocklist() {
    if (!confirm('Svuotare completamente la blocklist?')) return;
    await api('/api/clear-blocklist');
}

// ========================================================================
// Sessione
// ========================================================================

/**
 * Toggle del lifecycle sessione.
 * - Se attiva: chiede conferma e chiama `/api/session/stop` (il server
 *   archivia subito, i dati restano visibili per revisione).
 * - Se ferma: chiede conferma (testo diverso a seconda che ci siano dati
 *   residui o meno) e chiama `/api/session/start` (azzera + reparte).
 * Il `renderAll` successivo avviene via messaggio SSE `session-state` +
 * `reset` dal server.
 */
export async function toggleSessione() {
    if (state.sessioneAttiva) {
        if (!confirm('Fermare la sessione?\n\nLa registrazione si interrompe e la sessione viene archiviata.\nI dati restano visibili finche\' non avvii una nuova sessione.')) return;
        const r = await api('/api/session/stop');
        if (r.archiviata) await ricaricaSessioni();
        return;
    }
    const hadData = state.entries.length > 0 && state.sessioneInizio;
    const messaggio = hadData
        ? 'Avviare una nuova sessione?\n\nIl buffer corrente verra\' azzerato (la sessione precedente e\' gia\' archiviata).'
        : 'Avviare la sessione?\n\nIl monitor inizia a registrare il traffico.';
    if (!confirm(messaggio)) return;
    const r = await api('/api/session/start');
    state.sessioneInizio = r.sessioneInizio;
    state.sessioneAttiva = true;
    state.sessioneFineISO = null;
    if (r.archiviata) await ricaricaSessioni();
}

/**
 * Scarica la sessione corrente come JSON usando l'endpoint `/api/export`,
 * che include `Content-Disposition: attachment; filename="..."`.
 * Il filename proposto viene estratto dall'header.
 */
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

/**
 * Carica un preset (sovrascrive completamente la blocklist corrente).
 * Chiamato dal `change` sul dropdown preset-load.
 * @param {string} nome
 */
export async function caricaPreset(nome) {
    if (!nome) return;
    if (!confirm(`Caricare il preset "${nome}"? Sostituira\' la blocklist corrente.`)) return;
    await api('/api/preset/load?nome=' + encodeURIComponent(nome));
}

/**
 * Salva la blocklist corrente come nuovo preset (o sovrascrive uno esistente
 * con lo stesso nome — il server non avvisa dell'overwrite).
 */
export async function salvaPreset() {
    const nome = prompt('Nome del preset (lettere, numeri, _, -):');
    if (!nome) return;
    const r = await api('/api/preset/save?nome=' + encodeURIComponent(nome));
    if (r.ok) {
        state.cfg.presets = r.presets;
        aggiornaSelectPresets();
        alert(`Preset "${nome}" salvato.`);
    } else {
        alert('Errore salvataggio preset');
    }
}

// ========================================================================
// Pausa / Deadline
// ========================================================================

/** Toggle globale "Pausa": blocca tutto tranne `dominiIgnorati`. */
export async function togglePausa() { await api('/api/pausa/toggle'); }

/**
 * Programma una scadenza a `HH:MM` (locale); se l'orario e' gia' passato,
 * il server risolve al giorno successivo.
 * @param {string} time - Formato "HH:MM".
 */
export async function impostaDeadline(time) {
    if (!time) return;
    await api('/api/deadline/set?time=' + encodeURIComponent(time));
}

/** Annulla la scadenza attiva. */
export async function annullaDeadline() {
    await api('/api/deadline/clear');
}

// ========================================================================
// UI: nascosti / filtro / focus / sezioni / vista / collassi
// ========================================================================

/** Nasconde un dominio dalla sidebar (solo UI, persistito in localStorage). */
export function nascondiDominio(d) { state.nascosti.add(d); salvaNascosti(); renderAll(); }
/** Ri-mostra un dominio prima nascosto. */
export function mostraDominio(d) { state.nascosti.delete(d); salvaNascosti(); renderAll(); }
/** Svuota completamente il set dei nascosti. */
export function resetNascosti() { state.nascosti.clear(); salvaNascosti(); renderAll(); }

/**
 * Toggle focus su un IP: filtra la Live tab sulle sole richieste di quell'IP.
 * Passare lo stesso IP due volte rimuove il focus.
 * @param {string} ip
 */
export function setFocus(ip) { state.focusIp = state.focusIp === ip ? null : ip; renderAll(); }
/** Rimuove il focus IP corrente. */
export function clearFocus() { state.focusIp = null; renderAll(); }

/** Aggiorna il filtro testuale della Live tab (domini, IP, studenti). */
export function setFiltro(val) { state.filtro = val; renderAll(); }

/**
 * Collassa/espande una sezione della sidebar (Sistema, Nascosti).
 * @param {string} nome - Es. "sistema", "nascosti".
 */
export function toggleSezione(nome) {
    const lista = document.getElementById('domini-' + nome + '-list');
    if (lista) lista.classList.toggle('hidden');
}

/**
 * Cambia la vista del pannello IP tra "griglia" e "lista".
 * @param {'griglia'|'lista'} vista
 */
export function cambiaVistaIp(vista) {
    if (vista !== 'griglia' && vista !== 'lista') return;
    if (state.vistaIp === vista) return;
    state.vistaIp = vista;
    salvaVistaIp();
    renderAll();
}

/** Collassa/espande la sidebar domini (sinistra). */
export function toggleSidebar() {
    state.sidebarCollassata = !state.sidebarCollassata;
    salvaCollassi();
    applicaCollassi();
}

/** Collassa/espande il pannello Ultime richieste (destra). */
export function toggleRichieste() {
    state.richiesteCollassate = !state.richiesteCollassate;
    salvaCollassi();
    applicaCollassi();
}

/**
 * Applica le classi `.collassata` ai due pannelli secondo `state`.
 * Chiamata a init prima del primo render (per evitare flicker "aperto→chiuso")
 * e ogni volta che l'utente toggla.
 */
export function applicaCollassi() {
    const sb = document.getElementById('sidebar-domini');
    const pr = document.getElementById('panel-richieste');
    if (sb) sb.classList.toggle('collassata', state.sidebarCollassata);
    if (pr) pr.classList.toggle('collassata', state.richiesteCollassate);
}

// ========================================================================
// Tema / notifiche
// ========================================================================

/** Toggle tema chiaro/scuro (classe `body.dark` + icona bottone). */
export function toggleDarkmode() {
    state.darkmode = !state.darkmode;
    salvaDarkmode();
    aggiornaToggleButtons();
}

/**
 * Toggle delle notifiche sonore/desktop. Alla prima attivazione richiede
 * il permesso del browser (via `Notification.requestPermission`).
 */
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

/**
 * Cambia tab attivo (persistito). Se l'utente esce dal tab Report mentre
 * stava visualizzando un archivio, resetta la vista archivio.
 * Se entra in Report o Impostazioni, rifresca la lista sessioni archiviate.
 * @param {'live'|'report'|'impostazioni'} nome
 */
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
}

// ========================================================================
// Studenti (mappa IP -> nome)
// ========================================================================

/** Ricarica la mappa studenti dal file `studenti.json`. */
export async function ricaricaStudenti() {
    const r = await api('/api/reload-studenti');
    if (r.ok) {
        state.cfg.studenti = r.studenti;
        renderAll();
    }
}

/**
 * Modifica il nome associato a un IP. Se il nuovo nome e' vuoto, cancella
 * la voce (comportamento dell'input: svuotare = eliminare).
 * @param {string} ip
 * @param {string} nome
 */
export async function modificaStudente(ip, nome) {
    const val = (nome || '').trim();
    if (!val) { await eliminaStudente(ip); return; }
    await api('/api/studenti/set?ip=' + encodeURIComponent(ip) + '&nome=' + encodeURIComponent(val));
}

/** Elimina una voce dalla mappa. @param {string} ip */
export async function eliminaStudente(ip) {
    await api('/api/studenti/delete?ip=' + encodeURIComponent(ip));
}

/**
 * Aggiunge una nuova voce leggendo i due input del form. In caso di successo
 * svuota gli input e riporta il focus sul primo.
 */
export async function aggiungiStudente() {
    const ipEl = document.getElementById('nuovo-ip');
    const nomeEl = document.getElementById('nuovo-nome');
    const ip = (ipEl.value || '').trim();
    const nome = (nomeEl.value || '').trim();
    if (!ip || !nome) { alert('Inserisci sia IP che nome.'); return; }
    const r = await api('/api/studenti/set?ip=' + encodeURIComponent(ip) + '&nome=' + encodeURIComponent(nome));
    if (r.ok) { ipEl.value = ''; nomeEl.value = ''; ipEl.focus(); }
    else alert('Errore: ' + (r.error || ''));
}

/** Svuota tutta la mappa (chiede conferma). */
export async function svuotaMappaStudenti() {
    if (!confirm('Svuotare completamente la mappa studenti?')) return;
    await api('/api/studenti/clear');
}

// ========================================================================
// Classi (coppia classe + laboratorio)
// ========================================================================

/**
 * Legge i due select della toolbar classi in un oggetto unificato.
 * @returns {{classe: string, lab: string}}
 */
function leggiSelCombo() {
    return {
        classe: (document.getElementById('sel-classe')?.value || '').trim(),
        lab: (document.getElementById('sel-lab')?.value || '').trim(),
    };
}

/**
 * Carica la mappa della combinazione (classe, lab) selezionata — sovrascrive
 * `studenti.json` con il contenuto dello snapshot.
 */
export async function caricaCombo() {
    const { classe, lab } = leggiSelCombo();
    if (!classe || !lab) return;
    if (!confirm(`Caricare la mappa "${classe}" in "${lab}"?\n\nLa mappa attuale verra' sostituita.`)) return;
    const r = await api(`/api/classi/load?classe=${encodeURIComponent(classe)}&lab=${encodeURIComponent(lab)}`);
    if (!r.ok) alert('Errore: ' + (r.error || ''));
}

/**
 * Salva la mappa corrente come nuova combinazione. Prompta classe+lab,
 * propone i valori correnti dei select come default.
 */
export async function salvaCombo() {
    const { classe: precClasse, lab: precLab } = leggiSelCombo();
    const classe = prompt('Nome classe (lettere, numeri, _ -):\nEsempio: 4dii', precClasse || '');
    if (!classe) return;
    const lab = prompt('Nome laboratorio (lettere, numeri, _ -):\nEsempio: lab1', precLab || '');
    if (!lab) return;
    const r = await api(`/api/classi/save?classe=${encodeURIComponent(classe)}&lab=${encodeURIComponent(lab)}`);
    if (r.ok) {
        state.cfg.classi = r.classi;
        alert(`Salvata: ${classe} in ${lab}`);
        const selC = document.getElementById('sel-classe');
        const selL = document.getElementById('sel-lab');
        if (selC) selC.value = classe;
        if (selL) selL.value = lab;
        renderAll();
    } else alert('Errore salvataggio: ' + (r.error || ''));
}

/** Elimina la combinazione (classe, lab) selezionata dai due dropdown. */
export async function eliminaCombo() {
    const { classe, lab } = leggiSelCombo();
    if (!classe || !lab) return;
    if (!confirm(`Eliminare la combinazione "${classe} / ${lab}"?`)) return;
    const r = await api(`/api/classi/delete?classe=${encodeURIComponent(classe)}&lab=${encodeURIComponent(lab)}`);
    if (r.ok) {
        state.cfg.classi = r.classi;
        renderAll();
    }
}

/** Chiamato quando l'utente cambia uno dei due select combo (ri-render per aggiornare lo stato abilitato dei pulsanti Load/Delete). */
export function aggiornaStatoCombo() {
    renderAll();
}

// ========================================================================
// Settings (config modificabile da UI)
// ========================================================================

/**
 * POST di un singolo campo di config a `/api/settings/update`. Chiamato
 * sull'evento `change` di un input con `data-action="settings-field"` e
 * `data-key="<dotted.path>"` (es. `web.auth.password`).
 *
 * Casi speciali:
 * - checkbox: usa `el.checked`.
 * - number: parse + validazione `isFinite`.
 * - password: se vuoto non invia (evita di sovrascrivere con "").
 *
 * Dopo il save, aggiorna `state.settings` con la risposta server (che
 * contiene la password mascherata come `{password:"", passwordSet:true}`).
 * Se la chiave e' in `SETTINGS_RESTART`, il banner "riavvio richiesto" si
 * accende.
 *
 * @param {HTMLInputElement|HTMLSelectElement} el - L'elemento che ha
 *   scatenato il change.
 */
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

/**
 * Aggiunge un dominio a `dominiIgnorati`. Dominio letto dall'input
 * `#nuovo-ignorato`. Il server aggiorna `config.json` a caldo.
 */
export async function aggiungiIgnorato() {
    const el = document.getElementById('nuovo-ignorato');
    const dominio = (el.value || '').trim();
    if (!dominio) return;
    const r = await fetch('/api/settings/ignorati/add?dominio=' + encodeURIComponent(dominio)).then(r => r.json());
    if (r.ok) { el.value = ''; el.focus(); }
    else alert('Errore: ' + (r.error || ''));
}

/** Rimuove un dominio da `dominiIgnorati`. */
export async function rimuoviIgnorato(dominio) {
    await fetch('/api/settings/ignorati/remove?dominio=' + encodeURIComponent(dominio));
}

// ========================================================================
// Archivio sessioni (cartella `sessioni/`)
// ========================================================================

/** Ricarica la lista dei file in `sessioni/`. Silenzia errori di rete. */
export async function ricaricaSessioni() {
    try {
        const r = await api('/api/sessioni');
        state.sessioniArchivio = r.sessioni || [];
        renderAll();
    } catch {}
}

/**
 * Forza l'archivio della sessione corrente senza interromperla (resta
 * attiva). Utile come "checkpoint" se la sessione e' lunga.
 */
export async function archiviaOra() {
    const r = await api('/api/sessioni/archivia');
    if (r.ok) {
        state.sessioniArchivio = r.sessioni || [];
        alert(`Sessione archiviata: ${r.nome}`);
        renderAll();
    } else {
        alert('Nessun dato da archiviare (sessione vuota).');
    }
}

/**
 * Apre una sessione archiviata nel tab Report.
 * Se `nome` e' falsy, torna a visualizzare la sessione corrente.
 * @param {string|null} nome - Filename della sessione archiviata.
 */
export async function apriSessioneArchiviata(nome) {
    if (!nome) {
        state.datiSessioneVisualizzata = null;
        state.sessioneVisualizzata = null;
        renderAll();
        return;
    }
    const r = await api('/api/sessioni/load?nome=' + encodeURIComponent(nome));
    if (r.ok) {
        state.datiSessioneVisualizzata = r.sessione;
        state.sessioneVisualizzata = nome;
        if (state.tabAttivo !== 'report') {
            state.tabAttivo = 'report';
            salvaTab();
            renderTabs();
        }
        renderAll();
    } else {
        alert('Errore caricamento: ' + (r.error || ''));
    }
}

/**
 * Elimina una sessione archiviata (chiede conferma). Se `nome` e' null,
 * elimina quella correntemente visualizzata nel Report.
 * @param {string|null} nome
 */
export async function eliminaSessioneArchiviata(nome) {
    if (!nome) {
        if (!state.sessioneVisualizzata) return;
        nome = state.sessioneVisualizzata;
    }
    if (!confirm(`Eliminare la sessione archiviata "${nome}"?`)) return;
    const r = await api('/api/sessioni/delete?nome=' + encodeURIComponent(nome));
    if (r.ok) {
        state.sessioniArchivio = r.sessioni || [];
        if (state.sessioneVisualizzata === nome) {
            state.datiSessioneVisualizzata = null;
            state.sessioneVisualizzata = null;
        }
        renderAll();
    } else {
        alert('Errore eliminazione');
    }
}

// Re-export per comodita' dei chiamanti (alcuni moduli importano solo da qui).
export { aggiornaInputDeadline };

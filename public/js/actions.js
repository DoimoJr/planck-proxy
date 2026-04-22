// Azioni utente: API + mutazioni state.

import { state, salvaNascosti, salvaDarkmode, salvaNotifiche, salvaTab, salvaVistaIp, salvaCollassi } from './state.js';
import { renderAll, aggiornaToggleButtons, aggiornaSelectPresets, aggiornaInputDeadline, renderTabs } from './render.js';

async function api(path) {
    const r = await fetch(path);
    return r.json();
}

// --- Blocklist ---
export async function bloccaDominio(d) { await api('/api/block?domain=' + encodeURIComponent(d)); }
export async function sbloccaDominio(d) { await api('/api/unblock?domain=' + encodeURIComponent(d)); }
export async function bloccaAI() { await api('/api/block-all-ai'); }
export async function sbloccaAI() { await api('/api/unblock-all-ai'); }
export async function svuotaBlocklist() {
    if (!confirm('Svuotare completamente la blocklist?')) return;
    await api('/api/clear-blocklist');
}

// --- Sessione ---
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

// --- Preset ---
export async function caricaPreset(nome) {
    if (!nome) return;
    if (!confirm(`Caricare il preset "${nome}"? Sostituira\' la blocklist corrente.`)) return;
    await api('/api/preset/load?nome=' + encodeURIComponent(nome));
}
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

// --- Pausa ---
export async function togglePausa() { await api('/api/pausa/toggle'); }

// --- Deadline ---
export async function impostaDeadline(time) {
    if (!time) return;
    await api('/api/deadline/set?time=' + encodeURIComponent(time));
}
export async function annullaDeadline() {
    await api('/api/deadline/clear');
}

// --- UI: nascosti / filtro / focus ---
export function nascondiDominio(d) { state.nascosti.add(d); salvaNascosti(); renderAll(); }
export function mostraDominio(d) { state.nascosti.delete(d); salvaNascosti(); renderAll(); }
export function resetNascosti() { state.nascosti.clear(); salvaNascosti(); renderAll(); }

export function setFocus(ip) { state.focusIp = state.focusIp === ip ? null : ip; renderAll(); }
export function clearFocus() { state.focusIp = null; renderAll(); }

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

// --- Tema / notifiche ---
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

// --- Tab ---
export function cambiaTab(nome) {
    state.tabAttivo = nome;
    salvaTab();
    // Se esce dal tab report, se stava visualizzando un archivio, torna alla corrente
    if (nome !== 'report' && state.datiSessioneVisualizzata) {
        state.datiSessioneVisualizzata = null;
        state.sessioneVisualizzata = null;
    }
    renderTabs();
    renderAll();
    // Se entra in impostazioni o report, aggiorna lista sessioni
    if (nome === 'impostazioni' || nome === 'report') ricaricaSessioni();
}

// --- Studenti ---
export async function ricaricaStudenti() {
    const r = await api('/api/reload-studenti');
    if (r.ok) {
        state.cfg.studenti = r.studenti;
        renderAll();
    }
}
export async function modificaStudente(ip, nome) {
    const val = (nome || '').trim();
    if (!val) { await eliminaStudente(ip); return; }
    await api('/api/studenti/set?ip=' + encodeURIComponent(ip) + '&nome=' + encodeURIComponent(val));
}
export async function eliminaStudente(ip) {
    await api('/api/studenti/delete?ip=' + encodeURIComponent(ip));
}
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
export async function svuotaMappaStudenti() {
    if (!confirm('Svuotare completamente la mappa studenti?')) return;
    await api('/api/studenti/clear');
}

// --- Classi (coppia classe+lab) ---
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
    const r = await api(`/api/classi/load?classe=${encodeURIComponent(classe)}&lab=${encodeURIComponent(lab)}`);
    if (!r.ok) alert('Errore: ' + (r.error || ''));
}

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
        // Seleziona la combinazione appena salvata
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
    const r = await api(`/api/classi/delete?classe=${encodeURIComponent(classe)}&lab=${encodeURIComponent(lab)}`);
    if (r.ok) {
        state.cfg.classi = r.classi;
        renderAll();
    }
}

// Chiamato quando l'utente cambia uno dei due select (aggiorna stato bottoni)
export function aggiornaStatoCombo() {
    renderAll();
}

// --- Settings ---
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
        if (!el.value) return; // non inviare password vuota
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
    if (el.type === 'password') el.value = ''; // svuota dopo il salvataggio
    renderAll();
}

export async function aggiungiIgnorato() {
    const el = document.getElementById('nuovo-ignorato');
    const dominio = (el.value || '').trim();
    if (!dominio) return;
    const r = await fetch('/api/settings/ignorati/add?dominio=' + encodeURIComponent(dominio)).then(r => r.json());
    if (r.ok) { el.value = ''; el.focus(); }
    else alert('Errore: ' + (r.error || ''));
}
export async function rimuoviIgnorato(dominio) {
    await fetch('/api/settings/ignorati/remove?dominio=' + encodeURIComponent(dominio));
}

// --- Archivio sessioni ---
export async function ricaricaSessioni() {
    try {
        const r = await api('/api/sessioni');
        state.sessioniArchivio = r.sessioni || [];
        renderAll();
    } catch {}
}

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
        // Porta l'utente sul tab report per la visione
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

export async function eliminaSessioneArchiviata(nome) {
    if (!nome) {
        // Elimina quella corrente visualizzata nel report
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

export { aggiornaInputDeadline };

/**
 * @file Entry point dell'applicazione frontend.
 *
 * Due responsabilita':
 *
 * 1. **Boot (`init`)**: carica in parallelo `/api/config`, `/api/history`,
 *    `/api/sessioni`, `/api/settings`; idrata `state`; applica toggle (tema,
 *    collassi, tab); avvia la connessione SSE; imposta i timer di refresh.
 *
 * 2. **Event delegation** via tre listener sul body (`click`, `input`,
 *    `change`) + uno per `keydown` nei campi di input: ogni elemento
 *    interattivo porta un `data-action="..."` che viene dispathato alla
 *    funzione corrispondente di `actions.js`. Zero `onclick` inline.
 *
 * Convenzione `data-*`:
 * - `data-action="..."` identifica l'azione.
 * - `data-dominio`, `data-ip`, `data-nome`, `data-tab`, `data-sezione` (e
 *   simili) passano l'argomento corrispondente.
 * - Per elementi "annidati cliccabili" (es. bottone blocca dentro card)
 *   l'handler fa `e.stopPropagation()` nel suo case per evitare che il
 *   click dell'outer (es. focus IP) si scateni anche.
 */

import { state, assorbiEntry } from './state.js';
import { renderAll, aggiornaToggleButtons, aggiornaSelectPresets, aggiornaInputDeadline, renderTabs, renderCountdown } from './render.js';
import { avviaSSE } from './sse.js';
import * as actions from './actions.js';

/**
 * Carica tutti gli endpoint di boot in parallelo, idrata `state`, fa il primo
 * render e apre la connessione SSE. Imposta anche due timer globali:
 * - `renderCountdown` ogni 1s (economico, aggiorna solo la cella countdown).
 * - `renderAll` ogni 5s (refresh di "ultima attivita'" e durata — campi
 *   relativi che dipendono da `Date.now()`).
 */
async function init() {
    const [cfgRes, histRes, sesRes, setRes] = await Promise.all([
        fetch('/api/config').then(r => r.json()),
        fetch('/api/history').then(r => r.json()),
        fetch('/api/sessioni').then(r => r.json()).catch(() => ({ sessioni: [] })),
        fetch('/api/settings').then(r => r.json()).catch(() => ({ settings: null })),
    ]);

    state.cfg = cfgRes;
    document.title = cfgRes.titolo + (cfgRes.classe ? ' - ' + cfgRes.classe : '');

    state.bloccati = new Set(histRes.bloccati);
    state.sessioneAttiva = !!histRes.sessioneAttiva;
    state.sessioneInizio = histRes.sessioneInizio || null;
    state.sessioneFineISO = histRes.sessioneFineISO || null;
    state.pausato = !!histRes.pausato;
    state.deadlineISO = histRes.deadlineISO || null;
    state.sessioniArchivio = sesRes.sessioni || [];
    state.settings = setRes.settings || null;

    if (histRes.alive) {
        for (const [ip, ts] of Object.entries(histRes.alive)) state.aliveMap.set(ip, ts);
    }

    for (const e of histRes.entries) assorbiEntry(e);

    aggiornaToggleButtons();
    aggiornaSelectPresets();
    aggiornaInputDeadline();
    renderTabs();
    actions.applicaCollassi();
    renderAll();
    avviaSSE();

    // Veyon: status (sapere se mostrare i bottoni inline + classe).
    // Async non bloccante: il primo render va comunque, le card si
    // accendono di Veyon-buttons quando il fetch ritorna.
    actions.veyonAggiornaStato();

    // Watchdog plugins: carica config + ultimi eventi (Phase 5).
    actions.watchdogAggiornaPlugins();
    actions.watchdogAggiornaEventi();

    // Countdown: tick ogni secondo (solo aggiorna la UI, zero allocazioni)
    setInterval(renderCountdown, 1000);
    // Refresh complessivo (durata, "ultima attivita'") - ogni 5s
    setInterval(renderAll, 5000);
}

/**
 * Chiude qualsiasi `<details class="menu-overflow">` aperto.
 * Invocato nei tre contesti: click fuori dal menu (capture phase),
 * click su un item del menu (bubble, dopo l'azione), change sul select
 * preset (che vive dentro il menu).
 */
function chiudiMenuOverflow() {
    document.querySelectorAll('details.menu-overflow[open]').forEach(d => { d.open = false; });
}
document.body.addEventListener('click', (e) => {
    // Se l'utente clicca fuori dal menu (e fuori dal summary), chiudilo.
    if (!e.target.closest('details.menu-overflow')) chiudiMenuOverflow();
}, true);

// --- Click delegation ---
document.body.addEventListener('click', (e) => {
    const el = e.target.closest('[data-action]');
    if (!el) return;
    const action = el.dataset.action;
    const d = el.dataset.dominio;
    const ip = el.dataset.ip;
    const nome = el.dataset.nome;

    // Se l'azione e' stata attivata dentro al menu overflow, chiudi il menu dopo l'azione.
    // (Eccezione: il summary stesso, che serve per aprire il menu — non e' un data-action.)
    const dentroMenu = el.closest('.menu-overflow-panel');

    switch (action) {
        case 'blocca': e.stopPropagation(); actions.bloccaDominio(d); break;
        case 'sblocca': actions.sbloccaDominio(d); break;
        case 'nascondi-dominio': actions.nascondiDominio(d); break;
        case 'mostra-dominio': actions.mostraDominio(d); break;
        case 'reset-nascosti': actions.resetNascosti(); break;
        case 'focus-ip': actions.handleCardClick(ip, e); break;
        case 'clear-selection': actions.clearSelection(); break;
        case 'focus-clear': e.stopPropagation(); actions.clearFocus(); break;
        case 'toggle-sezione': actions.toggleSezione(el.dataset.sezione); break;
        case 'vista-griglia': actions.cambiaVistaIp('griglia'); break;
        case 'vista-lista': actions.cambiaVistaIp('lista'); break;
        case 'toggle-sidebar': actions.toggleSidebar(); break;
        case 'toggle-richieste': actions.toggleRichieste(); break;
        case 'session-toggle': actions.toggleSessione(); break;
        case 'export': actions.esportaSessione(); break;
        case 'block-all-ai': actions.bloccaAI(); break;
        case 'unblock-all-ai': actions.sbloccaAI(); break;
        case 'clear-blocklist': actions.svuotaBlocklist(); break;
        case 'preset-save': actions.salvaPreset(); break;
        case 'darkmode': actions.toggleDarkmode(); break;
        case 'notifiche': actions.toggleNotifiche(); break;
        case 'spegni-server': actions.spegniServer(); break;
        case 'pausa-toggle': actions.togglePausa(); break;
        case 'clear-deadline': actions.annullaDeadline(); break;
        case 'tab': actions.cambiaTab(el.dataset.tab); break;
        case 'reload-studenti': actions.ricaricaStudenti(); break;
        case 'elimina-studente': actions.eliminaStudente(el.dataset.ip); break;
        case 'aggiungi-studente': actions.aggiungiStudente(); break;
        case 'svuota-studenti': actions.svuotaMappaStudenti(); break;
        case 'combo-load': actions.caricaCombo(); break;
        case 'combo-save': actions.salvaCombo(); break;
        case 'combo-delete': actions.eliminaCombo(); break;
        case 'aggiungi-ignorato': actions.aggiungiIgnorato(); break;
        case 'rimuovi-ignorato': actions.rimuoviIgnorato(el.dataset.dominio); break;
        case 'veyon-configure': actions.veyonConfigura(); break;
        case 'veyon-clear': actions.veyonRimuovi(); break;
        case 'veyon-test': actions.veyonTest(); break;
        case 'veyon-card-lock': e.stopPropagation(); actions.veyonCardLock(el.dataset.ip); break;
        case 'veyon-card-unlock': e.stopPropagation(); actions.veyonCardUnlock(el.dataset.ip); break;
        case 'veyon-card-msg': e.stopPropagation(); actions.veyonCardMsg(el.dataset.ip); break;
        case 'veyon-classe-lock': actions.veyonClasseLock(); break;
        case 'veyon-classe-unlock': actions.veyonClasseUnlock(); break;
        case 'veyon-classe-msg': actions.veyonClasseMsg(); break;
        case 'veyon-classe-reboot': actions.veyonClasseReboot(); break;
        case 'veyon-classe-poweroff': actions.veyonClassePowerDown(); break;
        case 'veyon-distribuisci-proxy': actions.veyonDistribuisciProxy(); break;
        case 'veyon-disinstalla-proxy': actions.veyonDisinstallaProxy(); break;
        case 'watchdog-toggle': actions.watchdogTogglePlugin(el.dataset.plugin); break;
        case 'archivia-ora': actions.archiviaOra(); break;
        case 'ricarica-sessioni': actions.ricaricaSessioni(); break;
        case 'sessione-apri': e.stopPropagation(); actions.apriSessioneArchiviata(nome); break;
        case 'sessione-elimina': e.stopPropagation(); actions.eliminaSessioneArchiviata(nome); break;
        case 'report-elimina': actions.eliminaSessioneArchiviata(null); break;
    }

    if (dentroMenu) chiudiMenuOverflow();
});

// --- Input / change ---
document.body.addEventListener('input', (e) => {
    const el = e.target;
    if (el.dataset.action === 'filtro') actions.setFiltro(el.value);
});
document.body.addEventListener('change', (e) => {
    const el = e.target;
    if (el.dataset.action === 'preset-load') {
        actions.caricaPreset(el.value);
        el.value = '';
        chiudiMenuOverflow();
    } else if (el.dataset.action === 'set-deadline') {
        actions.impostaDeadline(el.value);
    } else if (el.dataset.action === 'report-sessione-select') {
        actions.apriSessioneArchiviata(el.value);
    } else if (el.dataset.action === 'sel-combo') {
        actions.aggiornaStatoCombo();
    } else if (el.dataset.action === 'edit-studente') {
        actions.modificaStudente(el.dataset.ip, el.value);
    } else if (el.dataset.action === 'settings-field') {
        actions.settingsCampoModificato(el);
    }
});

// Invio nei campi di aggiunta studente = aggiungi
document.body.addEventListener('keydown', (e) => {
    const el = e.target;
    if (e.key === 'Enter' && el.dataset.action === 'nuovo-studente-key') {
        e.preventDefault();
        actions.aggiungiStudente();
    } else if (e.key === 'Enter' && el.dataset.action === 'nuovo-ignorato-key') {
        e.preventDefault();
        actions.aggiungiIgnorato();
    }
});

init();

/**
 * @file Connessione Server-Sent Events al backend.
 *
 * Gestisce:
 * - Connessione a `/api/stream` con auto-reconnect (2s) su errore.
 * - Dispatch dei vari tipi di messaggio (`traffic`, `blocklist`, `reset`,
 *   `studenti`, `classi`, `settings`, `session-state`, `pausa`, `deadline`,
 *   `deadline-reached`, `alive`) sulle mutazioni appropriate di `state`.
 * - Feedback audio/visivo su rilevamento AI e scadenza deadline.
 *
 * Il server emette un heartbeat `: hb` ogni 20s per tenere la connessione
 * viva dietro eventuali proxy/reverse-proxy; EventSource lo ignora.
 */

import { state, assorbiEntry, resetDatiTraffico } from './state.js';
import { renderAll, aggiornaInputDeadline } from './render.js';
import { $ } from './util.js';

let audioCtx = null;

/**
 * Emette un beep sinusoidale a 880 Hz per 150ms. No-op se le notifiche sono
 * disattivate o se il browser blocca l'AudioContext (autoplay policy).
 * Lazy-inizializza l'AudioContext al primo uso.
 */
function beep() {
    if (!state.notifiche) return;
    try {
        if (!audioCtx) audioCtx = new (window.AudioContext || window.webkitAudioContext)();
        const osc = audioCtx.createOscillator();
        const gain = audioCtx.createGain();
        osc.type = 'sine';
        osc.frequency.value = 880;
        gain.gain.value = 0.08;
        osc.connect(gain); gain.connect(audioCtx.destination);
        osc.start();
        osc.stop(audioCtx.currentTime + 0.15);
    } catch {}
}

/**
 * Mostra il banner rosso lampeggiante per 5s con il dominio AI rilevato.
 * Sparisce dopo 5s (timer reimpostato se arrivano nuove detection).
 * Emette anche notifica desktop + beep se le notifiche sono attive.
 * @param {string} dominio
 */
function lampeggiaBannerAI(dominio) {
    const b = $('ai-banner');
    b.textContent = `ATTENZIONE: accesso AI rilevato - ${dominio}`;
    b.style.display = 'block';
    clearTimeout(lampeggiaBannerAI._t);
    lampeggiaBannerAI._t = setTimeout(() => { b.style.display = 'none'; }, 5000);

    if (state.notifiche && typeof Notification !== 'undefined' && Notification.permission === 'granted') {
        try { new Notification('AI rilevata', { body: dominio, silent: true }); } catch {}
    }
    beep();
}

/**
 * Mostra il banner "TEMPO SCADUTO" (resta visibile fino al prossimo reset),
 * emette notifica desktop non silenziosa + tripla serie di beep.
 */
function lampeggiaBannerDeadline() {
    const b = $('ai-banner');
    b.textContent = 'TEMPO SCADUTO - fine verifica';
    b.style.display = 'block';
    if (state.notifiche && typeof Notification !== 'undefined' && Notification.permission === 'granted') {
        try { new Notification('Tempo scaduto', { body: 'La verifica e\' terminata', silent: false }); } catch {}
    }
    beep();
    setTimeout(beep, 300);
    setTimeout(beep, 600);
}

/**
 * Aggiorna il badge LIVE/OFF in base allo stato della connessione SSE.
 * @param {boolean} connesso
 */
function setStato(connesso) {
    state.connesso = connesso;
    const card = $('stat-status').parentElement;
    card.classList.remove('connected', 'disconnected');
    card.classList.add(connesso ? 'connected' : 'disconnected');
    $('stat-status').textContent = connesso ? 'LIVE' : 'OFF';
}

/**
 * Apre la connessione SSE a `/api/stream` e si auto-riconnette a 2s su errore.
 * Esportata: chiamata una volta da `app.js::init()` al boot.
 *
 * I tipi di messaggio gestiti sono:
 * | type               | Effetto                                                        |
 * |--------------------|----------------------------------------------------------------|
 * | `traffic`          | Incorpora l'entry, banner AI se tipo=ai non-bloccato, render.  |
 * | `blocklist`        | Rimpiazza `state.bloccati`, render.                            |
 * | `reset`            | Azzera buffer client, aggiorna `sessioneInizio`, render.       |
 * | `studenti`         | Rimpiazza `state.cfg.studenti`, render.                        |
 * | `classi`           | Rimpiazza `state.cfg.classi`, render.                          |
 * | `settings`         | Rimpiazza `state.settings`, propaga i campi derivati.          |
 * | `session-state`    | Aggiorna `sessioneAttiva`/`sessioneInizio`/`sessioneFineISO`.  |
 * | `pausa`            | Aggiorna `state.pausato`, render.                              |
 * | `deadline`         | Aggiorna `deadlineISO`, aggiorna input countdown, render.      |
 * | `deadline-reached` | Mostra banner "TEMPO SCADUTO" + triplo beep + notifica.        |
 * | `alive`            | Aggiorna `aliveMap[ip] = ts` (dot watchdog).                   |
 */
export function avviaSSE() {
    const es = new EventSource('/api/stream');
    es.onopen = () => setStato(true);
    es.onerror = () => {
        setStato(false);
        es.close();
        setTimeout(avviaSSE, 2000);
    };
    es.onmessage = (ev) => {
        const msg = JSON.parse(ev.data);
        if (msg.type === 'traffic') {
            assorbiEntry(msg.entry);
            if (msg.entry.tipo === 'ai' && !state.bloccati.has(msg.entry.dominio)) {
                lampeggiaBannerAI(msg.entry.dominio);
            }
            renderAll();
        } else if (msg.type === 'blocklist') {
            state.bloccati = new Set(msg.list);
            renderAll();
        } else if (msg.type === 'reset') {
            resetDatiTraffico();
            if (msg.sessioneInizio) state.sessioneInizio = msg.sessioneInizio;
            renderAll();
        } else if (msg.type === 'studenti') {
            state.cfg.studenti = msg.studenti;
            renderAll();
        } else if (msg.type === 'classi') {
            state.cfg.classi = msg.classi;
            renderAll();
        } else if (msg.type === 'settings') {
            state.settings = msg.settings;
            if (msg.settings.titolo !== undefined) state.cfg.titolo = msg.settings.titolo;
            if (msg.settings.classe !== undefined) state.cfg.classe = msg.settings.classe;
            if (msg.settings.modo !== undefined) state.cfg.modo = msg.settings.modo;
            if (msg.settings.inattivitaSogliaSec !== undefined) state.cfg.inattivitaSogliaSec = msg.settings.inattivitaSogliaSec;
            document.title = state.cfg.titolo + (state.cfg.classe ? ' - ' + state.cfg.classe : '');
            renderAll();
        } else if (msg.type === 'session-state') {
            state.sessioneAttiva = !!msg.sessioneAttiva;
            state.sessioneInizio = msg.sessioneInizio;
            state.sessioneFineISO = msg.sessioneFineISO;
            renderAll();
        } else if (msg.type === 'pausa') {
            state.pausato = !!msg.pausato;
            renderAll();
        } else if (msg.type === 'deadline') {
            state.deadlineISO = msg.deadlineISO;
            aggiornaInputDeadline();
            renderAll();
        } else if (msg.type === 'deadline-reached') {
            lampeggiaBannerDeadline();
            renderAll();
        } else if (msg.type === 'alive') {
            state.aliveMap.set(msg.ip, msg.ts);
            renderAll();
        }
    };
}

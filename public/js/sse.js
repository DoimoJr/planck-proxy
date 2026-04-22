// Connessione SSE con reconnect + heartbeat

import { state, assorbiEntry, resetDatiTraffico } from './state.js';
import { renderAll, aggiornaInputDeadline } from './render.js';
import { $ } from './util.js';

let audioCtx = null;
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

function setStato(connesso) {
    state.connesso = connesso;
    const card = $('stat-status').parentElement;
    card.classList.remove('connected', 'disconnected');
    card.classList.add(connesso ? 'connected' : 'disconnected');
    $('stat-status').textContent = connesso ? 'LIVE' : 'OFF';
}

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
            // Propaga ai campi derivati usati altrove nella UI
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

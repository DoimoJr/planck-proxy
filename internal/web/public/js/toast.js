/**
 * @file Toast system: notifiche non-modali in basso a destra,
 * auto-dismiss. Sostituiscono i `confirm()`/`alert()` per i casi
 * non bloccanti.
 *
 * API:
 *   toast.success(msg, [opts])   verde, dismiss 3s
 *   toast.error(msg, [opts])     rosso, dismiss 5s
 *   toast.info(msg, [opts])      grigio, dismiss 3s
 *
 * `opts.ms` override dell'auto-dismiss (0 = persistente, dismiss su click).
 *
 * Per casi che richiedono conferma utente (es. azioni distruttive
 * di classe), continuare a usare `confirm()` nativo — il toast e'
 * non-bloccante per definizione.
 */

const DEFAULT_MS = { success: 3000, error: 5000, info: 3000 };

function ensureContainer() {
    let c = document.getElementById('toast-container');
    if (!c) {
        c = document.createElement('div');
        c.id = 'toast-container';
        document.body.appendChild(c);
    }
    return c;
}

/**
 * Mostra un toast.
 *
 * @param {string} type - 'success' | 'error' | 'info'
 * @param {string} msg
 * @param {{ms?: number}} [opts]
 */
function show(type, msg, opts) {
    const container = ensureContainer();
    const el = document.createElement('div');
    el.className = 'toast toast-' + type;
    el.setAttribute('role', 'status');

    const icons = { success: '✅', error: '⚠️', info: 'ℹ️' };
    const ico = document.createElement('span');
    ico.className = 'toast-icon';
    ico.textContent = icons[type] || '';
    const txt = document.createElement('span');
    txt.className = 'toast-msg';
    txt.textContent = msg;
    const close = document.createElement('button');
    close.className = 'toast-close';
    close.type = 'button';
    close.textContent = '×';
    close.title = 'Chiudi';
    close.addEventListener('click', () => dismiss(el));

    el.append(ico, txt, close);
    container.appendChild(el);

    // Anim entrata
    requestAnimationFrame(() => el.classList.add('toast-show'));

    const ms = (opts && opts.ms !== undefined) ? opts.ms : DEFAULT_MS[type];
    if (ms > 0) {
        setTimeout(() => dismiss(el), ms);
    }
}

function dismiss(el) {
    if (!el || !el.parentNode) return;
    el.classList.remove('toast-show');
    el.classList.add('toast-hide');
    setTimeout(() => { if (el.parentNode) el.parentNode.removeChild(el); }, 250);
}

export const toast = {
    success: (msg, opts) => show('success', msg, opts),
    error:   (msg, opts) => show('error', msg, opts),
    info:    (msg, opts) => show('info', msg, opts),
};

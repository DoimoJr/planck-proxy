/**
 * @file Utility condivise tra i moduli del frontend: accesso DOM, escaping,
 * ordinamento IP, parsing e formattazione di timestamp/durate.
 *
 * Dipendenze: nessuna (solo API browser standard).
 */

/**
 * Shortcut per `document.getElementById`.
 * @param {string} id - ID dell'elemento nel DOM.
 * @returns {HTMLElement|null}
 */
export function $(id) { return document.getElementById(id); }

/**
 * Escape dei cinque caratteri speciali per testo HTML inseribile con innerHTML.
 * Applicare a qualsiasi stringa di origine utente che finisce in `innerHTML`.
 * @param {unknown} s - Valore da sanitizzare (convertito a stringa).
 * @returns {string} Stringa escaped.
 */
export function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
}

/**
 * Escape minimale per valori inseriti in attributi HTML (`data-*`, `title`, ...).
 * Piu' veloce di `escapeHtml` quando il contesto e' solo un attributo quotato.
 * @param {unknown} s
 * @returns {string}
 */
export function attrEscape(s) {
    return String(s).replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

/**
 * Converte un IPv4 in un intero a 32 bit per ordinamento numerico.
 * Restituisce 0 se la stringa non e' un IPv4 valido (safe fallback).
 * @param {string} ip - IPv4 in forma "a.b.c.d".
 * @returns {number}
 */
export function ip2long(ip) {
    const p = ip.split('.').map(Number);
    return p.length === 4 ? ((p[0] << 24) | (p[1] << 16) | (p[2] << 8) | p[3]) >>> 0 : 0;
}

/**
 * Parse del formato timestamp usato dal server: "YYYY-MM-DD HH:MM:SS" (UTC).
 * Il server scrive orari con `new Date().toISOString()` e poi sostituisce 'T'
 * con spazio: qui facciamo l'inverso, poi aggiungiamo 'Z' per forzare UTC.
 * @param {string} ora
 * @returns {Date}
 */
export function parseOra(ora) {
    return new Date(ora.replace(' ', 'T') + 'Z');
}

/**
 * Formatta una durata in secondi come "H:MM:SS" o "M:SS" (se < 1h).
 * Usato per la durata di sessione e il countdown.
 * @param {number} secTot
 * @returns {string}
 */
export function formatDurata(secTot) {
    const h = Math.floor(secTot / 3600);
    const m = Math.floor((secTot % 3600) / 60);
    const s = secTot % 60;
    if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
    return `${m}:${String(s).padStart(2, '0')}`;
}

/**
 * Formatta uno scostamento in secondi come stringa relativa al passato.
 * Usato per la colonna "Ultima attivita'" della tabella IP.
 * @param {number} secFa - Secondi trascorsi rispetto ad adesso.
 * @returns {string} "ora" | "Xs fa" | "Xm fa" | "Xh fa"
 */
export function formatRelativo(secFa) {
    if (secFa < 2) return 'ora';
    if (secFa < 60) return `${secFa}s fa`;
    if (secFa < 3600) return `${Math.floor(secFa / 60)}m fa`;
    return `${Math.floor(secFa / 3600)}h fa`;
}

/**
 * Riconcilia i figli di `container` con la lista `items` usando `getKey`
 * come chiave stabile. Nodi con chiavi gia' presenti vengono riutilizzati
 * (aggiornati via `update`) invece di essere ricreati: niente `innerHTML`
 * wipe, niente flicker, scroll/hover/focus preservati.
 *
 * @template T
 * @param {HTMLElement} container
 * @param {T[]} items
 * @param {(item: T) => string} getKey
 * @param {(item: T) => HTMLElement} create - Costruisce il nodo (chiamato la prima volta per una chiave).
 * @param {(el: HTMLElement, item: T) => void} update - Aggiorna in-place (chiamato anche sul nodo appena creato).
 */
export function syncChildren(container, items, getKey, create, update) {
    const keyToEl = new Map();
    for (const child of Array.from(container.children)) {
        const k = child.dataset.key;
        if (k != null) keyToEl.set(k, child);
    }
    const wantedKeys = new Set();
    for (const item of items) wantedKeys.add(getKey(item));
    for (const [k, el] of keyToEl) {
        if (!wantedKeys.has(k)) { el.remove(); keyToEl.delete(k); }
    }
    let anchor = container.firstChild;
    for (const item of items) {
        const key = getKey(item);
        let el = keyToEl.get(key);
        if (!el) {
            el = create(item);
            el.dataset.key = key;
            container.insertBefore(el, anchor);
        } else if (el === anchor) {
            anchor = anchor.nextSibling;
        } else {
            container.insertBefore(el, anchor);
        }
        update(el, item);
    }
}

/**
 * @file Modal helper in stile app per sostituire `prompt()` nativi.
 *
 * Usato dove serve un'interazione testuale piu' "presentabile" del
 * prompt browser (es. Veyon broadcast message, blocco dominio puntuale).
 * I confirm() distruttivi (reboot, poweroff, elimina) restano nativi
 * — sono volutamente brutti per evitare click distratti.
 *
 * API:
 *   const text = await showPromptModal({
 *       title: 'Titolo',
 *       message: 'Sotto-testo opzionale',
 *       defaultValue: '',
 *       placeholder: 'Scrivi qui...',
 *       okLabel: 'Invia',
 *       cancelLabel: 'Annulla',
 *   });
 *   // text = string premuto OK, null = annullato (Esc / X / click fuori)
 *
 * Tastiera: Esc chiude, Ctrl/Cmd+Enter conferma.
 */

function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => (
        { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
    ));
}

export function showPromptModal({
    title = '',
    message = '',
    defaultValue = '',
    placeholder = '',
    okLabel = 'OK',
    cancelLabel = 'Annulla',
} = {}) {
    return new Promise((resolve) => {
        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        overlay.innerHTML =
            '<div class="modal-card" role="dialog" aria-labelledby="modal-title">'
            + '<div class="modal-head">'
            + `<div class="modal-title" id="modal-title">${escapeHtml(title)}</div>`
            + '<button class="modal-x" aria-label="Chiudi" title="Chiudi (Esc)">&times;</button>'
            + '</div>'
            + (message ? `<div class="modal-body">${escapeHtml(message)}</div>` : '')
            + `<textarea class="modal-input" placeholder="${escapeHtml(placeholder)}" rows="4">${escapeHtml(defaultValue)}</textarea>`
            + '<div class="modal-actions">'
            + `<button type="button" class="btn modal-cancel">${escapeHtml(cancelLabel)}</button>`
            + `<button type="button" class="btn btn-primary modal-ok">${escapeHtml(okLabel)}</button>`
            + '</div>'
            + '</div>';
        document.body.appendChild(overlay);

        const input = overlay.querySelector('.modal-input');
        const okBtn = overlay.querySelector('.modal-ok');
        const cancelBtn = overlay.querySelector('.modal-cancel');
        const xBtn = overlay.querySelector('.modal-x');

        const close = (val) => {
            document.removeEventListener('keydown', onKey, true);
            overlay.remove();
            resolve(val);
        };
        const onKey = (e) => {
            if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close(null); }
            else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                e.preventDefault(); close(input.value);
            }
        };
        document.addEventListener('keydown', onKey, true); // capture per intercettare prima di altri handler

        okBtn.addEventListener('click', () => close(input.value));
        cancelBtn.addEventListener('click', () => close(null));
        xBtn.addEventListener('click', () => close(null));
        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) close(null);
        });

        // Focus + select del testo dopo che il browser ha applicato il layout.
        setTimeout(() => { input.focus(); input.select(); }, 30);
    });
}

// Utility condivise

export function $(id) { return document.getElementById(id); }

export function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
}

export function attrEscape(s) {
    return String(s).replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

export function ip2long(ip) {
    const p = ip.split('.').map(Number);
    return p.length === 4 ? ((p[0] << 24) | (p[1] << 16) | (p[2] << 8) | p[3]) >>> 0 : 0;
}

export function parseOra(ora) {
    // "2026-04-21 16:26:22" -> Date (interpretato come UTC come fa il server)
    return new Date(ora.replace(' ', 'T') + 'Z');
}

export function formatDurata(secTot) {
    const h = Math.floor(secTot / 3600);
    const m = Math.floor((secTot % 3600) / 60);
    const s = secTot % 60;
    if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
    return `${m}:${String(s).padStart(2, '0')}`;
}

export function formatRelativo(secFa) {
    if (secFa < 2) return 'ora';
    if (secFa < 60) return `${secFa}s fa`;
    if (secFa < 3600) return `${Math.floor(secFa / 60)}m fa`;
    return `${Math.floor(secFa / 3600)}h fa`;
}

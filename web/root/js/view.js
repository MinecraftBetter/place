// Position links (/ace?x=&y=&z=) and French formatting helpers. No DOM here.

export const MIN_ZOOM = 0.1;
export const MAX_ZOOM = 60;

/**
 * Reads ?x=&y=&z= from a query string. Returns null when x/y are missing or outside
 * the canvas; z is optional (null) and clamped.
 */
export function parseView(search, width, height) {
    const q = new URLSearchParams(search);
    if (!q.has("x") || !q.has("y")) return null;
    const x = Number(q.get("x")), y = Number(q.get("y"));
    if (!Number.isInteger(x) || !Number.isInteger(y) || x < 0 || y < 0 || x >= width || y >= height) return null;
    let z = q.has("z") ? Number(q.get("z")) : null;
    if (z !== null && !(z > 0)) z = null;
    if (z !== null) z = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, z));
    return {x, y, z};
}

// Rounds a zoom level for links and labels: 1 decimal below 10, integer above.
export function roundZoom(z) {
    return z >= 10 ? Math.round(z) : Math.round(z * 10) / 10;
}

/** Builds the query string of a view, e.g. "?x=470&y=300&z=6". */
export function viewQuery({x, y, z}, withZoom = true) {
    let q = `?x=${Math.floor(x)}&y=${Math.floor(y)}`;
    if (withZoom && z) q += `&z=${roundZoom(z)}`;
    return q;
}

export function formatZoom(z) {
    return "×" + String(roundZoom(z)).replace(".", ",");
}

export function formatCoord(x, y) {
    return `x ${x} · y ${y}`;
}

export function formatNumber(n) {
    return new Intl.NumberFormat("fr-FR").format(n).replace(/ /g, " ");
}

const MONTHS = ["janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."];

/** "à l'instant", "il y a 3 min", "il y a 2 h", "il y a 4 j", then "13 sept. 2022". */
export function relativeTime(ts, now = Date.now()) {
    const s = Math.max(0, Math.round((now - ts) / 1000));
    if (s < 45) return "à l'instant";
    const m = Math.round(s / 60);
    if (m < 60) return `il y a ${m} min`;
    const h = Math.round(m / 60);
    if (h < 24) return `il y a ${h} h`;
    const d = Math.round(h / 24);
    if (d < 7) return `il y a ${d} j`;
    const date = new Date(ts);
    return `${date.getDate()} ${MONTHS[date.getMonth()]} ${date.getFullYear()}`;
}

/** Cooldown label: 4.2 → "0:05". */
export function formatCountdown(seconds) {
    const s = Math.max(0, Math.ceil(seconds));
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

// Small DOM helpers shared by the pages: icons, toasts, sheets, theme.

export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

// Icons of the design system: 2px stroke, square caps, 24 grid.
const PATHS = {
    grid: '<rect x="3" y="3" width="18" height="18"></rect><path d="M9 3v18M15 3v18M3 9h18M3 15h18"></path>',
    search: '<circle cx="11" cy="11" r="7"></circle><path d="M16 16l5 5"></path>',
    pipette: '<path d="M14 4l6 6-3 3-6-6zM11 7l-7 7v3h3l7-7"></path>',
    link: '<path d="M10 14a4 4 0 0 0 6 0l3-3a4 4 0 0 0-6-6l-1 1M14 10a4 4 0 0 0-6 0l-3 3a4 4 0 0 0 6 6l1-1"></path>',
    minus: '<path d="M5 12h14"></path>',
    plus: '<path d="M12 5v14M5 12h14"></path>',
    close: '<path d="M6 6l12 12M18 6L6 18"></path>',
    back: '<path d="M15 5l-7 7 7 7"></path>',
    chevron: '<path d="M9 5l7 7-7 7"></path>',
    pixel: '<rect x="7" y="7" width="10" height="10"></rect>',
    hourglass: '<path d="M6 3h12M6 21h12M7 3c0 5 10 5 10 9s-10 4-10 9M17 3c0 5-10 5-10 9s10 4 10 9"></path>',
    canvas: '<rect x="3" y="3" width="18" height="18"></rect><path d="M9 3v18M15 3v18M3 9h18M3 15h18"></path>',
    film: '<rect x="3" y="5" width="18" height="14"></rect><path d="M7 5v14M17 5v14M3 9h4M3 15h4M17 9h4M17 15h4"></path>',
    home: '<path d="M3 11l9-7 9 7M5 10v10h14V10"></path>',
    user: '<rect x="8" y="3" width="8" height="8"></rect><path d="M4 21v-3a4 4 0 0 1 4-4h8a4 4 0 0 1 4 4v3"></path>',
    target: '<rect x="5" y="5" width="14" height="14"></rect><path d="M12 2v6M12 16v6M2 12h6M16 12h6"></path>',
    copy: '<rect x="8" y="8" width="12" height="12"></rect><path d="M16 8V4H4v12h4"></path>',
    check: '<path d="M4 12l5 5L20 6"></path>',
    share: '<path d="M12 15V3M7 8l5-5 5 5M5 13v8h14v-8"></path>',
    offline: '<path d="M2 8a15 15 0 0 1 20 0M5 12a10 10 0 0 1 14 0M8.5 15.5a5 5 0 0 1 7 0M12 19h.01M3 3l18 18"></path>',
    sun: '<rect x="8" y="8" width="8" height="8"></rect><path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M5 19l2-2M17 7l2-2"></path>',
    moon: '<path d="M20 14A8 8 0 0 1 10 4a8 8 0 1 0 10 10z"></path>',
    logout: '<path d="M10 4H4v16h6M14 8l4 4-4 4M8 12h10"></path>',
};

export function icon(name, size = 20, strokeWidth = 2) {
    return `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="${strokeWidth}" stroke-linecap="square" aria-hidden="true">${PATHS[name] ?? ""}</svg>`;
}

// Replaces <span data-icon="grid" data-size="18"></span> with the SVG.
export function fillIcons(root = document) {
    for (const el of $$("[data-icon]", root)) {
        el.innerHTML = icon(el.dataset.icon, Number(el.dataset.size) || 20, Number(el.dataset.stroke) || 2);
        el.style.display = "inline-flex";
    }
}

export function escapeHTML(s) {
    return String(s ?? "").replace(/[&<>"']/g, c => ({"&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"})[c]);
}

// Avatar <img>, or a pixel initial when the player has no avatar.
export function avatarHTML(user, cls = "av av-32", extraStyle = "") {
    if (user?.avatar) return `<img class="${cls}" src="${escapeHTML(user.avatar)}" alt="" style="${extraStyle}">`;
    const letter = escapeHTML((user?.pseudo ?? "?").trim().charAt(0).toUpperCase() || "?");
    const bg = user?.accent || "var(--surface-3)";
    return `<span class="${cls} av-letter pixel" aria-hidden="true" style="background:${escapeHTML(bg)};${extraStyle}">${letter}</span>`;
}

// ------------------------------------------------------------------
// Toasts

export function toast(html, {timeout = 3200, cls = ""} = {}) {
    let box = $("#toasts");
    if (!box) {
        box = document.createElement("div");
        box.id = "toasts";
        box.setAttribute("role", "status");
        box.setAttribute("aria-live", "polite");
        document.body.appendChild(box);
    }
    const el = document.createElement("div");
    el.className = "toast anim-feed " + cls;
    el.innerHTML = html;
    box.appendChild(el);
    while (box.children.length > 3) box.firstChild.remove();
    if (timeout) setTimeout(() => el.remove(), timeout);
    return el;
}

// ------------------------------------------------------------------
// Sheets (mobile drawers) and the scrim behind them

let openSheet = null;

export function showSheet(el, {onClose} = {}) {
    if (openSheet && openSheet.el !== el) hideSheet();
    const scrim = $("#scrim");
    scrim.hidden = false;
    el.hidden = false;
    openSheet = {el, onClose};
    const focusable = el.querySelector("button, a, input");
    focusable?.focus({preventScroll: true});
}

export function hideSheet() {
    if (!openSheet) return;
    const {el, onClose} = openSheet;
    openSheet = null;
    el.hidden = true;
    $("#scrim").hidden = true;
    onClose?.();
}

export function sheetOpen() {
    return openSheet !== null;
}

// ------------------------------------------------------------------
// Theme: dark by default, follows the system on first visit, then remembers the choice.

const THEME_KEY = "bp-theme";

export function currentTheme() {
    return document.documentElement.dataset.theme === "clair" ? "clair" : "sombre";
}

export function setTheme(theme) {
    document.documentElement.dataset.theme = theme;
    try { localStorage.setItem(THEME_KEY, theme); } catch { /* private mode */ }
    document.dispatchEvent(new CustomEvent("bp-theme", {detail: theme}));
}

export function toggleTheme() {
    setTheme(currentTheme() === "clair" ? "sombre" : "clair");
}

export function storedGet(key, fallback) {
    try {
        const v = localStorage.getItem(key);
        return v === null ? fallback : JSON.parse(v);
    } catch {
        return fallback;
    }
}

export function storedSet(key, value) {
    try { localStorage.setItem(key, JSON.stringify(value)); } catch { /* private mode */ }
}

// Copies text, with a fallback for non-secure contexts (http://192.168…).
export async function copyText(text) {
    try {
        await navigator.clipboard.writeText(text);
        return true;
    } catch {
        const ta = document.createElement("textarea");
        ta.value = text;
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        const ok = document.execCommand("copy");
        ta.remove();
        return ok;
    }
}

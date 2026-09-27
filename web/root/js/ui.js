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
    play: '<path d="M7 4l13 8-13 8z"></path>',
    pause: '<path d="M7 4v16M17 4v16"></path>',
    drop: '<path d="M4 13l7-7 7 7-7 7zM18 13c0 2 2 3 2 5a2 2 0 0 1-4 0c0-2 2-3 2-5z"></path>',
    blueprint: '<rect x="3" y="3" width="18" height="18"></rect><path d="M3 9h6V3M9 21v-6h6v6M15 9h6"></path>',
    museum: '<path d="M3 9l9-6 9 6M5 10v9M9.5 10v9M14.5 10v9M19 10v9M3 21h18"></path>',
    activity: '<path d="M3 12h4l3-7 4 14 3-7h4"></path>',
    trophy: '<path d="M8 4h8v6a4 4 0 0 1-8 0zM8 6H4v2a4 4 0 0 0 4 4M16 6h4v2a4 4 0 0 1-4 4M12 14v4M8 21h8"></path>',
    shield: '<path d="M12 3l8 3v6c0 5-4 8-8 9-4-1-8-4-8-9V6z"></path>',
    edit: '<path d="M4 20h4L20 8l-4-4L4 16zM14 6l4 4"></path>',
    heart: '<path d="M12 20s-8-5-8-11a4 4 0 0 1 8-1 4 4 0 0 1 8 1c0 6-8 11-8 11z"></path>',
    star: '<path d="M12 3l2.6 5.6 6.1.7-4.5 4.2 1.2 6L12 16.6 6.6 19.5l1.2-6L3.3 9.3l6.1-.7z"></path>',
    upload: '<path d="M12 15V3M7 8l5-5 5 5M5 13v8h14v-8"></path>',
    dice: '<rect x="4" y="4" width="16" height="16"></rect><path d="M9 9h.01M15 15h.01M15 9h.01M9 15h.01"></path>',
    bell: '<path d="M6 16V11a6 6 0 0 1 12 0v5l2 2H4zM10 21h4"></path>',
    filter: '<path d="M4 5h16l-6 7v6l-4 2v-8z"></path>',
    flag: '<path d="M5 21V4M5 4h12l-2 4 2 4H5"></path>',
    eye: '<path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12z"></path><circle cx="12" cy="12" r="3"></circle>',
    trash: '<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"></path>',
    undo: '<path d="M9 14l-5-5 5-5M4 9h10a6 6 0 0 1 0 12h-3"></path>',
    settings: '<circle cx="12" cy="12" r="3"></circle><path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1"></path>',
    users: '<rect x="6" y="3" width="6" height="6"></rect><path d="M2 21v-3a4 4 0 0 1 4-4h6a4 4 0 0 1 4 4v3M16 3h4v6h-4M18 14a4 4 0 0 1 4 4v3"></path>',
    lasso: '<ellipse cx="12" cy="9" rx="8" ry="5"></ellipse><path d="M7 13c-1 3 1 5 3 5s2 3 0 4"></path>',
    wand: '<path d="M4 20L16 8M14 4v2M18 8h2M16 4l1 1M19 11l1 1M20 4l-1 1"></path>',
    rect: '<rect x="4" y="6" width="16" height="12" stroke-dasharray="3 2"></rect>',
    music: '<path d="M9 18V5l11-2v13M9 18a3 3 0 1 1-3-3 3 3 0 0 1 3 3zM20 16a3 3 0 1 1-3-3 3 3 0 0 1 3 3z"></path>',
    mic: '<rect x="9" y="3" width="6" height="11"></rect><path d="M5 11a7 7 0 0 0 14 0M12 18v3"></path>',
    prev: '<path d="M18 5l-9 7 9 7zM6 5v14"></path>',
    next: '<path d="M6 5l9 7-9 7zM18 5v14"></path>',
    clock: '<circle cx="12" cy="12" r="9"></circle><path d="M12 7v5l3 3"></path>',
    download: '<path d="M12 3v12M7 10l5 5 5-5M5 21h14"></path>',
    pinch: '<path d="M4 10V4h6M20 14v6h-6M4 4l6 6M20 20l-6-6"></path>',
    move: '<path d="M12 3v18M3 12h18M12 3l-3 3M12 3l3 3M12 21l-3-3M12 21l3-3M3 12l3-3M3 12l3 3M21 12l-3-3M21 12l-3 3"></path>',
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

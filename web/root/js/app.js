// /ace — the canvas page: loading, WebSocket, palette, cooldown, aim, inspector, sharing.

import {GLWindow} from "../glwindow.js";
import {Connection, downloadWithProgress, loadImage} from "./net.js";
import {PALETTE, DEFAULT_RECENT, parseHex, hexToRgb, rgbToHex, colorName, colorLabel, pushRecent} from "./palette.js";
import {parseView, viewQuery, formatZoom, formatCoord, formatNumber, formatCountdown} from "./view.js";
import {OwnersMap, loadOwners, UserCache} from "./owners.js";
import {$, $$, fillIcons, icon, toast, showSheet, hideSheet, sheetOpen, setTheme, toggleTheme, currentTheme, storedGet, storedSet, copyText, escapeHTML, avatarHTML} from "./ui.js";
import {Inspector} from "./inspector.js";
import {setupDesktop} from "./desktop.js";
import {setupMobile} from "./mobile.js";
import {setupBlueprint} from "./blueprint-ui.js";

const mobileMQ = matchMedia("(max-width: 767px), (pointer: coarse)");
const LOUPE_HIDE_DELAY = 2500;
const MAX_PENDING = 10;

const app = {
    gl: null,
    conn: null,
    me: null,
    blocked: null,
    cooldown: 5,
    readyAt: 0,
    color: "#000000",
    recent: storedGet("bp-recent", DEFAULT_RECENT).map(parseHex),
    owners: null,
    users: new UserCache(),
    inspector: null,
    loaded: false,
    width: 0,
    height: 0,
    aim: null,            // {x, y}: mobile reticle, or keyboard aim on desktop
    aimF: null,           // fractional aim while a finger refines it
    keyboardAim: false,
    hover: null,          // desktop: pixel under the mouse
    picking: false,
    userMoved: false,
    pending: [],          // pixels placed while offline
    sent: new Map(),      // "x,y" → previous colour, to undo a refused pixel
    buffered: [],         // WebSocket pixels received before the image
    placedAt: new Map(),  // "x,y" → ms, from the live feed (for the hover card)
};
window.bpApp = app; // handy in the console

// ------------------------------------------------------------------
// Rendering: one frame per batch of changes

let frame = 0;
app.render = () => {
    if (!frame) frame = requestAnimationFrame(drawFrame);
};

function drawFrame() {
    frame = 0;
    if (!app.loaded) return;
    app.gl.draw();
    app.drawBlueprint?.();
    renderOverlay();
    renderCoords();
    app.inspector.reposition();
}

function renderOverlay() {
    const gl = app.gl;
    const zoom = gl.getZoom();
    const mobile = mobileMQ.matches;

    const hoverEl = $("#hover-px");
    if (app.hover && !mobile && !app.inspector.pinned) {
        const s = gl.pixelToScreen(app.hover.x, app.hover.y);
        const size = Math.max(zoom, 3);
        Object.assign(hoverEl.style, {left: s.x + "px", top: s.y + "px", width: size + "px", height: size + "px"});
        hoverEl.hidden = false;
    } else {
        hoverEl.hidden = true;
    }

    const showReticle = app.aim && app.me && (mobile || app.keyboardAim);
    const reticle = $("#reticle");
    if (showReticle) {
        const s = gl.pixelToScreen(app.aim.x, app.aim.y);
        const size = Math.max(zoom, 8);
        const off = (size - zoom) / 2;
        Object.assign(reticle.style, {left: (s.x - off) + "px", top: (s.y - off) + "px", width: size + "px", height: size + "px"});
        reticle.hidden = false;
    } else {
        reticle.hidden = true;
    }
    renderLoupe(showReticle);

    const layer = $("#pending-layer");
    layer.textContent = "";
    for (const p of app.pending) {
        const s = gl.pixelToScreen(p.x, p.y);
        const el = document.createElement("div");
        el.className = "pending-px";
        const size = Math.max(zoom, 6);
        Object.assign(el.style, {left: s.x + "px", top: s.y + "px", width: size + "px", height: size + "px"});
        layer.appendChild(el);
    }
}

// ------------------------------------------------------------------
// Loupe: 7×7 pixels around the aim

let loupeUntil = 0;
let loupeTimer = null;

function showLoupeFor(ms) {
    loupeUntil = ms === Infinity ? Infinity : performance.now() + ms;
    clearTimeout(loupeTimer);
    if (ms !== Infinity) loupeTimer = setTimeout(app.render, ms + 20);
    app.render();
}

function renderLoupe(reticleShown) {
    const loupe = $("#loupe");
    if (!reticleShown || !mobileMQ.matches || sheetOpen() || performance.now() > loupeUntil) {
        loupe.hidden = true;
        return;
    }
    const {x, y} = app.aim;
    const block = app.gl.getColors(x - 3, y - 3, 7, 7);
    const ctx = loupe.querySelector("canvas").getContext("2d");
    const img = ctx.createImageData(7, 7);
    const voidRGB = currentTheme() === "clair" ? [228, 220, 203] : [12, 11, 10];
    for (let j = 0; j < 7; j++) {
        for (let i = 0; i < 7; i++) {
            const bx = x - 3 + i - block.x, by = y - 3 + j - block.y;
            const o = (j * 7 + i) * 4;
            if (bx >= 0 && by >= 0 && bx < block.w && by < block.h) {
                const b = (by * block.w + bx) * 4;
                img.data.set(block.data.subarray(b, b + 3), o);
            } else {
                img.data.set(voidRGB, o);
            }
            img.data[o + 3] = 255;
        }
    }
    ctx.putImageData(img, 0, 0);
    const s = app.gl.pixelToScreen(x + 0.5, y + 0.5);
    let top = s.y - 150;
    if (top < 70) top = s.y + 40;
    const left = Math.min(Math.max(s.x - 56, 8), window.innerWidth - 120);
    Object.assign(loupe.style, {left: left + "px", top: top + "px"});
    loupe.hidden = false;
}

// ------------------------------------------------------------------
// Coordinates, position link

function renderCoords() {
    const gl = app.gl;
    const zoom = formatZoom(gl.getZoom());
    const c = gl.getCenterPixel();
    const center = {x: Math.min(app.width - 1, Math.max(0, Math.floor(c.x))), y: Math.min(app.height - 1, Math.max(0, Math.floor(c.y)))};
    const p = mobileMQ.matches ? (app.me && app.aim) || center : app.hover || (app.keyboardAim && app.aim) || center;
    for (const el of $$(".js-coords")) {
        el.textContent = mobileMQ.matches ? formatCoord(p.x, p.y) : `${formatCoord(p.x, p.y)} · ${zoom}`;
    }
    for (const el of $$(".js-zoom")) el.textContent = zoom;
}

let urlTimer = null;
app.viewChanged = () => {
    app.userMoved = true;
    app.render();
    clearTimeout(urlTimer);
    urlTimer = setTimeout(() => {
        const v = currentView();
        history.replaceState(null, "", location.pathname + viewQuery(v));
    }, 400);
};

function currentView(target) {
    const c = app.gl.getCenterPixel();
    const t = target ?? (mobileMQ.matches && app.me && app.aim) ?? null;
    return {x: t ? t.x : Math.floor(c.x), y: t ? t.y : Math.floor(c.y), z: app.gl.getZoom()};
}

function viewURL(target, withZoom = true) {
    return location.origin + "/ace" + viewQuery(currentView(target), withZoom);
}

app.zoomBy = f => {
    if (!app.loaded) return;
    const v = app.gl.getViewSize();
    app.gl.zoomAt(f, v.x / 2, v.y / 2);
    app.viewChanged();
};

function defaultZoom() {
    const v = app.gl.getViewSize();
    if (mobileMQ.matches) return v.y * 0.75 / app.height;
    return Math.min((v.x - 32) / app.width, (v.y - 180) / app.height);
}

// ------------------------------------------------------------------
// Colour and palette

app.setColor = (hex) => {
    app.color = parseHex(hex);
    const name = colorLabel(app.color);
    for (const el of $$(".js-cur-swatch")) el.style.background = app.color;
    for (const el of $$(".js-cur-name")) el.textContent = name;
    for (const el of $$(".js-cur-hex")) el.textContent = app.color;
    for (const el of $$(".js-hex")) if (document.activeElement !== el) el.value = app.color;
    for (const el of $$(".js-swatches .swatch, .js-recent .swatch")) {
        el.classList.toggle("swatch-sel", parseHex(el.dataset.color) === app.color);
    }
    storedSet("bp-color", app.color);
};

function renderPalettes() {
    for (const box of $$(".js-swatches")) {
        box.innerHTML = PALETTE.map(([hex, name]) =>
            `<button class="swatch" type="button" data-color="${hex}" style="background: ${hex}" title="${escapeHTML(name)}" aria-label="${escapeHTML(name)}"></button>`).join("");
    }
    renderRecent();
}

function renderRecent() {
    const box = $(".js-recent");
    box.innerHTML = app.recent.map(hex => {
        const label = escapeHTML(colorLabel(hex));
        return `<button class="swatch swatch-s" type="button" data-color="${hex}" style="background: ${hex}" title="${label}" aria-label="${label}"></button>`;
    }).join("");
    app.setColor(app.color);
}

document.addEventListener("click", ev => {
    const sw = ev.target.closest(".js-swatches .swatch, .js-recent .swatch");
    if (!sw) return;
    app.setColor(sw.dataset.color);
    if (sw.closest("#sheet-palette")) hideSheet();
});

for (const input of $$(".js-hex")) {
    input.addEventListener("change", () => app.setColor(input.value));
    input.addEventListener("keydown", ev => {
        if (ev.key === "Enter") {
            app.setColor(input.value);
            input.blur();
        }
    });
}

app.pickColor = (x, y) => {
    app.setColor(rgbToHex(app.gl.getColor({x, y})));
};

app.setPicking = on => {
    app.picking = on;
    document.body.classList.toggle("picking", on);
    for (const b of $$(".js-pipette")) b.setAttribute("aria-pressed", String(on));
};

// ------------------------------------------------------------------
// Placing pixels, cooldown

app.placePixel = (x, y, {quiet = false} = {}) => {
    if (!app.loaded) return false;
    if (!app.me) {
        if (!quiet) toast(`<span class="toast-text"><span class="fs-small b">Tu regardes en invité</span><span class="fs-cap t3">Connecte-toi pour poser des pixels.</span></span><a class="btn btn-sm btn-primary" href="${loginURL()}">Entrer</a>`);
        return false;
    }
    if (app.blocked) {
        if (!quiet) blockedToast(app.blocked);
        return false;
    }
    const now = Date.now();
    if (now < app.readyAt) {
        if (!quiet) flashCooldown();
        return false;
    }
    const guide = app.blueprint?.follow ? app.blueprint.colorAt(x, y) : null;
    if (guide && guide !== app.color) app.setColor(guide);
    const rgb = hexToRgb(app.color);
    const old = app.gl.getColor({x, y});
    if (old[0] === rgb[0] && old[1] === rgb[1] && old[2] === rgb[2]) return false;

    app.readyAt = now + app.cooldown * 1000;
    startCooldownTicker();
    app.gl.setPixelColor(x, y, rgb);
    app.blueprint?.pixelChanged(x, y, rgb);
    if (app.conn.open) {
        app.conn.sendPixel(x, y, rgb);
        app.sent.set(x + "," + y, old);
    } else {
        app.pending.push({x, y, rgb, old});
        if (app.pending.length > MAX_PENDING) {
            const dropped = app.pending.shift();
            app.gl.setPixelColor(dropped.x, dropped.y, dropped.old);
        }
        updateConnText();
    }
    if (!guide) {
        app.recent = pushRecent(app.recent, app.color);
        storedSet("bp-recent", app.recent);
        renderRecent();
    }
    popFx(x, y, app.color);
    scheduleBlueprintPanel();
    if (mobileMQ.matches) {
        toast(`<div class="toast-swatch" style="background: ${app.color}"></div><div class="toast-text"><span class="fs-small b">${app.conn.open ? "Pixel posé" : "Pixel en attente"}</span><span class="fs-cap t3 mono">${formatCoord(x, y)} · ${escapeHTML(colorLabel(app.color))}</span></div>`, {timeout: 2200});
    }
    app.render();
    return true;
};

function popFx(x, y, hex) {
    const s = app.gl.pixelToScreen(x, y);
    const size = Math.max(app.gl.getZoom(), 6);
    const layer = $("#fx-layer");
    for (const cls of ["anim-ring", "anim-pop"]) {
        const el = document.createElement("div");
        el.className = "fx-px " + cls;
        Object.assign(el.style, {left: s.x + "px", top: s.y + "px", width: size + "px", height: size + "px"});
        if (cls === "anim-ring") el.style.border = `2px solid ${hex}`;
        else el.style.background = hex;
        layer.appendChild(el);
        setTimeout(() => el.remove(), 700);
    }
}

let cooldownTimer = null;

function startCooldownTicker() {
    clearInterval(cooldownTimer);
    renderCooldown();
    cooldownTimer = setInterval(renderCooldown, 100);
}

function renderCooldown() {
    const remaining = Math.max(0, app.readyAt - Date.now()) / 1000;
    const ready = remaining <= 0;
    const span = app.cooldown > 0 ? app.cooldown : Math.max(remaining, 0.5);
    const progress = ready ? 1 : 1 - remaining / span;
    const box = $(".d-cooldown");
    if (box) box.hidden = app.cooldown <= 0 && ready;
    for (const fill of $$(".js-cooldown-fill")) {
        fill.style.width = (Math.max(0, Math.min(1, progress)) * 100).toFixed(1) + "%";
        fill.style.background = ready ? "var(--accent)" : "var(--gold)";
    }
    const label = $(".js-cooldown-label");
    if (label) label.textContent = ready ? "Prêt à poser" : `Prochain pixel dans ${formatCountdown(remaining)}`;
    const btn = $(".js-place");
    if (btn) {
        btn.classList.toggle("cooling", !ready);
        btn.classList.toggle("btn-primary", ready);
        btn.setAttribute("aria-disabled", String(!ready || !app.aim));
        const ico = $(".js-place-icon", btn);
        const want = ready ? "pixel" : "hourglass";
        if (ico.dataset.icon !== want) {
            ico.dataset.icon = want;
            ico.innerHTML = icon(want, 18, ready ? 2.4 : 2);
        }
        const lbl = $(".js-place-label", btn);
        lbl.textContent = ready ? "Poser" : formatCountdown(remaining);
        lbl.classList.toggle("mono", !ready);
        btn.setAttribute("aria-label", ready ? "Poser le pixel" : `Prochain pixel dans ${formatCountdown(remaining)}`);
    }
    $(".js-m-cooldown").hidden = ready;
    if (ready) clearInterval(cooldownTimer);
}

function flashCooldown() {
    const box = $(".d-cooldown");
    box?.classList.add("flash");
    setTimeout(() => box?.classList.remove("flash"), 600);
    navigator.vibrate?.(20);
}

function blockedToast(code) {
    const text = code === "banned"
        ? "Ton compte ne peut plus dessiner. Tu peux toujours regarder le canvas."
        : "Ton compte est suspendu pour le moment. Tu peux toujours regarder le canvas.";
    toast(`<span class="toast-text"><span class="fs-small b">Pas de pixel pour l'instant</span><span class="fs-cap t3">${text}</span></span>`, {timeout: 5000});
}

// ------------------------------------------------------------------
// Aim (mobile reticle, keyboard on desktop)

function clampPixel(p) {
    return {x: Math.min(app.width - 1, Math.max(0, p.x)), y: Math.min(app.height - 1, Math.max(0, p.y))};
}

app.followBlueprint = () => {
    if (!app.blueprint?.follow || !app.aim) return;
    const c = app.blueprint.colorAt(app.aim.x, app.aim.y);
    if (c && c !== app.color) app.setColor(c);
};

let bpPanelTimer = null;
function scheduleBlueprintPanel() {
    if (bpPanelTimer) return;
    bpPanelTimer = setTimeout(() => { bpPanelTimer = null; app.renderBlueprintPanel?.(); }, 300);
}

// Brings a pixel into view (and aims at it when there is a reticle).
app.goTo = p => {
    if (app.gl.getZoom() < 8) app.gl.setZoom(8);
    app.gl.centerOn(p.x, p.y);
    if (app.me) {
        if (!mobileMQ.matches) app.keyboardAim = true;
        app.setAim(p, {loupe: true});
    }
    app.viewChanged();
};

app.setAim = (p, {loupe = false} = {}) => {
    app.aim = clampPixel(p);
    app.aimF = {x: app.aim.x + 0.5, y: app.aim.y + 0.5};
    app.followBlueprint();
    if (loupe) showLoupeFor(LOUPE_HIDE_DELAY);
    renderCooldown();
    app.render();
};

app.nudgeAim = (dx, dy) => {
    if (!app.aim) app.setAim(centerPixel());
    const z = app.gl.getZoom();
    app.aimF = {
        x: Math.min(app.width - 0.01, Math.max(0, app.aimF.x + dx / z)),
        y: Math.min(app.height - 0.01, Math.max(0, app.aimF.y + dy / z)),
    };
    app.aim = {x: Math.floor(app.aimF.x), y: Math.floor(app.aimF.y)};
    app.followBlueprint();
    followAim();
    showLoupeFor(Infinity);
};

app.aimSettled = () => showLoupeFor(LOUPE_HIDE_DELAY);

app.moveAim = (dx, dy, {follow = false} = {}) => {
    if (!app.me) return;
    app.keyboardAim = true;
    const base = app.aim ?? app.hover ?? centerPixel();
    app.setAim({x: base.x + dx, y: base.y + dy}, {loupe: mobileMQ.matches});
    if (follow) followAim();
};

// Keeps the aim on screen by moving the view when it gets close to an edge.
function followAim() {
    const s = app.gl.pixelToScreen(app.aim.x + 0.5, app.aim.y + 0.5);
    const v = app.gl.getViewSize();
    const margin = {l: 48, r: 72, t: 130, b: mobileMQ.matches ? 300 : 110};
    let dx = 0, dy = 0;
    if (s.x < margin.l) dx = margin.l - s.x;
    if (s.x > v.x - margin.r) dx = v.x - margin.r - s.x;
    if (s.y < margin.t) dy = margin.t - s.y;
    if (s.y > v.y - margin.b) dy = v.y - margin.b - s.y;
    if (dx || dy) {
        app.gl.move(dx, dy);
        app.viewChanged();
    } else {
        app.render();
    }
}

app.centerPixel = () => centerPixel();

function centerPixel() {
    const c = app.gl.getCenterPixel();
    return clampPixel({x: Math.floor(c.x), y: Math.floor(c.y)});
}

app.setHover = p => {
    const same = p && app.hover && p.x === app.hover.x && p.y === app.hover.y;
    if (same || (!p && !app.hover)) return;
    app.hover = p;
    app.render();
};

app.lastPlaced = (x, y) => app.placedAt.get(x + "," + y) ?? null;

// ------------------------------------------------------------------
// Grid, menus, sharing, escape

app.toggleGrid = () => {
    if (!app.loaded) return;
    const on = !app.gl.getGrid();
    app.gl.setGrid(on);
    for (const b of $$(".js-grid")) b.setAttribute("aria-pressed", String(on));
    storedSet("bp-grid", on);
    app.render();
};

app.closeMenus = () => {
    $("#d-menu").hidden = true;
};

app.escape = () => {
    if (app.blueprintDrag) {
        app.blueprintDrag = false;
        document.body.classList.remove("bp-dragging");
        return;
    }
    if (!$("#d-menu").hidden) return app.closeMenus();
    if (sheetOpen()) return hideSheet();
    if (app.inspector.pinned) return app.inspector.close();
    if (app.picking) return app.setPicking(false);
    if (app.keyboardAim) {
        app.keyboardAim = false;
        app.render();
    }
};

app.copyLink = async target => {
    if (!app.loaded) return;
    const url = viewURL(target);
    const ok = await copyText(url);
    toast(`<span data-icon="${ok ? "check" : "link"}" data-size="18"></span><span class="toast-text"><span class="fs-small b">${ok ? "Lien de la vue copié" : "Copie impossible"}</span><span class="fs-cap t3 mono ellip">${escapeHTML(url.replace(/^https?:\/\//, ""))}</span></span>`);
    fillIcons($("#toasts"));
};

let shareTarget = null;
let shareZoom = true;

app.openShare = target => {
    if (!app.loaded) return;
    shareTarget = target ?? (app.me && app.aim) ?? centerPixel();
    renderShare();
    const canvas = $(".js-share-canvas");
    const w = 96, h = 42;
    const block = app.gl.getColors(shareTarget.x - w / 2, shareTarget.y - h / 2, w, h);
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext("2d");
    ctx.fillStyle = "#fff";
    ctx.fillRect(0, 0, w, h);
    if (block.w && block.h) {
        const img = ctx.createImageData(block.w, block.h);
        img.data.set(block.data);
        ctx.putImageData(img, block.x - (shareTarget.x - w / 2), block.y - (shareTarget.y - h / 2));
    }
    $(".js-share-native").hidden = !navigator.share;
    $(".js-share-hint").textContent = "Les URL existantes restent valables : /ace sans paramètre ouvre la vue d'ensemble.";
    showSheet($("#sheet-share"));
};

function renderShare() {
    const url = viewURL(shareTarget, shareZoom);
    $(".js-share-link").value = url.replace(/^https?:\/\//, "");
    $(".js-share-coord").textContent = formatZoom(app.gl.getZoom());
    const t = $(".js-share-zoom");
    t.classList.toggle("toggle-on", shareZoom);
    t.setAttribute("aria-pressed", String(shareZoom));
    const copyBtn = $(".js-share-copy");
    copyBtn.innerHTML = icon("copy");
}

$(".js-share-zoom").addEventListener("click", () => {
    shareZoom = !shareZoom;
    renderShare();
});
$(".js-share-copy").addEventListener("click", async () => {
    const ok = await copyText(viewURL(shareTarget, shareZoom));
    $(".js-share-copy").innerHTML = icon(ok ? "check" : "copy", 20, ok ? 2.6 : 2);
    $(".js-share-hint").textContent = ok ? "Lien copié !" : "Copie impossible : sélectionne le lien à la main.";
});
$(".js-share-native").addEventListener("click", () => {
    navigator.share?.({title: "BetterPlace", url: viewURL(shareTarget, shareZoom)}).catch(() => {});
});
for (const b of $$(".js-share")) b.addEventListener("click", () => app.openShare());
for (const b of $$(".js-open-palette")) b.addEventListener("click", () => showSheet($("#sheet-palette")));
for (const b of $$(".js-close-sheet")) b.addEventListener("click", hideSheet);
$("#scrim").addEventListener("click", hideSheet);
for (const b of $$(".js-grid")) b.addEventListener("click", app.toggleGrid);
for (const b of $$(".js-inspect-aim")) b.addEventListener("click", () => app.aim && app.inspector.openSheet(app.aim.x, app.aim.y));
for (const b of $$(".js-pick-aim")) b.addEventListener("click", () => {
    if (!app.aim) return;
    app.pickColor(app.aim.x, app.aim.y);
    hideSheet();
});

// ------------------------------------------------------------------
// Account, theme

function loginURL() {
    return "/login?next=" + encodeURIComponent(location.pathname + location.search);
}

for (const a of $$(".js-login")) a.addEventListener("click", () => { a.href = loginURL(); });
for (const f of $$("form[action='/auth/logout']")) {
    f.addEventListener("submit", () => { f.elements.next.value = location.pathname + location.search; });
}

function renderMe() {
    document.body.classList.toggle("is-guest", !app.me);
    document.body.classList.toggle("is-player", !!app.me);
    if (!app.me) return;
    const u = app.me;
    const ring = u.accent || "var(--accent)";
    for (const b of $$(".js-me.me-button")) b.innerHTML = avatarHTML(u, "av av-40");
    for (const b of $$(".js-me.me-button")) b.style.setProperty("--ring", ring);
    for (const b of $$(".m-avatar.js-me")) b.innerHTML = avatarHTML(u, "av av-32");
    for (const el of $$(".js-tab-avatar")) el.innerHTML = avatarHTML(u, "av av-24");
    for (const row of $$(".js-me-row")) {
        row.innerHTML = `${avatarHTML(u, "av av-40", `box-shadow: 0 0 0 2px var(--surface), 0 0 0 4px ${escapeHTML(ring)}`)}
            <div class="me-text"><span class="fs-body b ellip">${escapeHTML(u.pseudo)}</span><span class="fs-cap t3">Voir mon profil</span></div>`;
    }
    for (const a of $$(".js-profile-link")) a.href = "/u/" + encodeURIComponent(u.slug);
    for (const a of $$(".js-admin-link")) a.hidden = u.role !== "admin";
}

for (const b of $$("button.js-me")) b.addEventListener("click", ev => {
    ev.stopPropagation();
    if (mobileMQ.matches) {
        showSheet($("#sheet-me"));
    } else {
        $("#d-menu").hidden = !$("#d-menu").hidden;
    }
});
document.addEventListener("click", ev => {
    if (!ev.target.closest("#d-menu, .js-me")) app.closeMenus();
});

function renderTheme() {
    const t = currentTheme();
    for (const el of $$(".js-theme-label")) el.textContent = t === "clair" ? "Mode sombre" : "Mode clair";
    for (const b of $$(".js-theme")) b.setAttribute("aria-label", t === "clair" ? "Passer en mode sombre" : "Passer en mode clair");
    for (const el of $$(".js-theme [data-icon]")) {
        el.dataset.icon = t === "clair" ? "moon" : "sun";
    }
    for (const b of $$(".js-set-theme")) b.classList.toggle("on", b.dataset.theme === t);
    fillIcons($(".d-header"));
    fillIcons($("#d-menu"));
    document.querySelector("meta[name=theme-color]").content = t === "clair" ? "#f3eee3" : "#11100e";
    app.render();
}

for (const b of $$(".js-theme")) b.addEventListener("click", () => toggleTheme());
for (const b of $$(".js-set-theme")) b.addEventListener("click", () => setTheme(b.dataset.theme));
document.addEventListener("bp-theme", renderTheme);

// ------------------------------------------------------------------
// Connection state (HANDOFF §4.4 « Connexion perdue »)

let retryInfo = null;
let retryTick = null;

function updateConnText() {
    const text = $(".js-conn-text");
    const n = app.pending.length;
    const pendingText = n ? ` · ${n === 1 ? "ton pixel en attente sera envoyé" : `tes ${n} pixels en attente seront envoyés`} au retour` : "";
    if (retryInfo) {
        const s = Math.max(0, Math.ceil((retryInfo.at - Date.now()) / 1000));
        text.textContent = `Connexion perdue · nouvel essai dans ${s} s…${pendingText}`;
        $(".js-conn-attempt").textContent = `${retryInfo.attempt}/${retryInfo.of}`;
    } else {
        text.textContent = "Connexion perdue" + pendingText;
        $(".js-conn-attempt").textContent = "";
    }
    $(".js-conn-modal-text").textContent = "Impossible de rejoindre le serveur après 3 essais." +
        (n ? ` ${n === 1 ? "Ton pixel en attente sera envoyé" : `Tes ${n} pixels en attente seront envoyés`} dès le retour de la connexion.` : "");
    app.render();
}

function setOffline(on) {
    document.body.classList.toggle("frozen", on);
    $("#conn-banner").hidden = !on;
    $("#conn-frozen").hidden = !on;
}

function wireConnection(conn) {
    conn.addEventListener("open", ev => {
        stepDone(".js-step-ws", conn.latency + " ms");
        clearInterval(retryTick);
        retryInfo = null;
        $("#conn-modal").hidden = true;
        $("#conn-scrim").hidden = true;
        const wasOffline = document.body.classList.contains("frozen");
        setOffline(false);
        if (ev.detail.reconnected) {
            resync().then(() => {
                if (wasOffline) toast(`<span data-icon="check" data-size="18"></span><span class="fs-small b">Connexion rétablie</span>`, {timeout: 2200});
                fillIcons($("#toasts"));
            });
        }
    });
    conn.addEventListener("retrying", ev => {
        retryInfo = ev.detail;
        if (!app.loaded) stepMeta(".js-step-ws", `essai ${ev.detail.attempt}/${ev.detail.of}`);
        setOffline(app.loaded);
        clearInterval(retryTick);
        updateConnText();
        retryTick = setInterval(updateConnText, 500);
    });
    conn.addEventListener("failed", () => {
        clearInterval(retryTick);
        retryInfo = null;
        stepError(".js-step-ws", "hors ligne");
        setOffline(true);
        updateConnText();
        $("#conn-modal").hidden = false;
        $("#conn-scrim").hidden = false;
    });
    conn.addEventListener("pixel", ev => onPixel(ev.detail));
    conn.addEventListener("badge", ev => {
        const b = ev.detail;
        toast(`<span class="badge-toast anim-shine"><img class="px anim-bob" src="${escapeHTML(b.sprite)}" alt="" width="36" height="36"></span>
            <span class="toast-text"><span class="fs-small b">Nouveau badge : ${escapeHTML(b.name)}</span><span class="fs-cap t3">Il apparaît sur ton profil.</span></span>
            <a class="btn btn-sm" href="/u/${encodeURIComponent(app.me?.slug ?? "")}">Voir</a>`, {timeout: 6000});
    });
    conn.addEventListener("error", ev => onServerError(ev.detail));
    conn.addEventListener("stat", ev => {
        const n = ev.detail.online;
        for (const el of $$(".js-online")) el.textContent = formatNumber(n);
        for (const el of $$(".js-online-word")) el.textContent = n > 1 ? "connectés" : "connecté";
    });
}

for (const b of $$(".js-conn-retry")) b.addEventListener("click", () => {
    $("#conn-modal").hidden = true;
    $("#conn-scrim").hidden = true;
    app.conn.retryNow();
});
$(".js-reload").addEventListener("click", () => location.reload());

function onPixel(msg) {
    if (!app.loaded || resyncing) {
        app.buffered.push(msg);
        return;
    }
    applyPixel(msg);
}

function applyPixel(msg) {
    const {x, y, color, u, t} = msg;
    ownersCatchUp?.push(msg);
    app.gl.setPixelColor(x, y, [color.R, color.G, color.B]);
    if (app.blueprint?.contains(x, y)) {
        app.blueprint.pixelChanged(x, y, [color.R, color.G, color.B]);
        scheduleBlueprintPanel();
    }
    const key = x + "," + y;
    if (u) {
        app.owners?.set(x, y, u);
        if (app.me && u === app.me.id) app.sent.delete(key);
    }
    if (t) {
        app.placedAt.delete(key);
        app.placedAt.set(key, t);
        if (app.placedAt.size > 5000) app.placedAt.delete(app.placedAt.keys().next().value);
    }
    app.inspector.pixelChanged(x, y);
    if (app.aim && Math.abs(app.aim.x - x) <= 3 && Math.abs(app.aim.y - y) <= 3) app.render();
    app.render();
}

function onServerError(msg) {
    const key = msg.x + "," + msg.y;
    const old = app.sent.get(key);
    if (old) {
        app.gl.setPixelColor(msg.x, msg.y, old);
        app.blueprint?.pixelChanged(msg.x, msg.y, old);
        app.sent.delete(key);
        app.render();
    }
    switch (msg.err) {
        case "cooldown":
        case "flood":
            app.readyAt = Date.now() + msg.retry * 1000;
            startCooldownTicker();
            flashCooldown();
            break;
        case "auth":
            toast(`<span class="toast-text"><span class="fs-small b">Tu n'es plus connecté</span><span class="fs-cap t3">Reconnecte-toi pour continuer à dessiner.</span></span><a class="btn btn-sm btn-primary" href="${loginURL()}">Entrer</a>`, {timeout: 6000});
            break;
        case "suspended":
        case "banned":
            app.blocked = msg.err;
            blockedToast(msg.err);
            break;
    }
}

// After a reconnection: download the canvas again (others kept drawing), then send
// the pixels placed while offline, one per cooldown.
let resyncing = false;

async function resync() {
    resyncing = true;
    try {
        const bytes = await downloadWithProgress("/place.png");
        const img = await loadImage(bytes);
        app.gl.setTexture(img, true);
        for (const p of app.pending) app.gl.setPixelColor(p.x, p.y, p.rgb);
        const buffered = app.buffered;
        app.buffered = [];
        resyncing = false;
        buffered.forEach(applyPixel);
        app.blueprint?.recompute();
        app.render();
        refreshOwners();
        flushPending();
    } catch (err) {
        console.error("Resync failed", err);
    } finally {
        resyncing = false;
    }
}

let flushTimer = null;

function flushPending() {
    clearTimeout(flushTimer);
    if (!app.pending.length || !app.conn.open) return;
    const wait = app.readyAt - Date.now();
    const p = app.pending[0];
    if (wait > 0) {
        flushTimer = setTimeout(flushPending, wait + 50);
        return;
    }
    app.pending.shift();
    app.readyAt = Date.now() + app.cooldown * 1000;
    startCooldownTicker();
    app.conn.sendPixel(p.x, p.y, p.rgb);
    app.sent.set(p.x + "," + p.y, p.old);
    app.render();
    if (app.pending.length) flushTimer = setTimeout(flushPending, app.cooldown * 1000 + 50);
}

// Pixels received while /api/owners downloads: the file may be older than them.
let ownersCatchUp = null;

async function refreshOwners() {
    ownersCatchUp = [];
    try {
        const map = new OwnersMap(app.width, app.height);
        await loadOwners("/api/owners", map);
        for (const msg of ownersCatchUp) if (msg.u) map.set(msg.x, msg.y, msg.u);
        app.owners = map;
        stepDone(".js-step-owners", "");
    } catch (err) {
        console.warn("Owners map unavailable", err);
        stepError(".js-step-owners", "indisponible");
    } finally {
        ownersCatchUp = null;
        app.render();
    }
}

// ------------------------------------------------------------------
// Loading screen

function stepState(sel, state, meta) {
    const el = $(sel);
    if (!el) return;
    el.classList.remove("active", "done", "error", "pending");
    el.classList.add(state);
    $(".step-ico", el).innerHTML = state === "done" ? icon("check", 16, 3) : "";
    if (meta !== undefined) $(".step-meta", el).textContent = meta;
}
const stepDone = (sel, meta) => stepState(sel, "done", meta);
const stepError = (sel, meta) => stepState(sel, "error", meta);
const stepMeta = (sel, meta) => { const el = $(sel); if (el) $(".step-meta", el).textContent = meta; };

function setProgress(pos, len) {
    const pct = len ? Math.round(pos / len * 100) : 0;
    $(".js-load-fill").style.width = pct + "%";
    $(".js-load-pct").textContent = pct + " %";
    $(".js-load-ko").textContent = `${formatNumber(Math.round(pos / 1024))} Ko / ${formatNumber(Math.round(len / 1024))} Ko`;
}

function applyInitialView() {
    const gl = app.gl;
    const v = parseView(location.search, app.width, app.height);
    gl.setZoom(v?.z ?? defaultZoom());
    if (v) {
        gl.centerOn(v.x, v.y);
        if (app.me && mobileMQ.matches) app.setAim({x: v.x, y: v.y});
        app.userMoved = true;
    } else {
        gl.setCam(0, 0);
    }
    if (storedGet("bp-grid", false)) {
        gl.setGrid(true);
        for (const b of $$(".js-grid")) b.setAttribute("aria-pressed", "true");
    }
}

async function start() {
    fillIcons();
    renderPalettes();
    app.setColor(storedGet("bp-color", "#000000"));
    renderTheme();

    const gl = new GLWindow($("#viewport-canvas"));
    if (!gl.ok()) {
        $(".js-load-title").textContent = "WebGL indisponible";
        $(".js-load-sub").textContent = "Ton navigateur ne peut pas afficher le canvas. Essaie avec un navigateur récent.";
        return;
    }
    app.gl = gl;
    app.inspector = new Inspector(app);

    const meP = fetch("/api/me", {cache: "no-store"}).then(r => r.json()).catch(() => null);

    stepState(".js-step-ws", "active");
    stepState(".js-step-img", "active");
    stepState(".js-step-owners", "pending");
    const proto = location.protocol === "https:" ? "wss://" : "ws://";
    app.conn = new Connection(proto + location.host + "/ws");
    wireConnection(app.conn);
    app.conn.connect();

    let img;
    try {
        const bytes = await downloadWithProgress("/place.png", setProgress);
        img = await loadImage(bytes);
    } catch (err) {
        console.error(err);
        stepError(".js-step-img", "échec");
        $(".js-load-title").textContent = "Le canvas n'a pas pu être chargé";
        $(".js-load-sub").textContent = "Vérifie ta connexion puis recharge la page.";
        return;
    }
    app.width = img.width;
    app.height = img.height;
    stepDone(".js-step-img", `${img.width} × ${img.height}`);
    $(".js-load-sub").textContent = `Le cowboy a récupéré les ${formatNumber(img.width * img.height)} pixels.`;

    const me = await meP;
    if (me) {
        app.me = me.user;
        app.cooldown = me.cooldown ?? 0;
        app.readyAt = Date.now() + (me.ready_in || 0) * 1000;
        app.blocked = me.blocked ?? null;
    }
    renderMe();

    gl.setTexture(img);
    applyInitialView();
    app.owners = new OwnersMap(app.width, app.height);
    app.loaded = true;
    if (app.me && !app.aim && mobileMQ.matches) app.setAim(centerPixel());
    const buffered = app.buffered;
    app.buffered = [];
    buffered.forEach(applyPixel);
    if (await app.blueprint.restore()) app.followBlueprint();
    renderCooldown();
    if (app.readyAt > Date.now()) startCooldownTicker();
    app.render();

    $(".js-load-title").textContent = "Canvas prêt";
    stepState(".js-step-owners", "active");
    setTimeout(() => {
        $("#loading").classList.add("done");
        setTimeout(() => $("#loading").hidden = true, 400);
    }, 250);

    // Who placed what: loaded after the image so the canvas shows up first.
    refreshOwners();
}

window.addEventListener("resize", () => {
    if (!app.gl) return;
    app.gl.updateViewScale();
    app.render();
});
mobileMQ.addEventListener("change", () => app.render());

setupDesktop(app);
setupMobile(app);
setupBlueprint(app);
start();

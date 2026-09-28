// /revendiquer — select an artwork (rectangle, lasso, magic wand), see what the backups
// say, send the claim (m-claim-zone, m-claim-form, d-claim).

import {GLWindow} from "../glwindow.js";
import {downloadWithProgress, loadImage} from "./net.js";
import {$, $$, avatarHTML, escapeHTML, fillIcons, icon, toast} from "./ui.js";
import {fetchMe, loginHref} from "./shell.js";
import {formatCoord, formatNumber, formatZoom, parseView} from "./view.js";

const mobileMQ = matchMedia("(max-width: 767px), (pointer: coarse)");
const HINTS = {
    rect: "Étire le cadre autour de ton œuvre. Les coins se déplacent ensuite pour ajuster.",
    lasso: "Dessine le contour de ton œuvre : idéal pour les formes irrégulières.",
    wand: "Touche une couleur : la sélection s'étend aux pixels voisins de même couleur. Touche encore pour ajouter.",
};
const MOVE_HINT = {d: " Clic droit ou molette pour te déplacer, molette pour zoomer.", m: " Pince pour zoomer, deux doigts pour te déplacer."};
const BADGE_INFO = {
    pionnier: ["Pionnier", "/img/badges/pionnier.png", true], veteran: ["Vétéran 2022", "/img/badges/veteran.png", true],
    batisseur: ["Bâtisseur", "/img/badges/batisseur.png", false], indemodable: ["Indémodable", "/img/badges/indemodable.png", false],
};

const st = {
    me: null, gl: null, W: 0, H: 0, pixels: null, zones: null, zonesOn: true,
    tool: "rect", rect: null, lasso: null, wand: new Set(), tol: 0,
    analysis: null, analysing: false, coauthors: [], rep: "egale", weights: {moi: 1}, step: 1,
};

const dmy = ms => { const d = new Date(ms); return `${String(d.getDate()).padStart(2, "0")}/${String(d.getMonth() + 1).padStart(2, "0")}/${String(d.getFullYear()).slice(2)}`; };

// ------------------------------------------------------------------
// Selection → mask

function currentMask() {
    if (st.tool === "rect" && st.rect) {
        const x0 = Math.min(st.rect.x0, st.rect.x1), y0 = Math.min(st.rect.y0, st.rect.y1);
        const x1 = Math.max(st.rect.x0, st.rect.x1), y1 = Math.max(st.rect.y0, st.rect.y1);
        if (x1 - x0 < 1 || y1 - y0 < 1) return null;
        return {type: "rect", rect: [x0, y0, x1 - x0 + 1, y1 - y0 + 1]};
    }
    if (st.tool === "lasso" && st.lasso?.length >= 3 && st.lasso.closed) return {type: "poly", points: st.lasso};
    if (st.tool === "wand" && st.wand.size) return bitmapMask(st.wand);
    return null;
}

function bitmapMask(set) {
    let minX = Infinity, minY = Infinity, maxX = -1, maxY = -1;
    for (const p of set) {
        const x = p % st.W, y = Math.floor(p / st.W);
        minX = Math.min(minX, x); maxX = Math.max(maxX, x); minY = Math.min(minY, y); maxY = Math.max(maxY, y);
    }
    const w = maxX - minX + 1, h = maxY - minY + 1;
    const bits = new Uint8Array(Math.ceil(w * h / 8));
    for (const p of set) {
        const i = (Math.floor(p / st.W) - minY) * w + p % st.W - minX;
        bits[i >> 3] |= 0x80 >> (i & 7);
    }
    let bin = "";
    for (let i = 0; i < bits.length; i += 0x8000) bin += String.fromCharCode.apply(null, bits.subarray(i, i + 0x8000));
    return {type: "bitmap", x: minX, y: minY, w, h, bits: btoa(bin)};
}

function maskBounds(m) {
    if (!m) return null;
    if (m.type === "rect") return {x: m.rect[0], y: m.rect[1], w: m.rect[2], h: m.rect[3]};
    if (m.type === "bitmap") return {x: m.x, y: m.y, w: m.w, h: m.h};
    const xs = m.points.map(p => p[0]), ys = m.points.map(p => p[1]);
    const x = Math.min(...xs), y = Math.min(...ys);
    return {x, y, w: Math.max(...xs) - x + 1, h: Math.max(...ys) - y + 1};
}

// Magic wand: contiguous pixels of (almost) the same colour.
function wandFill(sx, sy) {
    const px = st.pixels, W = st.W, H = st.H, tol = st.tol;
    const o = (sy * W + sx) * 4;
    const r0 = px[o], g0 = px[o + 1], b0 = px[o + 2];
    const seen = new Uint8Array(W * H);
    const stack = [sy * W + sx];
    let added = 0;
    while (stack.length && st.wand.size < 400000) {
        const p = stack.pop();
        if (seen[p]) continue;
        seen[p] = 1;
        const q = p * 4;
        if (Math.max(Math.abs(px[q] - r0), Math.abs(px[q + 1] - g0), Math.abs(px[q + 2] - b0)) > tol) continue;
        if (!st.wand.has(p)) { st.wand.add(p); added++; }
        const x = p % W, y = (p - x) / W;
        if (x > 0) stack.push(p - 1);
        if (x < W - 1) stack.push(p + 1);
        if (y > 0) stack.push(p - W);
        if (y < H - 1) stack.push(p + W);
    }
    return added;
}

// ------------------------------------------------------------------
// Zones already taken (artworks, claims waiting for the team): tinted, outlined along
// their real shape, titled. From /api/zones and its map /api/zones.png.

const ZONE_RGB = {oeuvre: [240, 183, 90], attente: [94, 179, 255]};

async function loadZones() {
    try {
        const [j, img] = await Promise.all([
            fetch("/api/zones", {cache: "no-store"}).then(r => r.json()),
            fetch("/api/zones.png", {cache: "no-store"}).then(r => r.arrayBuffer()).then(loadImage),
        ]);
        const W = st.W, H = st.H, list = j.zones ?? [];
        const c = document.createElement("canvas");
        c.width = W;
        c.height = H;
        const cx = c.getContext("2d", {willReadFrequently: true});
        cx.drawImage(img, 0, 0);
        const src = cx.getImageData(0, 0, W, H).data;
        const idx = new Uint16Array(W * H);
        const tint = cx.createImageData(W, H);
        for (let p = 0; p < W * H; p++) {
            if (!src[p * 4 + 3]) continue;
            const n = src[p * 4] << 8 | src[p * 4 + 1];
            const z = list[n - 1];
            if (!z) continue;
            idx[p] = n;
            tint.data.set([...ZONE_RGB[z.type] ?? ZONE_RGB.oeuvre, 64], p * 4);
        }
        cx.putImageData(tint, 0, 0);
        // outlines: the pixel edges where the zone changes, merged in runs
        const at = (x, y) => x < 0 || y < 0 || x >= W || y >= H ? 0 : idx[y * W + x];
        const type = (a, b) => a === b ? null : list[(b || a) - 1]?.type ?? null;
        const paths = {oeuvre: new Path2D(), attente: new Path2D()};
        for (let y = 0; y <= H; y++) {
            let run = null, x0 = 0;
            for (let x = 0; x <= W; x++) {
                const t = x < W ? type(at(x, y - 1), at(x, y)) : null;
                if (t === run) continue;
                if (run) { paths[run].moveTo(x0, y); paths[run].lineTo(x, y); }
                run = t;
                x0 = x;
            }
        }
        for (let x = 0; x <= W; x++) {
            let run = null, y0 = 0;
            for (let y = 0; y <= H; y++) {
                const t = y < H ? type(at(x - 1, y), at(x, y)) : null;
                if (t === run) continue;
                if (run) { paths[run].moveTo(x, y0); paths[run].lineTo(x, y); }
                run = t;
                y0 = y;
            }
        }
        st.zones = {list, idx, tint: c, paths};
        const n = {oeuvre: 0, attente: 0};
        for (const z of list) n[z.type]++;
        const btn = $(".js-zones");
        $(".js-zones-text", btn).textContent = [
            `${n.oeuvre} déjà revendiquée${n.oeuvre > 1 ? "s" : ""}`, n.attente ? `${n.attente} en attente` : "",
        ].filter(Boolean).join(" · ");
        $(".lg-attente", btn).hidden = !n.attente;
        btn.hidden = !list.length;
        render();
    } catch { /* no layer: the analysis still reports the overlaps */ }
}

function zoneAt(px) {
    const n = st.zones?.idx[px.y * st.W + px.x];
    return n ? st.zones.list[n - 1] : null;
}

const byLine = z => z.par?.length ? ` par ${z.par.map(u => u.pseudo).join(", ")}` : "";

function drawZones(gl, z) {
    if (!st.zones || !st.zonesOn) return;
    const o = gl.pixelToScreen(0, 0);
    octx.save();
    octx.imageSmoothingEnabled = false;
    octx.drawImage(st.zones.tint, o.x, o.y, st.W * z, st.H * z);
    octx.setTransform(z, 0, 0, z, o.x, o.y);
    octx.lineWidth = Math.min(3, Math.max(1.5, z / 4)) / z;
    for (const [t, path] of Object.entries(st.zones.paths)) {
        octx.strokeStyle = `rgb(${ZONE_RGB[t].join(", ")})`;
        octx.stroke(path);
    }
    octx.restore();
    // the titles of the zones big enough on screen
    for (const zn of st.zones.list) {
        if (zn.w * z < 56 || zn.h * z < 28) continue;
        const s = gl.pixelToScreen(zn.x, zn.y);
        if (s.x > overlay.width || s.y > overlay.height || s.x + zn.w * z < 0 || s.y + zn.h * z < 0) continue;
        const text = (zn.type === "attente" ? "En attente · " : "") + zn.titre;
        label(text, Math.max(4, s.x + 4), Math.max(4, s.y + 4), zn.type === "attente" ? ["#0d2a45", "#9fd2ff"] : undefined, zn.w * z - 8);
    }
}

// a zone already taken under the pointer: said in the hint (desktop), once in a toast (touch)
let warned = null;
function warnZone(px, {toastIt = false} = {}) {
    const zn = st.zonesOn ? zoneAt(px) : null;
    const hint = $(".js-hint");
    if (zn) {
        const text = zn.type === "attente"
            ? `« ${zn.titre} » est déjà demandée${byLine(zn)}, en attente de l'équipe.`
            : `« ${zn.titre} » est déjà revendiquée${byLine(zn)}.`;
        hint.textContent = text + " Si c'est la tienne, envoie quand même ta demande : l'équipe départagera.";
        if (toastIt && warned !== zn) toast(`<span class="fs-small b">${escapeHTML(text)}</span>`, {timeout: 3500});
    } else if (warned) {
        hint.textContent = HINTS[st.tool] + MOVE_HINT[mobileMQ.matches ? "m" : "d"];
    }
    warned = zn;
}

// ------------------------------------------------------------------
// Drawing

const overlay = $("#claim-overlay");
const octx = overlay.getContext("2d");
let frame = 0;
const render = () => { if (!frame) frame = requestAnimationFrame(draw); };

function sizeCanvases() {
    const r = $(".js-stage").getBoundingClientRect();
    overlay.width = r.width;
    overlay.height = r.height;
    st.gl?.updateViewScale();
    render();
}

function draw() {
    frame = 0;
    if (!st.gl) return;
    st.gl.draw();
    const gl = st.gl, z = gl.getZoom();
    octx.clearRect(0, 0, overlay.width, overlay.height);
    drawZones(gl, z);
    const m = currentMask();
    const toS = (x, y) => gl.pixelToScreen(x, y);
    octx.save();
    if (st.tool === "rect" && st.rect) {
        const b = maskBounds(m) ?? {x: st.rect.x0, y: st.rect.y0, w: 1, h: 1};
        const a = toS(b.x, b.y);
        dim(() => octx.rect(a.x, a.y, b.w * z, b.h * z));
        octx.strokeStyle = "#f0b75a";
        octx.lineWidth = 2;
        octx.strokeRect(a.x, a.y, b.w * z, b.h * z);
        octx.fillStyle = "#fff";
        octx.strokeStyle = "#000";
        for (const [hx, hy] of corners(b)) {
            const s = toS(hx, hy);
            octx.fillRect(s.x - 6, s.y - 6, 12, 12);
            octx.strokeRect(s.x - 6, s.y - 6, 12, 12);
        }
        label(`${b.w} × ${b.h} · ${formatNumber(b.w * b.h)} px`, a.x, a.y + b.h * z + 8);
    } else if (st.tool === "lasso" && st.lasso?.length) {
        const path = () => {
            st.lasso.forEach(([x, y], i) => {
                const s = toS(x + 0.5, y + 0.5);
                i ? octx.lineTo(s.x, s.y) : octx.moveTo(s.x, s.y);
            });
            if (st.lasso.closed) octx.closePath();
        };
        if (st.lasso.closed) dim(path);
        octx.beginPath();
        path();
        octx.strokeStyle = "#f0b75a";
        octx.lineWidth = 2.5;
        octx.setLineDash([6, 4]);
        octx.stroke();
    } else if (st.tool === "wand" && st.wand.size) {
        const b = maskBounds(m);
        const a = toS(b.x, b.y);
        octx.fillStyle = "rgba(8, 7, 6, .5)";
        octx.fillRect(0, 0, overlay.width, overlay.height);
        // show the selected pixels in their real colours (clear the dim)
        const x0 = Math.max(b.x, Math.floor(gl.screenToCanvas(0, 0).x)), x1 = Math.min(b.x + b.w, Math.ceil(gl.screenToCanvas(overlay.width, 0).x));
        const y0 = Math.max(b.y, Math.floor(gl.screenToCanvas(0, 0).y)), y1 = Math.min(b.y + b.h, Math.ceil(gl.screenToCanvas(0, overlay.height).y));
        if ((x1 - x0) * (y1 - y0) < 600000) {
            for (let y = y0; y < y1; y++) for (let x = x0; x < x1; x++) {
                if (st.wand.has(y * st.W + x)) {
                    const s = toS(x, y);
                    octx.clearRect(Math.floor(s.x), Math.floor(s.y), Math.ceil(z) + 1, Math.ceil(z) + 1);
                }
            }
        }
        octx.strokeStyle = "#f0b75a";
        octx.lineWidth = 2;
        octx.setLineDash([6, 4]);
        octx.strokeRect(a.x, a.y, b.w * z, b.h * z);
        label(`Baguette · ${formatNumber(st.wand.size)} px · tolérance ${st.tol}`, a.x, a.y + b.h * z + 8);
    }
    octx.restore();
    const c = gl.getCenterPixel();
    $(".js-coords").textContent = formatCoord(Math.floor(c.x), Math.floor(c.y));
    $(".js-zoom").textContent = formatZoom(z);
}

function dim(pathFn) {
    octx.beginPath();
    octx.rect(0, 0, overlay.width, overlay.height);
    pathFn();
    octx.fillStyle = "rgba(8, 7, 6, .55)";
    octx.fill("evenodd");
}

function label(text, x, y, [bg, fg] = ["#241604", "#f5c77a"], maxW = Infinity) {
    octx.font = "600 12px 'Instrument Sans', sans-serif";
    while (text.length > 4 && octx.measureText(text).width + 16 > maxW) text = text.slice(0, -2).trimEnd() + "…";
    const w = octx.measureText(text).width + 16;
    octx.fillStyle = bg;
    octx.fillRect(x, y, w, 22);
    octx.fillStyle = fg;
    octx.fillText(text, x + 8, y + 15);
}

function corners(b) {
    return [[b.x, b.y], [b.x + b.w, b.y], [b.x, b.y + b.h], [b.x + b.w, b.y + b.h]];
}

// ------------------------------------------------------------------
// Input: tool with one pointer, move with two fingers / right or middle button

function canvasAt(ev) {
    const r = overlay.getBoundingClientRect();
    return st.gl.screenToCanvas(ev.clientX - r.left, ev.clientY - r.top);
}

function clampPx(p) {
    return {x: Math.max(0, Math.min(st.W - 1, Math.floor(p.x))), y: Math.max(0, Math.min(st.H - 1, Math.floor(p.y)))};
}

let action = null; // {kind: "pan"|"rect"|"corner"|"lasso", ...}
const pointers = new Map();

function setupInput() {
    const el = overlay;
    el.style.touchAction = "none";
    el.addEventListener("contextmenu", e => e.preventDefault());
    el.addEventListener("pointerdown", ev => {
        el.setPointerCapture(ev.pointerId);
        pointers.set(ev.pointerId, {x: ev.clientX, y: ev.clientY});
        if (pointers.size === 2) {
            action = {kind: "pinch", ...pinchState()};
            if (st.tool === "lasso" && st.lasso && !st.lasso.closed) st.lasso = null;
            return;
        }
        if (ev.button === 1 || ev.button === 2) {
            action = {kind: "pan", x: ev.clientX, y: ev.clientY};
            return;
        }
        const p = canvasAt(ev);
        const px = clampPx(p);
        warnZone(px, {toastIt: ev.pointerType !== "mouse"});
        if (st.tool === "rect") {
            if (st.rect) {
                const b = maskBounds(currentMask());
                const hit = b && corners(b).findIndex(([hx, hy]) => {
                    const s = st.gl.pixelToScreen(hx, hy);
                    const r = overlay.getBoundingClientRect();
                    return Math.hypot(s.x - (ev.clientX - r.left), s.y - (ev.clientY - r.top)) < 16;
                });
                if (hit >= 0) {
                    // the opposite corner stays, the grabbed one follows the pointer
                    st.rect = {x0: hit === 0 || hit === 2 ? b.x + b.w - 1 : b.x, y0: hit < 2 ? b.y + b.h - 1 : b.y, x1: px.x, y1: px.y};
                    action = {kind: "rect"};
                    return;
                }
            }
            st.rect = {x0: px.x, y0: px.y, x1: px.x, y1: px.y};
            action = {kind: "rect"};
        } else if (st.tool === "lasso") {
            st.lasso = [[px.x, px.y]];
            action = {kind: "lasso"};
        } else if (st.tool === "wand") {
            action = {kind: "tap", x: ev.clientX, y: ev.clientY, px};
        }
        render();
    });
    el.addEventListener("pointermove", ev => {
        if (ev.pointerType === "mouse" && !pointers.size && st.gl) warnZone(clampPx(canvasAt(ev)));
        if (!pointers.has(ev.pointerId)) return;
        pointers.set(ev.pointerId, {x: ev.clientX, y: ev.clientY});
        if (!action) return;
        if (action.kind === "pinch" && pointers.size >= 2) {
            const now = pinchState();
            st.gl.move(now.cx - action.cx, now.cy - action.cy);
            const r = overlay.getBoundingClientRect();
            if (action.d > 0) st.gl.zoomAt(now.d / action.d, now.cx - r.left, now.cy - r.top);
            Object.assign(action, now);
            render();
            return;
        }
        if (action.kind === "pan") {
            st.gl.move(ev.clientX - action.x, ev.clientY - action.y);
            action.x = ev.clientX;
            action.y = ev.clientY;
            render();
            return;
        }
        const px = clampPx(canvasAt(ev));
        if (action.kind === "rect") {
            st.rect.x1 = px.x;
            st.rect.y1 = px.y;
        } else if (action.kind === "lasso") {
            const last = st.lasso[st.lasso.length - 1];
            if (last[0] !== px.x || last[1] !== px.y) st.lasso.push([px.x, px.y]);
        } else if (action.kind === "tap" && Math.hypot(ev.clientX - action.x, ev.clientY - action.y) > 8) {
            action = {kind: "pan", x: ev.clientX, y: ev.clientY};
        }
        render();
    });
    const up = ev => {
        pointers.delete(ev.pointerId);
        if (!action) return;
        if (action.kind === "pinch") {
            if (pointers.size < 2) action = null;
            return;
        }
        if (action.kind === "lasso") {
            if (st.lasso.length >= 3) st.lasso.closed = true;
            else st.lasso = null;
            selectionChanged();
        } else if (action.kind === "rect") {
            selectionChanged();
        } else if (action.kind === "tap") {
            const n = wandFill(action.px.x, action.px.y);
            if (!n && st.wand.has(action.px.y * st.W + action.px.x)) toast(`<span class="fs-small b">Ce pixel est déjà sélectionné</span>`);
            selectionChanged();
        }
        action = null;
        render();
    };
    el.addEventListener("pointerup", up);
    el.addEventListener("pointercancel", up);
    el.addEventListener("wheel", ev => {
        ev.preventDefault();
        const r = overlay.getBoundingClientRect();
        st.gl.zoomAt(ev.deltaY > 0 ? 1 / 1.1 : 1.1, ev.clientX - r.left, ev.clientY - r.top);
        render();
    }, {passive: false});
    document.addEventListener("keydown", ev => {
        if (ev.target.closest("input, textarea")) return;
        const k = ev.key.toLowerCase();
        if (k === "r") setTool("rect");
        if (k === "l") setTool("lasso");
        if (k === "b") setTool("wand");
        if (k === "escape") { clearSelection(); }
    });
}

function pinchState() {
    const [a, b] = [...pointers.values()];
    return {cx: (a.x + b.x) / 2, cy: (a.y + b.y) / 2, d: Math.hypot(a.x - b.x, a.y - b.y)};
}

function setTool(t) {
    if (st.tool === t) return;
    st.tool = t;
    for (const b of $$(".js-tools [data-tool]")) b.setAttribute("aria-pressed", String(b.dataset.tool === t));
    $(".js-wandopts").hidden = t !== "wand";
    const hint = HINTS[t] + (mobileMQ.matches ? MOVE_HINT.m : MOVE_HINT.d);
    $(".js-hint").textContent = hint;
    $(".js-hint-m").textContent = hint;
    selectionChanged();
    render();
}

function clearSelection() {
    st.rect = null;
    st.lasso = null;
    st.wand.clear();
    selectionChanged();
    render();
}

// ------------------------------------------------------------------
// Analysis

let analyseTimer = null;
let analyseSeq = 0;

function selectionChanged() {
    clearTimeout(analyseTimer);
    const m = currentMask();
    renderChips(m);
    $(".js-next").disabled = !m;
    updateSend();
    if (!m) {
        st.analysis = null;
        renderAnalysis();
        return;
    }
    st.analysing = true;
    renderAnalysis();
    analyseTimer = setTimeout(async () => {
        const seq = ++analyseSeq;
        try {
            const r = await fetch("/api/claims/analyse", {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({masque: m})});
            const j = await r.json();
            if (seq !== analyseSeq) return;
            st.analysis = r.ok ? j : {error: j.error};
        } catch {
            if (seq === analyseSeq) st.analysis = {error: "Le serveur ne répond pas."};
        }
        st.analysing = false;
        renderChips(currentMask());
        renderAnalysis();
    }, 350);
}

function renderChips(m) {
    const b = maskBounds(m);
    const n = st.analysis?.pixels;
    $(".js-chips").innerHTML = b ? `<span class="coord">${formatCoord(b.x, b.y)}</span><span class="coord">${b.w} × ${b.h}</span>${n ? `<span class="coord">${formatNumber(n)} px</span>` : ""}` : "";
}

function analysisFacts(a) {
    if (!a) return "";
    const facts = [];
    if (a.apparition) facts.push([dmy(a.apparition), a.deja_la ? "déjà là à la 1re sauvegarde" : "premiers pixels"]);
    if (a.terminee) facts.push([dmy(a.terminee), "terminée"]);
    if (a.pixels_poses_estimes) facts.push(["≈ " + formatNumber(a.pixels_poses_estimes), "pixels posés"]);
    if (a.jours?.length) facts.push([`${a.jours.length} jour${a.jours.length > 1 ? "s" : ""}`, "de dessin"]);
    facts.push([`${Math.round(a.etat_actuel_pct)} %`, "intacte aujourd'hui", a.etat_actuel_pct >= 90 ? "tac" : ""]);
    return `<div class="claim-facts">${facts.map(([v, k, cls]) => `<div><span class="mono b ${cls || ""}">${v}</span><span class="fs-cap t3">${k}</span></div>`).join("")}</div>`;
}

function conflictLine(res) {
    const k = res?.conflits ?? [];
    if (!k.length) return "Personne d'autre ne revendique cette zone.";
    return k.map(c => c.oeuvre_id
        ? `Chevauche l'œuvre « ${escapeHTML(c.titre)} » déjà attribuée (${Math.round(c.recouvrement_pct)} %).`
        : `${escapeHTML(c.par?.pseudo ?? "Quelqu'un")} revendique aussi « ${escapeHTML(c.titre)} » (${Math.round(c.recouvrement_pct)} % en commun).`).join(" ");
}

function renderAnalysis() {
    const res = st.analysis, m = currentMask();
    let inner;
    if (!m) {
        inner = `<span class="fs-over">D'après les sauvegardes</span><span class="fs-small t2">Sélectionne ton œuvre sur le canvas : on regarde dans les sauvegardes quand elle a été dessinée.</span>`;
    } else if (st.analysing && !res) {
        inner = `<span class="fs-over">D'après les sauvegardes</span><span class="fs-small t2"><span class="live" style="display: inline-block; background: var(--gold)"></span> On fouille les sauvegardes…</span>`;
    } else if (res?.error) {
        inner = `<span class="fs-small tdanger">${escapeHTML(res.error)}</span>`;
    } else if (res && !res.analyse) {
        const s = res.sauvegardes;
        const why = s?.status === "indexing"
            ? `Les sauvegardes sont en cours d'indexation (${s.total ? Math.floor(s.done / s.total * 100) : 0} %) : l'analyse sera jointe à ta demande.`
            : "Pas de sauvegardes sur ce serveur : l'équipe vérifiera à la main.";
        inner = `<span class="fs-over">D'après les sauvegardes</span><span class="fs-small t2">${why}</span><span class="fs-cap t2">${conflictLine(res)}</span>`;
    } else if (res) {
        const a = res.analyse;
        const found = a.apparition || a.pixels_poses_estimes;
        inner = `<span class="fs-over tgold">D'après les sauvegardes</span>
            ${found ? analysisFacts(a) : `<span class="fs-small t2">Rien de tel dans les sauvegardes : cette zone était peut-être vide, ou dessinée après la dernière sauvegarde.</span>`}
            <span class="fs-cap t2">${conflictLine(res)}</span>`;
    } else {
        inner = "";
    }
    const b = maskBounds(m);
    const thumb = b ? `<div class="claim-thumb"><img class="px" src="/api/crop.png?x=${b.x - 4}&y=${b.y - 4}&w=${b.w + 8}&h=${b.h + 8}&z=${Math.max(1, Math.min(8, Math.floor(144 / Math.max(b.w + 8, b.h + 8))))}" alt=""><span class="pill">aujourd'hui</span></div>` : "";
    $(".js-analysis").innerHTML = `<div class="card-2 claim-analysis">${thumb}<div class="claim-analysis-text">${inner}</div></div>`;
    $(".js-analysis-mini").innerHTML = m ? `<div class="card-2 claim-analysis-mini">${icon("clock", 18)}<div><span class="fs-small b">D'après les sauvegardes</span><br><span class="fs-cap t2 pretty">${miniText(res)}</span></div></div>` : "";
    renderExpect();
}

function miniText(res) {
    if (!res || st.analysing) return "On fouille les sauvegardes…";
    if (res.error) return escapeHTML(res.error);
    const a = res.analyse;
    let s = "";
    if (a?.apparition) s = `${a.deja_la ? "Présente dès le" : "Dessinée à partir du"} ${new Date(a.apparition).toLocaleDateString("fr-FR", {day: "numeric", month: "long", year: "numeric"})}, ≈ ${formatNumber(a.pixels_poses_estimes)} pixels. `;
    else if (a) s = "Rien de tel dans les sauvegardes. ";
    return s + conflictLine(res);
}

function renderExpect() {
    const a = st.analysis?.analyse;
    const box = $(".js-expect");
    const badges = a?.badges ?? [];
    const people = 1 + st.coauthors.length;
    const px = a?.pixels_poses_estimes ?? 0;
    let share = "";
    if (px && badges.includes("pionnier")) {
        if (st.rep === "egale") share = `+ ≈ ${formatNumber(Math.round(px / people))} px ${people > 1 ? "chacun" : "sur ton profil"}`;
        else share = `+ ≈ ${formatNumber(px)} px à répartir`;
    }
    box.hidden = !badges.length && !share;
    box.innerHTML = `<span class="fs-small t2">Si c'est validé :</span>${badges.map(b => BADGE_INFO[b] ? `<span class="claim-badge"><img src="${BADGE_INFO[b][1]}" alt="" class="px" width="24" height="24"><span class="fs-cap b${BADGE_INFO[b][2] ? " tgold" : ""}">${BADGE_INFO[b][0]}</span></span>` : "").join("")}<span class="fs-cap t3">${share}</span>`;
    const hint = $(".js-rep-hint");
    hint.textContent = px && badges.includes("pionnier")
        ? (st.rep === "egale" ? `≈ ${formatNumber(Math.round(px / people))} pixels ${people > 1 ? "chacun" : "pour toi"}, ajoutés à vos stats de profil.` : "Donne un poids à chacun : 2 = deux fois plus de pixels.")
        : "Les pixels pionniers sont ajoutés aux profils des auteurs.";
}

// ------------------------------------------------------------------
// Form

function renderCoauthors() {
    $(".js-coauthors").innerHTML = st.coauthors.map((u, i) =>
        `<span class="chip">${avatarHTML(u, "av av-24")}${escapeHTML(u.pseudo)}<button type="button" class="chip-x" data-remove="${i}" aria-label="Retirer ${escapeHTML(u.pseudo)}">${icon("close", 14)}</button></span>`).join("") +
        (st.coauthors.length < 5 ? `<button type="button" class="chip js-add-co">+ Ajouter</button>` : "");
    $(".js-co-hint").textContent = st.coauthors.length
        ? `${st.coauthors.map(u => u.pseudo).join(", ")} ${st.coauthors.length > 1 ? "devront" : "devra"} confirmer depuis le fil des revendications.`
        : "Tu as dessiné à plusieurs ? Ajoute tes co-auteurs.";
    const weights = $(".js-weights");
    weights.hidden = st.rep !== "poids";
    weights.innerHTML = [{slug: "moi", pseudo: "Toi"}, ...st.coauthors].map(u =>
        `<label class="claim-weight"><span class="fs-small ellip">${escapeHTML(u.pseudo)}</span><input class="input input-mono" type="number" min="1" max="100" step="1" value="${st.weights[u.slug] ?? 1}" data-weight="${escapeHTML(u.slug)}"></label>`).join("");
    renderExpect();
}

function updateSend() {
    const ok = !!currentMask() && $(".js-titre").value.trim().length >= 2;
    $(".js-send").disabled = !ok;
}

let searchTimer = null;

function setupForm() {
    $(".js-titre").addEventListener("input", updateSend);
    $(".js-rep").addEventListener("click", e => {
        const b = e.target.closest("[data-rep]");
        if (!b) return;
        st.rep = b.dataset.rep;
        for (const x of $$(".js-rep button")) x.classList.toggle("on", x === b);
        renderCoauthors();
    });
    $(".js-weights").addEventListener("input", e => {
        const i = e.target.closest("[data-weight]");
        if (i) st.weights[i.dataset.weight] = Math.max(1, Math.min(100, Number(i.value) || 1));
    });
    $(".js-coauthors").addEventListener("click", e => {
        const rm = e.target.closest("[data-remove]");
        if (rm) {
            st.coauthors.splice(Number(rm.dataset.remove), 1);
            renderCoauthors();
            return;
        }
        if (e.target.closest(".js-add-co")) {
            $(".js-search-wrap").hidden = false;
            $(".js-search").focus();
        }
    });
    $(".js-search").addEventListener("input", e => {
        clearTimeout(searchTimer);
        const q = e.target.value.trim();
        if (!q) { $(".js-results").hidden = true; return; }
        searchTimer = setTimeout(async () => {
            const j = await fetch("/api/users/search?q=" + encodeURIComponent(q)).then(r => r.json()).catch(() => ({users: []}));
            const users = j.users.filter(u => u.id !== st.me.id && !st.coauthors.some(c => c.id === u.id));
            $(".js-results").innerHTML = users.length
                ? users.map(u => `<button type="button" class="claim-result" data-user='${escapeHTML(JSON.stringify(u))}'>${avatarHTML(u, "av av-24")}<span class="fs-small b">${escapeHTML(u.pseudo)}</span><span class="mono fs-cap t3">/u/${escapeHTML(u.slug)}</span></button>`).join("")
                : `<span class="fs-small t3" style="padding: 8px">Aucun joueur trouvé.</span>`;
            $(".js-results").hidden = false;
        }, 200);
    });
    $(".js-results").addEventListener("click", e => {
        const b = e.target.closest("[data-user]");
        if (!b) return;
        st.coauthors.push(JSON.parse(b.dataset.user));
        $(".js-search").value = "";
        $(".js-results").hidden = true;
        $(".js-search-wrap").hidden = true;
        renderCoauthors();
    });
    $(".js-next").addEventListener("click", () => goStep(2));
    $(".js-back").addEventListener("click", () => goStep(1));
    $(".js-form").addEventListener("submit", async e => {
        e.preventDefault();
        const btn = $(".js-send");
        btn.disabled = true;
        $(".js-error").textContent = "";
        try {
            const r = await fetch("/api/claims", {
                method: "POST", headers: {"Content-Type": "application/json"},
                body: JSON.stringify({
                    masque: currentMask(), titre: $(".js-titre").value, message: $(".js-msg").value,
                    co_auteurs: st.coauthors.map(u => u.slug), repartition: st.rep, poids: st.weights,
                }),
            });
            const j = await r.json();
            if (!r.ok) throw new Error(j.error || "Envoi impossible.");
            location.href = "/revendications?envoyee=" + j.id;
        } catch (err) {
            $(".js-error").textContent = err.message;
            btn.disabled = false;
        }
    });
}

function goStep(n) {
    st.step = n;
    document.documentElement.classList.toggle("claim-step2", n === 2);
    if (n === 2) {
        const b = maskBounds(currentMask());
        $(".js-zone-card").innerHTML = b ? `<div class="claim-zone-img"><img class="px" src="/api/crop.png?x=${b.x - 4}&y=${b.y - 4}&w=${b.w + 8}&h=${b.h + 8}&z=${Math.max(1, Math.min(8, Math.floor(320 / Math.max(b.w + 8, b.h + 8))))}" alt="La zone sélectionnée"></div>
            <div class="claim-zone-meta"><span class="coord">${formatCoord(b.x, b.y)}</span><span class="coord">${b.w} × ${b.h}</span><button type="button" class="btn btn-sm btn-ghost js-edit-zone">Modifier</button></div>` : "";
        $(".js-edit-zone")?.addEventListener("click", () => goStep(1));
        window.scrollTo(0, 0);
    } else {
        requestAnimationFrame(sizeCanvases);
    }
}

// ------------------------------------------------------------------

async function main() {
    fillIcons();
    const me = await fetchMe();
    if (!me.user) {
        location.replace(loginHref());
        return;
    }
    st.me = me.user;
    document.body.classList.replace("is-guest", "is-player");
    const gl = new GLWindow($("#viewport-canvas"));
    if (!gl.ok()) {
        toast(`<span class="fs-small b">WebGL indisponible : impossible d'afficher le canvas.</span>`, {timeout: 0});
        return;
    }
    st.gl = gl;
    const img = await loadImage(await downloadWithProgress("/place.png"));
    st.W = img.width;
    st.H = img.height;
    sizeCanvases();
    gl.setTexture(img);
    const v = parseView(location.search, st.W, st.H);
    const r = $(".js-stage").getBoundingClientRect();
    gl.setZoom(v?.z ?? Math.max(1, Math.min(r.width / st.W, r.height / st.H) * 0.95));
    if (v) gl.centerOn(v.x, v.y);
    st.pixels = gl.getColors(0, 0, st.W, st.H).data;
    setupInput();
    setupForm();
    renderCoauthors();
    $(".js-tools").addEventListener("click", e => {
        const b = e.target.closest("[data-tool]");
        if (b) setTool(b.dataset.tool);
    });
    $(".js-tol").addEventListener("input", e => {
        st.tol = Number(e.target.value);
        $(".js-tol-v").textContent = st.tol;
    });
    $(".js-clear").addEventListener("click", clearSelection);
    $(".js-zones").addEventListener("click", e => {
        st.zonesOn = !st.zonesOn;
        e.currentTarget.setAttribute("aria-pressed", String(st.zonesOn));
        render();
    });
    loadZones();
    st.tool = "";
    setTool("rect");
    window.addEventListener("resize", sizeCanvases);
    render();
}

main();

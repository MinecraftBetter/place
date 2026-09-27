// A zone selector on a WebGL canvas: rectangle (resizable), lasso, magic wand, or a
// ready-made bitmap (all the pixels of a player). Used by the admin zone tool.

import {GLWindow} from "../glwindow.js";
import {downloadWithProgress, loadImage} from "./net.js";
import {formatNumber} from "./view.js";

export class ZoneSelector {
    tool = "rect"; rect = null; lasso = null; wand = new Set(); tol = 0; bitmap = null; label = "";
    onChange = () => {};

    constructor(stage) {
        this.stage = stage;
        this.cvs = stage.querySelector("canvas.zs-gl");
        this.overlay = stage.querySelector("canvas.zs-overlay");
        this.octx = this.overlay.getContext("2d");
        this.frame = 0;
        this.pointers = new Map();
        this.action = null;
    }

    async load() {
        this.gl = new GLWindow(this.cvs);
        const img = await loadImage(await downloadWithProgress("/place.png"));
        this.W = img.width;
        this.H = img.height;
        this.resize();
        this.gl.setTexture(img);
        const r = this.stage.getBoundingClientRect();
        this.gl.setZoom(Math.max(0.5, Math.min(r.width / this.W, r.height / this.H) * 0.95));
        this.pixels = this.gl.getColors(0, 0, this.W, this.H).data;
        this.input();
        window.addEventListener("resize", () => this.resize());
        this.render();
    }

    // reload the canvas image after an action changed it
    async reload() {
        const img = await loadImage(await downloadWithProgress("/place.png?t=" + Date.now()));
        this.gl.setTexture(img, true);
        this.pixels = this.gl.getColors(0, 0, this.W, this.H).data;
        this.render();
    }

    resize() {
        const r = this.stage.getBoundingClientRect();
        this.overlay.width = r.width;
        this.overlay.height = r.height;
        this.gl?.updateViewScale();
        this.render();
    }

    focus(x, y, w, h) {
        const r = this.stage.getBoundingClientRect();
        this.gl.setZoom(Math.max(1, Math.min(24, Math.min(r.width / (w * 1.8), r.height / (h * 1.8)))));
        this.gl.centerOn(x + w / 2, y + h / 2);
        this.render();
    }

    setTool(t) {
        this.tool = t;
        this.rect = null; this.lasso = null; this.wand.clear(); this.bitmap = null; this.label = "";
        this.changed();
    }

    setBitmap(mask, label) {
        this.tool = "bitmap";
        this.bitmap = mask;
        this.label = label;
        this.changed();
    }

    mask() {
        if (this.tool === "rect" && this.rect) {
            const x0 = Math.min(this.rect.x0, this.rect.x1), y0 = Math.min(this.rect.y0, this.rect.y1);
            const x1 = Math.max(this.rect.x0, this.rect.x1), y1 = Math.max(this.rect.y0, this.rect.y1);
            return {type: "rect", rect: [x0, y0, x1 - x0 + 1, y1 - y0 + 1]};
        }
        if (this.tool === "lasso" && this.lasso?.closed && this.lasso.length >= 3) return {type: "poly", points: this.lasso};
        if (this.tool === "wand" && this.wand.size) return bitmapOf(this.wand, this.W);
        if (this.tool === "bitmap" && this.bitmap) return this.bitmap;
        return null;
    }

    bounds() {
        const m = this.mask();
        if (!m) return null;
        if (m.type === "rect") return {x: m.rect[0], y: m.rect[1], w: m.rect[2], h: m.rect[3]};
        if (m.type === "bitmap") return {x: m.x, y: m.y, w: m.w, h: m.h};
        const xs = m.points.map(p => p[0]), ys = m.points.map(p => p[1]);
        return {x: Math.min(...xs), y: Math.min(...ys), w: Math.max(...xs) - Math.min(...xs) + 1, h: Math.max(...ys) - Math.min(...ys) + 1};
    }

    changed() {
        this.render();
        this.onChange(this.mask());
    }

    render() {
        if (this.frame) return;
        this.frame = requestAnimationFrame(() => { this.frame = 0; this.draw(); });
    }

    draw() {
        if (!this.gl) return;
        this.gl.draw();
        const ctx = this.octx, z = this.gl.getZoom();
        ctx.clearRect(0, 0, this.overlay.width, this.overlay.height);
        const b = this.bounds();
        if (!b && !(this.tool === "lasso" && this.lasso)) return;
        const s = p => this.gl.pixelToScreen(p[0], p[1]);
        ctx.save();
        const dimPath = pathFn => {
            ctx.beginPath();
            ctx.rect(0, 0, this.overlay.width, this.overlay.height);
            pathFn();
            ctx.fillStyle = "rgba(8, 7, 6, .55)";
            ctx.fill("evenodd");
        };
        if (this.tool === "lasso" && this.lasso) {
            const path = () => {
                this.lasso.forEach((p, i) => { const q = s([p[0] + .5, p[1] + .5]); i ? ctx.lineTo(q.x, q.y) : ctx.moveTo(q.x, q.y); });
                if (this.lasso.closed) ctx.closePath();
            };
            if (this.lasso.closed) dimPath(path);
            ctx.beginPath(); path();
            ctx.setLineDash([6, 4]); ctx.lineWidth = 2.5; ctx.strokeStyle = "#f0b75a"; ctx.stroke();
        } else if (b) {
            const a = s([b.x, b.y]);
            if (this.tool === "rect") {
                dimPath(() => ctx.rect(a.x, a.y, b.w * z, b.h * z));
            } else {
                ctx.fillStyle = "rgba(8, 7, 6, .5)";
                ctx.fillRect(0, 0, this.overlay.width, this.overlay.height);
                const set = this.tool === "wand" ? this.wand : bitmapSet(this.bitmap, this.W);
                const v0 = this.gl.screenToCanvas(0, 0), v1 = this.gl.screenToCanvas(this.overlay.width, this.overlay.height);
                const x0 = Math.max(b.x, Math.floor(v0.x)), x1 = Math.min(b.x + b.w, Math.ceil(v1.x));
                const y0 = Math.max(b.y, Math.floor(v0.y)), y1 = Math.min(b.y + b.h, Math.ceil(v1.y));
                if ((x1 - x0) * (y1 - y0) < 700000) {
                    for (let y = y0; y < y1; y++) for (let x = x0; x < x1; x++) {
                        if (set.has(y * this.W + x)) { const q = s([x, y]); ctx.clearRect(Math.floor(q.x), Math.floor(q.y), Math.ceil(z) + 1, Math.ceil(z) + 1); }
                    }
                }
            }
            ctx.setLineDash(this.tool === "rect" ? [] : [6, 4]);
            ctx.lineWidth = 2; ctx.strokeStyle = "#f0b75a";
            ctx.strokeRect(a.x, a.y, b.w * z, b.h * z);
            if (this.tool === "rect") {
                ctx.setLineDash([]);
                ctx.fillStyle = "#fff"; ctx.strokeStyle = "#000";
                for (const [hx, hy] of [[b.x, b.y], [b.x + b.w, b.y], [b.x, b.y + b.h], [b.x + b.w, b.y + b.h]]) {
                    const q = s([hx, hy]);
                    ctx.fillRect(q.x - 6, q.y - 6, 12, 12); ctx.strokeRect(q.x - 6, q.y - 6, 12, 12);
                }
            }
        }
        ctx.restore();
    }

    at(ev) {
        const r = this.overlay.getBoundingClientRect();
        const p = this.gl.screenToCanvas(ev.clientX - r.left, ev.clientY - r.top);
        return {x: Math.max(0, Math.min(this.W - 1, Math.floor(p.x))), y: Math.max(0, Math.min(this.H - 1, Math.floor(p.y)))};
    }

    input() {
        const el = this.overlay;
        el.style.touchAction = "none";
        el.addEventListener("contextmenu", e => e.preventDefault());
        el.addEventListener("pointerdown", ev => {
            el.setPointerCapture(ev.pointerId);
            this.pointers.set(ev.pointerId, {x: ev.clientX, y: ev.clientY});
            if (this.pointers.size === 2) { this.action = {kind: "pinch", ...this.pinch()}; return; }
            if (ev.button !== 0 || this.tool === "bitmap") { this.action = {kind: "pan", x: ev.clientX, y: ev.clientY}; return; }
            const p = this.at(ev);
            if (this.tool === "rect") {
                const b = this.bounds();
                if (b) {
                    const r = el.getBoundingClientRect();
                    const corners = [[b.x, b.y], [b.x + b.w, b.y], [b.x, b.y + b.h], [b.x + b.w, b.y + b.h]];
                    const hit = corners.findIndex(([hx, hy]) => { const q = this.gl.pixelToScreen(hx, hy); return Math.hypot(q.x - (ev.clientX - r.left), q.y - (ev.clientY - r.top)) < 16; });
                    if (hit >= 0) {
                        this.rect = {x0: hit === 0 || hit === 2 ? b.x + b.w - 1 : b.x, y0: hit < 2 ? b.y + b.h - 1 : b.y, x1: p.x, y1: p.y};
                        this.action = {kind: "rect"};
                        return;
                    }
                }
                this.rect = {x0: p.x, y0: p.y, x1: p.x, y1: p.y};
                this.action = {kind: "rect"};
            } else if (this.tool === "lasso") {
                this.lasso = [[p.x, p.y]];
                this.action = {kind: "lasso"};
            } else if (this.tool === "wand") {
                this.action = {kind: "tap", x: ev.clientX, y: ev.clientY, p};
            }
            this.render();
        });
        el.addEventListener("pointermove", ev => {
            if (!this.pointers.has(ev.pointerId)) return;
            this.pointers.set(ev.pointerId, {x: ev.clientX, y: ev.clientY});
            const a = this.action;
            if (!a) return;
            if (a.kind === "pinch" && this.pointers.size >= 2) {
                const n = this.pinch(), r = el.getBoundingClientRect();
                this.gl.move(n.cx - a.cx, n.cy - a.cy);
                if (a.d) this.gl.zoomAt(n.d / a.d, n.cx - r.left, n.cy - r.top);
                Object.assign(a, n);
            } else if (a.kind === "pan") {
                this.gl.move(ev.clientX - a.x, ev.clientY - a.y);
                a.x = ev.clientX; a.y = ev.clientY;
            } else if (a.kind === "rect") {
                const p = this.at(ev);
                this.rect.x1 = p.x; this.rect.y1 = p.y;
            } else if (a.kind === "lasso") {
                const p = this.at(ev), l = this.lasso[this.lasso.length - 1];
                if (l[0] !== p.x || l[1] !== p.y) this.lasso.push([p.x, p.y]);
            } else if (a.kind === "tap" && Math.hypot(ev.clientX - a.x, ev.clientY - a.y) > 8) {
                this.action = {kind: "pan", x: ev.clientX, y: ev.clientY};
            }
            this.render();
        });
        const up = ev => {
            this.pointers.delete(ev.pointerId);
            const a = this.action;
            if (!a) return;
            if (a.kind === "pinch" && this.pointers.size >= 2) return;
            this.action = null;
            if (a.kind === "rect") this.changed();
            if (a.kind === "lasso") { if (this.lasso.length >= 3) this.lasso.closed = true; else this.lasso = null; this.changed(); }
            if (a.kind === "tap") { this.flood(a.p.x, a.p.y); this.changed(); }
        };
        el.addEventListener("pointerup", up);
        el.addEventListener("pointercancel", up);
        el.addEventListener("wheel", ev => {
            ev.preventDefault();
            const r = el.getBoundingClientRect();
            this.gl.zoomAt(ev.deltaY > 0 ? 1 / 1.1 : 1.1, ev.clientX - r.left, ev.clientY - r.top);
            this.render();
        }, {passive: false});
    }

    pinch() {
        const [a, b] = [...this.pointers.values()];
        return {cx: (a.x + b.x) / 2, cy: (a.y + b.y) / 2, d: Math.hypot(a.x - b.x, a.y - b.y)};
    }

    flood(sx, sy) {
        const px = this.pixels, W = this.W, H = this.H, tol = this.tol;
        const o = (sy * W + sx) * 4, r0 = px[o], g0 = px[o + 1], b0 = px[o + 2];
        const seen = new Uint8Array(W * H), stack = [sy * W + sx];
        while (stack.length && this.wand.size < 400000) {
            const p = stack.pop();
            if (seen[p]) continue;
            seen[p] = 1;
            const q = p * 4;
            if (Math.max(Math.abs(px[q] - r0), Math.abs(px[q + 1] - g0), Math.abs(px[q + 2] - b0)) > tol) continue;
            this.wand.add(p);
            const x = p % W, y = (p - x) / W;
            if (x > 0) stack.push(p - 1);
            if (x < W - 1) stack.push(p + 1);
            if (y > 0) stack.push(p - W);
            if (y < H - 1) stack.push(p + W);
        }
    }

    describe() {
        const b = this.bounds();
        return b ? `x ${b.x} · y ${b.y} · ${b.w} × ${b.h}` : "Aucune zone";
    }
}

export function bitmapOf(set, W) {
    let minX = Infinity, minY = Infinity, maxX = -1, maxY = -1;
    for (const p of set) { const x = p % W, y = Math.floor(p / W); minX = Math.min(minX, x); maxX = Math.max(maxX, x); minY = Math.min(minY, y); maxY = Math.max(maxY, y); }
    const w = maxX - minX + 1, h = maxY - minY + 1, bits = new Uint8Array(Math.ceil(w * h / 8));
    for (const p of set) { const i = (Math.floor(p / W) - minY) * w + p % W - minX; bits[i >> 3] |= 0x80 >> (i & 7); }
    let bin = "";
    for (let i = 0; i < bits.length; i += 0x8000) bin += String.fromCharCode.apply(null, bits.subarray(i, i + 0x8000));
    return {type: "bitmap", x: minX, y: minY, w, h, bits: btoa(bin)};
}

const bitmapSets = new WeakMap();
export function bitmapSet(m, W) {
    if (!m) return new Set();
    if (bitmapSets.has(m)) return bitmapSets.get(m);
    const bits = Uint8Array.from(atob(m.bits), c => c.charCodeAt(0));
    const set = new Set();
    for (let i = 0; i < m.w * m.h; i++) if (bits[i >> 3] & (0x80 >> (i & 7))) set.add((m.y + Math.floor(i / m.w)) * W + m.x + i % m.w);
    bitmapSets.set(m, set);
    return set;
}

export {formatNumber};

// Blueprints (modèles): an image placed on the canvas as a guide, to know which colour
// goes where. Kept in this browser only (localStorage). The image can be reduced to the
// palette; transparent pixels are not part of the blueprint.

import {PALETTE, hexToRgb, rgbToHex} from "./palette.js";

const KEY = "bp-blueprint";
const MAX_SIDE = 512;
const PAL_RGB = PALETTE.map(([hex]) => Array.from(hexToRgb(hex)));

export function nearestPalette(r, g, b) {
    let best = 0, bestD = Infinity;
    for (let i = 0; i < PAL_RGB.length; i++) {
        const [pr, pg, pb] = PAL_RGB[i];
        // weighted distance, closer to how eyes see
        const d = 2 * (r - pr) ** 2 + 4 * (g - pg) ** 2 + 3 * (b - pb) ** 2;
        if (d < bestD) { bestD = d; best = i; }
    }
    return PAL_RGB[best];
}

// Turns RGBA data into blueprint cells: 0xRRGGBB, or -1 for transparent.
export function toCells(rgba, quantize) {
    const cells = new Int32Array(rgba.length / 4);
    for (let i = 0; i < cells.length; i++) {
        const a = rgba[i * 4 + 3];
        if (a < 128) { cells[i] = -1; continue; }
        let r = rgba[i * 4], g = rgba[i * 4 + 1], b = rgba[i * 4 + 2];
        if (quantize) [r, g, b] = nearestPalette(r, g, b);
        cells[i] = (r << 16) | (g << 8) | b;
    }
    return cells;
}

export class Blueprint {
    x = 0; y = 0; w = 0; h = 0;
    cells = null;           // Int32Array, -1 = not part of the blueprint
    wrong = null;           // Uint8Array, 1 = the canvas differs
    opacity = 0.5;
    mode = "points";        // "image" | "points"
    follow = true;          // placing a pixel uses the blueprint colour
    visible = true;
    name = "";
    #img = null;            // canvas with the blueprint, for drawing
    #app;

    constructor(app) {
        this.#app = app;
    }

    get active() { return !!this.cells; }

    // Reads an image file, scaled so that its larger side is at most maxSide canvas pixels.
    async fromFile(file, {maxSide = 128, quantize = true} = {}) {
        const bitmap = await createImageBitmap(file);
        const scale = Math.min(1, Math.min(maxSide, MAX_SIDE) / Math.max(bitmap.width, bitmap.height));
        const w = Math.max(1, Math.round(bitmap.width * scale)), h = Math.max(1, Math.round(bitmap.height * scale));
        const c = document.createElement("canvas");
        c.width = w; c.height = h;
        const ctx = c.getContext("2d", {willReadFrequently: true});
        ctx.imageSmoothingEnabled = scale < 1; // averaging when shrinking a photo, crisp for pixel art
        ctx.drawImage(bitmap, 0, 0, w, h);
        this.name = file.name || "modèle";
        this.#set(w, h, toCells(ctx.getImageData(0, 0, w, h).data, quantize));
        const center = this.#app.aim ?? this.#app.centerPixel();
        this.x = Math.max(0, Math.min(this.#app.width - w, Math.round(center.x - w / 2)));
        this.y = Math.max(0, Math.min(this.#app.height - h, Math.round(center.y - h / 2)));
        this.recompute();
        this.save();
    }

    #set(w, h, cells) {
        this.w = w; this.h = h; this.cells = cells;
        this.wrong = new Uint8Array(w * h);
        const c = document.createElement("canvas");
        c.width = w; c.height = h;
        const ctx = c.getContext("2d");
        const img = ctx.createImageData(w, h);
        cells.forEach((v, i) => {
            if (v < 0) return;
            img.data[i * 4] = v >> 16; img.data[i * 4 + 1] = (v >> 8) & 255; img.data[i * 4 + 2] = v & 255; img.data[i * 4 + 3] = 255;
        });
        ctx.putImageData(img, 0, 0);
        this.#img = c;
    }

    clear() {
        this.cells = this.wrong = this.#img = null;
        try { localStorage.removeItem(KEY); } catch { /* private mode */ }
    }

    save() {
        if (!this.cells) return;
        try {
            localStorage.setItem(KEY, JSON.stringify({
                x: this.x, y: this.y, w: this.w, h: this.h, opacity: this.opacity, mode: this.mode, follow: this.follow,
                visible: this.visible, name: this.name, png: this.#img.toDataURL("image/png"),
            }));
        } catch { /* quota, private mode: the blueprint stays for this visit */ }
    }

    async restore() {
        let s;
        try { s = JSON.parse(localStorage.getItem(KEY)); } catch { return false; }
        if (!s?.png) return false;
        const img = new Image();
        img.src = s.png;
        try { await img.decode(); } catch { return false; }
        const c = document.createElement("canvas");
        c.width = s.w; c.height = s.h;
        const ctx = c.getContext("2d", {willReadFrequently: true});
        ctx.drawImage(img, 0, 0);
        this.#set(s.w, s.h, toCells(ctx.getImageData(0, 0, s.w, s.h).data, false));
        Object.assign(this, {x: s.x, y: s.y, opacity: s.opacity ?? 0.5, mode: s.mode ?? "points", follow: s.follow ?? true, visible: s.visible ?? true, name: s.name ?? ""});
        this.recompute();
        return true;
    }

    moveTo(x, y) {
        this.x = Math.round(x);
        this.y = Math.round(y);
        this.recompute();
        this.save();
    }

    contains(x, y) {
        return this.cells && x >= this.x && y >= this.y && x < this.x + this.w && y < this.y + this.h && this.cells[(y - this.y) * this.w + x - this.x] >= 0;
    }

    // Blueprint colour at a canvas pixel ("#RRGGBB"), or null.
    colorAt(x, y) {
        if (!this.contains(x, y)) return null;
        const v = this.cells[(y - this.y) * this.w + x - this.x];
        return "#" + v.toString(16).padStart(6, "0").toUpperCase();
    }

    // Compares the whole blueprint with the canvas.
    recompute() {
        if (!this.cells || !this.#app.gl) return;
        const block = this.#app.gl.getColors(this.x, this.y, this.w, this.h);
        this.wrong.fill(0);
        for (let j = 0; j < this.h; j++) {
            for (let i = 0; i < this.w; i++) {
                const v = this.cells[j * this.w + i];
                if (v < 0) continue;
                const bx = this.x + i - block.x, by = this.y + j - block.y;
                if (bx < 0 || by < 0 || bx >= block.w || by >= block.h) continue; // outside the canvas
                const o = (by * block.w + bx) * 4;
                const c = (block.data[o] << 16) | (block.data[o + 1] << 8) | block.data[o + 2];
                this.wrong[j * this.w + i] = c !== v ? 1 : 0;
            }
        }
    }

    // A pixel of the canvas changed.
    pixelChanged(x, y, rgb) {
        if (!this.contains(x, y)) return;
        const i = (y - this.y) * this.w + x - this.x;
        this.wrong[i] = ((rgb[0] << 16) | (rgb[1] << 8) | rgb[2]) !== this.cells[i] ? 1 : 0;
    }

    progress() {
        let total = 0, wrong = 0;
        if (!this.cells) return {total, done: 0};
        for (let i = 0; i < this.cells.length; i++) {
            if (this.cells[i] >= 0 && this.x + i % this.w < this.#app.width && this.y + Math.floor(i / this.w) < this.#app.height) {
                total++;
                wrong += this.wrong[i];
            }
        }
        return {total, done: total - wrong};
    }

    // The wrong pixel closest to (fx, fy).
    nextWrong(fx, fy) {
        let best = null, bestD = Infinity;
        for (let i = 0; i < this.wrong.length; i++) {
            if (!this.wrong[i]) continue;
            const x = this.x + i % this.w, y = this.y + Math.floor(i / this.w);
            const d = (x - fx) ** 2 + (y - fy) ** 2;
            if (d > 0 && d < bestD) { bestD = d; best = {x, y}; }
        }
        return best;
    }

    // Draws the guide on the overlay canvas.
    draw(ctx, gl) {
        if (!this.cells || !this.visible) return;
        const zoom = gl.getZoom();
        const tl = gl.pixelToScreen(this.x, this.y);
        const vw = ctx.canvas.width, vh = ctx.canvas.height;
        ctx.save();
        ctx.imageSmoothingEnabled = false;
        // outline of the blueprint
        ctx.strokeStyle = "rgba(240, 183, 90, .9)";
        ctx.setLineDash([6, 4]);
        ctx.lineWidth = 2;
        ctx.strokeRect(Math.round(tl.x) - 1, Math.round(tl.y) - 1, this.w * zoom + 2, this.h * zoom + 2);
        ctx.setLineDash([]);
        if (this.mode === "image" || zoom < 4) {
            ctx.globalAlpha = this.opacity;
            ctx.drawImage(this.#img, tl.x, tl.y, this.w * zoom, this.h * zoom);
        } else {
            // dots on the pixels still to do, in the colour they should get
            const i0 = Math.max(0, Math.floor(-tl.x / zoom)), j0 = Math.max(0, Math.floor(-tl.y / zoom));
            const i1 = Math.min(this.w, Math.ceil((vw - tl.x) / zoom)), j1 = Math.min(this.h, Math.ceil((vh - tl.y) / zoom));
            const s = Math.max(3, zoom * 0.42), off = (zoom - s) / 2;
            for (let j = j0; j < j1; j++) {
                for (let i = i0; i < i1; i++) {
                    const k = j * this.w + i;
                    if (!this.wrong[k]) continue;
                    const v = this.cells[k];
                    const x = tl.x + i * zoom + off, y = tl.y + j * zoom + off;
                    ctx.fillStyle = "#000";
                    ctx.fillRect(x - 1, y - 1, s + 2, s + 2);
                    ctx.fillStyle = "#" + v.toString(16).padStart(6, "0");
                    ctx.fillRect(x, y, s, s);
                }
            }
        }
        ctx.restore();
    }
}

export {rgbToHex};

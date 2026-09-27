// A small pixel editor (drawn banners): pencil, bucket, pipette, undo, palette colours.

import {PALETTE, parseHex, hexToRgb, rgbToHex} from "./palette.js";
import {escapeHTML, icon} from "./ui.js";

const TOOLS = [["pencil", "Crayon", "edit"], ["bucket", "Remplir", "drop"], ["pipette", "Pipette", "pipette"]];

export class PixelEditor {
    #w; #h; #px; #cvs; #ctx; #root; #color = "#FF63AA"; #tool = "pencil"; #undo = []; #drawing = false; #last = null;
    onChange = () => {};

    constructor(root, {w = 48, h = 16} = {}) {
        this.#root = root;
        this.#w = w;
        this.#h = h;
        this.#px = new Array(w * h).fill("#5EB3FF");
        root.innerHTML = `
            <div class="pe-toolbar">
                <div class="seg pe-tools" role="group" aria-label="Outil">
                    ${TOOLS.map(([id, label, ic]) => `<button type="button" data-tool="${id}" aria-pressed="false">${icon(ic, 16)}${label}</button>`).join("")}
                </div>
                <span class="grow"></span>
                <button type="button" class="btn btn-sm btn-ghost js-pe-undo" aria-label="Annuler le dernier trait">${icon("undo", 16)}Annuler</button>
                <button type="button" class="btn btn-sm btn-ghost js-pe-fill">Tout remplir</button>
            </div>
            <div class="pe-canvas-wrap"><canvas class="pe-canvas" width="${w}" height="${h}" aria-label="Bannière à dessiner, ${w} × ${h} pixels"></canvas></div>
            <div class="pe-palette">${PALETTE.map(([hex, name]) =>
                `<button type="button" class="swatch pe-swatch" data-color="${hex.toUpperCase()}" style="background: ${hex}" title="${escapeHTML(name)}" aria-label="${escapeHTML(name)}"></button>`).join("")}</div>
            <span class="hint">${w} × ${h} pixels, aux couleurs du canvas. Clique ou glisse pour dessiner.</span>`;
        this.#cvs = root.querySelector("canvas");
        this.#ctx = this.#cvs.getContext("2d");
        root.addEventListener("click", ev => {
            const t = ev.target.closest("[data-tool], [data-color], .js-pe-undo, .js-pe-fill");
            if (!t) return;
            if (t.dataset.tool) this.#tool = t.dataset.tool;
            if (t.dataset.color) { this.#color = t.dataset.color; if (this.#tool === "pipette") this.#tool = "pencil"; }
            if (t.classList.contains("js-pe-undo")) this.undo();
            if (t.classList.contains("js-pe-fill")) { this.#snapshot(); this.#px.fill(this.#color); this.#changed(); }
            this.#renderUI();
        });
        this.#cvs.addEventListener("pointerdown", ev => {
            ev.preventDefault();
            this.#cvs.setPointerCapture(ev.pointerId);
            const c = this.#cell(ev);
            if (!c) return;
            if (this.#tool === "pipette") {
                this.#color = this.#px[c.y * w + c.x];
                this.#tool = "pencil";
                this.#renderUI();
                return;
            }
            this.#snapshot();
            if (this.#tool === "bucket") {
                this.#flood(c.x, c.y, this.#color);
                this.#changed();
                return;
            }
            this.#drawing = true;
            this.#last = c;
            this.#set(c.x, c.y);
            this.#changed();
        });
        this.#cvs.addEventListener("pointermove", ev => {
            if (!this.#drawing) return;
            const c = this.#cell(ev);
            if (!c) return;
            this.#line(this.#last, c);
            this.#last = c;
            this.#changed();
        });
        const stop = () => { this.#drawing = false; this.#last = null; };
        this.#cvs.addEventListener("pointerup", stop);
        this.#cvs.addEventListener("pointercancel", stop);
        this.#renderUI();
        this.#draw();
    }

    get width() { return this.#w; }
    get height() { return this.#h; }

    // A sunset to start from.
    startingPoint(accent = "#FF63AA") {
        const rows = ["#511E9F", "#511E9F", "#6A5CFF", "#B44AC0", "#FF63AA", "#FF63AA", "#FF2651", "#FFA800", "#FFA800", "#FFD623"];
        for (let y = 0; y < this.#h; y++) {
            for (let x = 0; x < this.#w; x++) {
                this.#px[y * this.#w + x] = y < rows.length ? rows[y] : (y < rows.length + 2 ? "#9C451A" : "#6D302F");
            }
        }
        // the sun
        const cx = Math.round(this.#w * 0.7), cy = rows.length;
        for (let y = cy - 3; y < cy; y++) for (let x = cx - 4; x <= cx + 4; x++) {
            if ((x - cx) ** 2 / 20 + (y - cy) ** 2 / 9 <= 1) this.#px[y * this.#w + x] = "#FFF8B8";
        }
        this.#color = parseHex(accent);
        this.#undo = [];
        this.#renderUI();
        this.#changed();
    }

    // Loads an existing drawn banner.
    load(url) {
        return new Promise(resolve => {
            const img = new Image();
            img.onload = () => {
                const c = document.createElement("canvas");
                c.width = this.#w;
                c.height = this.#h;
                const ctx = c.getContext("2d");
                ctx.imageSmoothingEnabled = false;
                ctx.drawImage(img, 0, 0, this.#w, this.#h);
                const d = ctx.getImageData(0, 0, this.#w, this.#h).data;
                for (let i = 0; i < this.#px.length; i++) this.#px[i] = rgbToHex(d.subarray(i * 4, i * 4 + 3));
                this.#undo = [];
                this.#changed();
                resolve(true);
            };
            img.onerror = () => resolve(false);
            img.src = url;
        });
    }

    undo() {
        const prev = this.#undo.pop();
        if (prev) {
            this.#px = prev;
            this.#changed();
        }
    }

    toBlob() {
        return new Promise(res => this.#cvs.toBlob(res, "image/png"));
    }

    toDataURL() {
        return this.#cvs.toDataURL("image/png");
    }

    #cell(ev) {
        const r = this.#cvs.getBoundingClientRect();
        const x = Math.floor((ev.clientX - r.left) / r.width * this.#w);
        const y = Math.floor((ev.clientY - r.top) / r.height * this.#h);
        if (x < 0 || y < 0 || x >= this.#w || y >= this.#h) return null;
        return {x, y};
    }

    #set(x, y) {
        this.#px[y * this.#w + x] = this.#color;
    }

    #line(a, b) {
        let {x: x0, y: y0} = a;
        const dx = Math.abs(b.x - x0), dy = -Math.abs(b.y - y0), sx = x0 < b.x ? 1 : -1, sy = y0 < b.y ? 1 : -1;
        let err = dx + dy;
        for (;;) {
            this.#set(x0, y0);
            if (x0 === b.x && y0 === b.y) break;
            const e2 = 2 * err;
            if (e2 >= dy) { err += dy; x0 += sx; }
            if (e2 <= dx) { err += dx; y0 += sy; }
        }
    }

    #flood(x, y, color) {
        const from = this.#px[y * this.#w + x];
        if (from === color) return;
        const stack = [[x, y]];
        while (stack.length) {
            const [cx, cy] = stack.pop();
            if (cx < 0 || cy < 0 || cx >= this.#w || cy >= this.#h || this.#px[cy * this.#w + cx] !== from) continue;
            this.#px[cy * this.#w + cx] = color;
            stack.push([cx + 1, cy], [cx - 1, cy], [cx, cy + 1], [cx, cy - 1]);
        }
    }

    #snapshot() {
        this.#undo.push(this.#px.slice());
        if (this.#undo.length > 50) this.#undo.shift();
    }

    #draw() {
        const img = this.#ctx.createImageData(this.#w, this.#h);
        this.#px.forEach((hex, i) => {
            img.data.set(hexToRgb(hex), i * 4);
            img.data[i * 4 + 3] = 255;
        });
        this.#ctx.putImageData(img, 0, 0);
    }

    #changed() {
        this.#draw();
        this.onChange();
    }

    #renderUI() {
        for (const b of this.#root.querySelectorAll("[data-tool]")) {
            const on = b.dataset.tool === this.#tool;
            b.classList.toggle("on", on);
            b.setAttribute("aria-pressed", String(on));
        }
        for (const b of this.#root.querySelectorAll("[data-color]")) b.classList.toggle("swatch-sel", b.dataset.color === this.#color);
        this.#cvs.style.cursor = this.#tool === "pipette" ? "crosshair" : "cell";
    }
}

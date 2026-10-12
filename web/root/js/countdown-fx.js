// The last seconds of the countdown, in pixels: each number is built from pixels that fly in
// from everywhere, then blows up when the next one arrives. Pixels of the palette rain harder
// as zero gets close, the screen shakes for the last three, and zero ends in a white flash.

import {PALETTE} from "./palette.js";

const COLORS = PALETTE.map(p => p[0]);
const GOLD = ["#f5c77a", "#ffd623", "#ffa800"];
const HOT = ["#ff2d00", "#ff2651", "#ffa800"];
const pick = a => a[Math.floor(Math.random() * a.length)];

export function startFx(canvas) {
    const ctx = canvas.getContext("2d");
    let W, H, cell, cols, rows, parts = [], rain = [], sparks = [], flash = 0, heat = 0, shake = 0, raf = 0, last = 0;

    function resize() {
        const dpr = window.devicePixelRatio || 1;
        W = canvas.width = Math.round(innerWidth * dpr);
        H = canvas.height = Math.round(innerHeight * dpr);
        cell = Math.max(4, Math.floor(Math.min(W, H) / 100));
        cols = Math.ceil(W / cell);
        rows = Math.ceil(H / cell);
    }
    resize();
    addEventListener("resize", resize);

    // the pixels of a number, in cells, centred
    function targets(text) {
        const off = document.createElement("canvas");
        off.width = cols;
        off.height = rows;
        const o = off.getContext("2d");
        o.fillStyle = "#fff";
        o.textAlign = "center";
        o.textBaseline = "middle";
        o.font = `700 ${Math.floor(Math.min(rows * .7, cols * .9 / Math.max(1, text.length * .6)))}px "Pixelify Sans", monospace`;
        o.fillText(text, cols / 2, rows / 2);
        const d = o.getImageData(0, 0, cols, rows).data, out = [];
        for (let y = 0; y < rows; y++) for (let x = 0; x < cols; x++) if (d[(y * cols + x) * 4 + 3] > 140) out.push([x, y]);
        return out;
    }

    function explode(power) {
        for (const p of parts) {
            const a = Math.atan2(p.y - rows / 2, p.x - cols / 2) + (Math.random() - .5);
            const v = (.4 + Math.random()) * power;
            sparks.push({x: p.x, y: p.y, vx: Math.cos(a) * v, vy: Math.sin(a) * v - .3, c: p.c, life: 1});
        }
        parts = [];
    }

    function frame(t) {
        const dt = Math.min(3, (t - last) / 16.7 || 1);
        last = t;
        ctx.fillStyle = "#0b0a09";
        ctx.fillRect(0, 0, W, H);
        const px = (x, y, c) => { ctx.fillStyle = c; ctx.fillRect(Math.round(x) * cell, Math.round(y) * cell, cell, cell); };

        // the palette rains, harder and harder
        for (let i = 0; i < heat * 6 * dt; i++) rain.push({x: Math.random() * cols, y: -1, v: .3 + Math.random() * (.4 + heat), c: pick(COLORS)});
        ctx.globalAlpha = .55;
        rain = rain.filter(r => { r.y += r.v * dt; px(r.x, r.y, r.c); return r.y < rows; });
        // random pixels light up here and there, like players drawing
        for (let i = 0; i < heat * 30; i++) { ctx.globalAlpha = Math.random() * .25; px(Math.random() * cols, Math.random() * rows, pick(COLORS)); }

        // pieces of the previous number
        sparks = sparks.filter(s => {
            s.x += s.vx * dt; s.y += s.vy * dt; s.vy += .04 * dt; s.life -= .035 * dt;
            ctx.globalAlpha = Math.max(0, s.life * .7);
            px(s.x, s.y, s.c);
            return s.life > 0;
        });

        // the number: a dark shadow, then the pixels flying to their place
        ctx.globalAlpha = 1;
        for (const p of parts) {
            const k = 1 - Math.pow(1 - p.speed, dt);
            p.x += (p.tx - p.x) * k;
            p.y += (p.ty - p.y) * k;
            if (Math.abs(p.tx - p.x) < .5 && Math.abs(p.ty - p.y) < .5) px(p.tx, p.ty + 1, "#000");
        }
        for (const p of parts) px(p.x, p.y, p.c);

        if (flash > 0) { ctx.globalAlpha = Math.min(1, flash); ctx.fillStyle = "#fff"; ctx.fillRect(0, 0, W, H); flash -= .03 * dt; }
        ctx.globalAlpha = 1;
        canvas.style.transform = shake ? `translate(${(Math.random() - .5) * shake}px, ${(Math.random() - .5) * shake}px)` : "";
        raf = requestAnimationFrame(frame);
    }
    raf = requestAnimationFrame(frame);

    return {
        // shows n seconds left (1…15)
        number(n) {
            explode(2.2 + (15 - n) * .1);
            heat = (16 - n) / 15;
            shake = n <= 3 ? 14 - n * 3 : 0;
            const pal = n <= 3 ? HOT : GOLD;
            parts = targets(String(n)).map(([tx, ty]) => {
                const a = Math.random() * Math.PI * 2, r = Math.max(cols, rows) * (.35 + Math.random() * .3);
                return {x: cols / 2 + Math.cos(a) * r, y: rows / 2 + Math.sin(a) * r, tx, ty, speed: .24 + Math.random() * .12,
                    c: Math.random() < .08 ? pick(COLORS) : pick(pal)};
            });
            if (n <= 3) flash = .25;
        },
        // zero: everything blows up in a white flash
        boom() {
            explode(4);
            flash = 1.4;
            shake = 0;
            heat = 0;
        },
        stop() {
            cancelAnimationFrame(raf);
            removeEventListener("resize", resize);
        },
    };
}

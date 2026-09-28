// /timelapse — the canvas day by day from the real captures, and any day minute by minute
// (m-timelapse, d-timelapse). The video and the monthly archives keep their URLs.

import {$, $$, escapeHTML, fillIcons, icon} from "./ui.js";
import {mountShell} from "./shell.js";
import {formatNumber} from "./view.js";

const MONTHS = ["janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."];
const MONTHS_CAP = ["Janv.", "Févr.", "Mars", "Avr.", "Mai", "Juin", "Juil.", "Août", "Sept.", "Oct.", "Nov.", "Déc."];
const SPEEDS = [["1", "×1", 2], ["4", "×4", 8], ["10", "×10", 20]];
const dateLong = d => { const [y, m, dd] = d.split("-").map(Number); return `${dd} ${MONTHS[m - 1]} ${y}`; };

let data = null, i = 0, playing = false, speed = "1", timer = null;
const cache = new Map();

function frameURL(k) { return `/api/snapshots/${k}.png`; }

function preload(k) {
    if (cache.has(k)) return cache.get(k);
    const img = new Image();
    img.src = frameURL(k);
    cache.set(k, img);
    if (cache.size > 40) cache.delete(cache.keys().next().value);
    return img;
}

function show() {
    const d = data.days[i];
    const img = $(".js-frame");
    img.src = preload(d.frame).src;
    img.style.width = (d.w / data.width * 100) + "%";
    img.style.height = (d.h / data.height * 100) + "%";
    $(".js-grow").hidden = d.w === data.width && d.h === data.height;
    $(".js-date").textContent = dateLong(d.date);
    $(".js-meta").textContent = `${d.w} × ${d.h} · ${d.minutes} min d'activité ce jour-là`;
    $(".js-seek").value = i;
    $(".js-download").href = frameURL(d.frame);
    for (const [k, b] of $$(".js-bars > div").entries()) b.classList.toggle("on", k === i);
    for (let k = 1; k <= 3; k++) if (data.days[i + k]) preload(data.days[i + k].frame);
}

function setPlaying(on) {
    playing = on;
    clearInterval(timer);
    $(".js-play").innerHTML = icon(on ? "pause" : "play", 20);
    $(".js-play").setAttribute("aria-label", on ? "Pause" : "Lecture");
    if (!on) return;
    const fps = SPEEDS.find(s => s[0] === speed)[2];
    timer = setInterval(() => {
        if (i >= data.days.length - 1) { setPlaying(false); return; }
        i++;
        show();
    }, 1000 / fps);
}

// ------------------------------------------------------------------
// A day minute by minute: the capture before the day, then every change of the day.

let replay = null;

async function replayDay() {
    stopReplay();
    setPlaying(false);
    const d = data.days[i];
    const btn = $(".js-replay-day");
    btn.disabled = true;
    btn.textContent = "Chargement…";
    const buf = await fetch(`/api/snapshots/day/${d.date}.bin`).then(r => r.arrayBuffer());
    const dv = new DataView(buf);
    let o = 4;
    const u32 = () => { const v = dv.getUint32(o, true); o += 4; return v; };
    u32(); // version
    const start = u32(), nFrames = u32();
    const times = [];
    for (let k = 0; k < nFrames; k++) { times.push(Number(dv.getBigInt64(o, true))); o += 8; }
    const nChanges = u32();
    const changes = new Uint32Array(buf, o, nChanges * 3);
    const base = new Image();
    base.src = frameURL(start);
    await base.decode();
    const cvs = $(".js-replay-canvas");
    cvs.width = data.width;
    cvs.height = data.height;
    const ctx = cvs.getContext("2d");
    ctx.fillStyle = "#fff";
    ctx.fillRect(0, 0, data.width, data.height);
    ctx.drawImage(base, 0, 0);
    const img = ctx.getImageData(0, 0, data.width, data.height);
    cvs.hidden = false;
    $(".js-frame").hidden = true;
    btn.disabled = false;
    btn.innerHTML = `${icon("close", 16)}Arrêter`;
    let frame = 0, c = 0;
    const step = () => {
        const per = speed === "1" ? 1 : speed === "4" ? 4 : 10;
        for (let s = 0; s < per && frame < nFrames; s++, frame++) {
            while (c < nChanges && changes[c * 3] <= frame) {
                const p = changes[c * 3 + 1], rgb = changes[c * 3 + 2];
                img.data[p * 4] = rgb >> 16; img.data[p * 4 + 1] = (rgb >> 8) & 255; img.data[p * 4 + 2] = rgb & 255; img.data[p * 4 + 3] = 255;
                c++;
            }
        }
        ctx.putImageData(img, 0, 0);
        const t = new Date(times[Math.min(frame, nFrames) - 1] ?? times[0]);
        $(".js-meta").textContent = `${t.toLocaleTimeString("fr-FR", {hour: "2-digit", minute: "2-digit"})} · capture ${Math.min(frame, nFrames)} / ${nFrames} · ${formatNumber(c)} pixels changés`;
        if (frame >= nFrames) { replay.done = true; return; }
        replay.raf = setTimeout(step, 80);
    };
    replay = {raf: null, done: false};
    step();
}

function stopReplay() {
    if (!replay) return;
    clearTimeout(replay.raf);
    replay = null;
    $(".js-replay-canvas").hidden = true;
    $(".js-frame").hidden = false;
    const btn = $(".js-replay-day");
    btn.innerHTML = `${icon("clock", 16)}Rejouer cette journée minute par minute`;
    show();
}

function render() {
    const page = $("#page");
    const days = data.days;
    const maxMin = Math.max(...days.map(d => d.minutes));
    const years = {};
    for (const m of data.months) (years[m.month.slice(0, 4)] ??= []).push(m);
    const first = days[0], last = days[days.length - 1];
    const quarters = [0, Math.floor(days.length / 3), Math.floor(days.length * 2 / 3), days.length - 1].map(k => { const [y, m] = days[k].date.split("-"); return `${MONTHS[Number(m) - 1]} ${y}`; });
    page.innerHTML = `<div class="tl-layout">
        <section class="tl-main">
            <div class="tl-head"><h1 class="fs-h1">Timelapse</h1>
                <span class="fs-small t2">${days.length} jours d'activité, du ${dateLong(first.date)} au ${dateLong(last.date)} · une image par jour (la dernière capture de la journée), ${formatNumber(data.frames)} captures en tout</span></div>
            <div class="void tl-stage js-stage" style="aspect-ratio: ${data.width} / ${data.height}; --ratio: ${data.width / data.height}">
                <div class="tl-view js-view">
                    <div class="tl-final"></div>
                    <img class="px tl-frame js-frame" alt="Le canvas ce jour-là" draggable="false">
                    <canvas class="px tl-frame tl-replay js-replay-canvas" hidden></canvas>
                </div>
                <span class="pill tl-grow js-grow">le canvas grandira jusqu'ici · ${data.width} × ${data.height}</span>
                <div class="glass tl-label"><span class="mono b js-date"></span><span class="fs-cap t3 js-meta"></span></div>
                <div class="glass tl-zoom" role="group" aria-label="Zoom">
                    <button class="btn btn-icon btn-sm btn-ghost js-zoom-out" type="button" aria-label="Zoom arrière">${icon("minus", 18)}</button>
                    <button class="btn btn-sm btn-ghost mono js-zoom-reset" type="button" aria-label="Revenir à la vue entière">×1</button>
                    <button class="btn btn-icon btn-sm btn-ghost js-zoom-in" type="button" aria-label="Zoom avant">${icon("plus", 18)}</button>
                </div>
            </div>
            <div class="tl-controls">
                <button class="btn btn-icon btn-primary btn-round js-play" type="button" aria-label="Lecture">${icon("play", 20)}</button>
                <div class="tl-track">
                    <div class="tl-bars js-bars">${days.map(d => `<div style="height: ${Math.max(8, d.minutes / maxMin * 100)}%" title="${dateLong(d.date)} · ${d.minutes} min"></div>`).join("")}</div>
                    <input type="range" class="js-seek" min="0" max="${days.length - 1}" value="0" aria-label="Choisir une date">
                    <div class="fs-cap t3 mono tl-months">${quarters.map(q => `<span>${q}</span>`).join("")}</div>
                </div>
                <div class="seg js-speed" role="group" aria-label="Vitesse">${SPEEDS.map(([id, l]) => `<button type="button" class="${id === speed ? "on" : ""}" data-speed="${id}">${l}</button>`).join("")}</div>
                <a class="btn btn-icon js-download" href="#" download aria-label="Télécharger la capture de ce jour">${icon("download", 18)}</a>
            </div>
            <button class="btn btn-sm js-replay-day tl-replay-btn" type="button">${icon("clock", 16)}Rejouer cette journée minute par minute</button>
        </section>
        <aside class="tl-side">
            <div class="card tl-card"><span class="fs-h3">La vidéo</span><p class="fs-small t2 pretty">Toute l'histoire du canvas en accéléré, générée chaque nuit à partir des sauvegardes.</p>
                <a class="btn btn-primary" href="/timelapse.mp4" download>${icon("download", 18)}Télécharger timelapse.mp4</a></div>
            <div class="tl-box"><div class="sec-head"><span class="fs-h3">Les archives</span><span class="fs-cap t3">un zip de sauvegardes par mois</span></div>
                ${Object.entries(years).map(([y, ms]) => `<div class="tl-year"><span class="mono b">${y}</span><div class="tl-months-list">${ms.map(m => m.days && m.href
                    ? `<a class="chip" href="${m.href}" download>${MONTHS_CAP[Number(m.month.slice(5)) - 1]}<span class="mono t3">${m.days} j</span></a>`
                    : m.days ? `<span class="chip tl-nozip" title="Des captures existent, mais pas encore de zip pour ce mois">${MONTHS_CAP[Number(m.month.slice(5)) - 1]}<span class="mono t3">${m.days} j</span></span>`
                    : `<span class="chip tl-empty" title="Il n'y a pas eu d'activité sur la toile 😢"><del>${MONTHS_CAP[Number(m.month.slice(5)) - 1]}</del></span>`).join("")}</div></div>`).join("")}</div>
            <div class="tl-box"><span class="fs-h3">Les jours les plus actifs</span>
                ${data.top.map((d, k) => `<button class="card-2 tl-top" type="button" data-goto="${d.date}"><span class="mono fs-cap t3">${k + 1}</span><span class="fs-small b grow">${dateLong(d.date)}</span><span class="mono fs-cap t2">${d.minutes} min</span></button>`).join("")}</div>
            <div class="card-2 tl-sizes"><span class="fs-over">Le canvas a grandi</span>
                <span class="fs-small mono">${data.sizes.map(s => `${s.w}×${s.h}`).join(" → ")}</span>
                <span class="fs-cap t3">${data.sizes.map(s => dateLong(s.from)).join(" → ")}</span></div>
        </aside>
    </div>`;
    page.removeAttribute("aria-busy");
    fillIcons(page);
    show();
    setupZoom();
}

// ------------------------------------------------------------------
// Zoom in the capture: wheel, pinch, double-click or the buttons; drag to move. The view
// is resized (not scaled) so the pixels stay sharp.

const view = {s: 1, x: 0, y: 0};
const MAX_ZOOM = 24;

function applyZoom() {
    const stage = $(".js-stage"), el = $(".js-view");
    const W = stage.clientWidth, H = stage.clientHeight;
    view.s = Math.min(MAX_ZOOM, Math.max(1, view.s));
    view.x = Math.min(0, Math.max(W - W * view.s, view.x));
    view.y = Math.min(0, Math.max(H - H * view.s, view.y));
    Object.assign(el.style, {left: view.x + "px", top: view.y + "px", width: W * view.s + "px", height: H * view.s + "px"});
    $(".js-zoom-reset").textContent = "×" + (view.s < 10 ? String(Math.round(view.s * 10) / 10).replace(".", ",") : Math.round(view.s));
    stage.classList.toggle("zoomed", view.s > 1);
}

function zoomAt(f, cx, cy) {
    const s = Math.min(MAX_ZOOM, Math.max(1, view.s * f)), k = s / view.s;
    view.x = cx - (cx - view.x) * k;
    view.y = cy - (cy - view.y) * k;
    view.s = s;
    applyZoom();
}

function setupZoom() {
    const stage = $(".js-stage");
    const local = e => { const r = stage.getBoundingClientRect(); return {x: e.clientX - r.left, y: e.clientY - r.top}; };
    const centre = () => ({x: stage.clientWidth / 2, y: stage.clientHeight / 2});
    stage.addEventListener("wheel", e => {
        e.preventDefault();
        const p = local(e);
        zoomAt(Math.exp(-e.deltaY * (e.ctrlKey ? .01 : .002)), p.x, p.y);
    }, {passive: false});
    stage.addEventListener("dblclick", e => {
        if (e.target.closest("button, .tl-label")) return;
        const p = local(e);
        if (view.s >= MAX_ZOOM) { view.s = 1; applyZoom(); } else zoomAt(2, p.x, p.y);
    });
    const pts = new Map();
    let pinch = null;
    stage.addEventListener("pointerdown", e => {
        if (e.target.closest("button")) return;
        pts.set(e.pointerId, local(e));
        stage.setPointerCapture(e.pointerId);
        if (pts.size === 2) {
            const [a, b] = [...pts.values()];
            pinch = {d: Math.hypot(a.x - b.x, a.y - b.y), cx: (a.x + b.x) / 2, cy: (a.y + b.y) / 2};
        }
    });
    stage.addEventListener("pointermove", e => {
        if (!pts.has(e.pointerId)) return;
        const prev = pts.get(e.pointerId), p = local(e);
        pts.set(e.pointerId, p);
        if (pts.size >= 2 && pinch) {
            const [a, b] = [...pts.values()];
            const d = Math.hypot(a.x - b.x, a.y - b.y), cx = (a.x + b.x) / 2, cy = (a.y + b.y) / 2;
            view.x += cx - pinch.cx;
            view.y += cy - pinch.cy;
            if (pinch.d > 0) zoomAt(d / pinch.d, cx, cy); else applyZoom();
            pinch = {d, cx, cy};
        } else if (view.s > 1) {
            view.x += p.x - prev.x;
            view.y += p.y - prev.y;
            applyZoom();
            stage.classList.add("dragging");
        }
    });
    const up = e => {
        pts.delete(e.pointerId);
        if (pts.size < 2) pinch = null;
        if (!pts.size) stage.classList.remove("dragging");
    };
    stage.addEventListener("pointerup", up);
    stage.addEventListener("pointercancel", up);
    $(".js-zoom-in").addEventListener("click", () => { const c = centre(); zoomAt(2, c.x, c.y); });
    $(".js-zoom-out").addEventListener("click", () => { const c = centre(); zoomAt(.5, c.x, c.y); });
    $(".js-zoom-reset").addEventListener("click", () => { view.s = 1; applyZoom(); });
    window.addEventListener("resize", applyZoom);
    applyZoom();
}

async function main() {
    await mountShell({active: "timelapse"});
    data = await fetch("/api/snapshots", {cache: "no-store"}).then(r => r.json()).catch(() => null);
    if (!data?.days?.length) {
        const s = data?.sauvegardes;
        $(".js-fallback-note").textContent = s?.status === "indexing"
            ? `Les captures du canvas sont en cours d'indexation (${s.total ? Math.floor(s.done / s.total * 100) : 0} %) : reviens dans quelques minutes pour le timelapse jour par jour.`
            : "Le timelapse jour par jour arrivera quand les sauvegardes seront indexées sur ce serveur.";
        $(".js-fallback").hidden = false;
        $("#page").hidden = true;
        return;
    }
    render();
    $("#page").addEventListener("click", e => {
        if (e.target.closest(".js-play")) { stopReplay(); setPlaying(!playing); }
        const s = e.target.closest("[data-speed]");
        if (s) {
            speed = s.dataset.speed;
            for (const b of $$(".js-speed button")) b.classList.toggle("on", b === s);
            if (playing) setPlaying(true);
        }
        const g = e.target.closest("[data-goto]");
        if (g) { stopReplay(); i = data.days.findIndex(d => d.date === g.dataset.goto); show(); }
        if (e.target.closest(".js-replay-day")) replay ? stopReplay() : replayDay();
    });
    $("#page").addEventListener("input", e => {
        if (e.target.classList.contains("js-seek")) { stopReplay(); setPlaying(false); i = Number(e.target.value); show(); }
    });
    document.addEventListener("keydown", e => {
        if (e.target.closest("input, textarea")) return;
        if (e.key === " ") { e.preventDefault(); stopReplay(); setPlaying(!playing); }
        if (e.key === "ArrowRight" && i < data.days.length - 1) { stopReplay(); i++; show(); }
        if (e.key === "ArrowLeft" && i > 0) { stopReplay(); i--; show(); }
    });
}

main();

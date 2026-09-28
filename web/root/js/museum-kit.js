// Shared pieces of the players' museums: themes, frames, particles, artwork images,
// what the guide says. Used by the visit (/u/:pseudo/musee), the editor and the directory.

import {escapeHTML} from "./ui.js";
import {cropOf, authorsOf, dateFR} from "./common.js";
import {formatNumber} from "./view.js";

export const THEMES = {
    maree: {label: "Marée", wall: "linear-gradient(180deg, #0a2340 0%, #103a61 70%, #1c4f7c 100%)", floor: "repeating-linear-gradient(0deg, #2a1a0f 0 12px, #3a2616 12px 14px)", accent: "#5eb3ff", ink: "#f3efe6", sub: "#a9c8e6", waves: true},
    saloon: {label: "Saloon", wall: "repeating-linear-gradient(90deg, #5a3218 0 18px, #6d3d1e 18px 20px)", floor: "repeating-linear-gradient(0deg, #2b1a0e 0 12px, #3f2716 12px 14px)", accent: "#f0b75a", ink: "#f8ecd8", sub: "#e3c79d"},
    galerie: {label: "Galerie", wall: "linear-gradient(180deg, #f4f1ea, #e6e0d3)", floor: "repeating-linear-gradient(0deg, #c9bda5 0 12px, #b8ab91 12px 14px)", accent: "#13793a", ink: "#1c1a16", sub: "#544c40", light: true},
    etoiles: {label: "Étoiles", wall: "linear-gradient(180deg, #120b2a, #26164f)", floor: "repeating-linear-gradient(0deg, #0d0a18 0 12px, #1a1430 12px 14px)", accent: "#e4abff", ink: "#f3efe6", sub: "#c9b8ea"},
    desert: {label: "Désert", wall: "linear-gradient(180deg, #ff9d5c, #ffd623 70%)", floor: "repeating-linear-gradient(0deg, #9c451a 0 12px, #6d302f 12px 14px)", accent: "#6d302f", ink: "#2a1206", sub: "#5c2c10", light: true},
};

// the wall colours offered by the editor (the canvas palette, first 16)
export const WALL_COLORS = [["#000000", "noir"], ["#333434", "gris"], ["#d4d7d9", "gris clair"], ["#6d302f", "marron"], ["#6d001a", "bordeaux"], ["#be0027", "rouge"],
    ["#ffa800", "orange foncé"], ["#fff8b8", "beige"], ["#598d5a", "vert sauge"], ["#00a344", "vert foncé"], ["#004b6f", "bleu sous-marin"], ["#009eaa", "bleu marin"],
    ["#245aea", "bleu ciel"], ["#1832a4", "bleu nuit"], ["#511e9f", "violet"], ["#ff63aa", "rose"]];

export const LIGHTS = {spots: ["Spots", .34], tamise: ["Tamisé", .14], jour: ["Plein jour", 0]};
export const PARTICLES = {aucune: ["Aucune"], bulles: ["Bulles", "part-monte", "#cfe8ff"], etoiles: ["Étoiles", "part-scint", "#fff8b8"],
    sable: ["Sable", "part-tombe", "#ffd623"], neige: ["Neige", "part-tombe", "#ffffff"], confettis: ["Confettis", "part-tombe", "multi"]};
export const FRAMES = {or: ["Or", "cadre-or", "passe"], bois: ["Bois", "cadre", "passe"], pixel: ["Pixel", "cadre-pixel", ""], aucun: ["Aucun", "", ""]};
export const MUSICS = {vagues: ["Vagues 8-bit", "chiptune calme"], saloon: ["Saloon au piano", "ragtime"], desert: ["Désert de nuit", "guitare et vent"], silence: ["Silence", "juste les pas des visiteurs"]};
export const ENTREES = {rideau: "Rideau", camera: "Vol de caméra", fondu: "Fondu pixel"};
export const PARCOURS = {libre: ["Libre", "Les visiteurs avancent à leur rythme (flèches, glisser)."], guide: ["Visite guidée", "Le guide avance seul : 8 s par œuvre, 16 s pour une vedette."]};
export const GUIDES = {cowboy: "Le cowboy", avatar: "Mon avatar", aucun: "Aucun"};
export const VISIBILITES = {tous: "Tout le monde", connectes: "Joueurs connectés", moi: "Moi seul"};
export const ANIMS = {aucune: ["Aucune", "l'œuvre, tout simplement"], construction: ["Rejouer sa construction", "depuis les sauvegardes"], zoom: ["Zoom lent", "la caméra s'approche"], scintille: ["Scintille", "petites étoiles pixel"]};
export const SONS = {aucun: "Aucun", vague: "Vague", clochette: "Clochette", piece: "Pièce 8-bit", mouette: "Mouette"};
export const TAILLES = {petite: ["Petite", 150], moyenne: ["Moyenne", 190], grande: ["Grande", 230]};
export const WORK_FRAMES = {musee: "Comme le musée", or: "Or", bois: "Bois", pixel: "Pixel", aucun: "Aucun"};
export const SPOTS = [["#fff8b8", "beige"], ["#5eb3ff", "bleu evian"], ["#ff63aa", "rose"], ["#7eed38", "vert clair"], ["#ffa800", "orange foncé"], ["#e4abff", "rose clair"]];
const CONFETTIS = ["#ff63aa", "#ffd623", "#5eb3ff", "#7eed38", "#ffa800"];

export function theme(m) {
    return THEMES[m.theme] ?? THEMES.maree;
}

export function wallOf(m) {
    return m.mur ? `linear-gradient(180deg, ${m.mur}, ${m.mur}d0)` : theme(m).wall;
}

export const FOND_TYPES = {theme: "Thème", paysage: "Paysage", capture: "Le canvas", oeuvre: "Une œuvre", image: "Mon image"};
export const PAYSAGES = {desert: "Désert au couchant", nuit: "Nuit étoilée"};

// single quotes: the result goes inside a style="…" attribute
const cssURL = u => `url('${String(u).replace(/["'\\()<>]/g, "")}')`;

// the wall as a style attribute: the theme, or the background picked by the owner
// (a landscape, a capture of the canvas, an artwork, an image), blurred and darkened
export function wallStyle(m) {
    const f = m.fond;
    if (!f?.url) return `background: ${wallOf(m)}`;
    const a = (f.sombre ?? 0) / 100, blur = f.flou ?? 0;
    const size = f.mode === "mosaique" ? "0 0 / auto repeat" : "center / cover no-repeat";
    const pixel = f.type !== "image" ? "image-rendering: pixelated;" : "";
    const edge = blur ? `left: -${blur * 2}px; right: -${blur * 2}px; top: -${blur * 2}px; filter: blur(${blur}px);` : "";
    return `background: linear-gradient(rgba(0,0,0,${a}), rgba(0,0,0,${a})), ${cssURL(f.url)} ${size}; ${pixel}${edge}`;
}

// ink colours readable on a custom wall colour
export function inkOf(m) {
    if (m.fond?.url) return {ink: "#f3efe6", sub: "#d8d0c2", light: false};
    if (!m.mur) return theme(m);
    const n = parseInt(m.mur.slice(1), 16), l = (0.299 * (n >> 16) + 0.587 * (n >> 8 & 255) + 0.114 * (n & 255)) / 255;
    return l > .6 ? {ink: "#1c1a16", sub: "#544c40", light: true} : {ink: "#f3efe6", sub: "#d8d0c2", light: false};
}

export function particlesHTML(kind, count, w, h, seed = 0) {
    const p = PARTICLES[kind];
    if (!p?.[1]) return "";
    let html = "";
    for (let i = 0; i < count; i++) {
        const x = (17 + (i + seed) * 67) % Math.max(40, w), y = (11 + (i + seed) * 41) % Math.max(40, h);
        const c = p[2] === "multi" ? CONFETTIS[i % 5] : p[2];
        html += `<span class="part ${p[1]}" style="left: ${x}px; top: ${y}px; background: ${c}; animation-delay: -${(i * .45).toFixed(2)}s"></span>`;
    }
    return html;
}

export function frameOf(work, m) {
    const f = work.cadre && work.cadre !== "musee" ? work.cadre : m.cadre;
    return FRAMES[f] ?? FRAMES.or;
}

export function workHeight(work, scale = 1) {
    return Math.round((TAILLES[work.taille]?.[1] ?? 190) * scale);
}

// the artwork in its frame; the image is animated later by animateWorks()
export function workFrameHTML(work, m, {scale = 1, cls = ""} = {}) {
    const o = work.oeuvre;
    if (!o) return "";
    const [, frame, passe] = frameOf(work, m);
    const H = workHeight(work, scale);
    const pad = Math.round(Math.max(o.w, o.h) * (1 / .88 - 1) / 2); // as cropOf(o, …, 88)
    const ratio = (o.w + 2 * pad) / (o.h + 2 * pad);
    const W = Math.max(60, Math.round(H * ratio));
    const box = `left: ${pad / (o.w + 2 * pad) * 100}%; top: ${pad / (o.h + 2 * pad) * 100}%; width: ${o.w / (o.w + 2 * pad) * 100}%; height: ${o.h / (o.h + 2 * pad) * 100}%`;
    const img = `<div class="mp-img clip" style="width: ${W}px; height: ${H}px" data-anim="${work.animation}" data-oeuvre="${o.id}" data-ratio="${o.w / o.h}" data-box="${box}">
        <img class="px" src="${cropOf(o, Math.max(W, H) * 2, 88)}" alt="${escapeHTML(o.titre)}" loading="lazy" style="width: 100%; height: 100%; object-fit: contain">
        ${work.animation === "scintille" ? particlesHTML("etoiles", 7, W, H, o.id) : ""}</div>`;
    return `<div class="mp-frame ${frame} ${cls}">${passe ? `<div class="${passe}">${img}</div>` : img}</div>`;
}

export function spotStyle(work, m) {
    const a = LIGHTS[m.eclairage]?.[1] ?? .3;
    if (!a) return "";
    const c = work.spot || "#fff8b8";
    const n = parseInt(c.slice(1), 16);
    return `background: radial-gradient(ellipse 50% 60% at 50% 0%, rgba(${n >> 16}, ${n >> 8 & 255}, ${n & 255}, ${a}), transparent 72%)`;
}

export function cartelHTML(work) {
    const o = work.oeuvre;
    const year = o.apparue_le ? new Date(o.apparue_le).getFullYear() : new Date(o.creee_le).getFullYear();
    return `<div class="cartel mp-cartel"><span class="pixel b">${escapeHTML(o.titre)}</span><span class="fs-cap">${escapeHTML(authorsOf(o))} · ${year}</span></div>`;
}

// « Rejouer sa construction »: frames from the backups, then the still image if there are none
const sprites = new Map();
export function animateWorks(root) {
    const timers = [];
    for (const el of root.querySelectorAll('.mp-img[data-anim="construction"]')) {
        const id = el.dataset.oeuvre;
        const url = `/api/oeuvres/${id}/construction.png?n=12&z=2`;
        let p = sprites.get(url);
        if (!p) {
            p = new Promise(res => { const im = new Image(); im.onload = () => res(im); im.onerror = () => res(null); im.src = url; });
            sprites.set(url, p);
        }
        p.then(im => {
            if (!im || !el.isConnected) return;
            // fewer frames than asked when the zone changed only a few times
            const n = Math.max(1, Math.round(im.width / (im.height * Number(el.dataset.ratio || 1))));
            if (n < 2) return;
            const box = document.createElement("div");
            box.className = "mp-build";
            box.style.cssText = el.dataset.box;
            box.style.backgroundImage = `url("${url}")`;
            box.style.backgroundSize = `${n * 100}% 100%`;
            el.append(box);
            let i = 0;
            timers.push(setInterval(() => { i = (i + 1) % (n + 3); box.style.backgroundPosition = `${Math.min(i, n - 1) / (n - 1) * 100}% 0`; }, 550));
        });
    }
    for (const el of root.querySelectorAll('.mp-img[data-anim="zoom"] img')) el.classList.add("anim-zoomlent");
    return () => timers.forEach(clearInterval);
}

// what the pixel guide says in front of an artwork
export function guideLine(work, i) {
    const o = work.oeuvre;
    const by = authorsOf(o);
    const lines = [];
    if (work.vedette) lines.push(`Voici la vedette de la salle : « ${o.titre} ». Prends ton temps !`);
    if (o.apparue_le) lines.push(`« ${o.titre} » est apparue sur le canvas le ${dateFR(o.apparue_le)}.`);
    if (o.pixels) lines.push(`${formatNumber(o.pixels)} pixels, posés un par un${by ? ` par ${by}` : ""}.`);
    if (o.extra?.likes) lines.push(`${o.extra.likes} coup${o.extra.likes > 1 ? "s" : ""} de cœur pour celle-ci. Ça se comprend !`);
    if (work.voix) lines.push("L'artiste a enregistré un petit mot : écoute-le !");
    if (work.animation === "construction") lines.push("Regarde bien : elle se redessine comme le jour où elle est née.");
    if (!lines.length) lines.push(`« ${o.titre} »${by ? `, par ${by}` : ""}.`);
    return lines[i % lines.length];
}

export function allWorks(m) {
    const out = [];
    m.salles.forEach((s, si) => s.oeuvres.forEach((w, wi) => out.push({salle: si, index: wi, work: w})));
    return out;
}

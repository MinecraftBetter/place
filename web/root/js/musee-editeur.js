// /moi/musee — the museum editor (d-musee-editeur, m-musee-editeur, m-musee-oeuvre-perso):
// settings on the left, live preview in the middle, the selected artwork on the right.
// Every change is saved as a draft after a moment; « Ouvrir mon musée » publishes it.

import {$, $$, avatarHTML, copyText, escapeHTML, fillIcons, icon, toast} from "./ui.js";
import {mountShell, loginHref, emptyState} from "./shell.js";
import {relativeTime} from "./view.js";
import {artOf, cropOf, authorsOf} from "./common.js";
import {Chiptune} from "./chiptune.js";
import {THEMES, WALL_COLORS, LIGHTS, PARTICLES, FRAMES, MUSICS, ENTREES, PARCOURS, GUIDES, VISIBILITES, ANIMS, SONS, TAILLES, WORK_FRAMES, SPOTS,
    wallStyle, theme, inkOf, particlesHTML, workFrameHTML, spotStyle, cartelHTML, animateWorks, FOND_TYPES, PAYSAGES} from "./museum-kit.js";

const page = $("#page");
const audio = new Chiptune();
const TABS = [["ambiance", "Ambiance"], ["son", "Son"], ["parcours", "Parcours"], ["accueil", "Accueil"]];
let me = null, data = null, m = null, publie = false, tab = "ambiance", salle = 0, sel = 0, saveTimer = 0, savedAt = 0, saving = false;
let stopAnims = () => {}, preview = null, recorder = null;
let snap = null, fondTimer = 0; // /api/snapshots: every capture of the canvas, for the wall

const oeuvres = new Map(); // id → artwork (mine + favourites + already hung)
const works = () => m.salles[salle]?.oeuvres ?? [];
const selected = () => works()[sel] ?? null;
const sonsOf = type => data.sons.filter(s => s.type === type);

async function main() {
    me = (await mountShell({active: "musee"})).user;
    if (!me) {
        page.innerHTML = `<div class="me-wrap">${emptyState({title: "Ton musée t'attend", text: "Connecte-toi pour ouvrir ton musée.", mascot: true, action: `<a class="btn btn-primary btn-px" href="${loginHref()}">Se connecter</a>`})}</div>`;
        page.removeAttribute("aria-busy");
        return;
    }
    const r = await fetch("/api/me/musee", {cache: "no-store"});
    data = await r.json();
    m = data.musee;
    publie = data.publie;
    for (const o of [...data.miennes, ...data.coeurs]) oeuvres.set(o.id, o);
    for (const s of m.salles) for (const w of s.oeuvres) if (w.oeuvre) oeuvres.set(w.oeuvre.id, w.oeuvre);
    const want = Number(location.pathname.split("/")[4]);
    if (want) m.salles.forEach((s, si) => s.oeuvres.forEach((w, wi) => { if (w.oeuvre_id === want) { salle = si; sel = wi; } }));
    if (m.fond?.type === "capture") await ensureSnap();
    render();
    if (want) $(".js-work-panel")?.scrollIntoView({block: "start"});
}

// ------------------------------------------------------------------
// Rendering

function render() {
    page.innerHTML = `<div class="me-wrap">
        <header class="me-head">
            <div class="grow" style="min-width: 0"><a class="fs-small t2" href="/musees">${icon("back", 14)} Musées des joueurs</a>
                <h1 class="fs-h1 ellip js-title">${escapeHTML(m.nom)}</h1><span class="fs-small t3 js-status"></span></div>
            <div class="me-head-actions">
                <a class="btn" href="/u/${escapeHTML(me.slug)}/musee" target="_blank" rel="noopener">${icon("eye", 18)}Voir comme un visiteur</a>
                <button class="btn ${publie ? "" : "btn-gold btn-px"} js-publish" type="button">${publie ? `${icon("check", 18)}Ouvert · fermer` : "Ouvrir mon musée"}</button>
            </div>
        </header>
        <div class="me-grid">
            <section class="card me-settings" aria-label="Réglages">
                <div class="seg me-tabs" role="tablist">${TABS.map(([id, l]) => `<button type="button" role="tab" data-tab="${id}" aria-selected="${id === tab}" class="${id === tab ? "on" : ""}">${l}</button>`).join("")}</div>
                <div class="me-tab js-tab"></div>
            </section>
            <section class="me-center">
                <div class="me-preview js-preview" aria-label="Aperçu en direct"></div>
                <div class="card me-rooms">
                    <div class="me-rooms-head"><span class="fs-over">Salles</span><div class="me-chips js-salles"></div></div>
                    <div class="js-salle-edit"></div>
                    <span class="fs-over">Ordre</span>
                    <div class="me-order js-order"></div>
                </div>
            </section>
            <section class="card me-work js-work-panel" aria-label="Œuvre sélectionnée"></section>
        </div>
    </div>
    <dialog class="card elev me-picker js-picker"></dialog>`;
    page.removeAttribute("aria-busy");
    renderTab();
    renderPreview();
    renderRooms();
    renderWork();
    renderStatus();
    fillIcons(page);
    wire();
}

function opt(group, value, label, cur, extra = "") {
    return `<button type="button" class="${value === cur ? "on" : ""}" data-set="${group}" data-value="${escapeHTML(value)}" aria-pressed="${value === cur}"${extra}>${label}</button>`;
}

function toggleRow(key, label) {
    return `<div class="edit-toggle"><span class="fs-small grow">${label}</span><button class="toggle${m[key] ? " toggle-on" : ""}" type="button" data-bool="${key}" aria-pressed="${!!m[key]}" aria-label="${label}"></button></div>`;
}

function renderTab() {
    const el = $(".js-tab");
    let html = "";
    if (tab === "ambiance") {
        html = `<div class="field"><span class="label">Thème</span><div class="me-themes">${Object.entries(THEMES).map(([id, t]) =>
                `<button type="button" class="me-theme${id === m.theme && !m.mur ? " on" : id === m.theme ? " on-soft" : ""}" data-set="theme" data-value="${id}" aria-pressed="${id === m.theme}">
                    <span class="me-theme-sw" style="--wall: ${t.wall}; --floor: ${t.floor}"></span><span class="fs-cap b">${t.label}</span></button>`).join("")}</div></div>
            ${fondHTML()}
            <div class="field"${m.fond ? " hidden" : ""}><span class="label">Couleur des murs</span><div class="me-swatches">
                <button type="button" class="chip${!m.mur ? " chip-on" : ""}" data-set="mur" data-value="">Couleur du thème</button>
                ${WALL_COLORS.map(([hex, name]) => `<button type="button" class="swatch swatch-s${m.mur === hex ? " swatch-sel" : ""}" style="background: ${hex}" data-set="mur" data-value="${hex}" aria-label="${name}" title="${name}"></button>`).join("")}</div></div>
            <div class="field"><span class="label">Éclairage</span><div class="seg">${Object.entries(LIGHTS).map(([id, [l]]) => opt("eclairage", id, l, m.eclairage)).join("")}</div></div>
            <div class="field"><span class="label">Particules</span><div class="me-chips">${Object.entries(PARTICLES).map(([id, [l]]) => `<button type="button" class="chip${id === m.particules ? " chip-on" : ""}" data-set="particules" data-value="${id}">${l}</button>`).join("")}</div></div>
            <div class="field"><span class="label">Cadres</span><div class="seg">${Object.entries(FRAMES).map(([id, [l]]) => opt("cadre", id, l, m.cadre)).join("")}</div></div>`;
    } else if (tab === "son") {
        const mine = sonsOf("musique");
        html = `<div class="field"><span class="label">Musique d'ambiance</span><div class="me-musics">
                ${Object.entries(MUSICS).map(([id, [l, sub]]) => musicRow(id, l, sub)).join("")}
                ${mine.map(s => musicRow("son:" + s.id, s.nom, s.statut === "attente" ? "en attente de l'équipe" : "ton son", s)).join("")}</div>
                <label class="btn btn-sm btn-ghost me-import">${icon("upload", 16)}Importer un son (vérifié par l'équipe)<input type="file" accept="audio/*" class="sr-only js-import" data-type="musique"></label></div>
            <div class="field"><div class="sec-head"><label class="label" for="vol">Volume de la musique</label><span class="mono b js-vol">${m.volume} %</span></div>
                <input id="vol" type="range" min="0" max="100" step="5" value="${m.volume}" data-num="volume"></div>
            <div class="card-2 edit-toggles">${toggleRow("voix_auto", "Voix des artistes en lecture auto")}${toggleRow("sons_passage", "Sons au passage des œuvres")}${toggleRow("pas", "Bruit de pas sur le parquet")}</div>`;
    } else if (tab === "parcours") {
        html = `<div class="field"><span class="label">Animation d'entrée</span><div class="seg">${Object.entries(ENTREES).map(([id, l]) => opt("entree", id, l, m.entree)).join("")}</div></div>
            <div class="field"><span class="label">Parcours</span><div class="seg">${Object.entries(PARCOURS).map(([id, [l]]) => opt("parcours", id, l, m.parcours)).join("")}</div>
                <span class="hint">${PARCOURS[m.parcours][1]}</span></div>
            <div class="field"><span class="label">Le guide</span><div class="me-guides">${Object.entries(GUIDES).map(([id, l]) => `<button type="button" class="card-2 me-guide${id === m.guide ? " on" : ""}" data-set="guide" data-value="${id}" aria-pressed="${id === m.guide}">
                ${id === "cowboy" ? `<img class="px" src="/img/farwest-x8.png" alt="" width="26" height="45">` : id === "avatar" ? avatarHTML(me, "av av-40") : `<span class="me-guide-none">${icon("close", 18)}</span>`}<span class="fs-cap b">${l}</span></button>`).join("")}</div></div>
            <div class="field"><label class="label" for="phrase">Ce que dit le guide à l'entrée</label><input id="phrase" class="input" maxlength="120" data-text="phrase_guide" value="${escapeHTML(m.phrase_guide)}"></div>`;
    } else {
        const link = `${location.host}/u/${me.slug}/musee`;
        html = `<div class="field"><label class="label" for="nom">Nom du musée</label><input id="nom" class="input" maxlength="40" data-text="nom" value="${escapeHTML(m.nom)}"></div>
            <div class="field"><label class="label" for="accueil">Mot d'accueil</label><textarea id="accueil" class="textarea" maxlength="160" data-text="accueil" placeholder="Des vagues, un labyrinthe, et mes coups de cœur. Mettez le son !">${escapeHTML(m.accueil)}</textarea><span class="hint js-count-accueil">${m.accueil.length} / 160</span></div>
            <div class="field"><span class="label">Qui peut visiter</span><div class="seg">${Object.entries(VISIBILITES).map(([id, l]) => opt("visibilite", id, l, m.visibilite)).join("")}</div></div>
            <div class="card-2 edit-toggles">${toggleRow("livre_or", "Livre d'or ouvert")}</div>
            <div class="field"><span class="label">Lien du musée</span><div class="me-link"><span class="mono fs-small ellip">${escapeHTML(link)}</span><button class="btn btn-sm btn-ghost js-copy" type="button">${icon("copy", 16)}Copier</button></div></div>`;
    }
    el.innerHTML = html;
    fillIcons(el);
}

function musicRow(id, label, sub, son) {
    const on = m.musique === id;
    return `<div class="me-music${on ? " on" : ""}"><button type="button" class="me-music-pick" data-set="musique" data-value="${escapeHTML(id)}" aria-pressed="${on}">
        <span class="me-radio"></span><span class="grow"><span class="fs-small b">${escapeHTML(label)}</span><br><span class="fs-cap t3">${escapeHTML(sub)}</span></span></button>
        ${id !== "silence" ? `<button type="button" class="btn btn-icon btn-sm btn-ghost" data-listen="${escapeHTML(id)}" aria-label="Écouter ${escapeHTML(label)}">${icon("play", 16)}</button>` : ""}
        ${son ? `<button type="button" class="btn btn-icon btn-sm btn-ghost" data-del-son="${son.id}" aria-label="Supprimer ${escapeHTML(label)}">${icon("trash", 16)}</button>` : ""}</div>`;
}

// ------------------------------------------------------------------
// The wall's background: theme, landscape, any capture of the canvas, artwork, image

async function ensureSnap() {
    if (!snap) snap = await fetch("/api/snapshots", {cache: "no-store"}).then(r => r.json()).catch(() => ({days: [], frames: 0}));
    return snap;
}

// the day of a capture, and its place in that day
function frameInfo(k) {
    const days = snap?.days ?? [];
    let lo = 0, hi = days.length - 1;
    while (lo < hi) { const mid = (lo + hi) >> 1; if (days[mid].frame >= k) hi = mid; else lo = mid + 1; }
    const d = days[lo];
    if (!d) return "";
    const [y, mo, dd] = d.date.split("-").map(Number);
    const months = ["janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"];
    const n = k - (d.frame - d.minutes);
    return `${dd} ${months[mo - 1]} ${y} · capture ${n} sur ${d.minutes} ce jour-là`;
}

function fondURL(f) {
    if (!f) return "";
    if (f.type === "paysage") return `/img/${f.value}.png`;
    if (f.type === "capture") return `/api/snapshots/${f.frame}.png`;
    if (f.type === "oeuvre") { const o = oeuvres.get(Number(f.value)); return o ? cropOf(o, 480, 90) : ""; }
    if (f.type === "image") return f.value ?? "";
    return "";
}

function fondHTML() {
    const f = m.fond, type = f?.type ?? "theme";
    let sub = "";
    if (type === "paysage") {
        sub = `<div class="me-paysages">${Object.entries(PAYSAGES).map(([id, l]) => `<button type="button" class="me-paysage${f.value === id ? " on" : ""}" data-fond-paysage="${id}"><img src="/img/${id}.png" alt=""><span>${l}</span></button>`).join("")}</div>`;
    } else if (type === "capture") {
        const total = snap?.frames ?? 0;
        sub = `<div class="me-fond-prev"><img class="js-fond-img" src="${fondURL(f)}" alt="La capture choisie"><span class="pill mono js-fond-label">${escapeHTML(frameInfo(f.frame))}</span></div>
            <div class="me-fond-row"><button type="button" class="btn btn-icon btn-sm btn-ghost" data-fond-step="-1" aria-label="Capture précédente">${icon("prev", 16)}</button>
                <input type="range" class="js-fond-frame" min="0" max="${Math.max(0, total - 1)}" value="${f.frame}" aria-label="Capture du canvas">
                <button type="button" class="btn btn-icon btn-sm btn-ghost" data-fond-step="1" aria-label="Capture suivante">${icon("next", 16)}</button></div>
            <div class="me-chips"><button type="button" class="chip" data-fond-jump="first">Le tout premier</button><button type="button" class="chip" data-fond-jump="last">Aujourd'hui</button><button type="button" class="chip" data-fond-jump="random">Au hasard</button></div>
            <span class="hint">Les ${total.toLocaleString("fr-FR")} captures du canvas, une par minute d'activité depuis septembre 2022.</span>`;
    } else if (type === "oeuvre") {
        const list = [...oeuvres.values()];
        sub = list.length ? `<div class="me-fond-works">${list.map(o => `<button type="button" class="me-fond-work${String(o.id) === f.value ? " on" : ""}" data-fond-oeuvre="${o.id}" aria-label="${escapeHTML(o.titre)}" title="${escapeHTML(o.titre)}"><img class="px" src="${cropOf(o, 96, 90)}" alt="" loading="lazy"></button>`).join("")}</div>`
            : `<span class="fs-small t2">Pas encore d'œuvre : revendiques-en une ou donne des coups de cœur.</span>`;
    } else if (type === "image") {
        sub = `${f.value ? `<div class="me-fond-prev"><img src="${escapeHTML(f.value)}" alt="Ton image" style="object-fit: cover; image-rendering: auto"></div>` : ""}
            <label class="btn btn-sm me-import">${icon("upload", 16)}${f.value ? "Changer d'image" : "Importer une image"} (PNG, JPEG ou GIF, 8 Mo)<input type="file" accept="image/png,image/jpeg,image/gif" class="sr-only js-fond-file"></label>
            <span class="hint">L'équipe peut la vérifier avant qu'elle soit visible.</span>`;
    }
    const effects = type === "theme" ? "" : `
        <div class="me-fond-row"><span class="fs-cap t3" style="width: 72px">Flou</span><input type="range" min="0" max="12" value="${f.flou ?? 0}" data-fond-num="flou" aria-label="Flou"><span class="mono fs-cap js-fond-flou">${f.flou ?? 0} px</span></div>
        <div class="me-fond-row"><span class="fs-cap t3" style="width: 72px">Assombrir</span><input type="range" min="0" max="85" step="5" value="${f.sombre ?? 0}" data-fond-num="sombre" aria-label="Assombrir"><span class="mono fs-cap js-fond-sombre">${f.sombre ?? 0} %</span></div>
        <div class="seg"><button type="button" class="${f.mode !== "mosaique" ? "on" : ""}" data-fond-mode="remplir">Remplir le mur</button><button type="button" class="${f.mode === "mosaique" ? "on" : ""}" data-fond-mode="mosaique">Mosaïque</button></div>`;
    return `<div class="field me-fond"><span class="label">Fond du mur</span>
        <div class="seg me-seg-wrap" role="group" aria-label="Fond du mur">${Object.entries(FOND_TYPES).map(([id, l]) => `<button type="button" class="${id === type ? "on" : ""}" data-fond-type="${id}">${l}</button>`).join("")}</div>
        ${sub}${effects}</div>`;
}

function fondChanged({tab: tb = false} = {}) {
    if (m.fond) m.fond.url = fondURL(m.fond);
    if (tb) renderTab();
    clearTimeout(fondTimer);
    fondTimer = setTimeout(() => changed(), 180);
}

async function setFondType(type) {
    const keep = m.fond ? {flou: m.fond.flou ?? 0, sombre: m.fond.sombre ?? 30, mode: m.fond.mode ?? "remplir"} : {flou: 0, sombre: 30, mode: "remplir"};
    if (type === "theme") m.fond = null;
    else if (type === "paysage") m.fond = {type, value: "desert", ...keep, sombre: m.fond ? keep.sombre : 10};
    else if (type === "capture") { await ensureSnap(); m.fond = {type, frame: Math.max(0, (snap.frames ?? 1) - 1), ...keep}; }
    else if (type === "oeuvre") { const o = [...oeuvres.values()][0]; m.fond = o ? {type, value: String(o.id), ...keep} : {type, ...keep}; }
    else if (type === "image") m.fond = {type, ...keep, value: m.fond?.type === "image" ? m.fond.value : ""};
    fondChanged({tab: true});
}

async function uploadFond(input) {
    const file = input.files[0];
    input.value = "";
    if (!file) return;
    const fd = new FormData();
    fd.append("fichier", file);
    const r = await fetch("/api/me/musee/fond", {method: "POST", body: fd});
    const j = await r.json().catch(() => ({}));
    if (!r.ok) { toast(`<span class="fs-small b">${escapeHTML(j.error ?? "Import impossible")}</span>`); return; }
    m.fond = {type: "image", value: j.url, flou: m.fond?.flou ?? 0, sombre: m.fond?.sombre ?? 20, mode: m.fond?.mode ?? "remplir"};
    fondChanged({tab: true});
    toast(`<span class="fs-small b">Image importée</span>`);
}

function renderPreview() {
    stopAnims();
    const el = $(".js-preview"), t = theme(m), ink = inkOf(m);
    const guide = m.guide === "cowboy" ? `<img class="px anim-bob" src="/img/farwest-x8.png" alt="" width="26" height="45">` : m.guide === "avatar" ? avatarHTML(me, "av av-32 anim-bob") : "";
    const ws = works();
    el.style.setProperty("--mp-ink", ink.ink);
    el.style.setProperty("--mp-sub", ink.sub);
    el.innerHTML = `<div class="me-pv-wall" style="${wallStyle(m)}"></div><div class="me-pv-floor" style="background: ${t.floor}"></div>
        <div class="mp-parts">${particlesHTML(m.particules, 16, 640, 280)}</div>
        <span class="pill me-pv-tag">aperçu en direct</span>
        <div class="me-pv-works">${ws.length ? ws.map((w, i) => w.oeuvre ? `<button type="button" class="me-pv-work${i === sel ? " on" : ""}" data-sel="${i}" aria-label="${escapeHTML(w.oeuvre.titre)}">
                <div class="mp-spot" style="${spotStyle(w, m)}"></div>${workFrameHTML(w, m, {scale: .55})}${cartelHTML(w)}</button>` : "").join("")
            : `<div class="cartel me-pv-empty"><span class="pixel b">Salle vide</span><span class="fs-cap">Ajoute une œuvre avec « + Œuvre ».</span></div>`}</div>
        ${guide ? `<div class="me-pv-guide">${guide}<div class="bulle">${escapeHTML(m.phrase_guide || "Howdy !")}</div></div>` : ""}
        <div class="glass me-pv-bar fs-cap">${icon("music", 14)}<span class="ellip">${escapeHTML(musicLabel())}</span><span class="grow"></span><span>Entrée : ${ENTREES[m.entree].toLowerCase()} · ${PARCOURS[m.parcours][0].toLowerCase()}</span></div>`;
    fillIcons(el);
    stopAnims = animateWorks(el);
}

function musicLabel() {
    if (m.musique.startsWith("son:")) return data.sons.find(s => "son:" + s.id === m.musique)?.nom ?? "Musique perso";
    return MUSICS[m.musique]?.[0] ?? "Silence";
}

function renderRooms() {
    $(".js-salles").innerHTML = m.salles.map((s, i) => `<button type="button" class="chip${i === salle ? " chip-on" : ""}" data-salle="${i}">${escapeHTML(s.titre)} <span class="mono" style="opacity: .7">${s.oeuvres.length}</span></button>`).join("")
        + (m.salles.length < 6 ? `<button type="button" class="chip js-add-salle">${icon("plus", 14)}Salle</button>` : "");
    $(".js-salle-edit").innerHTML = `<div class="me-salle-edit"><input class="input" maxlength="32" aria-label="Nom de la salle" data-salle-title value="${escapeHTML(m.salles[salle].titre)}">
        ${m.salles.length > 1 ? `<button type="button" class="btn btn-sm btn-ghost js-del-salle">${icon("trash", 16)}Supprimer la salle</button>` : ""}</div>`;
    const ws = works();
    $(".js-order").innerHTML = ws.map((w, i) => w.oeuvre ? `<div class="me-order-item${i === sel ? " on" : ""}">
            <button type="button" class="me-order-thumb" data-sel="${i}" aria-label="Choisir ${escapeHTML(w.oeuvre.titre)}"><img class="px art" src="${artOf(w.oeuvre, 96)}" alt="" loading="lazy">${w.vedette ? `<span class="me-star">${icon("star", 12)}</span>` : ""}</button>
            <span class="fs-cap ellip">${escapeHTML(w.oeuvre.titre)}</span>
            <div class="me-order-move"><button type="button" class="btn btn-icon btn-sm btn-ghost" data-move="${i}:-1" aria-label="Avancer" ${i === 0 ? "disabled" : ""}>${icon("prev", 14)}</button>
                <button type="button" class="btn btn-icon btn-sm btn-ghost" data-move="${i}:1" aria-label="Reculer" ${i === ws.length - 1 ? "disabled" : ""}>${icon("next", 14)}</button></div></div>` : "").join("")
        + (ws.length < 12 ? `<button type="button" class="me-order-add js-add-work">${icon("plus", 20)}<span class="fs-cap b">Œuvre</span></button>` : "");
    fillIcons($(".js-salles").parentElement.parentElement);
}

function renderWork() {
    const el = $(".js-work-panel");
    const w = selected();
    if (!w?.oeuvre) {
        el.innerHTML = `<span class="fs-over">Œuvre sélectionnée</span><p class="fs-small t2 pretty">Choisis une œuvre dans l'ordre de la salle, ou ajoute-en une avec « + Œuvre ». Tu peux exposer tes œuvres et tes coups de cœur.</p>`;
        return;
    }
    const o = w.oeuvre;
    const voices = sonsOf("voix"), passages = sonsOf("passage");
    const voix = data.sons.find(s => s.id === w.voix_id);
    el.innerHTML = `<div class="me-work-head"><img class="px art" src="${artOf(o, 96)}" alt="" width="56" height="56"><div class="grow" style="min-width: 0"><span class="fs-over">Œuvre sélectionnée</span>
            <span class="fs-body b ellip">${escapeHTML(o.titre)}</span><span class="fs-cap t3 ellip">${escapeHTML(authorsOf(o))}</span></div></div>
        <div class="field"><span class="label">Animation</span><div class="me-anims">${Object.entries(ANIMS).map(([id, [l, sub]]) =>
            `<button type="button" class="card-2 me-anim${id === w.animation ? " on" : ""}" data-wset="animation" data-value="${id}" aria-pressed="${id === w.animation}"><span class="fs-small b">${l}</span><span class="fs-cap t3">${sub}</span></button>`).join("")}</div></div>
        <div class="field"><span class="label">Commentaire vocal</span>
            ${voix ? `<div class="card-2 me-voice"><button type="button" class="btn btn-icon btn-sm btn-round" data-play-url="${escapeHTML(voix.url)}" aria-label="Écouter">${icon("play", 14)}</button>
                <span class="fs-small grow">${escapeHTML(voix.nom)} · ${Math.round(voix.duree)} s${voix.statut === "attente" ? ` <span class="pill st-attente">en attente de l'équipe</span>` : ""}</span>
                <button type="button" class="btn btn-icon btn-sm btn-ghost js-voice-off" aria-label="Retirer le commentaire">${icon("close", 14)}</button></div>` : ""}
            <div class="me-voice-actions"><button type="button" class="btn btn-sm js-record">${recorder ? `${icon("pause", 16)}Arrêter · <span class="mono js-rec-t">0 s</span>` : `<span class="me-rec-dot"></span>Enregistrer (30 s max)`}</button>
                <label class="btn btn-sm btn-ghost">${icon("upload", 16)}Importer<input type="file" accept="audio/*" class="sr-only js-import" data-type="voix"></label>
                ${voices.length ? `<select class="select me-select js-voice-pick" aria-label="Mes commentaires"><option value="">Mes enregistrements…</option>${voices.map(s => `<option value="${s.id}"${s.id === w.voix_id ? " selected" : ""}>${escapeHTML(s.nom)} · ${Math.round(s.duree)} s</option>`).join("")}</select>` : ""}</div>
            <span class="hint">Joué quand un visiteur s'arrête devant l'œuvre. Les sons sont écoutés par l'équipe avant d'être publics.</span></div>
        <div class="field"><label class="label" for="texte">Texte du cartel</label><textarea id="texte" class="textarea" maxlength="200" data-wtext="texte" placeholder="Ma toute première œuvre…">${escapeHTML(w.texte)}</textarea></div>
        <div class="field"><span class="label">Son au passage</span><div class="me-chips">${Object.entries(SONS).map(([id, l]) => `<button type="button" class="chip${w.son === id ? " chip-on" : ""}" data-wset="son" data-value="${id}">${l}</button>`).join("")}
            ${passages.map(s => `<button type="button" class="chip${w.son === "son:" + s.id ? " chip-on" : ""}" data-wset="son" data-value="son:${s.id}">${escapeHTML(s.nom)}${s.statut === "attente" ? " ⏳" : ""}</button>`).join("")}
            <label class="chip me-import-chip">${icon("upload", 14)}Importer<input type="file" accept="audio/*" class="sr-only js-import" data-type="passage"></label></div></div>
        <div class="field"><span class="label">Taille au mur</span><div class="seg">${Object.entries(TAILLES).map(([id, [l]]) => wopt("taille", id, l, w.taille)).join("")}</div></div>
        <div class="field"><span class="label">Cadre</span><div class="seg me-seg-wrap">${Object.entries(WORK_FRAMES).map(([id, l]) => wopt("cadre", id, l, w.cadre)).join("")}</div></div>
        <div class="field"><span class="label">Couleur du spot</span><div class="me-swatches">${SPOTS.map(([hex, name]) => `<button type="button" class="swatch swatch-s${w.spot === hex ? " swatch-sel" : ""}" style="background: ${hex}" data-wset="spot" data-value="${hex}" aria-label="${name}" title="${name}"></button>`).join("")}</div></div>
        <div class="card-2 edit-toggles"><div class="edit-toggle"><span class="grow"><span class="fs-small b">Œuvre vedette</span><br><span class="fs-cap t3">seule sous un spot, la visite guidée s'y arrête plus longtemps</span></span>
            <button class="toggle${w.vedette ? " toggle-on" : ""}" type="button" data-wbool="vedette" aria-pressed="${!!w.vedette}" aria-label="Œuvre vedette"></button></div></div>
        <button type="button" class="btn btn-sm btn-outline-danger js-remove">${icon("trash", 16)}Retirer du musée</button>`;
    fillIcons(el);
}

function wopt(key, value, label, cur) {
    return `<button type="button" class="${value === cur ? "on" : ""}" data-wset="${key}" data-value="${value}" aria-pressed="${value === cur}">${label}</button>`;
}

function renderStatus() {
    const s = $(".js-status");
    if (!s) return;
    const n = m.salles.reduce((a, x) => a + x.oeuvres.length, 0);
    const state = saving ? "Enregistrement…" : savedAt ? `${publie ? "Enregistré" : "Brouillon enregistré"} ${relativeTime(savedAt)}` : publie ? "Ouvert aux visiteurs" : data.existe ? "Brouillon" : "Nouveau musée";
    s.textContent = `${state} · ${m.salles.length} salle${m.salles.length > 1 ? "s" : ""} · ${n} œuvre${n > 1 ? "s" : ""}`;
}
setInterval(renderStatus, 10000);

// ------------------------------------------------------------------
// Saving

function changed({preview: pv = true, rooms = false, work = false, tab: tb = false} = {}) {
    if (pv) renderPreview();
    if (rooms) renderRooms();
    if (work) renderWork();
    if (tb) renderTab();
    clearTimeout(saveTimer);
    saveTimer = setTimeout(save, 1200);
    renderStatus();
}

function payload() {
    return {...m, salles: m.salles.map(s => ({titre: s.titre, oeuvres: s.oeuvres.map(({oeuvre, voix, son_fichier, ...w}) => w)}))};
}

async function save() {
    clearTimeout(saveTimer);
    saveTimer = 0;
    saving = true;
    renderStatus();
    try {
        const r = await fetch("/api/me/musee", {method: "PUT", headers: {"Content-Type": "application/json"}, body: JSON.stringify({musee: payload(), publie})});
        const j = await r.json().catch(() => ({}));
        if (!r.ok) throw new Error(j.error ?? "Enregistrement impossible");
        savedAt = Date.now();
        data.existe = true;
    } catch (err) {
        toast(`<span class="fs-small b">${escapeHTML(err.message)}</span>`);
    }
    saving = false;
    renderStatus();
}

// ------------------------------------------------------------------
// Sounds: upload, record, listen

async function upload(blob, type, duree, nom) {
    const fd = new FormData();
    fd.append("type", type);
    fd.append("duree", String(Math.round(duree * 10) / 10));
    if (nom) fd.append("nom", nom);
    fd.append("fichier", blob, type + (blob.type.includes("ogg") ? ".ogg" : blob.type.includes("mpeg") ? ".mp3" : ".webm"));
    const r = await fetch("/api/me/musee/sons", {method: "POST", body: fd});
    const j = await r.json().catch(() => ({}));
    if (!r.ok) { toast(`<span class="fs-small b">${escapeHTML(j.error ?? "Import impossible")}</span>`); return null; }
    data.sons.unshift(j.son);
    toast(`<span class="fs-small b">${j.son.statut === "attente" ? "Son importé : l'équipe l'écoute avant qu'il soit public" : "Son importé"}</span>`, {timeout: 4500});
    return j.son;
}

function durationOf(file) {
    return new Promise(res => {
        const a = new Audio();
        a.preload = "metadata";
        a.onloadedmetadata = () => { res(isFinite(a.duration) ? a.duration : 0); URL.revokeObjectURL(a.src); };
        a.onerror = () => res(0);
        a.src = URL.createObjectURL(file);
    });
}

async function importFile(input) {
    const file = input.files[0];
    input.value = "";
    if (!file) return;
    const type = input.dataset.type;
    const d = await durationOf(file);
    if (type === "voix" && d > 30.5) { toast(`<span class="fs-small b">Le commentaire vocal dure 30 secondes au maximum.</span>`); return; }
    const son = await upload(file, type, d, file.name.replace(/\.[^.]+$/, "").slice(0, 40));
    if (!son) return;
    if (type === "musique") { m.musique = "son:" + son.id; changed({tab: true}); }
    else if (type === "voix" && selected()) { selected().voix_id = son.id; changed({work: true, preview: false}); }
    else if (type === "passage" && selected()) { selected().son = "son:" + son.id; changed({work: true, preview: false}); }
}

async function toggleRecord() {
    if (recorder) { recorder.rec.stop(); return; }
    if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) { toast(`<span class="fs-small b">Ton navigateur ne sait pas enregistrer : importe un fichier.</span>`); return; }
    let stream;
    try { stream = await navigator.mediaDevices.getUserMedia({audio: true}); } catch { toast(`<span class="fs-small b">Micro refusé ou indisponible.</span>`); return; }
    const rec = new MediaRecorder(stream);
    const chunks = [], t0 = Date.now();
    recorder = {rec, t0};
    rec.ondataavailable = e => chunks.push(e.data);
    const tick = setInterval(() => {
        const s = Math.floor((Date.now() - t0) / 1000);
        const el = $(".js-rec-t");
        if (el) el.textContent = `${s} s`;
        if (s >= 30) rec.stop();
    }, 250);
    rec.onstop = async () => {
        clearInterval(tick);
        stream.getTracks().forEach(tr => tr.stop());
        const secs = Math.min(30, (Date.now() - t0) / 1000);
        recorder = null;
        renderWork();
        if (secs < 1) return;
        const son = await upload(new Blob(chunks, {type: rec.mimeType || "audio/webm"}), "voix", secs, `Commentaire · ${selected()?.oeuvre?.titre ?? ""}`.slice(0, 40));
        if (son && selected()) { selected().voix_id = son.id; changed({work: true, preview: false}); }
    };
    rec.start();
    renderWork();
}

let listening = null;
function listen(id) {
    if (listening === id) { audio.stop(); listening = null; return; }
    listening = id;
    audio.setVolume(m.volume / 100);
    if (id.startsWith("son:")) audio.play(null, data.sons.find(s => "son:" + s.id === id)?.url);
    else audio.play(id);
    setTimeout(() => { if (listening === id) { audio.stop(); listening = null; } }, 12000);
}

// ------------------------------------------------------------------
// Picker: add an artwork to the current room

function openPicker() {
    const dlg = $(".js-picker");
    const hung = new Set(m.salles.flatMap(s => s.oeuvres.map(w => w.oeuvre_id)));
    const grid = list => list.length ? `<div class="me-pick-grid">${list.map(o => `<button type="button" class="card-2 me-pick" data-add="${o.id}" ${hung.has(o.id) ? "disabled" : ""}>
            <img class="px art" src="${artOf(o, 120)}" alt="" loading="lazy"><span class="fs-cap b ellip">${escapeHTML(o.titre)}</span><span class="fs-cap t3 ellip">${hung.has(o.id) ? "déjà exposée" : escapeHTML(authorsOf(o))}</span></button>`).join("")}</div>` : "";
    dlg.innerHTML = `<div class="me-pick-body"><div class="sec-head"><h2 class="fs-h3">Ajouter une œuvre</h2><button class="btn btn-icon btn-sm btn-ghost js-pick-close" type="button" aria-label="Fermer">${icon("close", 16)}</button></div>
        <span class="fs-over">Mes œuvres</span>${grid(data.miennes) || `<p class="fs-small t2">Tu n'as pas encore d'œuvre validée. <a href="/revendiquer">Revendique une œuvre</a> pour l'exposer.</p>`}
        <span class="fs-over">Mes coups de cœur</span>${grid(data.coeurs) || `<p class="fs-small t2">Donne des coups de cœur dans <a href="/musee">le musée</a> pour exposer les œuvres des autres.</p>`}</div>`;
    fillIcons(dlg);
    dlg.showModal();
}

// ------------------------------------------------------------------
// Events

function wire() {
    page.addEventListener("click", async e => {
        const t = e.target;
        const tb = t.closest("[data-tab]");
        if (tb) { tab = tb.dataset.tab; for (const b of $$("[data-tab]")) { b.classList.toggle("on", b === tb); b.setAttribute("aria-selected", String(b === tb)); } renderTab(); return; }
        const ft = t.closest("[data-fond-type]");
        if (ft) { setFondType(ft.dataset.fondType); return; }
        const fp = t.closest("[data-fond-paysage]");
        if (fp && m.fond) { m.fond.value = fp.dataset.fondPaysage; fondChanged({tab: true}); return; }
        const fo = t.closest("[data-fond-oeuvre]");
        if (fo && m.fond) { m.fond.value = fo.dataset.fondOeuvre; fondChanged({tab: true}); return; }
        const fm = t.closest("[data-fond-mode]");
        if (fm && m.fond) { m.fond.mode = fm.dataset.fondMode; fondChanged({tab: true}); return; }
        const fs = t.closest("[data-fond-step], [data-fond-jump]");
        if (fs && m.fond?.type === "capture") {
            const total = snap?.frames ?? 1;
            const j = fs.dataset.fondJump;
            m.fond.frame = j === "first" ? 0 : j === "last" ? total - 1 : j === "random" ? Math.floor(Math.random() * total) : Math.max(0, Math.min(total - 1, m.fond.frame + Number(fs.dataset.fondStep)));
            fondChanged({tab: true});
            return;
        }
        const set = t.closest("[data-set]");
        if (set) {
            const k = set.dataset.set;
            m[k] = set.dataset.value;
            if (k === "theme") m.mur = "";
            changed({tab: true});
            return;
        }
        const bo = t.closest("[data-bool]");
        if (bo) { m[bo.dataset.bool] = !m[bo.dataset.bool]; changed({tab: true, preview: false}); return; }
        const ls = t.closest("[data-listen]");
        if (ls) { listen(ls.dataset.listen); return; }
        const ds = t.closest("[data-del-son]");
        if (ds && confirm("Supprimer ce son ?")) {
            await fetch("/api/me/musee/sons?id=" + ds.dataset.delSon, {method: "DELETE"});
            data.sons = data.sons.filter(s => String(s.id) !== ds.dataset.delSon);
            if (m.musique === "son:" + ds.dataset.delSon) m.musique = "vagues";
            changed({tab: true});
            return;
        }
        const sl = t.closest("[data-salle]");
        if (sl) { salle = Number(sl.dataset.salle); sel = 0; renderRooms(); renderPreview(); renderWork(); return; }
        if (t.closest(".js-add-salle")) { m.salles.push({titre: `Salle ${m.salles.length + 1}`, oeuvres: []}); salle = m.salles.length - 1; sel = 0; changed({rooms: true, work: true}); return; }
        if (t.closest(".js-del-salle") && confirm(`Supprimer « ${m.salles[salle].titre} » ? Ses œuvres seront retirées du musée.`)) {
            m.salles.splice(salle, 1); salle = 0; sel = 0; changed({rooms: true, work: true}); return;
        }
        const se = t.closest("[data-sel]");
        if (se) { sel = Number(se.dataset.sel); renderRooms(); renderWork(); for (const b of $$(".me-pv-work")) b.classList.toggle("on", Number(b.dataset.sel) === sel); if (matchMedia("(max-width: 1099px)").matches) $(".js-work-panel").scrollIntoView({behavior: "smooth", block: "start"}); return; }
        const mv = t.closest("[data-move]");
        if (mv) {
            const [i, d] = mv.dataset.move.split(":").map(Number), ws = works();
            [ws[i], ws[i + d]] = [ws[i + d], ws[i]];
            sel = i + d;
            changed({rooms: true});
            return;
        }
        if (t.closest(".js-add-work")) { openPicker(); return; }
        const ws = t.closest("[data-wset]");
        if (ws && selected()) { selected()[ws.dataset.wset] = ws.dataset.value; if (ws.dataset.wset === "son" && m.sons_passage) { const v = ws.dataset.value; v.startsWith("son:") ? audio.sfx(null, data.sons.find(s => "son:" + s.id === v)?.url) : audio.sfx(v); } changed({work: true, rooms: ws.dataset.wset === "vedette"}); return; }
        const wb = t.closest("[data-wbool]");
        if (wb && selected()) { selected()[wb.dataset.wbool] = !selected()[wb.dataset.wbool]; changed({work: true, rooms: true}); return; }
        const pu = t.closest("[data-play-url]");
        if (pu) { new Audio(pu.dataset.playUrl).play().catch(() => {}); return; }
        if (t.closest(".js-voice-off") && selected()) { selected().voix_id = 0; changed({work: true, preview: false}); return; }
        if (t.closest(".js-record")) { toggleRecord(); return; }
        if (t.closest(".js-remove") && selected()) { works().splice(sel, 1); sel = Math.max(0, sel - 1); changed({rooms: true, work: true}); return; }
        if (t.closest(".js-copy")) { const ok = await copyText(`${location.origin}/u/${me.slug}/musee`); toast(`<span class="fs-small b">${ok ? "Lien copié" : "Copie impossible"}</span>`); return; }
        if (t.closest(".js-publish")) {
            publie = !publie;
            await save();
            const b = $(".js-publish");
            b.className = `btn ${publie ? "" : "btn-gold btn-px"} js-publish`;
            b.innerHTML = publie ? `${icon("check", 18)}Ouvert · fermer` : "Ouvrir mon musée";
            toast(publie ? `<span class="fs-small b">Ton musée est ouvert !</span><a class="btn btn-sm" href="/u/${escapeHTML(me.slug)}/musee">Visiter</a>` : `<span class="fs-small b">Musée fermé aux visiteurs</span>`);
        }
    });
    page.addEventListener("input", e => {
        const t = e.target;
        if (t.classList.contains("js-fond-frame") && m.fond) {
            m.fond.frame = Number(t.value);
            const img = $(".js-fond-img");
            if (img) img.src = fondURL(m.fond);
            const lab = $(".js-fond-label");
            if (lab) lab.textContent = frameInfo(m.fond.frame);
            fondChanged();
            return;
        }
        if (t.dataset.fondNum && m.fond) {
            m.fond[t.dataset.fondNum] = Number(t.value);
            const out = $(".js-fond-" + t.dataset.fondNum);
            if (out) out.textContent = t.value + (t.dataset.fondNum === "flou" ? " px" : " %");
            fondChanged();
            return;
        }
        if (t.dataset.text) {
            m[t.dataset.text] = t.value;
            if (t.dataset.text === "nom") $(".js-title").textContent = t.value || "Mon musée";
            if (t.dataset.text === "accueil") $(".js-count-accueil").textContent = `${t.value.length} / 160`;
            changed({preview: t.dataset.text === "phrase_guide"});
        } else if (t.dataset.num) {
            m[t.dataset.num] = Number(t.value);
            $(".js-vol").textContent = `${t.value} %`;
            audio.setVolume(m.volume / 100);
            changed({preview: false});
        } else if (t.dataset.wtext && selected()) {
            selected()[t.dataset.wtext] = t.value;
            changed({preview: false});
        } else if (t.hasAttribute("data-salle-title")) {
            m.salles[salle].titre = t.value;
            const chip = $(`.js-salles [data-salle="${salle}"]`);
            if (chip) chip.firstChild.textContent = (t.value || "Salle") + " ";
            changed({preview: false});
        }
    });
    page.addEventListener("change", e => {
        if (e.target.classList.contains("js-fond-file")) { uploadFond(e.target); return; }
        if (e.target.classList.contains("js-import")) importFile(e.target);
        if (e.target.classList.contains("js-voice-pick") && selected()) { selected().voix_id = Number(e.target.value) || 0; changed({work: true, preview: false}); }
    });
    const dlg = $(".js-picker");
    dlg.addEventListener("click", e => {
        if (e.target.closest(".js-pick-close") || e.target === dlg) { dlg.close(); return; }
        const a = e.target.closest("[data-add]");
        if (!a) return;
        const o = oeuvres.get(Number(a.dataset.add));
        works().push({oeuvre_id: o.id, animation: "aucune", son: "aucun", voix_id: 0, texte: "", taille: "moyenne", cadre: "musee", spot: "#fff8b8", vedette: false, oeuvre: o});
        sel = works().length - 1;
        dlg.close();
        changed({rooms: true, work: true});
    });
    // a change typed just before leaving: send it right away
    addEventListener("pagehide", () => {
        if (!saveTimer) return;
        fetch("/api/me/musee", {method: "PUT", headers: {"Content-Type": "application/json"}, body: JSON.stringify({musee: payload(), publie}), keepalive: true});
    });
}

main();

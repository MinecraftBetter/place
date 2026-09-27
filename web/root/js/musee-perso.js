// /u/:pseudo/musee — visit of a player's museum (d-musee-perso, m-musee-perso): curtain,
// welcome, music, particles, the pixel guide, artworks with their animations and voices,
// guestbook, coup de cœur.

import {$, $$, avatarHTML, escapeHTML, fillIcons, icon, toast} from "./ui.js";
import {fetchMe, loginHref, emptyState} from "./shell.js";
import {relativeTime, formatNumber} from "./view.js";
import {artView} from "./common.js";
import {Chiptune} from "./chiptune.js";
import {openReport} from "./report.js";
import {theme, wallOf, inkOf, particlesHTML, workFrameHTML, spotStyle, cartelHTML, animateWorks, guideLine, allWorks, MUSICS} from "./museum-kit.js";

const page = $("#page");
const slug = decodeURIComponent(location.pathname.split("/")[2] ?? "");
const audio = new Chiptune();
const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
const mobile = matchMedia("(max-width: 767px), (pointer: coarse)");

let me = null, data = null, m = null, works = [], focus = 0, entered = false, musicOn = true, playing = false, timer = 0, voice = null, stopAnims = () => {};

async function main() {
    me = (await fetchMe()).user;
    const r = await fetch(`/api/musees/${encodeURIComponent(slug)}?visite=1`, {cache: "no-store"});
    if (!r.ok) {
        const j = await r.json().catch(() => ({}));
        document.documentElement.classList.add("content-page");
        page.className = "page-main mp-error";
        page.innerHTML = `<div class="mp-error-box">${emptyState({title: j.error ?? "Ce musée est introuvable", mascot: true,
            text: r.status === 401 ? "Connecte-toi pour le visiter." : "Il ouvrira peut-être bientôt : repasse plus tard !",
            action: r.status === 401 ? `<a class="btn btn-primary btn-px" href="${loginHref()}">Se connecter</a>` : `<a class="btn" href="/musees">Voir les autres musées</a>`})}</div>`;
        page.removeAttribute("aria-busy");
        fillIcons(page);
        return;
    }
    data = await r.json();
    m = data.musee;
    works = allWorks(m);
    document.title = `${m.nom} · BetterPlace`;
    render();
}

function render() {
    const t = theme(m), ink = inkOf(m);
    page.style.setProperty("--mp-ink", ink.ink);
    page.style.setProperty("--mp-sub", ink.sub);
    page.style.setProperty("--mp-accent", t.accent);
    const an = parseInt(t.accent.slice(1), 16);
    const lightAccent = (0.299 * (an >> 16) + 0.587 * (an >> 8 & 255) + 0.114 * (an & 255)) / 255 > .55;
    page.style.setProperty("--mp-accent-ink", lightAccent ? "#06223f" : "#ffffff");
    page.style.setProperty("--mp-eq", lightAccent ? t.accent : "#f3efe6");
    page.classList.toggle("mp-light", !!ink.light);
    page.classList.toggle("mp-guided", m.parcours === "guide");
    const u = data.user;
    const music = data.musique;
    const musicName = music.type === "fichier" ? music.nom : MUSICS[music.type]?.[0] ?? "";
    const hasMusic = music.type !== "silence" && musicName;
    const salles = m.salles.map((s, i) => `<button type="button" data-salle="${i}">${escapeHTML(s.titre)}</button>`).join("");
    page.innerHTML = `
        <div class="mp-wall" style="background: ${wallOf(m)}"></div>
        <div class="mp-floor" style="background: ${t.floor}">${t.waves && !m.mur ? `<div class="vagues mp-waves"></div>` : ""}</div>
        <div class="mp-parts" aria-hidden="true">${particlesHTML(m.particules, 22, innerWidth, innerHeight * .7)}</div>
        <div class="mp-cam js-cam">
            <header class="mp-head">
                <a class="btn btn-icon glass" href="/u/${escapeHTML(u.slug)}" aria-label="Retour au profil">${icon("back", 20)}</a>
                ${avatarHTML(u, "av av-40 mp-av")}
                <div class="mp-title"><h1 class="pixel b ellip">${escapeHTML(m.nom)}</h1>${m.accueil ? `<span class="fs-cap mp-sub ellip">« ${escapeHTML(m.accueil)} »</span>` : `<span class="fs-cap mp-sub">par ${escapeHTML(u.pseudo)}</span>`}</div>
                <span class="grow"></span>
                ${m.salles.length > 1 ? `<div class="glass mp-salles only-d" role="group" aria-label="Salles">${salles}</div>` : ""}
                <button class="btn glass mp-like js-like" type="button" aria-pressed="${data.liked}" aria-label="Coup de cœur pour ce musée"><img class="px" src="/img/badges/coeur.png" alt="" width="18" height="18"><span class="mono js-likes">${data.likes}</span></button>
                ${me && !data.moi ? `<button class="btn btn-icon glass js-report" type="button" aria-label="Signaler ce musée">${icon("flag", 18)}</button>` : ""}
                ${data.moi ? `<a class="btn glass only-d" href="/moi/musee">${icon("edit", 18)}Modifier</a>` : ""}
            </header>
            ${data.moi && !data.publie ? `<div class="mp-draft glass fs-small">Aperçu : ton musée n'est pas encore ouvert aux visiteurs. <a href="/moi/musee">Le publier</a></div>` : ""}
            <section class="mp-room js-room" aria-live="polite">${works.length ? works.map((x, i) => workHTML(x, i)).join("") : `<div class="mp-empty cartel"><span class="pixel b">Les murs sont encore nus</span><span class="fs-cap">${data.moi ? "Ajoute des œuvres depuis l'éditeur." : "Les œuvres arrivent bientôt."}</span></div>`}</section>
            <nav class="mp-nav" aria-label="Parcours">
                <button class="btn btn-icon glass btn-round js-prev" type="button" aria-label="Œuvre précédente">${icon("prev", 20)}</button>
                <div class="mp-pos glass"><span class="fs-cap mp-sub js-room-name"></span><span class="mono fs-small b js-pos"></span></div>
                ${m.parcours === "guide" ? `<button class="btn btn-icon glass btn-round js-play" type="button" aria-label="Pause">${icon("pause", 20)}</button>` : ""}
                <button class="btn btn-icon glass btn-round js-next" type="button" aria-label="Œuvre suivante">${icon("next", 20)}</button>
            </nav>
        </div>
        ${m.guide !== "aucun" ? `<div class="mp-guide" aria-live="polite">${m.guide === "avatar" ? avatarHTML(u, "av av-48 anim-bob") : `<img class="px anim-bob" src="/img/farwest-x8.png" alt="Le guide cowboy" width="36" height="62">`}
            <div class="bulle js-bulle">${escapeHTML(m.phrase_guide || "Bienvenue !")}</div></div>` : ""}
        <div class="glass mp-music">
            <button class="btn btn-icon btn-round mp-music-btn js-music" type="button" aria-label="${hasMusic ? "Couper la musique" : "Pas de musique"}" ${hasMusic ? "" : "disabled"}>${icon(hasMusic ? "music" : "music", 18)}</button>
            <div class="mp-music-text"><span class="fs-small b ellip">${escapeHTML(hasMusic ? musicName : "Silence")}</span><span class="fs-cap mp-sub ellip">${hasMusic ? `musique choisie par ${escapeHTML(u.pseudo)} · boucle` : "juste le bruit des pas"}</span></div>
            ${hasMusic ? `<div class="eq js-eq" style="color: var(--mp-eq)"><span></span><span></span><span></span><span></span></div>` : ""}
            <button class="btn btn-sm glass only-m js-book-open" type="button">${icon("edit", 16)}Livre d'or <span class="mono">${data.mots}</span></button>
        </div>
        <aside class="glass mp-book js-book" aria-label="Livre d'or">
            <div class="mp-book-head">${icon("edit", 18)}<span class="fs-body b grow">Livre d'or</span><span class="fs-cap mp-sub js-mots">${data.mots} mot${data.mots > 1 ? "s" : ""}</span>
                <button class="btn btn-icon btn-sm btn-ghost only-m js-book-close" type="button" aria-label="Fermer">${icon("close", 16)}</button></div>
            <div class="mp-book-list js-book-list">${bookHTML()}</div>
            ${m.livre_or ? (me ? `<form class="mp-book-form js-sign"><input class="input" maxlength="200" aria-label="Laisser un mot" placeholder="Laisser un mot…" required><button class="btn btn-primary" type="submit">Signer</button></form>`
                : `<a class="btn btn-block" href="${loginHref()}">Se connecter pour signer</a>`) : `<span class="fs-cap mp-sub">Le livre d'or est fermé.</span>`}
        </aside>
        ${curtainHTML()}`;
    page.removeAttribute("aria-busy");
    fillIcons(page);
    wire();
    setFocus(0, {quiet: true});
    stopAnims = animateWorks(page);
}

function workHTML(x, i) {
    const w = x.work;
    const scale = mobile.matches ? 1 : 1.12;
    return `<figure class="mp-work${w.vedette ? " mp-vedette" : ""}" data-i="${i}">
        <div class="mp-spot" style="${spotStyle(w, m)}"></div>
        <button class="mp-hang" type="button" data-focus="${i}" aria-label="${escapeHTML(w.oeuvre.titre)}">${workFrameHTML(w, m, {scale})}</button>
        <figcaption>${cartelHTML(w)}
            ${w.texte ? `<p class="fs-small mp-mot pretty">« ${escapeHTML(w.texte)} »</p>` : ""}
            <div class="mp-work-actions">
                ${w.voix ? `<button class="glass mp-voice js-voice" type="button" data-i="${i}"><span class="mp-voice-ico">${icon("play", 14)}</span>
                    <span class="onde">${[6, 12, 8, 14, 10, 5, 12, 7, 13, 9].map(h => `<span style="height: ${h}px"></span>`).join("")}</span><span class="fs-cap b">L'artiste · ${Math.round(w.voix.duree || 0)} s${w.voix.statut === "attente" ? " · en attente" : ""}</span></button>` : ""}
                <a class="btn btn-sm glass" href="/oeuvre/${w.oeuvre.id}">Cartel complet</a>
                <a class="btn btn-sm glass" href="${artView(w.oeuvre)}">${icon("target", 14)}Sur le canvas</a>
            </div>
        </figcaption>
    </figure>`;
}

function bookHTML() {
    if (!data.livre.length) return `<p class="fs-small mp-sub">Personne n'a encore signé. Sois le premier !</p>`;
    return data.livre.map(e => `<div class="mp-entry">${avatarHTML(e.auteur, "av av-32")}<div class="grow"><span class="fs-small b">${escapeHTML(e.auteur?.pseudo ?? "")}</span> <span class="fs-cap mp-sub">${relativeTime(e.ts)}</span>
        <p class="fs-small mp-entry-text">${escapeHTML(e.texte)}</p></div>${me && e.auteur?.id !== me.id ? `<button class="btn btn-icon btn-sm btn-ghost mp-flag" type="button" data-report-entry="${e.id}" aria-label="Signaler ce message">${icon("flag", 14)}</button>` : ""}</div>`).join("");
}

function curtainHTML() {
    const n = works.length, s = m.salles.length;
    return `<div class="mp-enter js-enter mp-enter-${m.entree}">
        ${m.entree === "rideau" ? `<div class="rideau rideau-g"></div><div class="rideau rideau-d"></div>` : `<div class="mp-enter-veil"></div>`}
        <div class="mp-welcome">
            <div class="cartel mp-welcome-card"><span class="fs-over">Bienvenue au</span><span class="pixel b">${escapeHTML(m.nom)}</span>
                <span class="fs-small">${n} œuvre${n > 1 ? "s" : ""} · ${s} salle${s > 1 ? "s" : ""} · ${formatNumber(data.visites)} visite${data.visites > 1 ? "s" : ""}</span></div>
            <button class="btn btn-primary btn-lg btn-px js-enter-btn" type="button">Entrer</button>
            <span class="fs-cap mp-enter-hint">${data.musique.type !== "silence" ? "Mets le son !" : ""}</span>
        </div>
    </div>`;
}

function enter() {
    if (entered) return;
    entered = true;
    const el = $(".js-enter");
    el.classList.add("open");
    $$(".rideau", el).forEach(r => r.classList.add("rideau-ouvert"));
    $(".js-cam").classList.add("mp-cam-in");
    setTimeout(() => el.remove(), reduced ? 0 : 1000);
    const music = data.musique;
    audio.setVolume((m.volume ?? 60) / 100);
    if (music.type === "fichier") audio.play(null, music.url);
    else if (music.type !== "silence") audio.play(music.type);
    renderMusic();
    if (m.parcours === "guide") { playing = true; schedule(); }
    setTimeout(() => arrive(works[focus]), 900);
}

function renderMusic() {
    const on = audio.playing();
    $(".js-eq")?.classList.toggle("paused", !on);
    const b = $(".js-music");
    if (b && !b.disabled) b.setAttribute("aria-label", on ? "Couper la musique" : "Lancer la musique");
}

function setFocus(i, {quiet = false} = {}) {
    if (!works.length) return;
    focus = (i + works.length) % works.length;
    const cur = works[focus];
    for (const el of $$(".mp-work")) {
        const j = Number(el.dataset.i), x = works[j];
        el.classList.toggle("on", j === focus);
        el.classList.toggle("other-room", x.salle !== cur.salle);
    }
    for (const b of $$("[data-salle]")) b.classList.toggle("on", Number(b.dataset.salle) === cur.salle);
    $(".js-room-name").textContent = m.salles[cur.salle].titre;
    const inRoom = works.filter(x => x.salle === cur.salle);
    $(".js-pos").textContent = `${cur.index + 1} / ${inRoom.length}`;
    const el = $(`.mp-work[data-i="${focus}"]`);
    if (!mobile.matches) el?.scrollIntoView({behavior: reduced || quiet ? "auto" : "smooth", inline: "center", block: "nearest"});
    if (!quiet) arrive(cur);
}

// reaching an artwork: the guide talks, a sound plays, the artist's voice starts
function arrive(x) {
    if (!x || !entered) return;
    const w = x.work;
    const b = $(".js-bulle");
    if (b) b.textContent = guideLine(w, focus);
    if (m.pas) audio.sfx("pas");
    if (m.sons_passage) {
        if (w.son_fichier) setTimeout(() => audio.sfx(null, w.son_fichier.url), 350);
        else if (w.son && w.son !== "aucun") setTimeout(() => audio.sfx(w.son), 350);
    }
    stopVoice();
    if (m.voix_auto && w.voix) setTimeout(() => { if (works[focus] === x) playVoice(focus); }, 1200);
}

function playVoice(i) {
    const w = works[i].work;
    if (!w.voix) return;
    if (voice?.i === i) { stopVoice(); return; }
    stopVoice();
    const a = new Audio(w.voix.url);
    voice = {i, a};
    audio.duck(true);
    const btn = $(`.js-voice[data-i="${i}"]`);
    btn?.classList.add("on");
    if (btn) $(".mp-voice-ico", btn).innerHTML = icon("pause", 14);
    a.onended = stopVoice;
    a.play().catch(stopVoice);
}

function stopVoice() {
    if (!voice) return;
    voice.a.pause();
    const btn = $(`.js-voice[data-i="${voice.i}"]`);
    btn?.classList.remove("on");
    if (btn) $(".mp-voice-ico", btn).innerHTML = icon("play", 14);
    voice = null;
    audio.duck(false);
}

function schedule() {
    clearTimeout(timer);
    if (!playing || !works.length) return;
    const w = works[focus].work;
    timer = setTimeout(() => {
        if (focus === works.length - 1) {
            playing = false;
            updatePlay();
            const b = $(".js-bulle");
            if (b) b.textContent = m.livre_or ? "Merci de ta visite ! Laisse un mot au livre d'or avant de partir." : "Merci de ta visite, reviens quand tu veux !";
            return;
        }
        setFocus(focus + 1);
        schedule();
    }, (w.vedette ? 16 : 8) * 1000 + (voice ? 4000 : 0));
}

function updatePlay() {
    const b = $(".js-play");
    if (!b) return;
    b.innerHTML = icon(playing ? "pause" : "play", 20);
    b.setAttribute("aria-label", playing ? "Pause" : "Reprendre la visite");
}

function wire() {
    $(".js-enter-btn")?.addEventListener("click", enter);
    $(".js-prev").addEventListener("click", () => { setFocus(focus - 1); schedule(); });
    $(".js-next").addEventListener("click", () => { setFocus(focus + 1); schedule(); });
    $(".js-play")?.addEventListener("click", () => { playing = !playing; updatePlay(); schedule(); });
    $(".js-music")?.addEventListener("click", () => {
        if (audio.playing()) audio.stop();
        else if (data.musique.type === "fichier") audio.play(null, data.musique.url);
        else audio.play(data.musique.type);
        renderMusic();
    });
    page.addEventListener("click", e => {
        const f = e.target.closest("[data-focus]");
        if (f) { setFocus(Number(f.dataset.focus)); schedule(); return; }
        const s = e.target.closest("[data-salle]");
        if (s) { const k = works.findIndex(x => x.salle === Number(s.dataset.salle)); if (k >= 0) { setFocus(k); schedule(); } return; }
        const v = e.target.closest(".js-voice");
        if (v) { playVoice(Number(v.dataset.i)); return; }
        const r = e.target.closest("[data-report-entry]");
        if (r) { openReport({me, cible: `livre_or:${r.dataset.reportEntry}`, title: "Signaler ce message", choices: [["livre_or", "Ce message du livre d'or"]]}); return; }
    });
    $(".js-report")?.addEventListener("click", () => openReport({me, cible: `musee:${data.user.id}`, title: `Signaler « ${m.nom} »`, choices: [["musee", "Le nom ou les textes du musée"]]}));
    $(".js-like").addEventListener("click", async e => {
        if (!me) { location.href = loginHref(); return; }
        const r = await fetch(`/api/musees/${encodeURIComponent(slug)}/like`, {method: "POST"});
        if (!r.ok) return;
        const j = await r.json();
        e.currentTarget.setAttribute("aria-pressed", String(j.liked));
        $(".js-likes").textContent = j.likes;
        if (j.liked) toast(`<img class="px" src="/img/badges/coeur.png" alt="" width="20" height="20"><span class="fs-small b">Coup de cœur envoyé à ${escapeHTML(data.user.pseudo)}</span>`);
    });
    $(".js-sign")?.addEventListener("submit", async e => {
        e.preventDefault();
        const input = $("input", e.currentTarget);
        const r = await fetch(`/api/musees/${encodeURIComponent(slug)}/livre-or`, {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({texte: input.value})});
        const j = await r.json().catch(() => ({}));
        if (!r.ok) { toast(`<span class="fs-small b">${escapeHTML(j.error ?? "Impossible de signer")}</span>`); return; }
        input.value = "";
        data.livre.unshift(j.entree);
        data.mots++;
        $(".js-book-list").innerHTML = bookHTML();
        $(".js-mots").textContent = `${data.mots} mot${data.mots > 1 ? "s" : ""}`;
        fillIcons($(".js-book-list"));
        toast(`<span class="fs-small b">Merci pour ton mot !</span>`);
    });
    $(".js-book-open")?.addEventListener("click", () => $(".js-book").classList.add("open"));
    $(".js-book-close")?.addEventListener("click", () => $(".js-book").classList.remove("open"));
    document.addEventListener("keydown", e => {
        if (e.target.closest("input, textarea")) return;
        if (!entered && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); enter(); return; }
        if (e.key === "ArrowRight") { setFocus(focus + 1); schedule(); }
        if (e.key === "ArrowLeft") { setFocus(focus - 1); schedule(); }
    });
    // swipe between artworks on phones
    let sx = null;
    const room = $(".js-room");
    room.addEventListener("pointerdown", e => { sx = e.clientX; });
    room.addEventListener("pointerup", e => {
        if (sx === null) return;
        const dx = e.clientX - sx;
        sx = null;
        if (Math.abs(dx) > 50) { setFocus(focus + (dx < 0 ? 1 : -1)); schedule(); }
    });
    addEventListener("pagehide", () => { audio.stop(); stopAnims(); });
}

main();

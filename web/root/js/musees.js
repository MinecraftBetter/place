// /musees — the players' museums (m-musees): popular, new, my favourites, search, « Au hasard ».

import {$, avatarHTML, escapeHTML, fillIcons, icon} from "./ui.js";
import {mountShell, loginHref, emptyState} from "./shell.js";
import {formatNumber} from "./view.js";
import {THEMES, particlesHTML, FRAMES} from "./museum-kit.js";

const page = $("#page");
const TABS = [["populaires", "Populaires"], ["nouveaux", "Nouveaux"], ["coeur", "Mes coups de cœur"]];
let me = null, tri = "populaires", q = "", timer = 0;

function card(m) {
    const t = THEMES[m.theme] ?? THEMES.maree;
    const a = m.apercu;
    let art = "";
    if (a) {
        const side = Math.max(a.w, a.h), pad = Math.max(2, Math.round(side * .08));
        const z = Math.max(1, Math.min(8, Math.floor(160 / (side + 2 * pad))));
        const [, frame, passe] = FRAMES[m.cadre] ?? FRAMES.or;
        const img = `<img class="px" src="/api/crop.png?x=${Math.max(0, a.x - pad)}&y=${Math.max(0, a.y - pad)}&w=${a.w + 2 * pad}&h=${a.h + 2 * pad}&z=${z}" alt="" loading="lazy">`;
        art = `<div class="mus-art mp-frame ${frame}">${passe ? `<div class="${passe}">${img}</div>` : img}</div>`;
    }
    return `<a class="card mus-card" href="/u/${escapeHTML(m.user.slug)}/musee">
        <div class="mus-scene" style="--wall: ${escapeHTML(m.fond)}; --floor: ${t.floor}">${particlesHTML(m.particules, 8, 260, 110, m.user.id)}${art}
            ${m.nouveau ? `<span class="pill st-info mus-new">Nouveau</span>` : ""}
            ${m.musique ? `<span class="glass mus-music fs-cap"><span class="eq" style="color: ${escapeHTML(m.accent)}"><span></span><span></span><span></span><span></span></span>${escapeHTML(m.musique)}</span>` : ""}</div>
        <div class="mus-info">${avatarHTML(m.user, "av av-40")}<div class="grow" style="min-width: 0"><span class="fs-body b ellip">${escapeHTML(m.nom)}</span>
            <span class="fs-cap t3 ellip">${escapeHTML(m.user.pseudo)} · ${m.n} œuvre${m.n > 1 ? "s" : ""} · ${formatNumber(m.visites)} visite${m.visites > 1 ? "s" : ""}</span></div>
            <span class="mus-likes fs-small${m.liked ? " on" : ""}"><img class="px" src="/img/badges/coeur.png" alt="" width="16" height="16"><span class="mono">${m.likes}</span><span class="sr-only">coups de cœur</span></span></div>
    </a>`;
}

async function load() {
    const r = await fetch(`/api/musees?tri=${tri}&q=${encodeURIComponent(q)}`, {cache: "no-store"});
    const d = await r.json();
    $(".js-list").innerHTML = d.musees.length ? d.musees.map(card).join("")
        : emptyState({title: tri === "coeur" ? "Aucun coup de cœur pour l'instant" : q ? "Aucun musée ne correspond" : "Aucun musée n'a encore ouvert",
            text: tri === "coeur" ? "Visite des musées et donne un coup de cœur à ceux que tu aimes." : "Sois le premier à ouvrir le tien !", mascot: !q});
    const mine = $(".js-mine");
    if (mine && me) {
        const mo = d.moi;
        const t = THEMES[mo?.theme] ?? THEMES.maree;
        mine.innerHTML = mo ? `<div class="mus-mine-swatch" style="background: ${t.wall}"></div><div class="grow"><span class="fs-body b">${escapeHTML(mo.nom)}</span><br>
                <span class="fs-cap t3">${t.label} · ${mo.n} œuvre${mo.n > 1 ? "s" : ""} · ${formatNumber(mo.visites)} visite${mo.visites > 1 ? "s" : ""} · ${mo.mots} mot${mo.mots > 1 ? "s" : ""} au livre d'or${mo.publie ? "" : " · pas encore ouvert"}</span></div>
                ${mo.publie ? `<a class="btn btn-sm btn-ghost" href="/u/${escapeHTML(me.slug)}/musee">Visiter</a>` : ""}<a class="btn btn-sm" href="/moi/musee">Modifier</a>`
            : `<img class="px" src="/img/farwest-x8.png" alt="" width="30" height="52"><div class="grow"><span class="fs-body b">Ouvre ton musée</span><br><span class="fs-cap t3">Tes œuvres, tes coups de cœur, ta déco, ta musique.</span></div><a class="btn btn-sm btn-gold btn-px" href="/moi/musee">Créer</a>`;
    }
    fillIcons(page);
    return d;
}

async function main() {
    me = (await mountShell({active: "musee"})).user;
    page.innerHTML = `<div class="mus-wrap">
        <header class="mus-head"><div><h1 class="fs-h1">Musées des joueurs</h1><p class="fs-small t2 pretty">Chaque joueur peut ouvrir son propre musée : ses œuvres, ses coups de cœur, sa déco, sa musique.</p></div>
            <button class="btn btn-px js-random" type="button">${icon("dice", 18)}Au hasard</button></header>
        ${me ? `<div class="card mus-mine js-mine"></div>` : `<a class="card mus-mine" href="${loginHref()}"><img class="px" src="/img/farwest-x8.png" alt="" width="30" height="52"><div class="grow"><span class="fs-body b">Toi aussi, ouvre ton musée</span><br><span class="fs-cap t3">Connecte-toi pour commencer.</span></div></a>`}
        <div class="mus-tools"><div class="seg" role="group" aria-label="Trier">${TABS.map(([id, l]) => `<button type="button" data-tri="${id}" class="${id === tri ? "on" : ""}"${id === "coeur" && !me ? " hidden" : ""}>${l}</button>`).join("")}</div>
            <div class="search mus-search">${icon("search", 16)}<input class="input js-q" placeholder="Chercher un musée, un joueur…" aria-label="Chercher un musée"></div></div>
        <div class="mus-grid js-list"></div>
    </div>`;
    page.removeAttribute("aria-busy");
    fillIcons(page);
    page.addEventListener("click", e => {
        const t = e.target.closest("[data-tri]");
        if (!t) return;
        tri = t.dataset.tri;
        for (const b of page.querySelectorAll("[data-tri]")) b.classList.toggle("on", b === t);
        load();
    });
    $(".js-q").addEventListener("input", e => { clearTimeout(timer); timer = setTimeout(() => { q = e.target.value; load(); }, 250); });
    $(".js-random").addEventListener("click", async () => {
        const d = await (await fetch("/api/musees?hasard=1", {cache: "no-store"})).json();
        if (d.hasard) location.href = `/u/${encodeURIComponent(d.hasard.slug)}/musee`;
    });
    load();
}

main();

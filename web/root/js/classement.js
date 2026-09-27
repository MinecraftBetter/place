// /classement — for fun: who drew the most, and the favourite artworks (m-classement, d-classement).

import {$, avatarHTML, escapeHTML, fillIcons} from "./ui.js";
import {mountShell, emptyState} from "./shell.js";
import {formatNumber} from "./view.js";

const KINDS = [["joueurs", "Joueurs"], ["oeuvres", "Œuvres coup de cœur"]];
const PERIODS = [["jour", "Aujourd'hui"], ["semaine", "Semaine"], ["tout", "Tout le temps"]];
const RING = ["#ffd623", "#d4d7d9", "#9c451a"];
const HEIGHT = [100, 70, 52], SIZE = [64, 52, 48];

let me = null, kind = "joueurs", period = "semaine";

function who(e) {
    if (e.user) return {name: e.user.pseudo, href: `/u/${e.user.slug}`, img: avatarHTML(e.user, "av lb-av")};
    return {name: e.oeuvre.titre, href: `/oeuvre/${e.oeuvre.id}`, img: `<span class="lb-art"><img class="px" src="${escapeHTML(e.oeuvre.extra?.thumb ?? "")}" alt=""></span>`};
}

function value(e) {
    return kind === "joueurs" ? `${formatNumber(e.value)} px` : `${formatNumber(e.value)} ❤`;
}

function render(data) {
    const page = $("#page");
    const entries = data.entries ?? [];
    const top = entries.slice(0, 3);
    const order = [top[1], top[0], top[2]].map((e, j) => e ? {e, rank: [2, 1, 3][j]} : null);
    const podium = top.length ? `<div class="lb-podium">${order.map(o => o ? (() => {
        const w = who(o.e), i = o.rank - 1;
        return `<a class="lb-step" href="${escapeHTML(w.href)}">
            <span class="lb-step-img" style="--sz: ${SIZE[i]}px; --ring: ${RING[i]}">${w.img}</span>
            <span class="fs-small b ellip">${escapeHTML(w.name)}</span>
            <span class="mono fs-cap t2">${value(o.e)}</span>
            <span class="lb-block px-corners" style="height: ${HEIGHT[i]}px"><span class="pixel b">${o.rank}</span></span>
        </a>`;
    })() : `<span></span>`).join("")}</div>` : "";
    const rest = entries.slice(3).map(e => {
        const w = who(e);
        const mine = me && e.user?.id === me.id;
        return `<a class="lb-row${mine ? " mine" : ""}" href="${escapeHTML(w.href)}"><span class="mono fs-small t3 lb-rank">${e.rank}</span>${w.img}<span class="fs-small b grow ellip">${escapeHTML(w.name)}</span><span class="mono fs-small">${value(e)}</span></a>`;
    }).join("");
    const mine = data.moi && data.moi.rank ? `<div class="card-2 lb-me">${avatarHTML(data.moi.user, "av av-32")}<span class="fs-small grow"><b>Toi : #${data.moi.rank}</b> <span class="t3">· ${formatNumber(data.moi.value)} px</span></span><a class="btn btn-sm btn-primary btn-px" href="/ace">Dessiner</a></div>` : "";
    page.innerHTML = `<div class="lb-wrap">
        <div class="lb-head"><h1 class="fs-h1">Classement</h1><p class="fs-small t2">Pour le plaisir : qui a le plus dessiné, et les œuvres qu'on préfère.</p></div>
        <div class="seg lb-kinds" role="tablist">${KINDS.map(([id, l]) => `<button type="button" class="${id === kind ? "on" : ""}" data-kind="${id}">${l}</button>`).join("")}</div>
        <div class="lb-periods">${PERIODS.map(([id, l]) => `<button type="button" class="chip${id === period ? " chip-on" : ""}" data-period="${id}">${l}</button>`).join("")}</div>
        ${entries.length ? podium + `<div class="lb-list">${rest}</div>` + mine
            : emptyState({title: kind === "joueurs" ? "Personne n'a encore dessiné sur cette période" : "Aucun coup de cœur pour l'instant",
                text: kind === "joueurs" ? "Pose un pixel et prends la tête !" : "Donne un coup de cœur aux œuvres du musée.",
                action: kind === "joueurs" ? `<a class="btn btn-primary btn-px btn-sm" href="/ace">Aller dessiner</a>` : `<a class="btn btn-sm" href="/musee">Visiter le musée</a>`})}
    </div>`;
    page.removeAttribute("aria-busy");
    fillIcons(page);
}

async function load() {
    const data = await fetch(`/api/leaderboard?kind=${kind}&period=${period}`, {cache: "no-store"}).then(r => r.json()).catch(() => ({entries: []}));
    render(data);
}

async function main() {
    me = (await mountShell({active: "classement"})).user;
    $("#page").addEventListener("click", e => {
        const k = e.target.closest("[data-kind]");
        if (k) { kind = k.dataset.kind; load(); }
        const p = e.target.closest("[data-period]");
        if (p) { period = p.dataset.period; load(); }
    });
    load();
}

main();

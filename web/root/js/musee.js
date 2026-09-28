// /musee — the public museum: the artwork on show, rooms, exhibited artworks (m-musee, d-musee).

import {$, $$, avatarHTML, escapeHTML, fillIcons, icon, toast} from "./ui.js";
import {mountShell, emptyState, loginHref} from "./shell.js";
import {artOf, artFit, hugFrame, hugRatio, frameClass, authorsOf, dateFR, SALLE_LABELS} from "./common.js";
import {formatNumber} from "./view.js";

let me = null, data = null, salle = "coups-de-coeur";

function inRoom(o) {
    const e = o.extra?.exposition;
    if (salle === "coups-de-coeur") return (o.extra?.likes ?? 0) > 0;
    if (salle === "pionnieres") return o.origine === "avant_migration";
    return e?.salle === salle;
}

function work(o) {
    const e = o.extra?.exposition;
    return `<a class="mu-work" href="/oeuvre/${o.id}">
        <div class="${frameClass(e)}"><div class="passe"><div class="mu-img velours"><img class="px art" src="${artOf(o, 260)}" style="${artFit(e?.marge ?? 75)}" alt="" loading="lazy"></div></div></div>
        <div class="mu-cartel"><span class="fs-small b ellip">${escapeHTML(o.titre)}</span><span class="fs-cap t3 ellip">${escapeHTML(authorsOf(o))}</span>
        <span class="fs-cap t3 mu-likes"><img class="px" src="/img/badges/coeur.png" alt="" width="14" height="14">${o.extra?.likes ?? 0}</span></div>
    </a>`;
}

function render() {
    const page = $("#page");
    const a = data.affiche;
    let works = data.oeuvres.filter(inRoom);
    if (salle === "coups-de-coeur") works = works.sort((x, y) => (y.extra.likes ?? 0) - (x.extra.likes ?? 0));
    const ae = a?.extra?.exposition;
    const affiche = a ? `<section class="mu-affiche">
        <a class="cadre-or mu-affiche-frame" href="/oeuvre/${a.id}" style="${hugFrame(a, 460)}"><div class="mu-affiche-img velours" style="aspect-ratio: ${hugRatio(a)}"><img class="px art" src="${artOf(a, 720)}" style="${artFit(ae?.marge ?? 88)}" alt="${escapeHTML(a.titre)}"></div></a>
        <div class="mu-affiche-text">
            <span class="fs-over tgold">À l'affiche cette semaine</span>
            <h2 class="fs-display mu-affiche-title">${escapeHTML(a.titre)}</h2>
            <div class="mu-by">${avatarHTML(a.auteurs[0]?.user, "av av-32")}<span class="fs-body">${a.auteurs.map(x => `<a href="/u/${escapeHTML(x.user?.slug ?? "")}">${escapeHTML(x.user?.pseudo ?? "")}</a>`).join(" et ")}
                <span class="t3">· ${a.apparue_le ? dateFR(a.apparue_le) + " · " : ""}${a.w} × ${a.h} · salle ${escapeHTML(SALLE_LABELS[ae?.salle] ?? "")}</span></span></div>
            ${ae?.mot ? `<p class="fs-body t2 pretty mu-mot">« ${escapeHTML(ae.mot)} »</p>` : ""}
            <div class="mu-actions">
                <a class="btn btn-primary btn-lg btn-px" href="/musee/visite">${icon("play", 18)}Visite guidée</a>
                <a class="btn btn-lg" href="/oeuvre/${a.id}#construction">Rejouer la construction</a>
                <button class="btn btn-lg js-like" type="button" data-id="${a.id}" aria-pressed="${!!a.extra.liked}" aria-label="Coup de cœur"><img class="px" src="/img/badges/coeur.png" alt="" width="20" height="20"><span class="mono">${a.extra.likes}</span></button>
            </div>
        </div>
    </section>` : emptyState({title: "Le musée ouvre bientôt ses portes", text: "Les œuvres revendiquées et validées peuvent être exposées par leurs artistes.", mascot: true,
        action: me ? `<a class="btn btn-gold btn-px" href="/musee/exposer">Exposer une œuvre</a>` : ""});
    const players = data.musees?.length ? `<section class="mu-players"><div class="sec-head"><h2 class="fs-h2">Musées des joueurs</h2><a class="fs-small" href="/musees">Tous les musées</a></div>
        <div class="mu-players-list scroll-x">${data.musees.slice(0, 8).map(m => `<a class="mu-player" href="/u/${escapeHTML(m.user.slug)}/musee" style="background: ${escapeHTML(m.fond)}">
            <div class="mu-player-top">${avatarHTML(m.user, "av av-40")}${m.musique ? `<span class="eq" style="color: ${escapeHTML(m.accent)}"><span></span><span></span><span></span><span></span></span>` : ""}</div>
            <div class="mu-player-bottom"><span class="fs-small b">${escapeHTML(m.nom)}</span><span class="fs-cap">${escapeHTML(m.sub)}</span></div></a>`).join("")}</div></section>` : "";
    page.innerHTML = `<div class="mu-wrap">
        <header class="mu-head"><div><h1 class="fs-h1">Le Musée</h1><p class="fs-small t2 pretty">Les plus belles œuvres du place, exposées par leurs artistes.</p></div>
            <div class="mu-head-actions">${me ? `<a class="btn btn-sm only-d" href="/moi/musee">Mon musée</a><a class="btn btn-sm btn-gold btn-px" href="/musee/exposer">Exposer une œuvre</a>` : ""}
            <a class="btn btn-icon btn-round only-m" href="/musee/visite" aria-label="Lancer la visite guidée">${icon("play", 18)}</a></div></header>
        ${affiche}
        <section class="mu-rooms"><h2 class="fs-h2">Les salles</h2>
            <div class="mu-chips scroll-x">${data.salles.map(s => `<button type="button" class="chip${s.id === salle ? " chip-on" : ""}" data-salle="${s.id}">${escapeHTML(s.label)} <span class="mono" style="opacity: .7">${s.n}</span></button>`).join("")}</div>
            ${works.length ? `<div class="mu-grid">${works.map(work).join("")}</div>`
                : emptyState({title: "Cette salle est encore vide", text: "Les œuvres du canvas n'attendent que leurs artistes.", action: me ? `<a class="btn btn-sm" href="/musee/exposer">Exposer une œuvre ici</a>` : `<a class="btn btn-sm" href="${loginHref()}">Se connecter</a>`})}
        </section>
        ${players}
        <section class="card mu-cta"><img class="px" src="/img/farwest-x8.png" alt="" width="36" height="62"><div class="grow"><span class="fs-body b">Toi aussi, expose !</span><br>
            <span class="fs-small t2">Revendique une œuvre d'avant les comptes, puis choisis sa salle et son cadre.</span></div>
            <a class="btn btn-sm btn-gold btn-px" href="${me ? (data.miennes?.length ? "/musee/exposer" : "/revendiquer") : loginHref()}">${data.miennes?.length ? "Exposer" : "Revendiquer"}</a></section>
    </div>`;
    page.removeAttribute("aria-busy");
    fillIcons(page);
}

async function main() {
    me = (await mountShell({active: "musee"})).user;
    data = await fetch("/api/musee", {cache: "no-store"}).then(r => r.json());
    if (!data.oeuvres.some(o => (o.extra?.likes ?? 0) > 0)) salle = data.salles.find(s => s.n > 0 && s.id !== "coups-de-coeur")?.id ?? "coups-de-coeur";
    render();
    $("#page").addEventListener("click", async e => {
        const s = e.target.closest("[data-salle]");
        if (s) { salle = s.dataset.salle; render(); return; }
        const l = e.target.closest(".js-like");
        if (l) {
            if (!me) { location.href = loginHref(); return; }
            const on = l.getAttribute("aria-pressed") !== "true";
            const r = await fetch(`/api/oeuvres/${l.dataset.id}/like`, {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({on})}).then(r => r.json());
            l.setAttribute("aria-pressed", String(on));
            l.querySelector(".mono").textContent = r.likes;
        }
    });
}

main();

// /oeuvre/:id — an artwork: frame, cartel, before/after, construction replay, history (m-oeuvre, d-oeuvre).

import {openReport, reportButtonHTML} from "./report.js";
import {$, $$, avatarHTML, copyText, escapeHTML, fillIcons, icon, toast} from "./ui.js";
import {mountShell, emptyState, loginHref} from "./shell.js";
import {cropOf, frameClass, dateFR, artView, SALLE_LABELS} from "./common.js";
import {formatNumber} from "./view.js";

const BADGES = {pionnier: ["Pionnier", "st-attente"], veteran: ["Vétéran 2022", ""], batisseur: ["Bâtisseur", "st-neutre"], indemodable: ["Indémodable", "st-valide"]};
const id = location.pathname.split("/")[2];
let me = null;

// Same box as the server's cropBox (the zone and a small margin).
function box(o, W = 1024, H = 768) {
    const m = Math.max(2, Math.min(8, Math.floor(Math.max(o.w - 1, o.h - 1) / 6)));
    const x0 = Math.max(0, o.x - m), y0 = Math.max(0, o.y - m);
    const x1 = Math.min(W, o.x + o.w + m), y1 = Math.min(H, o.y + o.h + m);
    return {x: x0, y: y0, w: x1 - x0, h: y1 - y0};
}

const dmy = ms => new Date(ms).toLocaleDateString("fr-FR", {day: "2-digit", month: "2-digit", year: "2-digit"});

async function main() {
    me = (await mountShell({active: "musee"})).user;
    const page = $("#page");
    const r = await fetch("/api/oeuvres/" + id, {cache: "no-store"});
    if (!r.ok) {
        page.innerHTML = `<div class="page-pad">${emptyState({title: "Œuvre introuvable", text: "Elle a peut-être été retirée.", mascot: true, action: `<a class="btn" href="/musee">Retour au musée</a>`})}</div>`;
        return;
    }
    const d = await r.json();
    const o = d.oeuvre, e = o.extra?.exposition, a = d.analyse;
    document.title = `${o.titre} · BetterPlace`;
    const b = box(o);
    const z = Math.max(1, Math.min(16, Math.floor(480 / Math.max(b.w, b.h))));
    const mainAuthor = o.auteurs[0]?.user;
    const quote = e?.mot || d.message;
    const view = artView(o);

    const contrib = d.contributeurs.map(c => `<div class="oe-contrib">${avatarHTML(c.user, "av av-32")}<span class="fs-small"><a href="/u/${escapeHTML(c.user?.slug ?? "")}"><b>${escapeHTML(c.user?.pseudo ?? "")}</b></a>
        <span class="t3">· ${c.role === "auteur" ? "auteur" : c.role === "co_auteur" ? "co-auteur" : `a retouché ${formatNumber(c.pixels)} pixel${c.pixels > 1 ? "s" : ""}`}${c.pixels_pionniers ? ` · ${formatNumber(c.pixels_pionniers)} pixels pionniers (est.)` : ""}</span></span></div>`).join("");

    page.innerHTML = `<div class="oe-wrap">
        <nav class="oe-crumbs fs-small" aria-label="Fil d'Ariane"><a href="/musee">Musée</a>${e ? ` <span class="t3">/</span> <a href="/musee">${escapeHTML(SALLE_LABELS[e.salle] ?? "")}</a>` : ""} <span class="t3">/</span> <span class="t2">${escapeHTML(o.titre)}</span></nav>
        <section class="oe-top">
            <div class="${e ? frameClass(e) : "cadre-or"} oe-frame"><div class="passe"><div class="oe-img"><img class="px" src="${cropOf(o, 720, e?.marge ?? 80)}" alt="${escapeHTML(o.titre)}"></div></div></div>
            <div class="oe-info">
                <div class="oe-badges">${d.badges.map(x => BADGES[x] ? `<span class="pill ${BADGES[x][1]}"><img src="/img/badges/${x}.png" alt="" class="px" width="14" height="14">${BADGES[x][0]}</span>` : "").join("")}
                    ${o.extra?.retouchee ? `<span class="pill st-info">Retouchée · ${Math.round(o.extra.intact_pct)} % intacte</span>` : ""}</div>
                <h1 class="fs-display oe-title">${escapeHTML(o.titre)}</h1>
                <div class="oe-by">${avatarHTML(mainAuthor, "av av-32")}<span class="fs-body">${o.auteurs.map(x => `<a href="/u/${escapeHTML(x.user?.slug ?? "")}">${escapeHTML(x.user?.pseudo ?? "")}</a>`).join(" et ")}
                    ${e ? `<span class="t3">· salle ${escapeHTML(SALLE_LABELS[e.salle] ?? "")} · exposée depuis le ${dateFR(e.ts)}</span>` : `<span class="t3">· pas encore exposée</span>`}</span></div>
                ${quote ? `<p class="fs-body t2 pretty">« ${escapeHTML(quote)} »</p>` : ""}
                <div class="oe-kpis">
                    <div class="card kpi"><span class="fs-over">Pixels</span><span class="kpi-v">${formatNumber(a?.pixels_poses_estimes || o.pixels)}</span></div>
                    <div class="card kpi"><span class="fs-over">Zone</span><span class="kpi-v">${o.w}×${o.h}</span></div>
                    <div class="card kpi"><span class="fs-over">Apparue</span><span class="kpi-v">${o.apparue_le ? dmy(o.apparue_le) : "—"}</span></div>
                    <div class="card kpi"><span class="fs-over">Coups de cœur</span><span class="kpi-v js-likes">${o.extra?.likes ?? 0}</span></div>
                </div>
                <div class="oe-actions">
                    <a class="btn btn-primary btn-px" href="${view}">${icon("target", 18)}Voir sur le canvas · x ${o.x} y ${o.y}</a>
                    <button class="btn js-like" type="button" aria-pressed="${!!o.extra?.liked}"><img class="px" src="/img/badges/coeur.png" alt="" width="18" height="18">Coup de cœur</button>
                    <button class="btn btn-icon js-share" type="button" aria-label="Partager">${icon("link", 18)}</button>
                    ${d.auteur ? "" : reportButtonHTML("Signaler le titre de l'œuvre")}
                </div>
                ${d.auteur ? `<div class="oe-actions">
                    <a class="btn btn-sm btn-gold btn-px" href="/musee/exposer?oeuvre=${o.id}">${e ? "Modifier l'exposition" : "Exposer au musée"}</a>
                    ${o.extra?.retouchee ? `<a class="btn btn-sm" href="${view}&modele=oeuvre:${o.id}">${icon("blueprint", 16)}Restaurer avec le modèle</a>` : ""}
                </div>` : `<a class="btn btn-sm btn-ghost oe-blueprint" href="${view}&modele=oeuvre:${o.id}">${icon("blueprint", 16)}Utiliser comme modèle</a>`}
                <div class="oe-contribs"><span class="fs-over">Contributeurs</span>${contrib}</div>
            </div>
        </section>
        <section class="oe-bottom">
            ${d.sauvegardes && a ? `<div class="oe-col">
                <div class="sec-head"><h2 class="fs-h2">Avant / après</h2><span class="fs-cap t3 js-ba-dates">glisse pour comparer</span></div>
                <div class="card-2 oe-ba" style="aspect-ratio: ${b.w} / ${b.h}">
                    <img class="px oe-ba-after" src="/api/crop.png?x=${b.x}&y=${b.y}&w=${b.w}&h=${b.h}&z=${z}" alt="Aujourd'hui">
                    <div class="oe-ba-before js-ba-before"><img class="px js-before" alt="Avant" style="width: ${100 * 1}%"></div>
                    <div class="oe-ba-line js-ba-line"></div>
                    <span class="pill oe-ba-pill oe-ba-pill-r">aujourd'hui</span><span class="pill oe-ba-pill oe-ba-pill-l js-before-date"></span>
                </div>
                <input type="range" min="0" max="100" value="50" class="oe-range js-ba" aria-label="Comparer avant et après">
            </div>
            <div class="oe-col" id="construction">
                <div class="sec-head"><h2 class="fs-h2">Rejouer la construction</h2><span class="fs-cap t3">captures réelles du canvas</span></div>
                <div class="card oe-replay"><div class="oe-sprite js-sprite" style="aspect-ratio: ${b.w} / ${b.h}"></div>
                    <button class="btn btn-icon btn-round glass oe-replay-btn js-replay" type="button" aria-label="Pause">${icon("pause", 18)}</button>
                    <span class="pill oe-replay-date js-replay-date"></span></div>
                <div class="fs-cap t3 mono oe-replay-dates js-replay-dates"></div>
                <h2 class="fs-h2">Histoire</h2>
                <div class="oe-history">${d.histoire.map(h => `<div class="oe-hist"><span class="mono fs-cap t3">${escapeHTML(h.label)}</span><span class="fs-small">${escapeHTML(h.texte)}</span></div>`).join("") || `<span class="fs-small t3">Pas encore d'histoire.</span>`}</div>
            </div>` : `<div class="oe-col"><h2 class="fs-h2">Histoire</h2><div class="oe-history">${d.histoire.map(h => `<div class="oe-hist"><span class="mono fs-cap t3">${escapeHTML(h.label)}</span><span class="fs-small">${escapeHTML(h.texte)}</span></div>`).join("") || `<span class="fs-small t3">Les sauvegardes ne sont pas disponibles sur ce serveur.</span>`}</div></div>`}
        </section>
    </div>`;
    page.removeAttribute("aria-busy");
    fillIcons(page);

    // Before / after
    const before = $(".js-before");
    if (before) {
        const resp = await fetch(`/api/oeuvres/${o.id}/avant.png?z=${z}`);
        if (resp.ok) {
            const t = Number(resp.headers.get("X-Frame-Time"));
            before.src = URL.createObjectURL(await resp.blob());
            $(".js-before-date").textContent = dateFR(t);
            $(".js-ba-dates").textContent = `${dateFR(t)} → aujourd'hui · glisse pour comparer`;
        }
        const slide = () => {
            const v = $(".js-ba").value;
            $(".js-ba-before").style.clipPath = `inset(0 ${100 - v}% 0 0)`;
            $(".js-ba-line").style.left = v + "%";
        };
        $(".js-ba").addEventListener("input", slide);
        slide();
    }

    // Construction replay: a sprite animated frame by frame
    const sprite = $(".js-sprite");
    if (sprite) {
        const resp = await fetch(`/api/oeuvres/${o.id}/construction.png?n=16&z=${Math.max(1, Math.min(8, Math.floor(320 / Math.max(b.w, b.h))))}`);
        if (resp.ok) {
            const times = JSON.parse(resp.headers.get("X-Frame-Times") || "[]");
            const url = URL.createObjectURL(await resp.blob());
            const n = times.length;
            sprite.style.backgroundImage = `url(${url})`;
            sprite.style.backgroundSize = `${n * 100}% 100%`;
            let i = 0, playing = !matchMedia("(prefers-reduced-motion: reduce)").matches, timer = null;
            const show = () => {
                sprite.style.backgroundPosition = n > 1 ? `${i / (n - 1) * 100}% 0` : "0 0";
                $(".js-replay-date").textContent = dmy(times[i]);
            };
            const tick = () => { i = (i + 1) % n; show(); };
            const play = on => {
                playing = on;
                clearInterval(timer);
                if (on) timer = setInterval(tick, 600);
                $(".js-replay").innerHTML = icon(on ? "pause" : "play", 18);
                $(".js-replay").setAttribute("aria-label", on ? "Pause" : "Lecture");
            };
            const days = [...new Set(times.map(dmy))];
            $(".js-replay-dates").innerHTML = days.slice(0, 8).map(x => `<span>${x}</span>`).join("");
            show();
            play(playing);
            $(".js-replay").addEventListener("click", () => play(!playing));
        }
    }

    $(".js-like").addEventListener("click", async ev => {
        if (!me) { location.href = loginHref(); return; }
        const btn = ev.currentTarget;
        const on = btn.getAttribute("aria-pressed") !== "true";
        const res = await fetch(`/api/oeuvres/${o.id}/like`, {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({on})}).then(r => r.json());
        btn.setAttribute("aria-pressed", String(on));
        $(".js-likes").textContent = res.likes;
    });
    $(".js-report")?.addEventListener("click", () => openReport({me, cible: `oeuvre:${o.id}`, title: `Signaler « ${o.titre} »`, choices: [["oeuvre", "Le titre ou la description"]]}));
    $(".js-share").addEventListener("click", async () => {
        const url = location.origin + "/oeuvre/" + o.id;
        if (navigator.share && matchMedia("(pointer: coarse)").matches) return navigator.share({title: o.titre, url}).catch(() => {});
        const ok = await copyText(url);
        toast(`<span class="fs-small b">${ok ? "Lien de l'œuvre copié" : "Copie impossible"}</span>`);
    });
}

main();

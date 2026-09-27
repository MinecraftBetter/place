// /activite — live activity, the claims column and your alerts (m-activite, d-activite).

import {$, $$, avatarHTML, escapeHTML, fillIcons, icon, toast} from "./ui.js";
import {mountShell, emptyState, loginHref} from "./shell.js";
import {relativeTime, formatNumber} from "./view.js";
import {activityParts, ACTIVITY_FILTERS, claimThumb, authorsLine, statusPill} from "./common.js";
import {Connection} from "./net.js";

let me = null;
let items = [];
let hot = [];
let filter = "";
let view = new URLSearchParams(location.search).get("vue") === "alertes" ? "alertes" : "direct";
let alerts = [];
let unread = 0;
let online = 0;

function itemHTML(a) {
    const p = activityParts(a);
    const img = p.badge ? `<img class="px act-badge" src="${p.badge}" alt="">` : avatarHTML(p.img, "av av-32");
    return `<a class="act-item${p.hl ? " act-" + p.hl : ""} anim-feed" href="${escapeHTML(p.href)}" data-id="${a.id}">
        ${img}
        <span class="fs-small pretty act-text"><b>${escapeHTML(p.who)}</b> <span class="t2">${escapeHTML(p.what)}</span> ${p.target ? `<b>${escapeHTML(p.target)}</b>` : ""}</span>
        <span class="act-meta">${p.coord ? `<span class="coord">${p.coord}</span>` : ""}<span class="fs-cap t3 mono">${relativeTime(a.ts)}</span></span>
    </a>`;
}

function alertHTML(a) {
    const p = a.payload || {};
    let title = "", sub = "", href = "#", img = `<img class="px" src="/img/farwest-x8.png" alt="" width="20" height="34">`;
    switch (a.kind) {
        case "retouche":
            title = `${escapeHTML(p.by?.pseudo ?? "Quelqu'un")} a retouché « ${escapeHTML(p.titre)} »`;
            sub = `${formatNumber(p.count ?? 0)} pixel${p.count > 1 ? "s" : ""} · ${relativeTime(a.ts)}`;
            href = `/ace?alerte=${a.id}&x=${p.x + Math.floor(p.w / 2)}&y=${p.y + Math.floor(p.h / 2)}&z=${Math.max(2, Math.min(16, Math.floor(300 / Math.max(p.w, p.h))))}`;
            img = avatarHTML(p.by, "av av-40");
            break;
        case "claim_validee":
            title = `« ${escapeHTML(p.titre)} » est validée !`;
            sub = `${(p.badges ?? []).length ? "Badges : " + p.badges.join(", ") + " · " : ""}${p.pionniers ? `+${formatNumber(p.pionniers)} px pionniers · ` : ""}${relativeTime(a.ts)}`;
            href = `/oeuvre/${p.oeuvre}`;
            img = `<img class="px" src="/img/badges/pionnier.png" alt="" width="40" height="40">`;
            break;
        case "claim_refusee":
            title = `« ${escapeHTML(p.titre)} » n'a pas été validée`;
            sub = `${escapeHTML(p.motif || "")} · ${relativeTime(a.ts)}`;
            href = "/revendications?statut=refusee";
            break;
        case "coauteur":
            title = `${escapeHTML(p.by?.pseudo ?? "")} t'a ajouté·e comme co-auteur`;
            sub = `« ${escapeHTML(p.titre)} » · à confirmer · ${relativeTime(a.ts)}`;
            href = "/revendications";
            img = avatarHTML(p.by, "av av-40");
            break;
        case "musee":
            title = escapeHTML(p.texte || "Nouveau musée");
            sub = relativeTime(a.ts);
            href = p.href || "/musee";
            break;
        default:
            title = escapeHTML(a.kind);
            sub = relativeTime(a.ts);
    }
    return `<a class="card-2 alert-item${a.lu ? "" : " unread"}" href="${escapeHTML(href)}">${img}<span class="alert-text"><span class="fs-small b">${title}</span><span class="fs-cap t3">${sub}</span></span>${a.lu ? "" : `<span class="live"></span>`}</a>`;
}

function renderList() {
    const box = $(".js-feed");
    if (!box) return;
    const kinds = filter ? filter.split(",") : null;
    const shown = kinds ? items.filter(a => kinds.includes(a.kind)) : items;
    box.innerHTML = shown.length ? shown.map(itemHTML).join("")
        : emptyState({title: "Calme plat pour l'instant", text: "Dès que quelqu'un dessine, ça apparaît ici en direct.", action: `<a class="btn btn-primary btn-px btn-sm" href="/ace">Aller dessiner</a>`});
}

function renderHot() {
    const box = $(".js-hot");
    if (!box) return;
    box.innerHTML = `<img class="px" src="/api/crop.png?x=0&y=0&w=1024&h=768&z=1" alt="Le canvas">` +
        hot.slice(0, 8).map(h => `<a class="hot-pin" href="/ace?x=${h.x}&y=${h.y}&z=8" style="left: ${h.x / 1024 * 100}%; top: ${h.y / 768 * 100}%" aria-label="${escapeHTML(h.who?.pseudo ?? "")} dessine ici"><span class="anim-ring-loop"></span><span></span></a>`).join("") +
        (hot.length ? "" : `<span class="fs-cap t3 hot-empty">Personne ne dessine en ce moment.</span>`);
}

async function renderClaims() {
    const box = $(".js-claims");
    if (!box) return;
    const j = await fetch("/api/claims").then(r => r.json()).catch(() => ({claims: []}));
    box.innerHTML = j.claims.length ? j.claims.slice(0, 6).map(c => `
        <a class="card act-claim" href="/revendications">
            <span class="act-claim-thumb"><img class="px" src="${claimThumb(c)}" alt="" loading="lazy"></span>
            <span class="act-claim-text"><span class="act-claim-title"><span class="fs-body b ellip">${escapeHTML(c.titre)}</span>${statusPill(c)}</span>
            <span class="fs-cap t2 ellip">${authorsLine(c, me?.id)} · ${relativeTime(c.cree_le)}</span>
            <span class="fs-cap t3 pretty">${escapeHTML(c.resume)}</span>
            <span class="fs-cap"><span class="tac b">+${c.confirmations}</span> <span class="t3">confirmations · ${c.doutes} doutes</span></span></span>
        </a>`).join("") : emptyState({title: "Aucune revendication en cours", text: "", action: `<a class="btn btn-sm btn-gold btn-px" href="/revendiquer">Revendiquer une œuvre</a>`});
}

function render() {
    const page = $("#page");
    const tabs = `<div class="seg act-tabs" role="tablist">
        <button type="button" class="${view === "direct" ? "on" : ""}" data-view="direct">En direct</button>
        <a href="/revendications">Revendications</a>
        <button type="button" class="${view === "alertes" ? "on" : ""}" data-view="alertes">Mes alertes${unread ? `<span class="count">${unread}</span>` : ""}</button>
    </div>`;
    const chips = `<div class="act-filters">${ACTIVITY_FILTERS.map(([k, l]) => `<button type="button" class="chip${k === filter ? " chip-on" : ""}" data-filter="${k}">${l}</button>`).join("")}</div>`;
    const alertsBox = !me ? emptyState({title: "Connecte-toi pour recevoir des alertes", text: "Quand quelqu'un retouche une de tes œuvres, tu le sais ici.", action: `<a class="btn btn-primary btn-px btn-sm" href="${loginHref()}">Se connecter</a>`})
        : alerts.length ? alerts.map(alertHTML).join("") : emptyState({title: "Aucune alerte", text: "Tout est calme sur tes œuvres."});
    page.innerHTML = `<div class="act-layout">
        <section class="act-main">
            <div class="act-head">
                <h1 class="fs-h1 only-m">Activité</h1><h1 class="fs-h2 only-d">En direct</h1>
                <span class="fs-cap b act-online"><span class="live"></span><span class="js-online">${online || "–"}</span> en ligne</span>
            </div>
            <div class="only-m">${tabs}</div>
            ${view === "direct" ? chips + `<div class="card act-feed js-feed"></div><p class="fs-cap t3 pretty">Un clic recentre le canvas à l'endroit concerné. Les petits pixels isolés sont regroupés pour ne pas noyer le fil.</p>`
                : `<div class="act-alerts">${alertsBox}</div>`}
        </section>
        <section class="act-claims-col only-d">
            <div class="act-head"><h2 class="fs-h2">Revendications</h2><a class="btn btn-sm btn-gold btn-px" href="/revendiquer">Revendiquer</a></div>
            <div class="act-claims js-claims"></div>
            <a class="btn btn-sm btn-ghost" href="/revendications">Tout le fil des revendications</a>
        </section>
        <aside class="act-side only-d">
            <div class="act-box"><h2 class="fs-h3">Où ça dessine</h2><div class="hot-map js-hot"></div></div>
            <div class="act-box"><h2 class="fs-h3">Tes alertes</h2>
                ${me ? (alerts.slice(0, 3).map(alertHTML).join("") || `<span class="fs-small t3">Aucune alerte pour l'instant.</span>`) + `
                <div class="card act-toggles"><div class="edit-toggle"><span class="fs-small grow">Quand une de mes œuvres est retouchée</span><button class="toggle js-t-retouche ${me.alertes_retouche !== false ? "toggle-on" : ""}" type="button" aria-label="Alerte de retouche"></button></div></div>`
                : `<a class="btn btn-sm btn-primary" href="${loginHref()}">Se connecter</a>`}
            </div>
        </aside>
    </div>`;
    page.removeAttribute("aria-busy");
    renderList();
    renderHot();
    renderClaims();
    fillIcons(page);
}

async function loadAlerts() {
    if (!me) return;
    const j = await fetch("/api/alerts", {cache: "no-store"}).then(r => r.json()).catch(() => ({alerts: []}));
    alerts = j.alerts ?? [];
    unread = j.unread ?? 0;
    me.alertes_retouche = j.settings?.retouche;
}

async function main() {
    me = (await mountShell({active: "activite"})).user;
    const j = await fetch("/api/activity", {cache: "no-store"}).then(r => r.json()).catch(() => ({items: [], hot: []}));
    items = j.items;
    hot = j.hot;
    await loadAlerts();
    render();
    if (view === "alertes" && unread) fetch("/api/alerts", {method: "POST"});

    $("#page").addEventListener("click", async e => {
        const f = e.target.closest("[data-filter]");
        if (f) { filter = f.dataset.filter; render(); return; }
        const v = e.target.closest("[data-view]");
        if (v) {
            view = v.dataset.view;
            history.replaceState(null, "", view === "alertes" ? "/activite?vue=alertes" : "/activite");
            if (view === "alertes" && unread) { fetch("/api/alerts", {method: "POST"}); }
            render();
            return;
        }
        if (e.target.closest(".js-t-retouche")) {
            const on = !e.target.closest(".js-t-retouche").classList.contains("toggle-on");
            await fetch("/api/me/profile", {method: "PATCH", headers: {"Content-Type": "application/json"}, body: JSON.stringify({alertes_retouche: on})});
            me.alertes_retouche = on;
            e.target.closest(".js-t-retouche").classList.toggle("toggle-on", on);
        }
    });

    // Live updates
    const proto = location.protocol === "https:" ? "wss://" : "ws://";
    const conn = new Connection(proto + location.host + "/ws");
    conn.addEventListener("activity", ev => {
        const a = ev.detail;
        const i = items.findIndex(x => x.id === a.id);
        if (i >= 0) items[i] = a; else items.unshift(a);
        items = items.slice(0, 120);
        if (a.kind === "pixels") {
            hot = [{x: a.x, y: a.y, n: a.n, who: a.who}, ...hot.filter(h => h.who?.id !== a.who?.id)].slice(0, 8);
            renderHot();
        }
        renderList();
    });
    conn.addEventListener("stat", ev => {
        online = ev.detail.online;
        for (const el of $$(".js-online")) el.textContent = online;
    });
    conn.addEventListener("alert", async () => {
        await loadAlerts();
        if (view === "alertes") render();
        toast(`<span class="fs-small b">Nouvelle alerte</span><a class="btn btn-sm" href="/activite?vue=alertes">Voir</a>`);
    });
    conn.connect();
    setInterval(renderList, 30000); // "dessine près de" → "a posé N pixels"
}

main();

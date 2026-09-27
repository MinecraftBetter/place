// /admin — the admin area: side navigation, router, shared helpers.

import {$, $$, avatarHTML, escapeHTML, fillIcons, icon, toggleTheme, currentTheme} from "./ui.js";
import {fetchMe, loginHref} from "./shell.js";
import {relativeTime} from "./view.js";

export const VIEWS = [
    {id: "", label: "Tableau de bord", icon: "activity", module: "admin-dashboard.js"},
    {id: "revendications", label: "Revendications", icon: "flag", module: "admin-claims.js", count: "claims"},
    {id: "utilisateurs", label: "Utilisateurs", icon: "users", module: "admin-users.js"},
    {id: "zone", label: "Outil zone", icon: "rect", module: "admin-zone.js"},
    {id: "moderation", label: "Modération", icon: "eye", module: "admin-moderation.js", count: "reports"},
    {id: "journal", label: "Journal", icon: "clock", module: "admin-journal.js"},
    {id: "reglages", label: "Réglages", icon: "settings", module: "admin-settings.js"},
];

export async function api(path, {method = "GET", body} = {}) {
    const r = await fetch(path, {method, headers: body ? {"Content-Type": "application/json"} : {}, body: body ? JSON.stringify(body) : undefined, cache: "no-store"});
    const j = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error(j.error || `Erreur ${r.status}`);
    return j;
}

export {escapeHTML, icon, avatarHTML, relativeTime};

let me = null;
let current = null;

function currentView() {
    const id = location.pathname.replace(/^\/admin\/?/, "").split("/")[0];
    return VIEWS.find(v => v.id === id) ?? VIEWS[0];
}

async function renderSide(counts = {}) {
    const side = $(".js-side");
    const v = currentView();
    let status = null;
    try { status = await api("/api/admin/status"); } catch { /* older server */ }
    side.innerHTML = `
        <div class="admin-brand"><a href="/home" class="brand"><img src="/img/cube-logo.png" alt="" width="26" height="26"><span class="pixel b">BetterPlace</span></a><span class="pill st-neutre">Admin</span>
            <button class="btn btn-icon btn-sm btn-ghost only-m js-side-toggle" type="button" aria-label="Menu" aria-expanded="false">${icon("grid", 18)}</button></div>
        <nav class="sidenav admin-nav" aria-label="Administration">
            ${VIEWS.map(x => `<a href="/admin${x.id ? "/" + x.id : ""}" class="${x === v ? "on" : ""}" ${x === v ? 'aria-current="page"' : ""}>${icon(x.icon, 18)}<span class="grow">${x.label}</span>${x.count && counts[x.count] ? `<span class="count">${counts[x.count]}</span>` : ""}</a>`).join("")}
        </nav>
        <div class="admin-side-foot">
            ${status ? `<div class="card-2 admin-server"><span class="fs-cap t2"><span class="live"></span> ${status.mode === "normal" ? "Serveur en ligne" : status.mode === "readonly" ? "Lecture seule" : "Maintenance"}</span>
                <span class="fs-cap t3 mono">${status.online} / ${status.slots} connexions · sauvegarde ${status.saved_at ? relativeTime(status.saved_at) : "—"}</span></div>` : ""}
            <div class="admin-me">${avatarHTML(me, "av av-32")}<span class="grow"><span class="fs-small b">${escapeHTML(me.pseudo)}</span><br><span class="fs-cap t3">admin</span></span>
                <button class="btn btn-sm btn-icon btn-ghost js-theme" type="button" aria-label="Changer de thème">${icon(currentTheme() === "clair" ? "moon" : "sun", 16)}</button>
                <a class="btn btn-sm btn-icon btn-ghost" href="/ace" aria-label="Retour au site">${icon("canvas", 16)}</a></div>
        </div>`;
    $(".js-theme", side).addEventListener("click", () => { toggleTheme(); renderSide(counts); });
    $(".js-side-toggle", side).addEventListener("click", e => {
        const open = side.classList.toggle("open");
        e.currentTarget.setAttribute("aria-expanded", String(open));
    });
}

export async function refreshCounts() {
    try {
        const s = await api("/api/admin/status");
        renderSide(s.counts ?? {});
    } catch {
        renderSide({});
    }
}

async function route() {
    const v = currentView();
    document.title = `${v.label} · Admin · BetterPlace`;
    const page = $("#page");
    page.setAttribute("aria-busy", "true");
    page.innerHTML = `<div class="admin-loading t3 fs-small">Chargement…</div>`;
    refreshCounts();
    try {
        const mod = await import("./" + v.module);
        if (current?.unmount) current.unmount();
        current = mod;
        await mod.mount(page, {me, api, refreshCounts});
    } catch (err) {
        console.error(err);
        page.innerHTML = `<div class="admin-pad"><div class="card empty-state"><span class="fs-body b">Cette page n'a pas pu s'afficher</span><span class="fs-small t2">${escapeHTML(err.message)}</span></div></div>`;
    }
    page.removeAttribute("aria-busy");
    fillIcons(page);
}

async function main() {
    const m = await fetchMe();
    if (!m.user) {
        location.replace(loginHref());
        return;
    }
    if (m.user.role !== "admin") {
        document.body.innerHTML = `<div class="admin-pad" style="max-width: 480px; margin: 80px auto"><div class="card empty-state"><img class="px" src="/img/farwest-x8.png" alt="" width="42" height="72">
            <span class="fs-body b">Réservé à l'équipe</span><span class="fs-small t2">Cette partie du site sert à modérer le canvas.</span><a class="btn btn-primary btn-px" href="/ace">Retour au canvas</a></div></div>`;
        return;
    }
    me = m.user;
    document.addEventListener("click", e => {
        const a = e.target.closest("a[href^='/admin']");
        if (!a || e.metaKey || e.ctrlKey || e.shiftKey || a.target) return;
        e.preventDefault();
        history.pushState(null, "", a.getAttribute("href"));
        $(".js-side").classList.remove("open");
        route();
    });
    window.addEventListener("popstate", route);
    route();
}

main();

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

// ------------------------------------------------------------------
// Charts (dataviz: thin columns with a 4px rounded end, 2px gaps, hairline grid,
// a tooltip per bar, labels in text colours, a hidden table for screen readers).

export function columnChart(el, points, {height = 160, format = v => String(v), labelEvery = 1, caption = ""} = {}) {
    el.textContent = "";
    el.classList.add("vz");
    const max = Math.max(1, ...points.map(p => p.value));
    const step = niceStep(max);
    const top = Math.ceil(max / step) * step;
    const wrap = document.createElement("div");
    wrap.className = "vz-plot";
    wrap.style.height = height + "px";
    const axis = document.createElement("div");
    axis.className = "vz-axis mono fs-cap t3";
    for (let v = top; v >= 0; v -= step) {
        const t = document.createElement("span");
        t.textContent = format(v);
        axis.appendChild(t);
        const g = document.createElement("div");
        g.className = "vz-grid";
        g.style.bottom = (v / top * 100) + "%";
        wrap.appendChild(g);
    }
    const bars = document.createElement("div");
    bars.className = "vz-bars";
    const tip = document.createElement("div");
    tip.className = "vz-tip toast";
    tip.hidden = true;
    const tipV = document.createElement("span"), tipL = document.createElement("span");
    tipV.className = "mono fs-small b";
    tipL.className = "fs-cap t3";
    tip.append(tipV, tipL);
    const maxIdx = points.reduce((m, p, i) => p.value > points[m].value ? i : m, 0);
    points.forEach((p, i) => {
        const slot = document.createElement("div");
        slot.className = "vz-slot";
        slot.tabIndex = 0;
        slot.setAttribute("aria-label", `${p.label} : ${format(p.value)}`);
        const bar = document.createElement("div");
        bar.className = "vz-bar";
        bar.style.height = (p.value / top * 100) + "%";
        if (p.value > 0 && i === maxIdx) {
            const lab = document.createElement("span");
            lab.className = "vz-label mono fs-cap t2";
            lab.textContent = format(p.value);
            bar.appendChild(lab);
        }
        slot.appendChild(bar);
        const show = () => {
            tipV.textContent = format(p.value);
            tipL.textContent = p.tip ?? p.label;
            tip.hidden = false;
            const r = slot.getBoundingClientRect(), w = wrap.getBoundingClientRect();
            tip.style.left = Math.min(w.width - 140, Math.max(0, r.left - w.left + r.width / 2 - 60)) + "px";
        };
        slot.addEventListener("pointerenter", show);
        slot.addEventListener("focus", show);
        slot.addEventListener("pointerleave", () => { tip.hidden = true; });
        slot.addEventListener("blur", () => { tip.hidden = true; });
        bars.appendChild(slot);
    });
    wrap.append(bars, tip);
    const xl = document.createElement("div");
    xl.className = "vz-x mono fs-cap t3";
    points.forEach((p, i) => {
        const s = document.createElement("span");
        s.textContent = i % labelEvery === 0 ? (p.short ?? p.label) : "";
        xl.appendChild(s);
    });
    const body = document.createElement("div");
    body.className = "vz-body";
    const main = document.createElement("div");
    main.className = "vz-main";
    main.append(wrap, xl);
    body.append(axis, main);
    // the same data as a table, for screen readers
    const table = document.createElement("table");
    table.className = "sr-only";
    if (caption) { const c = document.createElement("caption"); c.textContent = caption; table.appendChild(c); }
    for (const p of points) {
        const tr = document.createElement("tr");
        const a = document.createElement("th"), b = document.createElement("td");
        a.textContent = p.label;
        b.textContent = format(p.value);
        tr.append(a, b);
        table.appendChild(tr);
    }
    el.append(body, table);
}

function niceStep(max) {
    const raw = max / 3;
    const pow = Math.pow(10, Math.floor(Math.log10(raw)));
    const n = raw / pow;
    return (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * pow;
}

export function barRows(el, rows, {format = v => String(v)} = {}) {
    // horizontal bars: label, track, value (top lists, colours)
    const max = Math.max(1, ...rows.map(r => r.value));
    el.textContent = "";
    for (const r of rows) {
        const row = document.createElement(r.href ? "a" : "div");
        row.className = "vz-row";
        if (r.href) row.href = r.href;
        if (r.lead) row.insertAdjacentHTML("afterbegin", r.lead); // trusted markup (avatar / swatch built by us)
        const name = document.createElement("span");
        name.className = "fs-small ellip vz-row-name";
        name.textContent = r.label;
        const track = document.createElement("span");
        track.className = "bar-track vz-track";
        const fill = document.createElement("span");
        fill.className = "bar-fill";
        fill.style.width = (r.value / max * 100) + "%";
        if (r.color) fill.style.background = r.color;
        track.appendChild(fill);
        const val = document.createElement("span");
        val.className = "mono fs-cap t2 vz-row-val";
        val.textContent = format(r.value);
        row.append(name, track, val);
        el.appendChild(row);
    }
}

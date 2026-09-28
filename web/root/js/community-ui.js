// /ace — community bits: the live panel, alert counts, the « Ton œuvre a bougé » sheet.

import {$, $$, avatarHTML, escapeHTML, hideSheet, showSheet, storedGet, storedSet, toast} from "./ui.js";
import {alertText, notifyDevice, deviceSupported, deviceOn, toggleDevice} from "./notify.js";
import {activityParts} from "./common.js";
import {relativeTime, formatNumber} from "./view.js";

export function setupCommunity(app) {
    let items = [];
    let unread = 0;

    // ----- live panel
    const panel = $("#d-live");
    const list = $(".js-live-list");
    if (storedGet("bp-live-collapsed", false)) panel.classList.add("collapsed");
    $(".js-live-toggle").addEventListener("click", () => {
        const c = panel.classList.toggle("collapsed");
        storedSet("bp-live-collapsed", c);
        $(".js-live-toggle").setAttribute("aria-expanded", String(!c));
    });
    const renderList = () => {
        list.innerHTML = items.length ? items.slice(0, 8).map(a => {
            const p = activityParts(a);
            const img = p.badge ? `<img class="px" src="${p.badge}" alt="">` : avatarHTML(p.img, "av av-24");
            return `<a class="live-item${p.hl ? " act-" + p.hl : ""}" href="${escapeHTML(p.href)}" data-x="${a.x}" data-y="${a.y}" data-kind="${a.kind}">
                ${img}<span class="fs-cap grow"><b>${escapeHTML(p.who)}</b> <span class="t2">${escapeHTML(p.what)}</span>${p.target ? ` <b>${escapeHTML(p.target)}</b>` : p.coord ? ` <span class="mono">${p.coord}</span>` : ""}</span>
                <span class="fs-cap t3 mono">${relativeTime(a.ts).replace("il y a ", "").replace("à l'instant", "0 s")}</span></a>`;
        }).join("") : `<span class="fs-cap t3 live-empty">Personne ne dessine pour l'instant. Lance-toi !</span>`;
    };
    // pixel lines recentre the canvas instead of reloading the page
    list.addEventListener("click", e => {
        const a = e.target.closest(".live-item");
        if (!a || !["pixels", "retouche"].includes(a.dataset.kind) || !app.loaded) return;
        e.preventDefault();
        app.goTo({x: Number(a.dataset.x), y: Number(a.dataset.y)});
    });
    fetch("/api/activity").then(r => r.json()).then(j => { items = j.items; renderList(); }).catch(() => {});
    setInterval(renderList, 20000);

    // ----- alerts
    const renderCount = () => {
        for (const el of $$(".js-alert-count")) {
            el.textContent = unread;
            el.hidden = !unread;
        }
        for (const b of $$(".me-button")) {
            let c = b.querySelector(".count");
            if (!c) { c = document.createElement("span"); c.className = "count"; b.style.position = "relative"; b.appendChild(c); }
            c.textContent = unread;
            c.hidden = !unread;
        }
    };

    const wire = conn => {
        conn.addEventListener("activity", ev => {
            const a = ev.detail;
            const k = items.findIndex(x => x.id === a.id);
            if (k >= 0) items[k] = a; else items.unshift(a);
            items = items.slice(0, 40);
            renderList();
        });
        conn.addEventListener("alert", ev => {
            const a = ev.detail;
            unread++;
            renderCount();
            const txt = alertText(a.kind, a);
            notifyDevice(txt.title, txt.body, txt.href, a.id); // the tab is in the background
            if (a.kind === "retouche") {
                toast(`${avatarHTML(a.by, "av av-24")}<span class="toast-text"><span class="fs-small b">${escapeHTML(a.by?.pseudo ?? "")} retouche « ${escapeHTML(a.titre)} »</span><span class="fs-cap t3">${formatNumber(a.count)} pixels</span></span>
                    <button class="btn btn-sm" type="button" data-open-alert="${a.id}">Voir</button>`, {timeout: 8000});
            } else if (a.kind === "claim_validee") {
                toast(`<img class="px anim-bob" src="/img/badges/pionnier.png" alt="" width="28" height="28"><span class="toast-text"><span class="fs-small b">« ${escapeHTML(a.titre)} » est validée !</span><span class="fs-cap t3">Ton œuvre a son cartel.</span></span><a class="btn btn-sm" href="/oeuvre/${a.oeuvre}">Voir</a>`, {timeout: 8000});
            } else if (a.kind === "livre_or") {
                toast(`${avatarHTML(a.by, "av av-24")}<span class="toast-text"><span class="fs-small b">${escapeHTML(a.by?.pseudo ?? "")} a signé ton livre d'or</span><span class="fs-cap t3">« ${escapeHTML(a.texte ?? "")} »</span></span><a class="btn btn-sm" href="${escapeHTML(a.href ?? "/moi/musee")}">Voir</a>`, {timeout: 8000});
            } else if (a.kind !== "badge_oeuvre") { // the "badge" message already shows a toast
                toast(`<span class="fs-small b">Nouvelle alerte</span><a class="btn btn-sm" href="/activite?vue=alertes">Voir</a>`);
            }
        });
    };
    document.addEventListener("click", e => {
        const b = e.target.closest("[data-open-alert]");
        if (b) openAlert(Number(b.dataset.openAlert));
    });

    // ----- « Ton œuvre a bougé »
    async function openAlert(id) {
        const j = await fetch("/api/alerts").then(r => r.json()).catch(() => null);
        const al = j?.alerts?.find(x => x.id === id);
        if (!al || al.kind !== "retouche") return;
        fetch("/api/alerts", {method: "POST"});
        unread = 0;
        renderCount();
        const p = al.payload;
        app.goTo({x: p.x + Math.floor(p.w / 2), y: p.y + Math.floor(p.h / 2)});
        if (app.gl.getZoom() < 4) { app.gl.setZoom(Math.max(4, Math.min(16, 300 / Math.max(p.w, p.h)))); app.viewChanged(); }
        const sheet = $("#sheet-alert");
        sheet.innerHTML = `<div class="grab"></div>
            <h2 class="fs-h1">Ton œuvre a bougé</h2>
            <p class="fs-small t2 pretty"><b>${escapeHTML(p.by?.pseudo ?? "Quelqu'un")}</b> a retouché <b>${formatNumber(p.count)} pixels</b> de « ${escapeHTML(p.titre)} » ${relativeTime(al.ts)}. Une blague, une collab ? À toi de voir.</p>
            <div class="insp-actions"><button class="btn btn-primary btn-px js-retouch" type="button">Retoucher</button><button class="btn js-keep" type="button">Je garde, c'est drôle</button></div>
            <p class="fs-cap t3 pretty">« Retoucher » te laisse devant ton œuvre pour remettre ses pixels : sa page montre l'avant et l'après. « Je garde » met à jour ton œuvre avec l'ajout.</p>
            <a class="btn btn-ghost btn-sm" href="/oeuvre/${p.oeuvre}">Voir l'avant / après</a>
            <a class="btn btn-ghost btn-sm" href="/activite?vue=alertes">Réglages des alertes</a>`;
        sheet.onclick = async e => {
            if (e.target.closest(".js-retouch")) {
                hideSheet();
                toast(`<span class="toast-text"><span class="fs-small b">À toi de jouer</span><span class="fs-cap t3">Remets les pixels de « ${escapeHTML(p.titre)} ».</span></span>`);
            }
            if (e.target.closest(".js-keep")) {
                const r = await fetch(`/api/oeuvres/${p.oeuvre}/keep`, {method: "POST"});
                if (r.ok) {
                    e.target.closest(".js-keep").textContent = "Gardé !";
                    setTimeout(hideSheet, 900);
                }
            }
        };
        showSheet(sheet);
    }

    // notifications of the device, from the account menu
    const nb = $(".js-notif");
    if (nb && deviceSupported()) {
        const label = () => { nb.setAttribute("aria-pressed", String(deviceOn())); $(".js-notif-label", nb).textContent = deviceOn() ? "Notifications de l'appareil : oui" : "Notifications de l'appareil"; };
        nb.hidden = false;
        label();
        nb.addEventListener("click", async () => { await toggleDevice(); label(); });
    }

    // Called once the canvas is ready.
    app.onLoaded.push(async () => {
        if (app.conn) wire(app.conn);
        if (app.me) {
            unread = app.meAlerts ?? 0;
            renderCount();
        }
        const q = new URLSearchParams(location.search);
        const alertID = Number(q.get("alerte"));
        if (alertID) openAlert(alertID);
    });
}

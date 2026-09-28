// Notifications on the screen, on every page: a new alert (your artwork retouched,
// a claim decided, your guestbook signed, a badge from the backups, a message of the
// team…) pops up as a toast; with the permission, also as a notification of the device
// while the tab is in the background. The canvas gets them live (WebSocket), the other
// pages ask every 30 seconds.

import {avatarHTML, escapeHTML, toast} from "./ui.js";

const DEVICE_KEY = "bp-notif-appareil";

// what an alert says: {title, body, href, img} (text, not HTML, except img)
export function alertText(kind, p = {}) {
    const by = p.by?.pseudo ?? "Quelqu'un";
    switch (kind) {
        case "retouche": return {title: `${by} a retouché « ${p.titre ?? ""} »`, body: `${p.count ?? ""} pixels`, href: p.x !== undefined ? `/ace?x=${p.x + Math.floor((p.w ?? 0) / 2)}&y=${p.y + Math.floor((p.h ?? 0) / 2)}&z=8` : "/activite?vue=alertes", img: avatarHTML(p.by, "av av-24")};
        case "claim_validee": return {title: `« ${p.titre ?? ""} » est validée !`, body: "Ton œuvre a son cartel.", href: p.oeuvre ? `/oeuvre/${p.oeuvre}` : "/revendications", img: `<img class="px" src="/img/badges/pionnier.png" alt="" width="24" height="24">`};
        case "claim_refusee": return {title: `« ${p.titre ?? ""} » n'a pas été validée`, body: p.motif ?? "", href: "/revendications?statut=refusee"};
        case "coauteur": return {title: `${by} t'a ajouté·e comme co-auteur`, body: `« ${p.titre ?? ""} » · à confirmer`, href: "/revendications", img: avatarHTML(p.by, "av av-24")};
        case "livre_or": return {title: `${by} a signé ton livre d'or`, body: `« ${p.texte ?? ""} »`, href: p.href ?? "/moi/musee", img: avatarHTML(p.by, "av av-24")};
        case "badge_oeuvre": return {title: `Nouveau badge : ${p.nom ?? ""}`, body: `grâce à « ${p.titre ?? ""} »`, href: p.oeuvre ? `/oeuvre/${p.oeuvre}` : "/moi/profil", img: p.sprite ? `<img class="px" src="${escapeHTML(p.sprite)}" alt="" width="24" height="24">` : ""};
        case "moderation": return {title: "L'équipe a masqué un contenu", body: p.texte ?? "", href: "/moi/profil"};
        case "message_equipe": return {title: "Message de l'équipe", body: p.texte ?? "", href: "/activite?vue=alertes", img: `<img src="/img/cube-logo.png" alt="" width="24" height="24">`};
        default: return {title: "Nouvelle alerte", body: "", href: "/activite?vue=alertes"};
    }
}

export function showAlert(kind, p, id) {
    const a = alertText(kind, p);
    toast(`${a.img ?? ""}<span class="toast-text"><span class="fs-small b">${escapeHTML(a.title)}</span>${a.body ? `<span class="fs-cap t3">${escapeHTML(a.body)}</span>` : ""}</span>
        <a class="btn btn-sm" href="${escapeHTML(a.href)}">Voir</a>`, {timeout: 8000});
    notifyDevice(a.title, a.body, a.href, id);
}

// ------------------------------------------------------------------
// Notifications of the device (the permission is asked from the account menu)

export function deviceSupported() {
    return "Notification" in window;
}

export function deviceOn() {
    try { return deviceSupported() && Notification.permission === "granted" && localStorage.getItem(DEVICE_KEY) !== "0"; } catch { return false; }
}

// asked from a click; returns the new state
export async function toggleDevice() {
    if (!deviceSupported()) return false;
    if (deviceOn()) { try { localStorage.setItem(DEVICE_KEY, "0"); } catch { /* private mode */ } return false; }
    const perm = Notification.permission === "granted" ? "granted" : await Notification.requestPermission();
    try { localStorage.setItem(DEVICE_KEY, perm === "granted" ? "1" : "0"); } catch { /* private mode */ }
    if (perm === "granted") new Notification("BetterPlace", {body: "Les alertes s'afficheront aussi ici.", icon: "/favicon.png"});
    return perm === "granted";
}

export function notifyDevice(title, body, href, id) {
    if (!deviceOn() || !document.hidden) return;
    try {
        const n = new Notification(title, {body, icon: "/favicon.png", tag: id ? `bp-alert-${id}` : undefined});
        n.onclick = () => { window.focus(); if (href) location.href = href; n.close(); };
    } catch { /* some browsers only allow it from a service worker */ }
}

// ------------------------------------------------------------------
// The pages without a live connection: ask for new alerts every 30 s

export function startAlertPolling(me, onUnread = () => {}) {
    if (!me?.user) return;
    const key = `bp-alertes-vues-${me.user.id}`;
    let last = null;
    try { last = Number(localStorage.getItem(key)) || null; } catch { /* private mode */ }
    const poll = async () => {
        if (document.hidden && !deviceOn()) return;
        const r = await fetch("/api/alerts", {cache: "no-store"}).catch(() => null);
        if (!r?.ok) return;
        const j = await r.json().catch(() => null);
        if (!j) return;
        onUnread(j.unread ?? 0);
        const alerts = j.alerts ?? [];
        const max = alerts.reduce((m, a) => Math.max(m, a.id), 0);
        if (last === null) last = max; // first visit: no flood of old alerts
        const fresh = alerts.filter(a => a.id > last && !a.lu).sort((a, b) => a.id - b.id).slice(-3);
        for (const a of fresh) showAlert(a.kind, a.payload, a.id);
        if (max > last) {
            last = max;
            try { localStorage.setItem(key, String(last)); } catch { /* private mode */ }
        }
    };
    poll();
    setInterval(poll, 30000);
    document.addEventListener("visibilitychange", () => { if (!document.hidden) poll(); });
}

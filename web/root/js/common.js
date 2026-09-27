// Helpers shared by the content pages.

import {escapeHTML} from "./ui.js";

const MONTHS = ["janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."];
export const dateFR = ts => { const d = new Date(ts); return `${d.getDate()} ${MONTHS[d.getMonth()]} ${d.getFullYear()}`; };

export function bannerHTML(banner, accent, cls = "prof-banner", url = "") {
    const b = banner || "desert";
    if (b === "custom" && url) return `<div class="${cls} banner-custom"><img class="px" src="${escapeHTML(url)}" alt=""></div>`;
    if (b === "uni") return `<div class="${cls}" style="background: ${escapeHTML(hexAlpha(accent || "#3ee06c", .35))}"></div>`;
    return `<div class="${cls}"><img class="px" src="/img/${b === "nuit" ? "nuit" : "desert"}.png" alt=""></div>`;
}

export function hexAlpha(hex, a) {
    const n = parseInt(String(hex).replace("#", ""), 16) || 0;
    return `rgba(${n >> 16}, ${(n >> 8) & 255}, ${n & 255}, ${a})`;
}

export function badgeTile(b, {small = false} = {}) {
    return `<div class="badge-tile${b.earned ? "" : " badge-lock"}${b.fresh ? " anim-shine" : ""}" title="${escapeHTML(b.rule)}">
        <img class="badge-ico" src="${escapeHTML(b.sprite)}" alt="" style="${small ? "width: 36px; height: 36px" : ""}">
        <span class="fs-${small ? "cap" : "small"} b${b.gold ? " tgold" : ""}">${escapeHTML(b.name)}</span>
        ${small ? "" : `<span class="fs-cap t3">${b.earned ? escapeHTML(b.detail || "") : "pas encore"}</span>`}
    </div>`;
}


export function claimThumb(c, size = 72) {
    const m = Math.max(2, Math.min(8, Math.floor(Math.max(c.w, c.h) / 6)));
    const w = c.w + 2 * m, h = c.h + 2 * m;
    const z = Math.max(1, Math.min(16, Math.floor(size * 2 / Math.max(w, h))));
    return `/api/crop.png?x=${Math.max(0, c.x - m)}&y=${Math.max(0, c.y - m)}&w=${w}&h=${h}&z=${z}`;
}

export function authorsLine(c, meId) {
    const names = [c.demandeur, ...c.co_auteurs.map(a => a.user)].filter(Boolean)
        .map(u => escapeHTML(u.pseudo) + (u.id === meId ? " (toi)" : ""));
    if (names.length <= 1) return names[0] ?? "";
    return names.slice(0, -1).join(", ") + " et " + names[names.length - 1];
}

export function statusPill(c) {
    if (c.statut === "validee") return `<span class="pill st-valide">Validée</span>`;
    if (c.statut === "refusee") return `<span class="pill st-refuse">Refusée</span>`;
    const rivals = c.conflits.filter(k => k.claim_id && k.recouvrement_pct >= 30).length;
    return rivals ? `<span class="pill st-conflit">${rivals + 1} revendications</span>` : `<span class="pill st-attente">En attente</span>`;
}


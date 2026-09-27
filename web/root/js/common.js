// Helpers shared by the content pages.

import {escapeHTML} from "./ui.js";

const MONTHS = ["janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."];
export const dateFR = ts => { const d = new Date(ts); return `${d.getDate()} ${MONTHS[d.getMonth()]} ${d.getFullYear()}`; };

export function bannerHTML(banner, accent, cls = "prof-banner") {
    const b = banner || "desert";
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


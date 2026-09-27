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


// ------------------------------------------------------------------
// Activity lines (m-activite, d-activite, the "En direct" panel of the canvas)

const BADGE_SPRITES = {
    "premier-pixel": "/img/badges/premier-pixel.png", millier: "/img/badges/millier.png", restaurateur: "/img/badges/restaurateur.png",
    noctambule: "/img/badges/noctambule.png", top10: "/img/badges/top10.png", temoin: "/img/badges/temoin.png",
    pionnier: "/img/badges/pionnier.png", veteran: "/img/badges/veteran.png", batisseur: "/img/badges/batisseur.png", indemodable: "/img/badges/indemodable.png",
};

export function viewHref(x, y, z = 8) {
    return `/ace?x=${x}&y=${y}&z=${z}`;
}

// Returns {who, what, target, href, img, badge, hl, coord} for one activity item.
export function activityParts(a, now = Date.now()) {
    const who = a.who?.pseudo ?? "Quelqu'un";
    const refId = (a.ref || "").split(":")[1];
    const p = {who, what: "", target: "", href: "#", img: a.who, badge: null, hl: "", coord: null};
    switch (a.kind) {
        case "pixels":
            p.what = now - a.ts < 30000 ? "dessine près de" : `a posé ${a.n} pixel${a.n > 1 ? "s" : ""} près de`;
            p.coord = `${a.x} · ${a.y}`;
            p.href = viewHref(a.x, a.y);
            break;
        case "retouche":
            p.what = `a retouché ${a.n} pixel${a.n > 1 ? "s" : ""} de`;
            p.target = a.texte;
            p.coord = `${a.x} · ${a.y}`;
            p.href = `/oeuvre/${refId}`;
            p.hl = "info";
            break;
        case "claim":
            p.what = "revendique";
            p.target = a.texte;
            p.href = "/revendications";
            break;
        case "claim_valid":
            p.what = "devient Pionnier avec";
            p.target = a.texte;
            p.href = `/oeuvre/${refId}`;
            p.badge = BADGE_SPRITES.pionnier;
            p.hl = "gold";
            break;
        case "badge":
            p.what = "gagne le badge";
            p.target = a.texte;
            p.href = `/u/${a.who?.slug ?? ""}`;
            p.badge = BADGE_SPRITES[refId] ?? null;
            break;
        case "exposition":
            p.what = "expose au musée :";
            p.target = a.texte;
            p.href = `/oeuvre/${refId}`;
            break;
        case "musee":
            p.what = "a ouvert son musée";
            p.target = a.texte ? `« ${a.texte} »` : "";
            p.href = `/u/${a.who?.slug ?? ""}/musee`;
            break;
        case "livre_or":
            p.what = "a signé le livre d'or du";
            p.target = a.texte;
            p.href = `/u/${refId}/musee`;
            break;
        default:
            p.what = a.texte || a.kind;
    }
    return p;
}

export const ACTIVITY_FILTERS = [
    ["", "Tout"], ["pixels", "Pixels"], ["retouche,exposition,claim_valid", "Œuvres"], ["claim,claim_valid", "Revendications"], ["exposition,musee,livre_or", "Musées"],
];

// ------------------------------------------------------------------
// Artwork images (crops of the live canvas)

// cropOf returns /api/crop.png for an artwork, about `size` px on its longer side.
// marge (50–100) is how much of the frame the artwork fills.
export function cropOf(o, size = 240, marge = 75) {
    const fill = Math.max(50, Math.min(100, marge)) / 100;
    const side = Math.max(o.w, o.h);
    const pad = Math.round(side * (1 / fill - 1) / 2);
    const w = o.w + 2 * pad, h = o.h + 2 * pad;
    const z = Math.max(1, Math.min(16, Math.floor(size / Math.max(w, h))));
    return `/api/crop.png?x=${Math.max(0, o.x - pad)}&y=${Math.max(0, o.y - pad)}&w=${w}&h=${h}&z=${z}`;
}

export function frameClass(expo) {
    return expo?.cadre === "or" ? "cadre-or" : expo?.cadre === "sans" ? "cadre-sans" : "cadre";
}

export function authorsOf(o) {
    const names = (o.auteurs ?? []).map(a => a.user?.pseudo).filter(Boolean);
    if (names.length <= 1) return names[0] ?? "";
    return names.slice(0, -1).join(", ") + " et " + names[names.length - 1];
}

export function artView(o) {
    return `/ace?x=${o.x + Math.floor(o.w / 2)}&y=${o.y + Math.floor(o.h / 2)}&z=${Math.max(2, Math.min(24, Math.floor(400 / Math.max(o.w, o.h))))}`;
}

export const SALLE_LABELS = {
    "coups-de-coeur": "Coups de cœur", pionnieres: "Pionnières", paysages: "Paysages", personnages: "Personnages",
    "clins-d-oeil": "Clins d'œil", "drapeaux-logos": "Drapeaux & logos", motifs: "Motifs", monuments: "Monuments",
};

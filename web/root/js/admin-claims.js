// /admin/revendications — the claims queue and decisions (a-claims, am-claims).

import {api, escapeHTML, icon, avatarHTML, relativeTime} from "./admin.js";
import {$, $$, fillIcons, toast} from "./ui.js";
import {claimThumb, authorsLine, statusPill, dateFR} from "./common.js";
import {formatNumber} from "./view.js";

const BADGES = {
    pionnier: ["Pionnier", "/img/badges/pionnier.png"], veteran: ["Vétéran 2022", "/img/badges/veteran.png"],
    batisseur: ["Bâtisseur", "/img/badges/batisseur.png"], indemodable: ["Indémodable", "/img/badges/indemodable.png"],
};
const TABS = [["attente", "À traiter"], ["validee", "Validées"], ["refusee", "Refusées"]];

let root, ctx, tab = "attente", data = {claims: [], counts: {}}, sel = 0, badgeOff = {};

const dmyy = ms => ms ? new Date(ms).toLocaleDateString("fr-FR", {day: "2-digit", month: "2-digit", year: "numeric"}) : "—";

function facts(c) {
    const a = c.analyse;
    if (!a) return `<p class="fs-small t2">Pas d'analyse : les sauvegardes ne sont pas disponibles (ou en cours d'indexation).</p>`;
    const rivals = c.conflits.filter(k => k.claim_id);
    const list = [
        [a.deja_la ? "Apparition" : "Premiers pixels", dmyy(a.apparition), a.deja_la ? "déjà là à la 1re sauvegarde" : ""],
        ["Terminée", dmyy(a.terminee), a.terminee ? "" : "jamais à 90 %"],
        ["Pixels posés (est.)", formatNumber(a.pixels_poses_estimes), `sur ${formatNumber(a.pixels_oeuvre)} px d'œuvre`],
        ["Jours de dessin", String(a.jours.length), a.jours.slice(0, 3).map(d => d.slice(8, 10) + "/" + d.slice(5, 7)).join(", ") + (a.jours.length > 3 ? "…" : "")],
        rivals.length ? ["Autres demandes", String(rivals.length), rivals.map(k => escapeHTML(k.par?.pseudo ?? "")).join(", ")]
            : ["Stabilité 12 mois", Math.round(a.stabilite_12_mois_pct) + " %", a.retouches.length ? `retouchée ${a.retouches.length} fois` : "jamais vraiment retouchée"],
        ["Aujourd'hui", Math.round(a.etat_actuel_pct) + " %", a.etat_actuel_pct >= 90 ? "intacte" : "abîmée"],
    ];
    return `<div class="adm-facts">${list.map(([k, v, s]) => `<div><span class="fs-cap t3">${k}</span><span class="mono b">${v}</span><span class="fs-cap t3">${s}</span></div>`).join("")}</div>
        <span class="fs-cap t3">${a.minute_par_minute ? "Sauvegardes minute par minute." : "Sauvegardes espacées : les dates sont approximatives."} ${formatNumber(a.sauvegardes)} captures, du ${dateFR(a.premiere_sauvegarde)} au ${dateFR(a.derniere_sauvegarde)}.</span>`;
}

function zonePreview(c) {
    const pad = Math.max(8, Math.floor(Math.max(c.w, c.h) * 0.4));
    const rival = c.conflits.find(k => k.claim_id && k.recouvrement_pct >= 30);
    let x0 = Math.max(0, c.x - pad), y0 = Math.max(0, c.y - pad), x1 = c.x + c.w + pad, y1 = c.y + c.h + pad;
    const w = x1 - x0, h = y1 - y0;
    const z = Math.max(1, Math.min(16, Math.floor(260 / Math.max(w, h))));
    const box = (m, cls) => {
        const b = m.type === "rect" ? {x: m.rect[0], y: m.rect[1], w: m.rect[2], h: m.rect[3]} : m.type === "bitmap" ? m : bounds(m.points);
        return `<div class="${cls}" style="left: ${(b.x - x0) * z}px; top: ${(b.y - y0) * z}px; width: ${b.w * z}px; height: ${b.h * z}px"></div>`;
    };
    return `<div class="adm-zone void" style="width: ${w * z}px; height: ${h * z}px">
        <img class="px" src="/api/crop.png?x=${x0}&y=${y0}&w=${w}&h=${h}&z=${z}" alt="La zone revendiquée">
        ${box(c.masque, "adm-zone-box")}
        ${rival ? box(rival.masque, "adm-zone-rival") + `<span class="pill adm-zone-note">pointillés : zone de ${escapeHTML(rival.par?.pseudo ?? "")}</span>` : ""}
    </div>`;
}

function bounds(points) {
    const xs = points.map(p => p[0]), ys = points.map(p => p[1]);
    const x = Math.min(...xs), y = Math.min(...ys);
    return {x, y, w: Math.max(...xs) - x + 1, h: Math.max(...ys) - y + 1};
}

function detail(c) {
    if (!c) return `<div class="card empty-state"><span class="fs-body b">${tab === "attente" ? "Tout est traité" : "Rien ici"}</span><span class="fs-small t2">${tab === "attente" ? "Aucune revendication n'attend de décision. Bravo !" : ""}</span></div>`;
    const off = badgeOff[c.id] ?? {};
    const proposed = c.analyse?.badges ?? [];
    const allBadges = [...new Set([...proposed, ...Object.keys(BADGES)])];
    const rivals = c.conflits.filter(k => k.claim_id && k.recouvrement_pct >= 30);
    const rivalClaims = rivals.map(k => data.claims.find(x => x.id === k.claim_id)).filter(Boolean);
    const confirms = c.votes.filter(v => v.type === "confirme");
    const doubt = c.votes.find(v => v.type === "doute" && v.commentaire) ?? c.votes.find(v => v.type === "doute");
    const main = authorsLine(c);
    const view = `/ace?x=${c.x + Math.floor(c.w / 2)}&y=${c.y + Math.floor(c.h / 2)}&z=${Math.max(2, Math.min(24, Math.floor(400 / Math.max(c.w, c.h))))}`;
    const decided = c.statut !== "attente";
    return `
    <header class="adm-detail-head">
        <div><div class="adm-title"><h2 class="fs-h1">${escapeHTML(c.titre)}</h2>${c.a_departager ? `<span class="pill st-conflit">À départager</span>` : statusPill(c)}</div>
            <span class="fs-small t2">${main} · <span class="coord">x ${c.x} · y ${c.y} · ${c.w}×${c.h}</span> · ${relativeTime(c.cree_le)}</span></div>
        <a class="btn btn-sm" href="${view}">Voir sur le canvas</a>
    </header>
    ${c.message ? `<p class="fs-small t2 pretty">« ${escapeHTML(c.message)} »</p>` : ""}
    <section class="adm-row">
        ${zonePreview(c)}
        <div class="card adm-card grow"><div class="adm-card-head"><span class="fs-h3">Analyse des sauvegardes</span><span class="pill st-valide">calculée automatiquement</span></div>${facts(c)}</div>
    </section>
    ${c.analyse ? `<section class="card adm-card"><div class="adm-card-head"><span class="fs-h3">Mini-timelapse</span><span class="fs-cap t3">les sauvegardes autour des changements</span></div>
        <div class="adm-strip"><img class="px js-strip" src="/api/claims/${c.id}/strip.png" alt="Mini-timelapse de la zone" loading="lazy"><div class="adm-strip-dates mono fs-cap t3 js-strip-dates"></div></div></section>` : ""}
    ${rivalClaims.length ? `<section class="adm-rivals"><span class="fs-h3">Plusieurs joueurs revendiquent la même œuvre</span><div class="adm-rival-grid">
        ${[c, ...rivalClaims].map((x, i) => `<div class="card adm-card${i === 0 ? " adm-main" : ""}">
            <div class="adm-person">${avatarHTML(x.demandeur, "av av-32")}<span class="fs-body b grow">${escapeHTML(x.demandeur?.pseudo ?? "")}</span><span class="fs-cap t3">${relativeTime(x.cree_le)}</span></div>
            <p class="fs-small pretty">${x.message ? `« ${escapeHTML(x.message)} »` : `<span class="t3">Pas de message.</span>`}</p>
            <span class="fs-cap t3">Zone : ${x.w} × ${x.h}</span>
            <div class="fs-cap"><span class="tac b">+${x.confirmations} « je l'ai vu la faire »</span> · <span class="t3">${x.doutes} doutes</span></div></div>`).join("")}
    </div></section>` : ""}
    <section class="adm-row">
        <div class="card adm-card grow">
            <span class="fs-h3">La communauté</span>
            <div class="adm-person"><div class="av-stack">${confirms.slice(0, 5).map(v => avatarHTML(v.user, "av av-32")).join("")}</div>
                <span class="fs-small"><b class="tac">+${c.confirmations}</b> <span class="t2">confirmations</span> · <b>${c.doutes}</b> <span class="t2">doutes</span></span></div>
            ${doubt ? `<div class="card-2 adm-doubt">${avatarHTML(doubt.user, "av av-24")}<span><span class="fs-cap b">${escapeHTML(doubt.user?.pseudo ?? "")} émet un doute</span><br><span class="fs-cap t2 pretty">${doubt.commentaire ? `« ${escapeHTML(doubt.commentaire)} »` : "sans commentaire"}</span></span></div>` : ""}
            ${c.co_auteurs.length ? `<span class="fs-cap t3">Co-auteurs : ${c.co_auteurs.map(a => `${escapeHTML(a.user?.pseudo ?? "")} ${a.confirme ? "(a confirmé)" : "(pas encore confirmé)"}`).join(", ")}</span>` : ""}
        </div>
        <div class="card adm-card grow">
            <span class="fs-h3">Badges ${decided ? "donnés" : "proposés"}</span>
            <div class="adm-badges">${(decided ? c.badges_donnes : allBadges).map(b => BADGES[b] ? `<button type="button" class="chip adm-badge${(decided || (proposed.includes(b) && !off[b]) || off[b] === false) ? " on" : ""}" data-badge="${b}" aria-pressed="${decided || (proposed.includes(b) && !off[b]) || off[b] === false}" ${decided ? "disabled" : ""}><img src="${BADGES[b][1]}" alt="" class="px" width="20" height="20">${BADGES[b][0]}${proposed.includes(b) || decided ? "" : " <span class=\"t3\">(non proposé)</span>"}</button>` : "").join("") || `<span class="fs-small t3">Aucun.</span>`}</div>
            <span class="fs-cap t3 pretty">Pixels pionniers : ≈ ${formatNumber(c.analyse?.pixels_poses_estimes ?? 0)} px, ajoutés au profil des auteurs (répartis ${c.repartition === "poids" ? "selon leurs poids" : "à parts égales"}). Noctambule demande les sauvegardes minute par minute.</span>
        </div>
    </section>
    <section class="card adm-actions">
        ${decided ? `
            <span class="stamp anim-stamp">${c.statut === "validee" ? "VALIDÉ" : "REFUSÉ"}</span>
            <span class="fs-small t2 pretty grow">${escapeHTML(c.resume)}</span>
            <button class="btn btn-sm btn-ghost js-undo" type="button">${icon("undo", 16)}Annuler la décision</button>`
        : `
            <button class="btn btn-primary btn-px js-decide" data-action="valider" type="button">Valider pour ${main}</button>
            ${rivalClaims.length ? `<button class="btn js-decide" data-action="fusionner" type="button">Fusionner en co-auteurs</button>` : ""}
            <div class="search adm-assign">${icon("search", 16)}<input class="input js-assign" placeholder="Attribuer à un autre compte…" aria-label="Attribuer à un autre compte" autocomplete="off"><div class="card claim-results js-assign-results" hidden></div></div>
            <span class="grow"></span>
            <button class="btn btn-outline-danger js-refuse" type="button">Refuser…</button>`}
    </section>`;
}

function queue() {
    return `<div class="seg adm-tabs" role="tablist">${TABS.map(([id, l]) => `<button type="button" class="${id === tab ? "on" : ""}" data-tab="${id}">${l} · ${data.counts[id] ?? 0}</button>`).join("")}</div>
    <div class="adm-queue">${data.claims.map((c, i) => `
        <button type="button" class="card adm-q${i === sel ? " sel" : ""}${c.a_departager ? " disputed" : ""}" data-sel="${i}" aria-pressed="${i === sel}">
            <span class="adm-q-thumb"><img class="px" src="${claimThumb(c, 60)}" alt="" loading="lazy"></span>
            <span class="adm-q-text">
                <span class="adm-q-title"><span class="fs-small b ellip">${escapeHTML(c.titre)}</span>${c.a_departager ? `<span class="pill st-conflit">À départager</span>` : statusPill(c)}</span>
                <span class="fs-cap t2 ellip">${authorsLine(c)}</span>
                <span class="fs-cap t3">+${c.confirmations} confirmations · ${c.doutes} doutes · ${relativeTime(c.cree_le)}</span>
            </span>
        </button>`).join("") || `<span class="fs-small t3">Aucune demande.</span>`}</div>
    <div class="card-2 adm-hint"><span class="fs-small b">Pas de demande ?</span><span class="fs-cap t3 pretty">Tu peux attribuer une zone à un compte directement, par exemple pour une œuvre dont tout le monde connaît l'auteur.</span><a class="btn btn-sm" href="/admin/zone">Attribuer une zone</a></div>`;
}

function render() {
    const c = data.claims[sel];
    root.innerHTML = `<div class="adm-claims">
        <section class="adm-list"><h1 class="fs-h2">Revendications</h1>${queue()}</section>
        <section class="adm-detail">${detail(c)}</section>
    </div>`;
    fillIcons(root);
    const strip = $(".js-strip", root);
    if (strip && c) {
        fetch(strip.src).then(r => r.headers.get("X-Frame-Times")).then(t => {
            const times = JSON.parse(t || "[]");
            const day = ms => new Date(ms).toLocaleDateString("fr-FR", {day: "2-digit", month: "2-digit", year: "2-digit"});
            const days = times.map(day);
            $(".js-strip-dates", root).innerHTML = times.map((ms, i) => {
                const same = days.filter(d => d === days[i]).length > 1;
                return `<span>${same ? day(ms).slice(0, 5) + " " + new Date(ms).toLocaleTimeString("fr-FR", {hour: "2-digit", minute: "2-digit"}) : days[i]}</span>`;
            }).join("");
        }).catch(() => {});
    }
}

async function load() {
    data = await api("/api/admin/claims?statut=" + tab);
    sel = Math.min(sel, Math.max(0, data.claims.length - 1));
    render();
}

async function decide(c, body) {
    try {
        await api(`/api/admin/claims/${c.id}/decision`, {method: "POST", body});
        toast(`<span class="fs-small b">Décision enregistrée</span>`);
        await load();
        ctx.refreshCounts();
    } catch (err) {
        toast(`<span class="fs-small b">${escapeHTML(err.message)}</span>`, {timeout: 5000});
    }
}

export async function mount(el, context) {
    root = el;
    ctx = context;
    root.onclick = async e => {
        const t = e.target.closest("[data-tab]");
        if (t) { tab = t.dataset.tab; sel = 0; load(); return; }
        const s = e.target.closest("[data-sel]");
        if (s) { sel = Number(s.dataset.sel); render(); return; }
        const c = data.claims[sel];
        if (!c) return;
        const b = e.target.closest("[data-badge]");
        if (b && !b.disabled) {
            const id = b.dataset.badge;
            const off = badgeOff[c.id] ??= {};
            const on = b.getAttribute("aria-pressed") === "true";
            off[id] = on; // true = switched off; false = switched on (not proposed)
            render();
            return;
        }
        const chosen = () => $$(".adm-badge", root).filter(x => x.getAttribute("aria-pressed") === "true").map(x => x.dataset.badge);
        if (e.target.closest(".js-decide")) {
            const action = e.target.closest(".js-decide").dataset.action;
            const body = {action, badges: chosen()};
            if (action === "fusionner") body.fusion = c.conflits.filter(k => k.claim_id && k.recouvrement_pct >= 30).map(k => k.claim_id);
            decide(c, body);
        }
        if (e.target.closest(".js-refuse")) {
            const motif = prompt("Motif du refus (le joueur le recevra) :");
            if (motif?.trim()) decide(c, {action: "refuser", motif});
        }
        const pick = e.target.closest(".js-assign-results [data-slug]");
        if (pick && confirm(`Attribuer « ${c.titre} » à ${pick.dataset.pseudo} ?`)) decide(c, {action: "attribuer", user: pick.dataset.slug, badges: chosen()});
        if (e.target.closest(".js-undo")) {
            try {
                await api(`/api/admin/claims/${c.id}/undo`, {method: "POST"});
                toast(`<span class="fs-small b">Décision annulée</span>`);
                load();
                ctx.refreshCounts();
            } catch (err) {
                toast(`<span class="fs-small b">${escapeHTML(err.message)}</span>`);
            }
        }
    };
    let timer = null;
    root.oninput = e => {
        if (!e.target.classList.contains("js-assign")) return;
        clearTimeout(timer);
        const q = e.target.value.trim();
        const box = $(".js-assign-results", root);
        if (!q) { box.hidden = true; return; }
        timer = setTimeout(async () => {
            const j = await api("/api/users/search?q=" + encodeURIComponent(q)).catch(() => ({users: []}));
            box.innerHTML = j.users.map(u => `<button type="button" class="claim-result" data-slug="${escapeHTML(u.slug)}" data-pseudo="${escapeHTML(u.pseudo)}">${avatarHTML(u, "av av-24")}<span class="fs-small b">${escapeHTML(u.pseudo)}</span></button>`).join("") || `<span class="fs-small t3" style="padding: 8px">Personne.</span>`;
            box.hidden = false;
        }, 200);
    };
    await load();
}

export function unmount() {
    if (root) { root.onclick = null; root.oninput = null; }
}

// /revendications — the claims feed: in progress, validated, refused (m-revendications).

import {$, $$, avatarHTML, escapeHTML, fillIcons, icon, toast} from "./ui.js";
import {mountShell, emptyState, loginHref} from "./shell.js";
import {relativeTime} from "./view.js";
import {claimThumb, authorsLine, statusPill} from "./common.js";

const TABS = [["attente", "En cours"], ["validee", "Validées"], ["refusee", "Refusées"]];
let me = null;
let tab = new URLSearchParams(location.search).get("statut") || "attente";
let data = {claims: [], counts: {}};

function card(c) {
    const mine = me && c.demandeur?.id === me.id;
    const coMe = me && c.co_auteurs.find(a => a.user?.id === me.id);
    const involved = mine || coMe;
    const when = c.statut === "attente" ? c.cree_le : (c.decide_le || c.cree_le);
    const disputed = c.statut === "attente" && c.conflits.some(k => k.claim_id && k.recouvrement_pct >= 30);
    const view = `/ace?x=${c.x + Math.floor(c.w / 2)}&y=${c.y + Math.floor(c.h / 2)}&z=${Math.max(2, Math.min(24, Math.floor(400 / Math.max(c.w, c.h))))}`;
    const pendingCo = c.co_auteurs.filter(a => !a.confirme).map(a => escapeHTML(a.user?.pseudo ?? "?"));
    return `<article class="card claim-card${disputed ? " disputed" : ""}" data-id="${c.id}">
        <div class="claim-card-head">
            <a class="claim-card-thumb" href="${view}" aria-label="Voir sur le canvas"><img class="px" src="${claimThumb(c)}" alt="" loading="lazy"></a>
            <div class="claim-card-meta">
                <span class="fs-body b ellip">${escapeHTML(c.titre)}</span>
                <span class="claim-card-by">${avatarHTML(c.demandeur, "av av-20")}<span class="fs-small t2 ellip">${authorsLine(c, me?.id)}</span></span>
                <span class="claim-card-status">${statusPill(c)}<span class="fs-cap t3">${relativeTime(when)}</span></span>
            </div>
        </div>
        ${c.message ? `<p class="fs-small t2 pretty">« ${escapeHTML(c.message)} »</p>` : ""}
        <div class="card-2 claim-card-save">${icon(c.statut === "refusee" ? "flag" : "clock", 16)}<span class="fs-cap t2">${escapeHTML(c.resume)}</span></div>
        ${coMe && !coMe.confirme && c.statut === "attente" ? `<div class="card-2 claim-card-co"><span class="fs-small">${escapeHTML(c.demandeur?.pseudo)} t'a ajouté·e comme co-auteur.</span>
            <button class="btn btn-sm btn-primary" type="button" data-act="accept">Confirmer</button><button class="btn btn-sm btn-ghost" type="button" data-act="decline">Ce n'est pas moi</button></div>` : ""}
        ${mine && pendingCo.length && c.statut === "attente" ? `<span class="fs-cap t3">En attente de la confirmation de ${pendingCo.join(", ")}.</span>` : ""}
        <div class="claim-card-actions">
            ${c.statut === "attente" && !involved && me ? `
                <button class="btn btn-sm${c.mon_vote === "confirme" ? " btn-primary" : ""}" type="button" data-act="confirme" aria-pressed="${c.mon_vote === "confirme"}">${icon("check", 16)}Je l'ai vu·e la faire · ${c.confirmations}</button>
                <button class="btn btn-sm${c.mon_vote === "doute" ? " btn-outline-danger" : " btn-ghost"}" type="button" data-act="doute" aria-pressed="${c.mon_vote === "doute"}">Un doute · ${c.doutes}</button>`
            : `<span class="fs-cap t3">${icon("check", 14)} ${c.confirmations} confirmation${c.confirmations > 1 ? "s" : ""} · ${c.doutes} doute${c.doutes > 1 ? "s" : ""}</span>`}
            <span class="grow"></span>
            ${c.oeuvre_id ? `<a class="btn btn-sm btn-ghost" href="/oeuvre/${c.oeuvre_id}">Voir l'œuvre</a>` : `<a class="btn btn-sm btn-ghost" href="${view}">Sur le canvas</a>`}
            ${mine && c.statut === "attente" ? `<button class="btn btn-sm btn-ghost" type="button" data-act="cancel">Annuler</button>` : ""}
        </div>
        <form class="claim-doubt" hidden>
            <textarea class="textarea" maxlength="280" placeholder="Qu'est-ce qui te fait douter ? (facultatif)"></textarea>
            <div class="claim-card-actions"><button class="btn btn-sm btn-outline-danger" type="submit">Émettre un doute</button><button class="btn btn-sm btn-ghost" type="button" data-act="nodoubt">Annuler</button></div>
        </form>
    </article>`;
}

function render() {
    const page = $("#page");
    const list = data.claims.length
        ? data.claims.map(card).join("")
        : emptyState({
            title: tab === "attente" ? "Aucune demande en cours" : tab === "validee" ? "Aucune revendication validée pour l'instant" : "Aucune demande refusée",
            text: tab === "attente" ? "Tu as dessiné ici avant les comptes ? Sois le premier à revendiquer ton œuvre." : "",
            action: tab === "attente" ? `<a class="btn btn-gold btn-px" href="/revendiquer">Revendiquer une œuvre</a>` : "",
        });
    page.innerHTML = `<div class="feed-wrap">
        <div class="feed-head">
            <h1 class="fs-h1">Revendications</h1>
            <a class="btn btn-sm btn-gold btn-px" href="${me ? "/revendiquer" : loginHref()}">Revendiquer</a>
        </div>
        <div class="seg feed-tabs" role="tablist" aria-label="Filtrer">
            ${TABS.map(([id, label]) => `<button type="button" role="tab" class="${id === tab ? "on" : ""}" aria-selected="${id === tab}" data-tab="${id}">${label} · ${data.counts[id] ?? 0}</button>`).join("")}
        </div>
        <div class="feed-list">${list}</div>
    </div>`;
    page.removeAttribute("aria-busy");
    fillIcons(page);
}

async function load() {
    data = await fetch("/api/claims?statut=" + tab, {cache: "no-store"}).then(r => r.json()).catch(() => ({claims: [], counts: {}}));
    render();
}

async function act(id, path, body) {
    const r = await fetch(`/api/claims/${id}/${path}`, {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(body ?? {})});
    const j = await r.json().catch(() => ({}));
    if (!r.ok) {
        toast(`<span class="fs-small b">${escapeHTML(j.error || "Action impossible.")}</span>`);
        return null;
    }
    return j.claim;
}

async function main() {
    me = (await mountShell({active: "activite"})).user;
    const sent = new URLSearchParams(location.search).get("envoyee");
    await load();
    if (sent) {
        const c = data.claims.find(x => String(x.id) === sent);
        const co = c?.co_auteurs.map(a => a.user?.pseudo).filter(Boolean) ?? [];
        toast(`<img class="px" src="/img/badges/pionnier.png" alt="" width="28" height="28"><span class="toast-text"><span class="fs-small b">Demande envoyée</span>
            <span class="fs-cap t3">${co.length ? `${escapeHTML(co.join(", "))} ${co.length > 1 ? "doivent" : "doit"} confirmer, puis la communauté vote.` : "La communauté peut maintenant confirmer, puis l'équipe tranche."}</span></span>`, {timeout: 6000});
        history.replaceState(null, "", "/revendications");
    }
    $("#page").addEventListener("click", async e => {
        const t = e.target.closest("[data-tab]");
        if (t) {
            tab = t.dataset.tab;
            history.replaceState(null, "", tab === "attente" ? "/revendications" : "/revendications?statut=" + tab);
            load();
            return;
        }
        const b = e.target.closest("[data-act]");
        if (!b) return;
        const art = b.closest("[data-id]");
        const id = art.dataset.id;
        const c = data.claims.find(x => String(x.id) === id);
        let updated = null;
        switch (b.dataset.act) {
            case "confirme":
                updated = await act(id, "votes", {type: c.mon_vote === "confirme" ? "" : "confirme"});
                break;
            case "doute":
                if (c.mon_vote === "doute") updated = await act(id, "votes", {type: ""});
                else { art.querySelector(".claim-doubt").hidden = false; art.querySelector("textarea").focus(); }
                break;
            case "nodoubt":
                art.querySelector(".claim-doubt").hidden = true;
                break;
            case "accept":
            case "decline":
                updated = await act(id, "confirm", {accepte: b.dataset.act === "accept"});
                break;
            case "cancel":
                if (confirm("Annuler ta demande ?")) {
                    await act(id, "cancel");
                    load();
                }
                return;
        }
        if (updated) {
            Object.assign(c, updated);
            art.outerHTML = card(c);
            fillIcons($("#page"));
        }
    });
    $("#page").addEventListener("submit", async e => {
        e.preventDefault();
        const art = e.target.closest("[data-id]");
        const c = data.claims.find(x => String(x.id) === art.dataset.id);
        const updated = await act(c.id, "votes", {type: "doute", commentaire: art.querySelector("textarea").value});
        if (updated) {
            Object.assign(c, updated);
            art.outerHTML = card(c);
            fillIcons($("#page"));
        }
    });
}

main();

// « Demander à rejoindre » a build (/oeuvre/:id, /revendications): the button and its form
// for the others, the requests to accept or refuse for its authors.

import {avatarHTML, escapeHTML, icon, toast} from "./ui.js";

export const noJoin = {a_decider: [], miennes: []};

export async function loadJoin(me) {
    if (!me) return noJoin;
    return fetch("/api/rejoindre", {cache: "no-store"}).then(r => r.ok ? r.json() : noJoin).catch(() => noJoin);
}

// canAsk: the viewer is logged in and not one of the authors.
export function joinHTML(cible, j, canAsk) {
    const todo = j.a_decider.filter(d => d.cible === cible);
    const mine = j.miennes.find(d => d.cible === cible);
    let out = todo.length ? `<div class="card-2 claim-card-co join-todo"><span class="fs-over">Demandes pour rejoindre</span>${todo.map(d => `
        <div class="join-row" data-demande="${d.id}">${avatarHTML(d.user, "av av-24")}<span class="fs-small grow"><a href="/u/${escapeHTML(d.user?.slug ?? "")}"><b>${escapeHTML(d.user?.pseudo ?? "")}</b></a>${d.message ? ` <span class="t2">« ${escapeHTML(d.message)} »</span>` : ""}</span>
            <button class="btn btn-sm btn-primary" type="button" data-join-answer="1">Accepter</button><button class="btn btn-sm btn-ghost" type="button" data-join-answer="0">Refuser</button></div>`).join("")}</div>` : "";
    if (!canAsk) return out;
    if (mine?.statut === "attente") return out + `<span class="fs-cap t3">${icon("clock", 14)} Demande pour rejoindre envoyée : les auteurs vont répondre.</span>`;
    if (mine?.statut === "refusee") return out + `<span class="fs-cap t3">Les auteurs ont refusé ta demande pour rejoindre ce build.</span>`;
    return out + `<div class="join-ask"><button class="btn btn-sm" type="button" data-join-open>${icon("users", 16)}J'y ai participé · demander à rejoindre</button>
        <form class="claim-doubt" data-join-cible="${escapeHTML(cible)}" hidden>
            <textarea class="textarea" maxlength="280" placeholder="Qu'as-tu dessiné dans ce build ? (facultatif, lu par les auteurs)"></textarea>
            <div class="claim-card-actions"><button class="btn btn-sm btn-primary" type="submit">Envoyer la demande</button><button class="btn btn-sm btn-ghost" type="button" data-join-close>Annuler</button></div>
        </form></div>`;
}

// Delegated handlers on root; onDone() reloads and redraws.
export function wireJoin(root, onDone) {
    root.addEventListener("click", async e => {
        const open = e.target.closest("[data-join-open]");
        if (open) {
            open.hidden = true;
            const f = open.parentElement.querySelector("form[data-join-cible]");
            f.hidden = false;
            f.querySelector("textarea").focus();
            return;
        }
        const close = e.target.closest("[data-join-close]");
        if (close) {
            const box = close.closest(".join-ask");
            box.querySelector("form").hidden = true;
            box.querySelector("[data-join-open]").hidden = false;
            return;
        }
        const ans = e.target.closest("[data-join-answer]");
        if (!ans) return;
        const row = ans.closest("[data-demande]");
        const ok = ans.dataset.joinAnswer === "1";
        const r = await fetch(`/api/rejoindre/${row.dataset.demande}`, {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({accepte: ok})});
        const j = await r.json().catch(() => ({}));
        toast(`<span class="fs-small b">${escapeHTML(r.ok ? (ok ? "Ajouté·e aux auteurs" : "Demande refusée") : j.error || "Action impossible.")}</span>`);
        onDone();
    });
    root.addEventListener("submit", async e => {
        const f = e.target.closest("form[data-join-cible]");
        if (!f) return;
        e.preventDefault();
        e.stopPropagation();
        const r = await fetch("/api/rejoindre", {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({cible: f.dataset.joinCible, message: f.querySelector("textarea").value})});
        const j = await r.json().catch(() => ({}));
        toast(`<span class="fs-small b">${escapeHTML(r.ok ? "Demande envoyée aux auteurs" : j.error || "Envoi impossible.")}</span>`);
        if (r.ok) onDone();
    }, true);
}

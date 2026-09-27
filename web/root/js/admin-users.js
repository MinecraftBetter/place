// /admin/utilisateurs — players (a-utilisateurs, am-utilisateurs).

import {api, escapeHTML, icon, avatarHTML, relativeTime} from "./admin.js";
import {$, $$, fillIcons, toast} from "./ui.js";
import {formatNumber} from "./view.js";
import {dateFR} from "./common.js";

const FILTERS = [["tous", "Tous"], ["actifs", "Actifs aujourd'hui"], ["suspendus", "Suspendus"], ["bannis", "Bannis"]];
const ST = {actif: ["Actif", "st-valide"], suspendu: ["Suspendu", "st-conflit"], banni: ["Banni", "st-refuse"]};
let root, q = "", filter = "tous", data = null, sel = null, detail = null, duree = "24h", timer = null;

async function loadList() {
    data = await api(`/api/admin/users?q=${encodeURIComponent(q)}&filtre=${filter}`);
    if (!sel && data.users.length) sel = new URLSearchParams(location.search).get("id") ?? String(data.users[0].id);
    if (sel) detail = await api("/api/admin/users/" + sel).catch(() => null);
    render();
}

function render() {
    const rows = data.users.map(u => `<tr class="${String(u.id) === sel ? "sel" : ""}" data-id="${u.id}" tabindex="0">
        <td><div class="adm-user">${avatarHTML(u, "av av-32")}<div><span class="b">${escapeHTML(u.pseudo)}</span><br><span class="fs-cap t3">/u/${escapeHTML(u.slug)}${u.role === "admin" ? " · admin" : ""}</span></div></div></td>
        <td class="num">${formatNumber(u.pixels_poses)}</td><td class="num t2">${formatNumber(u.pixels_visibles)}</td>
        <td class="t2 only-d-cell">${dateFR(u.cree_le)}</td><td class="t2 only-d-cell">${u.dernier_pixel ? relativeTime(u.dernier_pixel) : "—"}</td>
        <td><span class="pill ${ST[u.statut]?.[1] ?? "st-neutre"}">${ST[u.statut]?.[0] ?? escapeHTML(u.statut)}</span></td></tr>`).join("");
    const d = detail;
    const side = d ? `<aside class="adm-user-side card">
        <div class="adm-user-head">${avatarHTML(d.user, "av av-64")}<div><span class="fs-h3">${escapeHTML(d.user.pseudo)}</span><br><span class="pill ${ST[d.statut]?.[1] ?? ""}">${ST[d.statut]?.[0] ?? d.statut}</span>
            <span class="fs-cap t3">${d.fournisseur === "dev" ? "compte de test" : "JustBetter"} · ${escapeHTML(d.id_externe)}</span></div></div>
        ${d.statut !== "actif" ? `<div class="card-2 adm-sanction"><span class="fs-small">${d.statut === "banni" ? "Banni" : `Suspendu jusqu'au ${new Date(d.suspendu_jusqua).toLocaleString("fr-FR", {dateStyle: "medium", timeStyle: "short"})}`}</span>${d.motif ? `<span class="fs-cap t2">Motif : ${escapeHTML(d.motif)}</span>` : ""}</div>` : ""}
        <div class="adm-user-stats">
            <div class="card-2"><span class="fs-cap t3">Pixels posés</span><div class="mono b">${formatNumber(d.pixels_poses)}</div></div>
            <div class="card-2"><span class="fs-cap t3">Encore visibles</span><div class="mono b">${formatNumber(d.pixels_visibles)}</div></div>
            <div class="card-2"><span class="fs-cap t3">Œuvres</span><div class="mono b">${d.oeuvres}</div></div>
            <div class="card-2"><span class="fs-cap t3">Rythme max</span><div class="mono b">${formatNumber(d.rythme_max)}<span class="t3"> px/min</span></div></div>
        </div>
        ${d.recents_10min > 200 ? `<div class="banner banner-danger">${icon("flag", 16)}<span>${formatNumber(d.recents_10min)} pixels en 10 min : ça ressemble à un script.</span></div>` : ""}
        <div class="adm-user-map"><img class="px" src="/api/users/${encodeURIComponent(d.user.slug)}/contributions.png" alt="Ses pixels sur le canvas" loading="lazy"></div>
        <div class="field"><label class="label" for="note">Note de l'équipe (privée)</label><textarea id="note" class="textarea js-note" placeholder="Visible seulement par les admins">${escapeHTML(d.note)}</textarea>
            <button class="btn btn-sm btn-ghost js-save-note" type="button">Enregistrer la note</button></div>
        <div class="field"><span class="label">Suspendre temporairement</span>
            <div class="adm-user-row"><div class="seg" role="group" aria-label="Durée">${["1h", "24h", "7j"].map(x => `<button type="button" class="${x === duree ? "on" : ""}" data-duree="${x}">${x.replace("h", " h").replace("j", " j")}</button>`).join("")}</div>
            <button class="btn btn-sm js-suspend" type="button">Suspendre</button></div></div>
        <div class="adm-user-actions">
            ${d.statut !== "actif" ? `<button class="btn btn-sm btn-primary js-unban" type="button">Réactiver le compte</button>` : ""}
            <button class="btn btn-sm js-reset" type="button">Réinitialiser le profil (avatar, bio, bannière)</button>
            <a class="btn btn-sm btn-ghost" href="/u/${encodeURIComponent(d.user.slug)}">Voir le profil</a>
            <a class="btn btn-sm btn-ghost" href="/admin/zone?user=${d.user.id}">Sélectionner ses pixels</a>
            <button class="btn btn-sm btn-ghost js-role" type="button">${d.role === "admin" ? "Retirer le rôle admin" : "Faire admin"}</button>
            ${d.statut !== "banni" ? `<button class="btn btn-sm btn-outline-danger js-ban" type="button">Bannir le compte…</button>` : ""}
        </div>
    </aside>` : "";
    root.innerHTML = `<div class="admin-pad adm-users">
        <div class="adm-users-main">
            <header class="adm-dash-head"><h1 class="fs-h1">Utilisateurs</h1><span class="fs-small t2">${formatNumber(data.total)} comptes · ${formatNumber(data.actifs_jour)} actifs aujourd'hui</span></header>
            <div class="adm-users-filters"><div class="search">${icon("search", 16)}<input class="input js-q" aria-label="Chercher un joueur" placeholder="Pseudo, identifiant JustBetter…" value="${escapeHTML(q)}"></div>
                <div class="seg" role="group" aria-label="Statut">${FILTERS.map(([id, l]) => `<button type="button" class="${id === filter ? "on" : ""}" data-filter="${id}">${l}</button>`).join("")}</div></div>
            <div class="card adm-table-wrap"><table class="table"><thead><tr><th>Joueur</th><th class="num">Pixels</th><th class="num">Visibles</th><th class="only-d-cell">Arrivée</th><th class="only-d-cell">Dernier pixel</th><th>Statut</th></tr></thead>
                <tbody>${rows || `<tr><td colspan="6" class="t3">Personne ne correspond à cette recherche.</td></tr>`}</tbody></table></div>
        </div>
        ${side}
    </div>`;
    fillIcons(root);
}

async function act(path, body, ok) {
    try {
        await api(`/api/admin/users/${sel}/${path}`, {method: "POST", body});
        toast(`<span class="fs-small b">${ok}</span><a class="btn btn-sm btn-ghost" href="/admin/journal">Journal</a>`);
        await loadList();
    } catch (err) {
        toast(`<span class="fs-small b">${escapeHTML(err.message)}</span>`);
    }
}

export async function mount(el) {
    root = el;
    root.onclick = async e => {
        const tr = e.target.closest("tr[data-id]");
        if (tr) { sel = tr.dataset.id; detail = await api("/api/admin/users/" + sel); render(); return; }
        const f = e.target.closest("[data-filter]");
        if (f) { filter = f.dataset.filter; sel = null; loadList(); return; }
        const du = e.target.closest("[data-duree]");
        if (du) { duree = du.dataset.duree; render(); return; }
        if (e.target.closest(".js-suspend")) {
            const motif = prompt("Motif de la suspension (le joueur le verra) :", "Beaucoup de pixels posés en très peu de temps : on dirait un script automatique.");
            if (motif !== null) act("suspend", {duree, motif}, "Compte suspendu");
        }
        if (e.target.closest(".js-ban")) {
            const motif = prompt("Motif du bannissement (le joueur le verra) :");
            if (motif?.trim()) act("ban", {motif}, "Compte banni");
        }
        if (e.target.closest(".js-unban")) act("unban", {}, "Compte réactivé");
        if (e.target.closest(".js-reset") && confirm("Réinitialiser l'avatar, la bio et la bannière ?")) act("reset", {}, "Profil réinitialisé");
        if (e.target.closest(".js-role")) act("role", {role: detail.role === "admin" ? "joueur" : "admin"}, "Rôle modifié");
        if (e.target.closest(".js-save-note")) act("note", {note: $(".js-note", root).value}, "Note enregistrée");
    };
    root.onkeydown = e => { if (e.key === "Enter" && e.target.closest("tr[data-id]")) e.target.click(); };
    root.oninput = e => {
        if (!e.target.classList.contains("js-q")) return;
        clearTimeout(timer);
        timer = setTimeout(() => { q = e.target.value; sel = null; loadList().then(() => { const i = $(".js-q", root); i.focus(); i.setSelectionRange(q.length, q.length); }); }, 250);
    };
    await loadList();
}

export function unmount() {
    if (root) { root.onclick = root.oninput = root.onkeydown = null; }
}

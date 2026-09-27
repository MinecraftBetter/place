// /admin/moderation — reported contents (a-moderation).

import {api, escapeHTML, icon, avatarHTML, relativeTime} from "./admin.js";
import {$, $$, fillIcons, toast} from "./ui.js";

const TABS = [["", "Tout"], ["avatar,banniere", "Avatars"], ["bio,pseudo", "Bios & pseudos"], ["oeuvre,musee", "Noms"], ["son,livre_or", "Sons & livres d'or"]];
const KIND = {avatar: "avatar", banniere: "bannière", bio: "bio", pseudo: "pseudo", oeuvre: "titre d'œuvre", musee: "musée", son: "son", livre_or: "livre d'or"};
let root, tab = "", statut = "attente", data = null, revealed = new Set();

function item(r) {
    const isImg = r.type === "avatar" || r.type === "banniere";
    const content = isImg
        ? `<div class="mod-img${revealed.has(r.id) ? " shown" : ""}" data-reveal="${r.id}">${r.contenu ? `<img class="px" src="${escapeHTML(r.contenu)}" alt="">` : `<span class="fs-cap t3">image retirée</span>`}<span class="fs-cap t3 pretty">${revealed.has(r.id) ? "" : "Image floutée par défaut. Clique pour la voir en entier."}</span></div>`
        : r.type === "son" ? `<div class="card-2 mod-sound">${r.contenu ? `<audio controls preload="none" src="${escapeHTML(r.contenu)}"></audio>` : ""}</div>`
        : `<div class="card-2 mod-text">${r.contenu ? `<span class="fs-small">« ${escapeHTML(r.contenu)} »</span>` : `<span class="fs-small t3">(vide au moment du signalement)</span>`}</div>`;
    return `<article class="card mod-item" data-id="${r.id}">
        <div class="mod-head">${avatarHTML(r.user, "av av-32")}<div class="grow"><span class="fs-small b">${escapeHTML(r.user?.pseudo ?? "—")}</span><br><span class="fs-cap t3">${KIND[r.type] ?? r.type} · ${relativeTime(r.ts)}</span></div>
            <span class="pill ${r.source === "filtre" ? "st-info" : "st-attente"}">${r.source === "filtre" ? "filtre auto" : "signalé"}</span></div>
        ${content}
        <span class="fs-cap t2">${escapeHTML(r.raison || (r.auteur ? `Signalé par ${r.auteur.pseudo}` : ""))}${r.auteur && r.raison ? ` · signalé par ${escapeHTML(r.auteur.pseudo)}` : ""}</span>
        <div class="mod-actions">${r.statut === "attente"
            ? `<button class="btn btn-sm" type="button" data-act="garder">Garder</button><button class="btn btn-sm" type="button" data-act="masquer">Masquer</button><button class="btn btn-sm btn-ghost" type="button" data-act="message">Écrire au joueur</button>`
            : `<span class="pill st-neutre">${r.statut === "masque" ? "Masqué" : "Gardé"}</span><a class="btn btn-sm btn-ghost" href="/admin/journal?filtre=moderation">Annuler depuis le journal</a>`}</div>
    </article>`;
}

async function load() {
    data = await api(`/api/admin/reports?statut=${statut}&type=${encodeURIComponent(tab)}`);
    const f = data.filtres;
    root.innerHTML = `<div class="admin-pad adm-mod">
        <div class="adm-mod-main">
            <header class="adm-dash-head"><h1 class="fs-h1">Modération</h1><span class="grow"></span>
                <div class="seg" role="group" aria-label="Statut"><button type="button" class="${statut === "attente" ? "on" : ""}" data-statut="attente">À traiter</button><button type="button" class="${statut === "masque" ? "on" : ""}" data-statut="masque">Masqués</button><button type="button" class="${statut === "garde" ? "on" : ""}" data-statut="garde">Gardés</button></div></header>
            <div class="seg adm-tabs" role="group" aria-label="Type">${TABS.map(([id, l]) => `<button type="button" class="${id === tab ? "on" : ""}" data-tab="${id}">${l}</button>`).join("")}</div>
            <p class="fs-small t2">Contenus signalés par les joueurs ou repérés par le filtre automatique. Les dessins sur le canvas ne passent pas par ici : entre amis, on peut dessiner par-dessus.</p>
            <div class="mod-list">${data.reports.map(item).join("") || `<div class="card empty-state"><span class="fs-body b">${statut === "attente" ? "Tout est traité" : "Rien ici"}</span><span class="fs-small t2">${statut === "attente" ? "Aucun contenu n'attend de décision." : ""}</span></div>`}</div>
        </div>
        <aside class="adm-mod-side">
            <span class="fs-h3">Filtre automatique</span>
            <div class="card adm-toggles">
                ${[["words_filter", "Mots interdits (pseudos, bios, titres)"], ["no_links", "Pas de liens dans les bios"], ["avatar_check", "Avatars et bannières : vérification"], ["sounds_check", "Sons des musées : validation obligatoire"]].map(([k, l]) =>
                    `<div class="edit-toggle"><span class="fs-small grow">${l}</span><button class="toggle${f[k] ? " toggle-on" : ""}" type="button" data-filter="${k}" aria-pressed="${!!f[k]}" aria-label="${l}"></button></div>`).join("")}
            </div>
            <div class="field"><label class="label" for="words">Mots interdits</label><textarea id="words" class="textarea js-words">${escapeHTML(f.words)}</textarea><span class="hint">Séparés par des virgules.</span>
                <button class="btn btn-sm js-save-words" type="button">Enregistrer la liste</button></div>
            <span class="fs-cap t3 pretty">« Masquer » remplace le contenu par la valeur par défaut (avatar généré, bio vide, titre « Sans titre ») et prévient le joueur.</span>
        </aside>
    </div>`;
    fillIcons(root);
}

async function saveFilter(patch) {
    const cur = (await api("/api/admin/settings")).settings;
    await api("/api/admin/settings", {method: "PUT", body: {...cur, ...patch}});
}

export async function mount(el) {
    root = el;
    root.onclick = async e => {
        const t = e.target.closest("[data-tab]");
        if (t) { tab = t.dataset.tab; load(); return; }
        const s = e.target.closest("[data-statut]");
        if (s) { statut = s.dataset.statut; load(); return; }
        const rv = e.target.closest("[data-reveal]");
        if (rv) { revealed.add(Number(rv.dataset.reveal)); rv.classList.add("shown"); rv.querySelector(".fs-cap").textContent = ""; return; }
        const f = e.target.closest("[data-filter]");
        if (f) { await saveFilter({[f.dataset.filter]: f.getAttribute("aria-pressed") !== "true"}); load(); return; }
        if (e.target.closest(".js-save-words")) { await saveFilter({words: $(".js-words", root).value}); toast(`<span class="fs-small b">Liste enregistrée</span>`); return; }
        const a = e.target.closest("[data-act]");
        if (!a) return;
        const id = a.closest("[data-id]").dataset.id;
        try {
            if (a.dataset.act === "message") {
                const message = prompt("Message au joueur (il le recevra dans ses alertes) :");
                if (!message?.trim()) return;
                await api(`/api/admin/reports/${id}/message`, {method: "POST", body: {message}});
                toast(`<span class="fs-small b">Message envoyé</span>`);
                return;
            }
            await api(`/api/admin/reports/${id}/${a.dataset.act}`, {method: "POST"});
            toast(`<span class="fs-small b">${a.dataset.act === "masquer" ? "Contenu masqué, le joueur est prévenu" : "Contenu gardé"}</span>`);
            load();
        } catch (err) {
            toast(`<span class="fs-small b">${escapeHTML(err.message)}</span>`);
        }
    };
    await load();
}

export function unmount() {
    if (root) root.onclick = null;
}

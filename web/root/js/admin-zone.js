// /admin/zone — restore, erase or give a zone of the canvas (a-zone).

import {api, escapeHTML, icon, avatarHTML} from "./admin.js";
import {$, $$, fillIcons, toast} from "./ui.js";
import {ZoneSelector} from "./zone-select.js";
import {formatNumber} from "./view.js";

const MODES = [["restore", "Restaurer"], ["erase", "Effacer"], ["assign", "Attribuer"]];
const TOOLS = [["rect", "Rectangle", "rect"], ["lasso", "Lasso", "lasso"], ["wand", "Baguette", "wand"], ["player", "Pixels d'un joueur", "users"]];
const BADGES = {pionnier: "Pionnier", veteran: "Vétéran 2022", batisseur: "Bâtisseur", indemodable: "Indémodable"};
let root, zs, mode = "restore", frame = null, backups = [], preview = null, badgeSel = new Set(), assignUser = null, pos = 50, previewTimer = null, seq = 0;

function renderPanel() {
    const m = zs.mask();
    const panel = $(".js-zone-panel", root);
    const impact = preview ? (mode === "assign"
        ? `${formatNumber(preview.pixels)} pixels · ${preview.joueurs.length ? `${preview.joueurs.length} joueur${preview.joueurs.length > 1 ? "s y ont" : " y a"} des pixels` : "aucun pixel de joueur connu"}`
        : `${formatNumber(preview.modifies ?? 0)} pixels changent sur ${formatNumber(preview.pixels)} · ${preview.joueurs.length} joueur${preview.joueurs.length > 1 ? "s" : ""} concerné${preview.joueurs.length > 1 ? "s" : ""}`) : "Sélectionne une zone sur le canvas.";
    const players = preview?.joueurs?.slice(0, 5).map(p => `${escapeHTML(p.user?.pseudo ?? "")} (${formatNumber(p.pixels)})`).join(", ") ?? "";
    let body = "";
    if (mode === "restore") {
        body = `<div class="field"><span class="label">Restaurer depuis la sauvegarde du</span>
            <div class="adm-backups">${backups.length ? backups.slice(0, 8).map(b => `<button type="button" class="card-2 adm-backup${b.frame === frame ? " on" : ""}" data-frame="${b.frame}">
                <span class="mono fs-small b">${new Date(b.t).toLocaleString("fr-FR", {day: "2-digit", month: "2-digit", year: "2-digit", hour: "2-digit", minute: "2-digit"})}</span>
                <span class="fs-cap t3">${b.differents ? `${formatNumber(b.differents)} px différents d'aujourd'hui` : "identique à aujourd'hui"}</span></button>`).join("")
                : `<span class="fs-small t3">${m ? "Aucune sauvegarde pour cette zone." : "Sélectionne une zone pour voir ses sauvegardes."}</span>`}</div></div>
            ${preview?.apres ? `<div class="field"><span class="label">Avant / après</span>
                <div class="adm-ba"><img class="px" src="${preview.apres}" alt="Après"><div class="adm-ba-now js-ba-now" style="clip-path: inset(0 0 0 ${pos}%)"><img class="px" src="${preview.maintenant}" alt="Maintenant"></div>
                <div class="adm-ba-line" style="left: ${pos}%"></div><span class="pill adm-ba-l">après</span><span class="pill adm-ba-r">maintenant</span></div>
                <input type="range" min="0" max="100" value="${pos}" class="js-ba" aria-label="Comparer avant et après"></div>` : ""}`;
    } else if (mode === "erase") {
        body = `<div class="card-2 adm-erase"><span class="fs-small b">Effacer la zone</span>
            <span class="fs-cap t2 pretty">${preview ? `Les ${formatNumber(preview.pixels)} pixels deviennent blancs.` : "Les pixels de la zone deviennent blancs."} À réserver aux cas où aucune sauvegarde propre n'existe.</span>
            <div class="adm-user-row"><span class="fs-cap t3">Remplir avec</span><span class="swatch swatch-s" style="background: #fff"></span><span class="fs-cap">blanc</span></div></div>`;
    } else {
        const proposed = preview?.analyse?.badges ?? [];
        body = `<div class="field"><label class="label" for="acompte">Attribuer la zone à</label>
                <div class="search">${icon("search", 16)}<input id="acompte" class="input js-assign-q" placeholder="Pseudo…" value="${escapeHTML(assignUser?.pseudo ?? "")}" autocomplete="off"><div class="card claim-results js-assign-res" hidden></div></div></div>
            <div class="field"><label class="label" for="atitre">Titre de l'œuvre</label><input id="atitre" class="input js-titre" placeholder="Dunes au couchant"></div>
            <div class="field"><span class="label">Badges ${proposed.length ? "calculés depuis les sauvegardes" : ""}</span>
                <div class="adm-badges">${Object.entries(BADGES).map(([id, l]) => `<button type="button" class="chip adm-badge${badgeSel.has(id) ? " on" : ""}" data-badge="${id}" aria-pressed="${badgeSel.has(id)}"><img src="/img/badges/${id}.png" alt="" class="px" width="18" height="18">${l}${proposed.includes(id) ? "" : " <span class=\"t3\">(non proposé)</span>"}</button>`).join("")}</div></div>
            <span class="fs-cap t3 pretty">Sans demande préalable : le joueur est prévenu et voit l'œuvre sur son profil.</span>`;
    }
    panel.innerHTML = `
        <div><h1 class="fs-h2">Outil zone</h1><span class="fs-small t2">Agir sur une zone du canvas. Tout est enregistré dans le journal et peut être annulé.</span></div>
        <div class="seg" role="group" aria-label="Action">${MODES.map(([id, l]) => `<button type="button" class="${id === mode ? "on" : ""}" data-mode="${id}">${l}</button>`).join("")}</div>
        ${body}
        <div class="card-2 adm-impact"><span class="fs-small"><b>${impact}</b></span>${players ? `<span class="fs-cap t3">${players}</span>` : ""}</div>
        <div class="field"><label class="label" for="amotif">Motif (journal)</label><input id="amotif" class="input js-motif" placeholder="Spam de bot_404, restauration demandée…"></div>
        <button type="button" class="btn btn-primary btn-px js-apply" ${m && (mode !== "restore" || frame !== null) ? "" : "disabled"}>${mode === "restore" ? "Restaurer la zone" : mode === "erase" ? "Effacer la zone" : "Attribuer la zone"}</button>`;
    fillIcons(panel);
}

async function refresh() {
    const m = zs.mask();
    $(".js-desc", root).textContent = zs.describe();
    preview = null;
    if (!m) { backups = []; frame = null; renderPanel(); return; }
    const my = ++seq;
    if (mode === "restore") {
        const b = await api("/api/admin/zones/backups", {method: "POST", body: {masque: m}}).catch(() => ({backups: []}));
        if (my !== seq) return;
        backups = b.backups;
        if (frame === null || !backups.some(x => x.frame === frame)) frame = backups.find(x => x.differents > 0)?.frame ?? backups[0]?.frame ?? null;
    }
    const p = await api("/api/admin/zones/preview", {method: "POST", body: {masque: m, action: mode, frame: frame ?? 0}}).catch(err => ({error: err.message}));
    if (my !== seq) return;
    preview = p.error ? null : p;
    if (mode === "assign" && preview?.analyse && !badgeSel.size) (preview.analyse.badges ?? []).forEach(b => BADGES[b] && badgeSel.add(b));
    renderPanel();
}

function scheduleRefresh() {
    clearTimeout(previewTimer);
    previewTimer = setTimeout(refresh, 250);
}

export async function mount(el) {
    root = el;
    root.innerHTML = `<div class="adm-zone-layout">
        <main class="adm-zone-stage void js-stage"><canvas class="zs-gl"></canvas><canvas class="zs-overlay"></canvas>
            <div class="adm-zone-top"><div class="glass adm-zone-tools js-tools">${TOOLS.map(([id, l, ic]) => `<button type="button" data-tool="${id}" aria-pressed="${id === "rect"}">${icon(ic, 16)}${l}</button>`).join("")}</div>
                <span class="grow"></span><div class="glass adm-zone-desc"><span class="mono fs-small js-desc">Aucune zone</span></div></div>
            <div class="glass adm-zone-hint js-hint"><span class="fs-small t2">Glisse pour sélectionner · clic droit ou molette pour te déplacer</span></div>
            <div class="glass adm-zone-player js-player" hidden><div class="search">${icon("search", 16)}<input class="input js-player-q" placeholder="Pseudo du joueur…" autocomplete="off"><div class="card claim-results js-player-res" hidden></div></div></div>
        </main>
        <aside class="adm-zone-panel js-zone-panel"></aside>
    </div>`;
    fillIcons(root);
    zs = new ZoneSelector($(".js-stage", root));
    zs.onChange = () => { frame = null; scheduleRefresh(); };
    await zs.load();
    const q = new URLSearchParams(location.search);
    if (q.has("x")) {
        const x = Number(q.get("x")), y = Number(q.get("y")), w = Number(q.get("w") || 32), h = Number(q.get("h") || 32);
        zs.rect = {x0: x, y0: y, x1: x + w - 1, y1: y + h - 1};
        zs.focus(x, y, w, h);
        zs.changed();
    }
    if (q.has("user")) selectPlayer(q.get("user"));
    renderPanel();

    async function selectPlayer(id, pseudo) {
        const r = await api(`/api/admin/users/${id}/pixels`);
        if (!r.masque) { toast(`<span class="fs-small b">Ce joueur n'a aucun pixel visible.</span>`); return; }
        const minutes = r.premier ? Math.max(1, Math.round((r.dernier - r.premier) / 60000)) : 0;
        zs.setBitmap(r.masque, pseudo ?? "");
        for (const b of $$(".js-tools [data-tool]", root)) b.setAttribute("aria-pressed", String(b.dataset.tool === "player"));
        $(".js-hint", root).innerHTML = `<span class="fs-small">Sélection auto : <b>tous les pixels${pseudo ? " de " + escapeHTML(pseudo) : ""}</b> (${formatNumber(r.pixels)} px${minutes ? ` en ${minutes < 120 ? minutes + " min" : Math.round(minutes / 60) + " h"}` : ""})</span>`;
        zs.focus(r.masque.x, r.masque.y, r.masque.w, r.masque.h);
    }

    root.onclick = async e => {
        const t = e.target.closest("[data-tool]");
        if (t) {
            for (const b of $$(".js-tools [data-tool]", root)) b.setAttribute("aria-pressed", String(b === t));
            $(".js-player", root).hidden = t.dataset.tool !== "player";
            if (t.dataset.tool !== "player") zs.setTool(t.dataset.tool);
            $(".js-hint", root).innerHTML = `<span class="fs-small t2">${t.dataset.tool === "wand" ? "Clique une couleur : les pixels voisins de même couleur s'ajoutent" : t.dataset.tool === "player" ? "Cherche un joueur : tous ses pixels visibles sont sélectionnés" : "Glisse pour sélectionner · clic droit ou molette pour te déplacer"}</span>`;
            return;
        }
        const md = e.target.closest("[data-mode]");
        if (md) { mode = md.dataset.mode; scheduleRefresh(); renderPanel(); return; }
        const fr = e.target.closest("[data-frame]");
        if (fr) { frame = Number(fr.dataset.frame); refresh(); return; }
        const bd = e.target.closest("[data-badge]");
        if (bd) { badgeSel.has(bd.dataset.badge) ? badgeSel.delete(bd.dataset.badge) : badgeSel.add(bd.dataset.badge); renderPanel(); return; }
        const pr = e.target.closest(".js-player-res [data-id]");
        if (pr) { $(".js-player-res", root).hidden = true; selectPlayer(pr.dataset.id, pr.dataset.pseudo); return; }
        const ar = e.target.closest(".js-assign-res [data-slug]");
        if (ar) { assignUser = {slug: ar.dataset.slug, pseudo: ar.dataset.pseudo}; renderPanel(); return; }
        if (e.target.closest(".js-apply")) {
            const m = zs.mask();
            const body = {masque: m, action: mode, frame: frame ?? 0, motif: $(".js-motif", root).value};
            let text = "";
            if (mode === "assign") {
                if (!assignUser) { toast(`<span class="fs-small b">Choisis le joueur.</span>`); return; }
                Object.assign(body, {user: assignUser.slug, titre: $(".js-titre", root).value, badges: [...badgeSel]});
                text = `Attribuer « ${body.titre || "?"} » à ${assignUser.pseudo} ?`;
            } else {
                text = `${mode === "restore" ? "Restaurer" : "Effacer"} ${formatNumber(preview?.modifies ?? preview?.pixels ?? 0)} pixels ? L'action peut être annulée depuis le journal pendant 30 jours.`;
            }
            if (!confirm(text)) return;
            try {
                await api("/api/admin/zones/apply", {method: "POST", body});
                toast(`<span class="fs-small b">C'est fait</span><a class="btn btn-sm btn-ghost" href="/admin/journal">Journal</a>`);
                await zs.reload();
                refresh();
            } catch (err) {
                toast(`<span class="fs-small b">${escapeHTML(err.message)}</span>`);
            }
        }
    };
    let searchTimer = null;
    root.oninput = e => {
        if (e.target.classList.contains("js-ba")) {
            pos = Number(e.target.value);
            $(".js-ba-now", root).style.clipPath = `inset(0 0 0 ${pos}%)`;
            $(".adm-ba-line", root).style.left = pos + "%";
            return;
        }
        const isPlayer = e.target.classList.contains("js-player-q"), isAssign = e.target.classList.contains("js-assign-q");
        if (!isPlayer && !isAssign) return;
        clearTimeout(searchTimer);
        const box = $(isPlayer ? ".js-player-res" : ".js-assign-res", root);
        const qv = e.target.value.trim();
        if (!qv) { box.hidden = true; return; }
        searchTimer = setTimeout(async () => {
            const j = await api("/api/users/search?q=" + encodeURIComponent(qv)).catch(() => ({users: []}));
            box.innerHTML = j.users.map(u => `<button type="button" class="claim-result" data-id="${u.id}" data-slug="${escapeHTML(u.slug)}" data-pseudo="${escapeHTML(u.pseudo)}">${avatarHTML(u, "av av-24")}<span class="fs-small b">${escapeHTML(u.pseudo)}</span></button>`).join("") || `<span class="fs-small t3" style="padding: 8px">Personne.</span>`;
            box.hidden = false;
        }, 200);
    };
}

export function unmount() {
    if (root) { root.onclick = root.oninput = null; }
}

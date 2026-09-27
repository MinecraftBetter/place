// /admin/journal — every admin action, undo within 30 days, CSV export (a-journal).

import {api, escapeHTML, icon, avatarHTML} from "./admin.js";
import {$, fillIcons, toast} from "./ui.js";

const FILTERS = [["", "Tout"], ["claims", "Revendications"], ["users", "Joueurs"], ["zones", "Zones"], ["moderation", "Modération"], ["settings", "Réglages"]];
let root, filter = new URLSearchParams(location.search).get("filtre") || "";

async function load() {
    const data = await api("/api/admin/journal?filtre=" + filter);
    root.innerHTML = `<div class="admin-pad adm-journal">
        <header class="adm-dash-head"><h1 class="fs-h1">Journal des actions</h1><span class="grow"></span>
            <a class="btn btn-sm" href="/api/admin/journal.csv?filtre=${filter}" download>${icon("download", 16)}Exporter en CSV</a></header>
        <div class="act-filters">${FILTERS.map(([id, l]) => `<button type="button" class="chip${id === filter ? " chip-on" : ""}" data-filter="${id}">${l}</button>`).join("")}</div>
        <div class="card adm-table-wrap"><table class="table"><thead><tr><th>Quand</th><th>Admin</th><th>Action</th><th>Détail</th><th></th></tr></thead><tbody>
            ${data.actions.map(a => `<tr class="${a.annulee ? "adm-undone" : ""}">
                <td class="mono fs-small t2">${new Date(a.ts).toLocaleString("fr-FR", {day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit"})}</td>
                <td><div class="adm-user">${avatarHTML(a.admin, "av av-24")}<span class="fs-small">${escapeHTML(a.admin?.pseudo ?? "")}</span></div></td>
                <td><span class="pill ${a.classe}">${escapeHTML(a.action)}</span></td>
                <td class="fs-small">${escapeHTML(a.detail)}${a.annulee ? ` <span class="t3">· annulée</span>` : ""}</td>
                <td>${a.annulable ? `<button class="btn btn-sm btn-ghost" type="button" data-undo="${a.id}">${icon("undo", 14)}Annuler</button>` : ""}</td>
            </tr>`).join("") || `<tr><td colspan="5" class="t3">Aucune action pour l'instant.</td></tr>`}
        </tbody></table></div>
        <p class="fs-cap t3">Chaque action garde l'état d'avant : les actions sur le canvas, les attributions, les sanctions et la modération sont annulables pendant 30 jours.</p>
    </div>`;
    fillIcons(root);
}

export async function mount(el, ctx) {
    root = el;
    root.onclick = async e => {
        const f = e.target.closest("[data-filter]");
        if (f) { filter = f.dataset.filter; load(); return; }
        const u = e.target.closest("[data-undo]");
        if (u && confirm("Annuler cette action ?")) {
            try {
                await api(`/api/admin/journal/${u.dataset.undo}/undo`, {method: "POST"});
                toast(`<span class="fs-small b">Action annulée</span>`);
                load();
                ctx.refreshCounts();
            } catch (err) {
                toast(`<span class="fs-small b">${escapeHTML(err.message)}</span>`);
            }
        }
    };
    await load();
}

export function unmount() {
    if (root) root.onclick = null;
}

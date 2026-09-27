// « Signaler » : a small dialog to report a profile or an artwork name to the team.

import {$, escapeHTML, icon, toast} from "./ui.js";
import {loginHref} from "./shell.js";

export const reportButtonHTML = label => `<button class="btn btn-icon btn-ghost js-report" type="button" aria-label="${escapeHTML(label)}" title="${escapeHTML(label)}">${icon("flag", 18)}</button>`;

// choices: [[type, label], …] — e.g. [["avatar", "Son avatar"], ["bio", "Sa bio"]]
export function openReport({me, cible, choices, title}) {
    if (!me) { location.href = loginHref(); return; }
    let dlg = $("#report-dialog");
    if (!dlg) {
        dlg = document.createElement("dialog");
        dlg.id = "report-dialog";
        dlg.className = "card elev report-dialog";
        document.body.appendChild(dlg);
    }
    dlg.innerHTML = `<form method="dialog" class="report-form">
        <div class="sec-head"><h2 class="fs-h3">${escapeHTML(title)}</h2><button class="btn btn-icon btn-sm btn-ghost" value="cancel" type="submit" aria-label="Fermer">${icon("close", 16)}</button></div>
        <p class="fs-small t2 pretty">L'équipe regarde chaque signalement. Les dessins sur le canvas ne se signalent pas : entre amis, on peut dessiner par-dessus.</p>
        <fieldset class="report-choices"><legend class="label">Qu'est-ce qui pose problème ?</legend>
            ${choices.map(([t, l], i) => `<label class="report-choice"><input type="radio" name="type" value="${t}"${i === 0 ? " checked" : ""}><span class="fs-small">${escapeHTML(l)}</span></label>`).join("")}</fieldset>
        <div class="field"><label class="label" for="report-raison">Un mot pour l'équipe (facultatif)</label>
            <textarea id="report-raison" class="textarea" maxlength="280" placeholder="Ce qui te gêne…"></textarea></div>
        <div class="report-actions"><button class="btn btn-ghost" value="cancel" type="submit">Annuler</button><button class="btn btn-primary js-send" type="button">Envoyer</button></div>
    </form>`;
    $(".js-send", dlg).addEventListener("click", async () => {
        const type = dlg.querySelector("input[name=type]:checked")?.value;
        const r = await fetch("/api/reports", {method: "POST", headers: {"Content-Type": "application/json"},
            body: JSON.stringify({type, cible, raison: $("#report-raison", dlg).value})}).catch(() => null);
        const j = await r?.json().catch(() => ({}));
        if (!r?.ok) { toast(`<span class="fs-small b">${escapeHTML(j?.error ?? "Envoi impossible")}</span>`); return; }
        dlg.close();
        toast(`<span class="fs-small b">Merci, l'équipe va regarder</span>`);
    });
    dlg.showModal();
}

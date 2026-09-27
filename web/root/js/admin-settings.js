// /admin/reglages — settings (a-reglages).

import {api, escapeHTML, icon, relativeTime} from "./admin.js";
import {$, $$, fillIcons, toast} from "./ui.js";
import {formatNumber} from "./view.js";

const MODES = [["normal", "Normal"], ["readonly", "Lecture seule"], ["maintenance", "Maintenance"]];
const MODE_HINT = {
    normal: "Tout le monde peut dessiner (les invités regardent).",
    readonly: "Le canvas est figé : on regarde, on admire. Le bouton Poser explique pourquoi. Les admins peuvent encore dessiner.",
    maintenance: "Le canvas affiche l'écran de maintenance ; le timelapse et le musée restent ouverts.",
};
const STYLES = [["event", "Événement"], ["info", "Info"], ["danger", "Alerte"]];
let root, st = null, saved = null, info = null;

const dirty = () => JSON.stringify(st) !== JSON.stringify(saved);

function render() {
    const bannerCls = {event: "banner-gold", info: "banner-info", danger: "banner-danger"}[st.banner_style];
    root.innerHTML = `<div class="admin-pad adm-settings">
        <header class="adm-dash-head"><h1 class="fs-h1">Réglages</h1></header>
        <div class="adm-settings-grid">
            <section class="card adm-card"><span class="fs-h3">Pose de pixels</span>
                <div class="field"><div class="sec-head"><label class="label" for="delai">Délai entre deux pixels</label><span class="mono b">${st.cooldown} s</span></div>
                    <input id="delai" type="range" min="0" max="60" value="${st.cooldown}" data-num="cooldown"><span class="hint">0 s = pas de limite. Les invités ne peuvent pas dessiner.</span></div>
                <div class="field"><div class="sec-head"><label class="label" for="delai2">Nouveaux comptes (première heure)</label><span class="mono b">${st.cooldown_new} s</span></div>
                    <input id="delai2" type="range" min="0" max="60" value="${st.cooldown_new}" data-num="cooldown_new"><span class="hint">Freine les scripts sans gêner les vrais joueurs.</span></div>
                <div class="field"><div class="sec-head"><label class="label" for="rate">Anti-flood (pixels par seconde)</label><span class="mono b">${st.max_rate || "illimité"}</span></div>
                    <input id="rate" type="range" min="0" max="120" step="5" value="${st.max_rate}" data-num="max_rate"><span class="hint">Invisible pour qui dessine à la main ; 0 = pas de limite.</span></div>
            </section>
            <section class="card adm-card"><span class="fs-h3">Mode du canvas</span>
                <div class="seg" role="group" aria-label="Mode">${MODES.map(([id, l]) => `<button type="button" class="${id === st.mode ? "on" : ""}" data-mode="${id}">${l}</button>`).join("")}</div>
                <span class="fs-small t2 pretty">${MODE_HINT[st.mode]}</span>
                ${st.mode !== "normal" ? `<div class="field"><label class="label" for="mmsg">Message affiché</label><input id="mmsg" class="input" data-text="mode_msg" value="${escapeHTML(st.mode_msg)}" placeholder="Vernissage du musée à 18 h : canvas figé jusque-là."></div>
                    <div class="field"><label class="label" for="mfin">Fin prévue</label><input id="mfin" class="input input-mono" data-text="mode_until" value="${escapeHTML(st.mode_until)}" placeholder="18:00"></div>` : ""}
            </section>
            <section class="card adm-card"><div class="sec-head"><span class="fs-h3">Bannière d'annonce</span><button class="toggle${st.banner_on ? " toggle-on" : ""}" type="button" data-bool="banner_on" aria-pressed="${st.banner_on}" aria-label="Afficher la bannière"></button></div>
                <div class="field"><label class="label" for="btxt">Texte</label><input id="btxt" class="input" data-text="banner_text" value="${escapeHTML(st.banner_text)}" placeholder="Soirée pixel samedi 21 h : délai réduit à 2 s !"></div>
                <div class="adm-user-row"><div class="seg" role="group" aria-label="Style">${STYLES.map(([id, l]) => `<button type="button" class="${id === st.banner_style ? "on" : ""}" data-style="${id}">${l}</button>`).join("")}</div></div>
                <div class="adm-two"><div class="field"><label class="label" for="bfrom">Du</label><input id="bfrom" type="date" class="input input-mono" data-text="banner_from" value="${escapeHTML(st.banner_from)}"></div>
                    <div class="field"><label class="label" for="buntil">Au</label><input id="buntil" type="date" class="input input-mono" data-text="banner_until" value="${escapeHTML(st.banner_until)}"></div></div>
                <span class="fs-cap t3">Aperçu</span>
                <div class="card-2 adm-banner-prev"><div class="banner ${bannerCls}">${icon(st.banner_style === "danger" ? "flag" : st.banner_style === "info" ? "bell" : "star", 16)}<span>${escapeHTML(st.banner_text || "Le texte de l'annonce")}</span></div></div>
            </section>
            <section class="card adm-card"><span class="fs-h3">Serveur et sauvegardes</span>
                <div class="adm-two"><div class="field"><label class="label" for="cmax">Connexions max</label><input id="cmax" type="number" min="1" class="input input-mono" data-num="max_conns" value="${st.max_conns}"><span class="hint">option -count</span></div>
                    <div class="field"><label class="label">Écriture de place.png</label><input class="input input-mono" value="${info.save_interval} s" disabled><span class="hint">option -saveInterval</span></div></div>
                <div class="field"><label class="label" for="contact">Adresse de contact</label><input id="contact" class="input" data-text="contact" value="${escapeHTML(st.contact)}" placeholder="equipe@justbetter.fr"><span class="hint">montrée aux comptes suspendus</span></div>
                <div class="card-2 adm-server-info">
                    <span class="fs-small"><span class="live" style="display: inline-block"></span> place.png écrit ${info.saved_at ? relativeTime(info.saved_at) : "—"}</span>
                    <span class="fs-small t2">Sauvegardes (backup.sh) : ${info.backups.status === "ready" ? `${formatNumber(info.backups.frames)} captures indexées, la dernière ${relativeTime(info.backups.last)}` : info.backups.status === "indexing" ? `indexation en cours (${info.backups.total ? Math.floor(info.backups.done / info.backups.total * 100) : 0} %)` : "aucune trouvée"}</span>
                    <span class="fs-small t2">Timelapse et archives mensuelles : timelapse.sh (inchangé)</span>
                </div>
                <div class="card-2 adm-server-info">
                    <span class="fs-small b">Analyse des œuvres</span>
                    <span class="fs-small t2 pretty">${info.reanalyse ? `Dernière analyse ${relativeTime(info.reanalyse.le)} sur ${formatNumber(info.reanalyse.sauvegardes)} captures : ${info.reanalyse.oeuvres} œuvre${info.reanalyse.oeuvres > 1 ? "s" : ""}, ${info.reanalyse.revendications} revendication${info.reanalyse.revendications > 1 ? "s" : ""} en attente, ${info.reanalyse.badges.length} nouveau${info.reanalyse.badges.length > 1 ? "x" : ""} badge${info.reanalyse.badges.length > 1 ? "s" : ""}.` : "Relancée automatiquement quand de nouvelles captures arrivent."}</span>
                    ${info.reanalyse?.badges.length ? `<span class="fs-cap t3">${info.reanalyse.badges.slice(0, 6).map(g => `${escapeHTML(g.user?.pseudo ?? "")} : ${escapeHTML(g.badge)} (« ${escapeHTML(g.titre)} »)`).join(" · ")}</span>` : ""}
                    <button class="btn btn-sm js-reanalyse" type="button">Réanalyser maintenant</button>
                </div>
            </section>
            <section class="card adm-card"><div class="sec-head"><span class="fs-h3">Musées perso</span><span class="bonus">bonus</span></div>
                <div class="adm-toggles">${[["sounds_check", "Sons importés validés par l'équipe"], ["voice_limit", "Commentaire vocal : 30 s max"], ["guestbook_flt", "Livre d'or : filtre de mots"]].map(([k, l]) =>
                    `<div class="edit-toggle"><span class="fs-small grow">${l}</span><button class="toggle${st[k] ? " toggle-on" : ""}" type="button" data-bool="${k}" aria-pressed="${!!st[k]}" aria-label="${l}"></button></div>`).join("")}</div>
            </section>
        </div>
        <div class="glass adm-save-bar"${dirty() ? "" : " hidden"}><span class="fs-small t2">Modifications non enregistrées</span><span class="grow"></span>
            <button class="btn btn-ghost js-reset" type="button">Annuler</button><button class="btn btn-primary js-save" type="button">Enregistrer</button></div>
    </div>`;
    fillIcons(root);
}

export async function mount(el) {
    root = el;
    info = await api("/api/admin/settings");
    st = {...info.settings};
    saved = {...info.settings};
    render();
    root.onclick = async e => {
        const m = e.target.closest("[data-mode]");
        if (m) { st.mode = m.dataset.mode; render(); return; }
        const s = e.target.closest("[data-style]");
        if (s) { st.banner_style = s.dataset.style; render(); return; }
        const b = e.target.closest("[data-bool]");
        if (b) { st[b.dataset.bool] = !st[b.dataset.bool]; render(); return; }
        if (e.target.closest(".js-reset")) { st = {...saved}; render(); return; }
        if (e.target.closest(".js-reanalyse")) {
            e.target.closest(".js-reanalyse").disabled = true;
            try {
                const r = await api("/api/admin/reanalyse", {method: "POST"});
                info.reanalyse = r;
                render();
                toast(`<span class="fs-small b">${r.oeuvres} œuvres analysées, ${r.badges.length} nouveau${r.badges.length > 1 ? "x" : ""} badge${r.badges.length > 1 ? "s" : ""}</span>`);
            } catch (err) {
                toast(`<span class="fs-small b">${escapeHTML(err.message)}</span>`);
                render();
            }
            return;
        }
        if (e.target.closest(".js-save")) {
            try {
                const r = await api("/api/admin/settings", {method: "PUT", body: st});
                st = {...r.settings};
                saved = {...r.settings};
                render();
                toast(`<span class="fs-small b">Réglages enregistrés, appliqués tout de suite</span>`);
            } catch (err) {
                toast(`<span class="fs-small b">${escapeHTML(err.message)}</span>`);
            }
        }
    };
    root.oninput = e => {
        const n = e.target.dataset.num, t = e.target.dataset.text;
        if (n) st[n] = Number(e.target.value);
        if (t) st[t] = e.target.value;
        if (n && e.target.type === "range") e.target.closest(".field").querySelector(".mono.b").textContent = n === "max_rate" ? (st[n] || "illimité") : st[n] + " s";
        const bar = $(".adm-save-bar", root);
        bar.hidden = !dirty();
        if (t === "banner_text") $(".adm-banner-prev span", root).textContent = st.banner_text || "Le texte de l'annonce";
    };
}

export function unmount() {
    if (root) { root.onclick = root.oninput = null; }
}

// /admin — dashboard (a-dashboard, am-dashboard).

import {api, escapeHTML, icon, avatarHTML, columnChart, barRows} from "./admin.js";
import {$, $$, fillIcons} from "./ui.js";
import {formatNumber} from "./view.js";
import {colorName} from "./palette.js";

const PERIODS = [["jour", "24 h"], ["semaine", "7 jours"], ["mois", "30 jours"]];
const MONTHS = ["janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."];
let root, period = "jour", timer = null;

function delta(now, prev) {
    if (!prev) return now ? `<span class="delta-up">nouveau</span>` : `<span class="fs-cap t3">—</span>`;
    const d = Math.round((now - prev) / prev * 100);
    return `<span class="${d >= 0 ? "delta-up" : "delta-down"}">${d >= 0 ? "+" : ""}${d} % vs période d'avant</span>`;
}

async function render() {
    const s = await api("/api/admin/stats?periode=" + period);
    const k = s.kpi;
    root.innerHTML = `<div class="admin-pad adm-dash">
        <header class="adm-dash-head"><h1 class="fs-h1">Tableau de bord</h1><span class="fs-small t2"><span class="live" style="display: inline-block"></span> mis à jour en direct</span>
            <span class="grow"></span>
            <div class="seg" role="group" aria-label="Période">${PERIODS.map(([id, l]) => `<button type="button" class="${id === period ? "on" : ""}" data-period="${id}">${l}</button>`).join("")}</div>
            <a class="btn btn-sm" href="/admin/zone">Attribuer une zone</a></header>
        <section class="adm-kpis">
            <div class="card kpi"><span class="fs-over">Connectés</span><span class="kpi-v">${k.connectes}<span class="t3"> / ${k.slots}</span></span><span class="fs-cap t3">connexions WebSocket</span></div>
            <div class="card kpi"><span class="fs-over">Pixels · ${escapeHTML(k.periode)}</span><span class="kpi-v">${formatNumber(k.pixels)}</span>${delta(k.pixels, k.pixels_prev)}</div>
            <div class="card kpi"><span class="fs-over">Pixels · dernière heure</span><span class="kpi-v">${formatNumber(k.pixels_1h)}</span><span class="fs-cap t3">posés par les joueurs</span></div>
            <div class="card kpi"><span class="fs-over">Joueurs actifs</span><span class="kpi-v">${formatNumber(k.actifs)}</span>${delta(k.actifs, k.actifs_prev)}</div>
            <div class="card kpi"><span class="fs-over">Nouveaux comptes</span><span class="kpi-v">${formatNumber(k.nouveaux)}</span>${delta(k.nouveaux, k.nouveaux_prev)}</div>
            <a class="card kpi adm-kpi-link" href="/admin/revendications"><span class="fs-over">À valider</span><span class="kpi-v${k.a_valider ? " tgold" : ""}">${k.a_valider}</span><span class="fs-cap t3">${k.claims_attente} revendication${k.claims_attente > 1 ? "s" : ""} · ${k.reports_attente} signalement${k.reports_attente > 1 ? "s" : ""}</span></a>
        </section>
        <section class="adm-dash-row">
            <div class="card adm-card grow"><div class="adm-card-head"><h2 class="fs-h3">Pixels par heure · aujourd'hui</h2><span class="pill st-valide">en direct</span></div><div class="js-hours"></div></div>
            <div class="card adm-card adm-side-card"><div class="adm-card-head"><h2 class="fs-h3">Top contributeurs · 7 jours</h2></div><div class="js-top"></div></div>
        </section>
        <section class="adm-dash-row">
            <div class="card adm-card grow"><div class="adm-card-head"><h2 class="fs-h3">Activité historique</h2><span class="pill st-valide">données réelles · sauvegardes</span></div>
                <p class="fs-cap t3">Minutes avec au moins un changement sur le canvas, par mois.</p><div class="js-history"></div></div>
            <div class="card adm-card adm-side-card"><div class="adm-card-head"><h2 class="fs-h3">Couleurs les plus utilisées</h2><span class="pill st-valide">canvas actuel</span></div>
                <div class="js-colors"></div><p class="fs-cap t3">${formatNumber(s.couleurs.distinctes)} couleurs différentes · ${String(s.couleurs.palette_pct).replace(".", ",")} % des pixels sont dans la palette.</p></div>
        </section>
        <section class="adm-dash-row">
            <div class="card adm-card grow"><div class="adm-card-head"><h2 class="fs-h3">Heatmap d'activité</h2><span class="pill st-valide">sauvegardes + journal des pixels</span></div>
                <div class="adm-heat"><img class="px" src="/api/admin/heatmap.png" alt="Carte des changements du canvas">${s.zones.map((z, i) => `<a class="adm-heat-pin" href="/admin/zone?x=${z.x}&y=${z.y}&w=32&h=32" style="left: ${(z.x + 16) / 1024 * 100}%; top: ${(z.y + 16) / 768 * 100}%" aria-label="Zone ${i + 1}">${i + 1}</a>`).join("")}</div>
                <div class="adm-heat-legend fs-cap t3"><span>calme</span><span class="adm-heat-ramp"></span><span>très retouché</span></div></div>
            <div class="adm-side-col">
                <div class="card adm-card"><h2 class="fs-h3">Zones les plus retouchées</h2><p class="fs-cap t3">Cases de 32 × 32 px, nombre de changements (sauvegardes et pixels posés).</p>
                    ${s.zones.map((z, i) => `<a class="adm-zone-row" href="/admin/zone?x=${z.x}&y=${z.y}&w=32&h=32"><span class="mono b">${i + 1}</span><span class="coord">x ${z.x} · y ${z.y}</span><span class="fs-small grow ellip">${escapeHTML(z.oeuvre ?? "")}</span><span class="mono fs-small t2">${formatNumber(z.score)}</span></a>`).join("") || `<span class="fs-small t3">Pas encore de données.</span>`}</div>
                <div class="card adm-card"><h2 class="fs-h3">Jours les plus actifs</h2><div class="js-days"></div></div>
            </div>
        </section>
    </div>`;
    fillIcons(root);
    columnChart($(".js-hours", root), s.heures.map((v, h) => ({label: `${h} h`, short: h % 6 === 0 ? `${h} h` : "", value: v, tip: `entre ${h} h et ${h + 1} h`})),
        {height: 150, format: formatNumber, caption: "Pixels posés par heure aujourd'hui"});
    barRows($(".js-top", root), s.top.map(t => ({label: t.user?.pseudo ?? "?", value: t.pixels, href: `/admin/utilisateurs?id=${t.user?.id}`, lead: avatarHTML(t.user, "av av-24")})), {format: v => formatNumber(v)});
    if (!s.top.length) $(".js-top", root).innerHTML = `<span class="fs-small t3">Personne n'a dessiné cette semaine.</span>`;
    if (s.historique?.length) {
        columnChart($(".js-history", root), s.historique.map((m, i) => {
            const [y, mo] = m.mois.split("-");
            const lab = `${MONTHS[Number(mo) - 1]} ${y.slice(2)}`;
            return {label: lab, short: i % 3 === 0 ? lab : "", value: m.minutes, tip: `${MONTHS[Number(mo) - 1]} ${y} · minutes actives`};
        }), {height: 150, format: formatNumber, caption: "Minutes d'activité par mois"});
    } else {
        $(".js-history", root).innerHTML = `<span class="fs-small t3">Les sauvegardes ne sont pas encore indexées.</span>`;
    }
    barRows($(".js-colors", root), s.couleurs.top.map(c => ({
        label: colorName(c.hex) ?? c.hex, value: c.pixels, color: "var(--text-3)",
        lead: `<span class="swatch swatch-xs" style="background: ${c.hex}"></span>`,
    })), {format: v => formatNumber(v)});
    barRows($(".js-days", root), (s.jours ?? []).map(d => {
        const [y, m, dd] = d.date.split("-");
        return {label: `${Number(dd)} ${MONTHS[Number(m) - 1]} ${y}`, value: d.minutes};
    }), {format: v => `${formatNumber(v)} min`});
}

export async function mount(el) {
    root = el;
    root.onclick = e => {
        const p = e.target.closest("[data-period]");
        if (p) { period = p.dataset.period; render(); }
    };
    await render();
    timer = setInterval(() => { if (document.visibilityState === "visible") render(); }, 30000);
}

export function unmount() {
    clearInterval(timer);
    if (root) root.onclick = null;
}

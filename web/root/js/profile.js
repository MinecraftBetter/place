// /u/:pseudo — public profile (m-profil, d-profil, e-vides-m).

import {openReport, reportButtonHTML} from "./report.js";
import {$, avatarHTML, copyText, escapeHTML, fillIcons, icon, toast} from "./ui.js";
import {mountShell, emptyState, fetchMe} from "./shell.js";
import {dateFR, bannerHTML, hexAlpha, badgeTile} from "./common.js";
import {colorName} from "./palette.js";
import {formatNumber, relativeTime} from "./view.js";

const slug = decodeURIComponent(location.pathname.replace(/^\/u\//, "").split("/")[0]);

async function main() {
    const me0 = await fetchMe();
    await mountShell({active: me0.user?.slug === slug ? "profil" : "", overlay: true});
    const page = $("#page");
    let p;
    try {
        const resp = await fetch("/api/users/" + encodeURIComponent(slug), {cache: "no-store"});
        if (resp.status === 404) {
            page.innerHTML = `<div class="page-pad">${emptyState({title: "Ce joueur n'existe pas", text: "Le lien est peut-être ancien : les pseudos peuvent changer.", mascot: true, action: `<a class="btn btn-primary btn-px" href="/ace">Aller au canvas</a>`})}</div>`;
            return;
        }
        p = await resp.json();
    } catch {
        page.innerHTML = `<div class="page-pad">${emptyState({title: "Profil indisponible", text: "Le serveur ne répond pas. Réessaie dans un instant."})}</div>`;
        return;
    }
    document.title = `${p.pseudo} · BetterPlace`;
    const accent = p.accent || "#3ee06c";
    const fav = p.couleur_pref ? colorName(p.couleur_pref) : null;
    const pct = p.pixels_poses ? Math.round(p.pixels_visibles / p.pixels_poses * 100) : 0;
    const badges = p.badges.slice().sort((a, b) => (b.earned - a.earned));
    const earnedCount = p.badges.filter(b => b.earned).length;
    const x = p.extra || {};

    const actions = `${x.musee ? `<a class="btn only-d" href="/u/${escapeHTML(p.slug)}/musee">${icon("museum", 18)}Son musée</a>` : ""}
        ${p.moi ? `<a class="btn" href="/moi/profil">${icon("edit", 18)}Modifier</a>` : ""}
        <button class="btn btn-icon js-share" type="button" aria-label="Partager le profil">${icon("link", 18)}</button>
        ${p.moi ? "" : reportButtonHTML("Signaler ce profil")}`;

    const kpis = `
        <div class="card kpi"><span class="fs-over">Pixels posés</span><span class="kpi-v">${formatNumber(p.pixels_poses)}</span></div>
        <div class="card kpi"><span class="fs-over">Encore visibles</span><span class="kpi-v">${formatNumber(p.pixels_visibles)}</span><span class="fs-cap t3">${p.pixels_poses ? `${pct} % de ses pixels` : "—"}</span></div>
        <div class="card kpi"><span class="fs-over">Pionniers (est.)</span><span class="kpi-v tgold">${formatNumber(p.pixels_pionniers)}</span><span class="fs-cap t3">d'avant les comptes</span></div>
        <div class="card kpi"><span class="fs-over">Arrivée · rang</span><span class="kpi-v kpi-date">${dateFR(p.cree_le)}</span><span class="fs-cap t3">${p.rang_total ? `#${p.rang_total} tous les temps` : "pas encore classé"}</span></div>`;

    const works = x.oeuvres?.length
        ? x.oeuvres.map(o => `<a class="card prof-work" href="${escapeHTML(o.href)}">
            <div class="prof-work-img"><img class="px" src="${escapeHTML(o.thumb)}" alt=""></div>
            <div class="prof-work-text"><span class="fs-body b">${escapeHTML(o.titre)}</span><span class="fs-small t3">${escapeHTML(o.sub || "")}</span></div>
            ${o.pill ? `<span class="pill ${escapeHTML(o.pillClass || "st-info")}">${escapeHTML(o.pill)}</span>` : ""}</a>`).join("")
        : emptyState({title: "Aucune œuvre pour l'instant", text: p.moi ? "Tu as dessiné ici avant les comptes ? Revendique ton œuvre, elle apparaîtra ici avec le badge Pionnier." : "Ses œuvres revendiquées apparaîtront ici.",
            action: p.moi ? `<a class="btn btn-sm btn-gold btn-px" href="/revendiquer">Revendiquer une œuvre</a>` : ""});

    const firstPixel = p.moi && p.pixels_poses === 0 ? `<div class="card first-pixel">
        <div class="anim-pop-loop first-pixel-sq"></div>
        <div class="first-pixel-text"><span class="fs-body b">Pose ton premier pixel</span><span class="fs-cap t3">Ça débloque ton premier badge.</span></div>
        <a class="btn btn-primary btn-px btn-sm" href="/ace">Y aller</a></div>` : "";

    const claim = x.claim_en_cours ? `<div class="card-2 prof-claim"><img src="/img/badges/pionnier.png" alt="" class="px badge-lock" width="32" height="32">
        <span class="fs-small t2">Revendication « ${escapeHTML(x.claim_en_cours.titre)} » en cours : une œuvre <b class="tgold">pionnière</b> à la clé.</span></div>` : "";

    const map = p.carte_publique || p.moi ? `<section class="prof-map-sec">
        <div class="sec-head"><h2 class="fs-h2">Ses pixels sur le canvas</h2><span class="fs-cap t3">canvas assombri, ${p.pixels_visibles ? `ses ${formatNumber(p.pixels_visibles)} pixels visibles en surbrillance` : "aucun pixel visible pour l'instant"}</span></div>
        <div class="prof-map"><img class="px" src="/api/users/${encodeURIComponent(p.slug)}/contributions.png" alt="Carte des contributions de ${escapeHTML(p.pseudo)}" loading="lazy">
        <a class="glass prof-map-btn" href="/ace"><span class="fs-cap b">Explorer</span></a></div>
        ${!p.carte_publique ? `<span class="fs-cap t3">Ta carte est masquée pour les autres joueurs.</span>` : ""}
    </section>` : "";

    page.innerHTML = `
        ${bannerHTML(p.banner, accent, "prof-banner", p.banner_url)}
        <section class="prof-head">
            <div class="prof-id">
                ${avatarHTML(p, "av prof-av", `box-shadow: 0 0 0 5px var(--bg), 0 0 0 9px ${escapeHTML(accent)}`)}
                <div class="prof-names">
                    <h1 class="fs-display prof-name">${escapeHTML(p.pseudo)}</h1>
                    <div class="prof-meta">
                        <span class="mono fs-small t3">/u/${escapeHTML(p.slug)}</span>
                        ${p.rang_semaine ? `<span class="pill" style="background: ${hexAlpha(accent, .16)}; color: ${escapeHTML(accent)}">#${p.rang_semaine} cette semaine</span>` : ""}
                        ${p.role === "admin" ? `<span class="pill st-info">Équipe</span>` : ""}
                        ${fav ? `<span class="fs-small t2 prof-fav"><span class="swatch swatch-xs" style="background: ${escapeHTML(p.couleur_pref)}"></span>couleur préférée : ${escapeHTML(fav)}</span>` : ""}
                    </div>
                </div>
            </div>
            <div class="prof-actions">${actions}</div>
        </section>
        <section class="prof-intro">
            ${p.bio ? `<p class="fs-h3 t2 pretty prof-bio">${escapeHTML(p.bio)}</p>` : `<p class="fs-small t3 prof-bio" style="font-style: italic">Pas encore de bio.</p>`}
            ${x.musee ? `<a class="card prof-museum only-m" href="/u/${escapeHTML(p.slug)}/musee">${icon("museum", 22)}<div class="grow"><span class="fs-small b">Visiter le musée de ${escapeHTML(p.pseudo)}</span><br><span class="fs-cap t3">${escapeHTML(x.musee.sub || "")}</span></div>${icon("chevron", 18)}</a>` : ""}
            <div class="prof-kpis">${kpis}</div>
        </section>
        ${firstPixel ? `<section class="prof-sec">${firstPixel}</section>` : ""}
        <section class="prof-grid">
            <div class="prof-col-map">${map}</div>
            <div class="prof-col-side">
                <section class="prof-sec">
                    <div class="sec-head"><h2 class="fs-h2">Badges</h2><span class="fs-cap t3">${earnedCount} sur ${p.badges.length}</span></div>
                    <div class="prof-badges">${badges.map(b => badgeTile(b)).join("")}</div>
                    ${claim}
                </section>
                <section class="prof-sec">
                    <div class="sec-head"><h2 class="fs-h2">Œuvres</h2>${x.oeuvres?.length ? `<a class="fs-small" href="/musee">Tout voir</a>` : ""}</div>
                    <div class="prof-works">${works}</div>
                </section>
            </div>
        </section>`;
    page.removeAttribute("aria-busy");
    fillIcons(page);

    $(".js-report", page)?.addEventListener("click", () => openReport({me: me0.user, cible: `user:${p.id}`, title: `Signaler le profil de ${p.pseudo}`,
        choices: [["avatar", "Son avatar"], ["banniere", "Sa bannière"], ["bio", "Sa bio"], ["pseudo", "Son pseudo"]]}));
    $(".js-share", page).addEventListener("click", async () => {
        const url = location.origin + "/u/" + p.slug;
        if (navigator.share && matchMedia("(pointer: coarse)").matches) {
            navigator.share({title: `${p.pseudo} sur BetterPlace`, url}).catch(() => {});
            return;
        }
        const ok = await copyText(url);
        toast(`<span class="fs-small b">${ok ? "Lien du profil copié" : "Copie impossible"}</span>`);
    });
}

main();

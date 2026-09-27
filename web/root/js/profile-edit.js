// /moi/profil — profile editor with a live preview (m-profil-edition, d-profil-edition).

import {$, $$, avatarHTML, escapeHTML, fillIcons, icon, toast} from "./ui.js";
import {mountShell, fetchMe, loginHref} from "./shell.js";
import {PALETTE, colorName} from "./palette.js";
import {bannerHTML, hexAlpha} from "./common.js";
import {formatNumber} from "./view.js";
import {avatarBlob, randomSeed} from "./avatar-gen.js";
import {PixelEditor} from "./pixel-editor.js";

const ACCENTS = [["#FF63AA", "rose"], ["#5EB3FF", "bleu evian"], ["#3EE06C", "vert"], ["#FFD623", "jaune"], ["#FFA800", "orange foncé"], ["#6A5CFF", "violet clair"], ["#00CCC0", "bleu sale de bain"], ["#FF2651", "rouge clair"]];
const BANNERS = [["desert", "Désert"], ["nuit", "Nuit"], ["uni", "Uni"], ["custom", "Dessin"]];

const slugify = s => s.toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/œ/g, "o").replace(/æ/g, "a")
    .replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 32).replace(/-+$/, "") || "joueur";

async function main() {
    const me = await fetchMe();
    if (!me.user) {
        location.replace(loginHref());
        return;
    }
    await mountShell({active: "profil"});
    const page = $("#page");
    const p = await fetch("/api/users/" + encodeURIComponent(me.user.slug), {cache: "no-store"}).then(r => r.json());
    const earned = p.badges.filter(b => b.earned);

    const st = {
        pseudo: p.pseudo, bio: p.bio, banner: p.banner || "desert",
        accent: (p.accent || "#3ee06c").toUpperCase(), fav: (p.couleur_pref || "").toUpperCase(),
        pins: p.badges_epingles.slice(), map: p.carte_publique, alerts: p.alertes_retouche ?? true,
        avatar: p.avatar, avatarBlob: null, avatarURL: null,
        bannerURL: p.banner_url || "", bannerDraft: null, bannerDirty: false,
    };

    page.innerHTML = `
    <header class="edit-head">
        <a class="btn btn-icon btn-ghost" href="/u/${escapeHTML(p.slug)}" aria-label="Retour au profil">${icon("back")}</a>
        <h1 class="fs-h2 grow">Modifier mon profil</h1>
        <a class="btn btn-ghost only-d" href="/u/${escapeHTML(p.slug)}">Annuler</a>
        <button class="btn btn-primary js-save" type="button">Enregistrer</button>
    </header>
    <div class="edit-grid">
        <div class="edit-preview-col">
            <span class="fs-over">Aperçu en direct</span>
            <div class="card elev edit-preview js-preview"></div>
        </div>
        <form class="edit-form js-form" novalidate>
            <div class="field">
                <label class="label" for="ed-pseudo">Pseudo</label>
                <input id="ed-pseudo" class="input js-pseudo" maxlength="24" autocomplete="off" value="${escapeHTML(st.pseudo)}">
                <span class="hint js-slug-hint"></span>
            </div>
            <div class="field">
                <span class="label">Avatar</span>
                <div class="edit-avatar">
                    <span class="js-avatar-now"></span>
                    <label class="btn btn-sm">${icon("upload", 16)}Importer<input type="file" accept="image/png,image/jpeg,image/gif" class="js-avatar-file" hidden></label>
                    <button class="btn btn-sm btn-ghost js-avatar-gen" type="button">${icon("dice", 16)}Générer un pixel-avatar</button>
                </div>
                <span class="hint">PNG, JPEG ou GIF, 2 Mo au plus. Il sera recadré en carré.</span>
            </div>
            <div class="field edit-wide">
                <label class="label" for="ed-bio">Bio</label>
                <textarea id="ed-bio" class="textarea js-bio" maxlength="160" style="min-height: 80px">${escapeHTML(st.bio)}</textarea>
                <span class="hint js-bio-count"></span>
            </div>
            <div class="field">
                <span class="label">Bannière</span>
                <div class="edit-banners js-banners"></div>
            </div>
            <div class="field edit-wide js-banner-editor-field" hidden>
                <span class="label">Dessine ta bannière</span>
                <div class="card pe js-banner-editor"></div>
            </div>
            <div class="field">
                <span class="label">Couleur d'accent</span>
                <div class="edit-accents js-accents"></div>
            </div>
            <div class="field edit-wide">
                <span class="label">Couleur préférée de la palette</span>
                <div class="edit-palette js-palette"></div>
            </div>
            <div class="field edit-wide">
                <div class="sec-head"><span class="label">Badges épinglés</span><span class="fs-cap t3">3 maximum</span></div>
                <div class="edit-badges js-badges">${earned.length ? "" : `<span class="fs-small t3">Tes badges apparaîtront ici : pose ton premier pixel pour commencer.</span>`}</div>
            </div>
            <div class="card edit-toggles edit-wide">
                <div class="edit-toggle"><span class="fs-small grow">Afficher ma carte de contributions</span><button class="toggle js-t-map" type="button" aria-label="Afficher ma carte de contributions"></button></div>
                <div class="edit-toggle"><span class="fs-small grow">M'avertir quand une de mes œuvres est retouchée</span><button class="toggle js-t-alerts" type="button" aria-label="Alertes de retouche"></button></div>
            </div>
            <p class="fs-cap t3 edit-wide js-error" role="alert"></p>
        </form>
    </div>`;
    page.removeAttribute("aria-busy");

    const render = () => {
        const slug = slugify(st.pseudo || p.pseudo);
        $(".js-slug-hint").textContent = `Ton adresse : ${location.host}/u/${slug}`;
        $(".js-bio-count").textContent = `${[...st.bio].length} / 160`;
        const avatarUser = {pseudo: st.pseudo, accent: st.accent, avatar: st.avatarURL || st.avatar};
        $(".js-avatar-now").innerHTML = avatarHTML(avatarUser, "av av-48");
        const drawnURL = st.bannerDraft || st.bannerURL;
        $(".js-banners").innerHTML = BANNERS.map(([id, label]) => `
            <button type="button" class="edit-banner${st.banner === id ? " on" : ""}" data-banner="${id}" aria-pressed="${st.banner === id}" style="--acc: ${st.accent}">
                ${id === "custom" && !drawnURL ? `<span class="edit-banner-draw">${icon("edit", 20)}</span>` : bannerHTML(id, st.accent, "edit-banner-img", drawnURL)}<span class="pill">${label}</span></button>`).join("");
        $(".js-banner-editor-field").hidden = st.banner !== "custom";
        $(".js-accents").innerHTML = ACCENTS.map(([hex, name]) =>
            `<button type="button" class="swatch swatch-s edit-accent${st.accent === hex ? " swatch-sel" : ""}" data-accent="${hex}" style="background: ${hex}" aria-label="${name}" title="${name}"></button>`).join("");
        $(".js-palette").innerHTML = PALETTE.map(([hex, name]) =>
            `<button type="button" class="swatch${st.fav === hex.toUpperCase() ? " swatch-sel" : ""}" data-fav="${hex.toUpperCase()}" style="background: ${hex}" aria-label="${escapeHTML(name)}" title="${escapeHTML(name)}"></button>`).join("");
        if (earned.length) {
            $(".js-badges").innerHTML = earned.map(b => `
                <button type="button" class="badge-tile edit-badge${st.pins.includes(b.id) ? " on" : ""}" data-pin="${b.id}" aria-pressed="${st.pins.includes(b.id)}" style="--acc: ${st.accent}">
                    <img class="badge-ico" src="${escapeHTML(b.sprite)}" alt=""><span class="fs-cap">${escapeHTML(b.name)}</span></button>`).join("");
        }
        for (const [sel, on] of [[".js-t-map", st.map], [".js-t-alerts", st.alerts]]) {
            $(sel).classList.toggle("toggle-on", on);
            $(sel).setAttribute("aria-pressed", String(on));
        }
        const favName = st.fav ? colorName(st.fav) : null;
        const pinned = earned.filter(b => st.pins.includes(b.id));
        $(".js-preview").innerHTML = `
            ${bannerHTML(st.banner, st.accent, "edit-prev-banner", st.bannerDraft || st.bannerURL)}
            <div class="edit-prev-body">
                ${avatarHTML(avatarUser, "av av-96", `box-shadow: 0 0 0 4px var(--surface), 0 0 0 8px ${st.accent}`)}
                <div class="edit-prev-name"><span class="pixel b">${escapeHTML(st.pseudo || "…")}</span><span class="mono fs-small t3">/u/${escapeHTML(slug)}</span></div>
                <p class="fs-body t2 pretty">${st.bio ? escapeHTML(st.bio) : `<span class="t3" style="font-style: italic">Pas encore de bio.</span>`}</p>
                <div class="edit-prev-meta">
                    ${p.rang_semaine ? `<span class="pill" style="background: ${hexAlpha(st.accent, .16)}; color: ${st.accent}">#${p.rang_semaine} cette semaine</span>` : ""}
                    ${favName ? `<span class="swatch swatch-xs" style="background: ${st.fav}"></span><span class="fs-cap t3">${escapeHTML(favName)}</span>` : ""}
                    <span class="grow"></span>
                    ${pinned.map(b => `<img src="${escapeHTML(b.sprite)}" alt="${escapeHTML(b.name)}" title="${escapeHTML(b.name)}" class="px" width="22" height="22">`).join("")}
                </div>
                <div class="edit-prev-stats">
                    <div class="card-2"><span class="mono b">${formatNumber(p.pixels_poses)}</span><br><span class="fs-cap t3">pixels posés</span></div>
                    <div class="card-2"><span class="mono b">${formatNumber(p.pixels_visibles)}</span><br><span class="fs-cap t3">encore visibles</span></div>
                    <div class="card-2"><span class="mono b tgold">${formatNumber(p.pixels_pionniers)}</span><br><span class="fs-cap t3">pionniers</span></div>
                </div>
            </div>`;
    };
    const editor = new PixelEditor($(".js-banner-editor"));
    if (st.bannerURL) await editor.load(st.bannerURL);
    else editor.startingPoint(st.accent);
    editor.onChange = () => {
        st.bannerDraft = editor.toDataURL();
        st.bannerDirty = true;
        render();
    };
    render();

    $(".js-pseudo").addEventListener("input", e => { st.pseudo = e.target.value; render(); });
    $(".js-bio").addEventListener("input", e => { st.bio = e.target.value.slice(0, 160); render(); });
    page.addEventListener("click", e => {
        const t = e.target.closest("[data-banner], [data-accent], [data-fav], [data-pin]");
        if (!t) return;
        if (t.dataset.banner) st.banner = t.dataset.banner;
        if (t.dataset.accent) st.accent = t.dataset.accent;
        if (t.dataset.fav) st.fav = st.fav === t.dataset.fav ? "" : t.dataset.fav;
        if (t.dataset.pin) {
            const id = t.dataset.pin;
            if (st.pins.includes(id)) st.pins = st.pins.filter(x => x !== id);
            else if (st.pins.length < 3) st.pins.push(id);
            else toast(`<span class="fs-small b">3 badges épinglés au maximum</span>`);
        }
        render();
    });
    $(".js-t-map").addEventListener("click", () => { st.map = !st.map; render(); });
    $(".js-t-alerts").addEventListener("click", () => { st.alerts = !st.alerts; render(); });

    const setBlob = blob => {
        if (st.avatarURL) URL.revokeObjectURL(st.avatarURL);
        st.avatarBlob = blob;
        st.avatarURL = URL.createObjectURL(blob);
        render();
    };
    $(".js-avatar-gen").addEventListener("click", async () => setBlob(await avatarBlob(randomSeed(), st.accent)));
    $(".js-avatar-file").addEventListener("change", e => {
        const f = e.target.files[0];
        if (!f) return;
        if (f.size > 2 * 1024 * 1024) {
            toast(`<span class="fs-small b">Image trop lourde : 2 Mo au plus.</span>`);
            return;
        }
        setBlob(f);
    });

    $(".js-save").addEventListener("click", async () => {
        const btn = $(".js-save");
        btn.disabled = true;
        $(".js-error").textContent = "";
        try {
            if (st.banner === "custom" && (st.bannerDirty || !st.bannerURL)) {
                const fd = new FormData();
                fd.append("banner", await editor.toBlob(), "banner.png");
                const r = await fetch("/api/me/banner", {method: "POST", body: fd});
                const j = await r.json();
                if (!r.ok) throw new Error(j.error || "Bannière refusée.");
            }
            if (st.avatarBlob) {
                const fd = new FormData();
                fd.append("avatar", st.avatarBlob, "avatar.png");
                const r = await fetch("/api/me/avatar", {method: "POST", body: fd});
                const j = await r.json();
                if (!r.ok) throw new Error(j.error || "Avatar refusé.");
            }
            const r = await fetch("/api/me/profile", {
                method: "PATCH", headers: {"Content-Type": "application/json"},
                body: JSON.stringify({pseudo: st.pseudo, bio: st.bio, banner: st.banner, accent: st.accent, couleur_pref: st.fav,
                    badges_epingles: st.pins, carte_publique: st.map, alertes_retouche: st.alerts}),
            });
            const j = await r.json();
            if (!r.ok) throw new Error(j.error || "Enregistrement impossible.");
            location.href = "/u/" + j.slug;
        } catch (err) {
            $(".js-error").textContent = err.message;
            $(".js-error").classList.add("tdanger");
            btn.disabled = false;
        }
    });
    fillIcons(page);
}

main();

// /musee/exposer — exhibit one of your artworks (m-exposer).

import {$, $$, escapeHTML, fillIcons, icon, toast} from "./ui.js";
import {mountShell, emptyState, loginHref, fetchMe} from "./shell.js";
import {artOf, artFit, frameClass, SALLE_LABELS} from "./common.js";

const SALLES = ["paysages", "personnages", "clins-d-oeil", "drapeaux-logos", "motifs", "monuments"];
const FRAMES = [["bois", "Bois"], ["or", "Or"], ["sans", "Sans cadre"]];

async function main() {
    const m = await fetchMe();
    if (!m.user) { location.replace(loginHref()); return; }
    await mountShell({active: "musee"});
    const page = $("#page");
    const data = await fetch("/api/musee", {cache: "no-store"}).then(r => r.json());
    const mine = data.miennes ?? [];
    if (!mine.length) {
        page.innerHTML = `<div class="page-pad">${emptyState({title: "Tu n'as pas encore d'œuvre à exposer", text: "Revendique une œuvre d'avant les comptes : une fois validée, tu pourras l'exposer ici.", mascot: true,
            action: `<a class="btn btn-gold btn-px" href="/revendiquer">Revendiquer une œuvre</a>`})}</div>`;
        page.removeAttribute("aria-busy");
        return;
    }
    const pre = Number(new URLSearchParams(location.search).get("oeuvre"));
    let sel = mine.find(o => o.id === pre) ?? mine[0];
    const st = {};
    const load = o => {
        const e = o.extra?.exposition;
        Object.assign(st, {salle: e?.salle ?? "paysages", marge: e?.marge ?? 75, cadre: e?.cadre ?? "bois", mot: e?.mot ?? "", timelapse: e ? e.timelapse : true, visite: e ? e.visite : true, exists: !!e});
    };
    load(sel);

    const render = () => {
        page.innerHTML = `<div class="ex-wrap">
            <header class="ex-head"><a class="btn btn-icon btn-ghost" href="/musee" aria-label="Retour au musée">${icon("back")}</a><h1 class="fs-h2">${st.exists ? "Modifier l'exposition" : "Exposer une œuvre"}</h1></header>
            <section class="ex-sec"><span class="fs-over">1 · L'œuvre</span>
                <div class="ex-works">${mine.map(o => `<button type="button" class="card-2 ex-work${o.id === sel.id ? " on" : ""}" data-oeuvre="${o.id}">
                    <span class="ex-work-img"><img class="px art" src="${artOf(o, 120)}" style="${artFit(90)}" alt=""></span><span class="fs-small b ellip">${escapeHTML(o.titre)}</span>
                    ${o.extra?.exposition ? `<span class="fs-cap t3">déjà exposée</span>` : ""}</button>`).join("")}</div></section>
            <section class="ex-sec"><div class="sec-head"><span class="fs-over">2 · Le cadrage</span><span class="mono fs-cap t3">${st.marge} %</span></div>
                <div class="${frameClass({cadre: st.cadre})} ex-frame"><div class="passe"><div class="ex-img"><img class="px art" src="${artOf(sel, 480)}" style="${artFit(st.marge)}" alt=""></div></div></div>
                <label class="label" for="marge">Marge autour de l'œuvre</label>
                <input id="marge" type="range" min="50" max="100" step="5" value="${st.marge}" class="js-marge">
                <div class="seg" role="group" aria-label="Cadre">${FRAMES.map(([id, l]) => `<button type="button" class="${id === st.cadre ? "on" : ""}" data-cadre="${id}">${l}</button>`).join("")}</div>
            </section>
            <section class="ex-sec"><span class="fs-over">3 · La salle</span>
                <div class="ex-salles">${SALLES.map(s => `<button type="button" class="chip${s === st.salle ? " chip-on" : ""}" data-salle="${s}">${SALLE_LABELS[s]}</button>`).join("")}</div>
                ${sel.origine === "avant_migration" ? `<span class="hint">Elle apparaîtra aussi dans la salle Pionnières.</span>` : ""}</section>
            <section class="ex-sec field"><label class="fs-over" for="mot">4 · Le mot de l'artiste</label>
                <textarea id="mot" class="textarea js-mot" maxlength="160" placeholder="Une anecdote, un souvenir…">${escapeHTML(st.mot)}</textarea>
                <span class="hint">Affiché sur le cartel et pendant la visite guidée · <span class="js-count">${[...st.mot].length}</span> / 160</span></section>
            <section class="card ex-toggles">
                <div class="edit-toggle"><span class="grow"><span class="fs-small b">Rejouer la construction</span><br><span class="fs-cap t3">timelapse de l'œuvre, tiré des sauvegardes</span></span><button type="button" class="toggle${st.timelapse ? " toggle-on" : ""}" data-toggle="timelapse" aria-pressed="${st.timelapse}" aria-label="Rejouer la construction"></button></div>
                <div class="edit-toggle"><span class="grow"><span class="fs-small b">Dans la visite guidée</span><br><span class="fs-cap t3">la caméra passera par ton œuvre</span></span><button type="button" class="toggle${st.visite ? " toggle-on" : ""}" data-toggle="visite" aria-pressed="${st.visite}" aria-label="Inclure dans la visite guidée"></button></div>
            </section>
            <button type="button" class="btn btn-primary btn-lg btn-px btn-block js-expose">${st.exists ? "Enregistrer" : "Exposer au musée"}</button>
            ${st.exists ? `<button type="button" class="btn btn-ghost btn-block js-remove">Retirer du musée</button>` : ""}
        </div>`;
        page.removeAttribute("aria-busy");
        fillIcons(page);
    };
    render();

    page.addEventListener("click", async e => {
        const t = e.target.closest("[data-oeuvre],[data-cadre],[data-salle],[data-toggle]");
        if (t?.dataset.oeuvre) { sel = mine.find(o => o.id === Number(t.dataset.oeuvre)); load(sel); render(); return; }
        if (t?.dataset.cadre) { st.cadre = t.dataset.cadre; render(); return; }
        if (t?.dataset.salle) { st.salle = t.dataset.salle; render(); return; }
        if (t?.dataset.toggle) { st[t.dataset.toggle] = !st[t.dataset.toggle]; render(); return; }
        if (e.target.closest(".js-expose")) {
            const r = await fetch(`/api/oeuvres/${sel.id}/exposition`, {method: "POST", headers: {"Content-Type": "application/json"},
                body: JSON.stringify({salle: st.salle, marge: st.marge, cadre: st.cadre, mot: st.mot, timelapse: st.timelapse, visite: st.visite})});
            const j = await r.json();
            if (!r.ok) { toast(`<span class="fs-small b">${escapeHTML(j.error || "Impossible d'exposer.")}</span>`); return; }
            sel.extra.exposition = {...st};
            st.exists = true;
            render();
            toast(`<img class="px" src="/img/badges/coeur.png" alt="" width="24" height="24"><span class="toast-text"><span class="fs-small b">Exposée dans la salle ${SALLE_LABELS[st.salle]}</span><span class="fs-cap t3">Visible dans le musée${st.visite ? " et la visite guidée" : ""}</span></span><a class="btn btn-sm" href="/oeuvre/${sel.id}">Voir</a>`, {timeout: 6000});
        }
        if (e.target.closest(".js-remove") && confirm("Retirer cette œuvre du musée ?")) {
            await fetch(`/api/oeuvres/${sel.id}/exposition`, {method: "DELETE"});
            delete sel.extra.exposition;
            load(sel);
            render();
        }
    });
    page.addEventListener("input", e => {
        if (e.target.classList.contains("js-marge")) {
            st.marge = Number(e.target.value);
            page.querySelector(".ex-img img").style.cssText = artFit(st.marge);
            page.querySelector(".sec-head .mono").textContent = st.marge + " %";
        }
        if (e.target.classList.contains("js-mot")) {
            st.mot = e.target.value;
            page.querySelector(".js-count").textContent = [...st.mot].length;
        }
    });
}

main();

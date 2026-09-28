// Canvas modes, announcement banner and account sanctions (e-lecture-seule-m,
// e-maintenance-d/m, e-banni-m). Shared by /ace and the content pages.

import {$, escapeHTML, fillIcons, icon, toast} from "./ui.js";

const mobileMQ = matchMedia("(max-width: 767px), (pointer: coarse)");
const ANNOUNCE = {event: ["banner-gold", "megaphone"], info: ["banner-info", "bell"], danger: ["banner-danger", "flag"]};
const RETRY = 30;

// The banners shown at the top: the admin's announcement, then the read-only notice.
export function bannersHTML(announce, mode, launch) {
    let html = "";
    if (launch?.active && launch.admin) {
        html += `<div class="banner banner-gold">${icon("shield", 18)}<span class="grow">Fermé au public jusqu'au ${escapeHTML(launch.label ?? "lancement")} : tu vois le site en tant qu'admin. <a href="/lancement">Voir le compte à rebours</a></span></div>`;
    }
    if (announce?.text) {
        const [cls, ic] = ANNOUNCE[announce.style] ?? ANNOUNCE.event;
        html += `<div class="banner ${cls}">${icon(ic, 18)}<span class="grow">${escapeHTML(announce.text)}</span></div>`;
    }
    if (mode?.mode === "readonly") {
        html += `<div class="banner banner-info">${icon("lock", 18)}<span class="grow">${mode.msg ? `${escapeHTML(mode.msg)} · ` : ""}Lecture seule${mode.until ? ` jusqu'à ${escapeHTML(mode.until)}` : ""}</span></div>`;
    } else if (mode?.mode === "maintenance") {
        html += `<div class="banner banner-danger">${icon("settings", 18)}<span class="grow">Maintenance en cours${mode.until ? ` · retour vers ${escapeHTML(mode.until)}` : ""} (tu es admin : tu peux encore dessiner)</span></div>`;
    }
    return html;
}

export function frozenText(mode) {
    return `Pas de pixel pour l'instant : on regarde, on admire${mode?.until ? `, on revient à ${escapeHTML(mode.until)}` : ""}.`;
}

// Full-screen maintenance page with an automatic retry.
export function showMaintenance(mode, {onBack} = {}) {
    let el = $("#maintenance");
    if (!el) {
        el = document.createElement("div");
        el.id = "maintenance";
        el.className = "state-screen void";
        document.body.appendChild(el);
    }
    el.innerHTML = `<div class="state-glow"></div>
        <div class="state-inner">
            <a href="/home" class="brand"><img src="/img/cube-logo.png" alt="" width="28" height="28"><span class="pixel b">BetterPlace</span></a>
            <div class="state-mascot"><img class="px anim-bob" src="/img/farwest-x8.png" alt="La mascotte cowboy" width="108" height="186"><div class="bulle">On bichonne le serveur !</div></div>
            <h1 class="fs-h1 state-title">Maintenance en cours</h1>
            <p class="fs-body t2 pretty state-text">${mode.until ? `Le canvas revient vers <b class="tx">${escapeHTML(mode.until)}</b>. ` : ""}${mode.msg ? escapeHTML(mode.msg) + " " : ""}Rien n'est perdu : tout a été sauvegardé juste avant.</p>
            <div class="state-retry"><div class="pbar"><div class="pfill js-retry-fill" style="width: 0%"></div></div><span class="mono fs-cap t3 js-retry">nouvel essai automatique dans ${RETRY} s</span></div>
            <div class="state-actions"><a class="btn" href="/timelapse">Revoir la timelapse</a><a class="btn btn-ghost" href="/musee">Visiter le musée</a></div>
            <p class="fs-cap t3 state-credit">Fait avec <span class="heart">&#10084;&#65038;</span> par Evan et Tiago</p>
        </div>`;
    el.hidden = false;
    document.body.classList.add("in-maintenance");
    clearInterval(el.timer);
    let s = RETRY;
    el.timer = setInterval(async () => {
        s--;
        const fill = $(".js-retry-fill", el), txt = $(".js-retry", el);
        if (fill) fill.style.width = Math.round((RETRY - s) / RETRY * 100) + "%";
        if (s > 0) { if (txt) txt.textContent = `nouvel essai automatique dans ${s} s`; return; }
        s = RETRY;
        if (txt) txt.textContent = "on vérifie…";
        try {
            const st = await (await fetch("/api/status", {cache: "no-store"})).json();
            if (st.mode?.mode !== "maintenance") { hideMaintenance(); onBack?.(st); }
        } catch { /* still down */ }
    }, 1000);
}

export function hideMaintenance() {
    const el = $("#maintenance");
    if (!el) return;
    clearInterval(el.timer);
    el.hidden = true;
    document.body.classList.remove("in-maintenance");
}

// « Ton compte est suspendu » / banned: shown once per sanction, then the canvas stays visible.
export function showSanction(blocked, sanction, {force = false} = {}) {
    const key = `${blocked}:${sanction?.until ?? 0}`;
    try { if (!force && sessionStorage.getItem("bp-sanction-seen") === key) return; } catch { /* private mode */ }
    let el = $("#sanction");
    if (!el) {
        el = document.createElement("div");
        el.id = "sanction";
        el.className = "state-screen void";
        el.setAttribute("role", "dialog");
        el.setAttribute("aria-modal", "true");
        el.setAttribute("aria-labelledby", "sanction-title");
        document.body.appendChild(el);
    }
    const banned = blocked === "banned";
    const until = sanction?.until ? new Date(sanction.until) : null;
    const when = until ? `${until.toLocaleDateString("fr-FR", {day: "numeric", month: "short"})} à ${until.toLocaleTimeString("fr-FR", {hour: "2-digit", minute: "2-digit"})}` : "";
    const contact = sanction?.contact;
    el.innerHTML = `<div class="state-inner state-narrow">
            <a href="/home" class="brand"><img src="/img/cube-logo.png" alt="" width="26" height="26"><span class="pixel b">BetterPlace</span></a>
            <div class="state-center">
                <div class="state-medal"><img class="px" src="/img/farwest-x8.png" alt="" width="54" height="93"></div>
                <h1 class="fs-h1 state-title-s" id="sanction-title">${banned ? "Ton compte ne peut plus dessiner" : "Ton compte est suspendu"}</h1>
                <p class="fs-body t2 pretty">${banned ? "C'est une décision définitive de l'équipe. Tu peux toujours regarder le canvas." : `${when ? `Jusqu'au <b class="tx">${when}</b>. ` : ""}Tu peux regarder le canvas, mais pas dessiner pour l'instant.`}</p>
                ${sanction?.motif ? `<div class="card state-motif"><span class="fs-over">Motif</span><span class="fs-small">${escapeHTML(sanction.motif)}</span><span class="fs-cap t3">Décision de l'équipe</span></div>` : ""}
            </div>
            <div class="state-actions-col">
                <button class="btn btn-primary btn-block js-sanction-ok" type="button">Regarder le canvas</button>
                ${contact ? `<a class="btn btn-ghost btn-block" href="mailto:${escapeHTML(contact)}">C'est une erreur ? Écrire à l'équipe</a>` : ""}
            </div>
        </div>`;
    el.hidden = false;
    $(".js-sanction-ok", el).focus();
    $(".js-sanction-ok", el).addEventListener("click", () => {
        el.hidden = true;
        try { sessionStorage.setItem("bp-sanction-seen", key); } catch { /* private mode */ }
    });
}

// Content pages: banners under the header, maintenance screen for everyone but admins.
export async function mountPageStates(me) {
    let st = me;
    if (!st || !("mode" in st)) {
        try { st = await (await fetch("/api/status", {cache: "no-store"})).json(); } catch { return; }
    }
    const launch = st.launch ? {...st.launch, admin: st.user?.role === "admin"} : null;
    const html = bannersHTML(st.announce, st.mode?.mode === "readonly" ? st.mode : null, launch);
    if (html) {
        const bar = document.createElement("div");
        bar.className = "state-banners state-banners-page";
        bar.setAttribute("role", "status");
        bar.innerHTML = html;
        const head = $(".shell-head");
        head ? head.after(bar) : document.body.prepend(bar);
    }
}

// /ace: everything live (WebSocket "mode", "announce" and "zone" messages).
export function setupStates(app) {
    let mode = {mode: "normal"}, announce = null, launch = null;
    const bar = document.createElement("div");
    bar.className = "state-banners state-banners-canvas";
    bar.setAttribute("role", "status");
    bar.hidden = true;
    document.body.appendChild(bar);

    const isAdmin = () => app.me?.role === "admin";
    window.addEventListener("resize", () => {
        if (!bar.hidden) document.documentElement.style.setProperty("--banners-h", (bar.offsetHeight + (mobileMQ.matches ? 0 : 8)) + "px");
    });
    const render = () => {
        const html = bannersHTML(announce, mode.mode !== "normal" && (mode.mode === "readonly" || isAdmin()) ? mode : null, launch && {...launch, admin: isAdmin()});
        bar.innerHTML = html;
        bar.hidden = !html;
        // push the mobile top bar and the toasts below the banners
        document.documentElement.style.setProperty("--banners-h", html ? (bar.offsetHeight + (mobileMQ.matches ? 0 : 8)) + "px" : "0px");
        const frozen = mode.mode !== "normal" && !isAdmin();
        document.body.classList.toggle("is-frozen", frozen);
        app.frozen = frozen ? mode : null;
        if (mode.mode === "maintenance" && !isAdmin()) showMaintenance(mode, {onBack: () => location.reload()});
        else hideMaintenance();
        app.onFrozenChange?.();
    };

    app.applyStatus = me => {
        mode = me.mode ?? mode;
        announce = me.announce ?? null;
        launch = me.launch ?? null;
        render();
        if (me.blocked) showSanction(me.blocked, me.sanction);
    };

    app.frozenToast = () => {
        toast(`<span data-icon="lock" data-size="20" style="color: var(--info)"></span><span class="toast-text"><span class="fs-small b">${mode.mode === "maintenance" ? "Maintenance en cours" : "Lecture seule"}</span><span class="fs-cap t3">${frozenText(mode)}</span></span>`, {timeout: 4000});
        fillIcons($("#toasts"));
    };

    // a sanction decided while the page is open: fetch the reason and show the screen
    app.sanctionFromServer = async code => {
        try {
            const me = await (await fetch("/api/me", {cache: "no-store"})).json();
            showSanction(me.blocked ?? code, me.sanction, {force: true});
        } catch { /* keep the toast */ }
    };

    app.onConnection = app.onConnection ?? [];
    app.onConnection.push(conn => {
        conn.addEventListener("mode", ev => {
            const was = mode.mode, wasMaintenance = was === "maintenance";
            mode = {mode: ev.detail.mode, msg: ev.detail.msg, until: ev.detail.until};
            render();
            if (wasMaintenance && mode.mode !== "maintenance" && !isAdmin()) location.reload();
            else if (mode.mode === "normal" && was !== "normal") toast(`<span data-icon="check" data-size="18"></span><span class="fs-small b">Le canvas est de nouveau ouvert</span>`, {timeout: 3000});
            fillIcons($("#toasts"));
        });
        conn.addEventListener("launch", ev => {
            launch = ev.detail.launch ?? null;
            render();
        });
        conn.addEventListener("announce", ev => {
            announce = ev.detail.text ? {text: ev.detail.text, style: ev.detail.style} : null;
            render();
        });
        // an admin restored or erased a zone: download the canvas again
        conn.addEventListener("zone", () => { if (app.loaded) app.resync?.(); });
    });
}

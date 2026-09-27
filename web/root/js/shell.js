// Shared frame of the content pages: glass header (desktop), bottom tabs (mobile),
// account menu, theme. Usage: const me = await mountShell({active: "musee"});

import {$, $$, avatarHTML, escapeHTML, fillIcons, icon, toggleTheme, currentTheme} from "./ui.js";

export const NAV = [
    {id: "canvas", href: "/ace", label: "Canvas", icon: "canvas"},
    {id: "musee", href: "/musee", label: "Musée", icon: "museum"},
    {id: "activite", href: "/activite", label: "Activité", icon: "activity"},
    {id: "classement", href: "/classement", label: "Classement", icon: "trophy", desktopOnly: true},
    {id: "timelapse", href: "/timelapse", label: "Timelapse", icon: "film", desktopOnly: true},
];

let mePromise = null;

export function fetchMe() {
    if (!mePromise) mePromise = fetch("/api/me", {cache: "no-store"}).then(r => r.json()).catch(() => ({user: null, auth: []}));
    return mePromise;
}

export function loginHref() {
    return "/login?next=" + encodeURIComponent(location.pathname + location.search);
}

export async function mountShell({active = "", overlay = false} = {}) {
    const me = await fetchMe();
    const u = me.user;
    document.body.classList.toggle("is-guest", !u);
    document.body.classList.toggle("is-player", !!u);

    const header = document.createElement("header");
    header.className = "glass shell-head only-d" + (overlay ? " shell-overlay" : "");
    header.innerHTML = `
        <a href="/home" class="brand"><img src="/img/cube-logo.png" alt="" width="28" height="28"><span class="pixel">BetterPlace</span></a>
        <nav class="shell-nav" aria-label="Sections">
            ${NAV.map(n => `<a class="btn btn-sm btn-ghost${n.id === active ? " on" : ""}" href="${n.href}"${n.id === active ? ' aria-current="page"' : ""}>${n.label}</a>`).join("")}
        </nav>
        <div class="grow"></div>
        <button class="btn btn-sm btn-icon btn-ghost js-shell-theme" type="button" aria-label="Changer de thème">${icon(currentTheme() === "clair" ? "moon" : "sun", 18)}</button>
        ${u ? `<button class="me-button js-shell-me" type="button" aria-label="Mon compte" aria-haspopup="true" style="--ring: ${escapeHTML(u.accent || "var(--accent)")}">${avatarHTML(u, "av av-40")}${me.alerts ? `<span class="count">${me.alerts}</span>` : ""}</button>`
            : `<a class="btn btn-sm btn-primary" href="${loginHref()}">Se connecter</a>`}`;
    document.body.prepend(header);

    if (u) {
        const menu = document.createElement("div");
        menu.className = "card elev d-menu shell-menu";
        menu.hidden = true;
        menu.innerHTML = `
            <a class="me-row" href="/u/${escapeHTML(u.slug)}">${avatarHTML(u, "av av-40")}<div class="me-text"><span class="fs-body b ellip">${escapeHTML(u.pseudo)}</span><span class="fs-cap t3">Voir mon profil</span></div></a>
            <div class="rule"></div>
            <a class="btn btn-sm btn-ghost btn-block menu-item" href="/moi/profil">${icon("user", 18)}Modifier mon profil</a>
            ${u.role === "admin" ? `<a class="btn btn-sm btn-ghost btn-block menu-item" href="/admin">${icon("shield", 18)}Administration</a>` : ""}
            <button class="btn btn-sm btn-ghost btn-block menu-item js-shell-theme" type="button">${icon(currentTheme() === "clair" ? "moon" : "sun", 18)}<span>${currentTheme() === "clair" ? "Mode sombre" : "Mode clair"}</span></button>
            <form method="post" action="/auth/logout"><input type="hidden" name="next" value="${escapeHTML(location.pathname)}">
                <button class="btn btn-sm btn-ghost btn-block menu-item" type="submit">${icon("logout", 18)}Se déconnecter</button></form>`;
        document.body.appendChild(menu);
        $(".js-shell-me", header).addEventListener("click", ev => {
            ev.stopPropagation();
            menu.hidden = !menu.hidden;
        });
        document.addEventListener("click", ev => {
            if (!ev.target.closest(".shell-menu, .js-shell-me")) menu.hidden = true;
        });
        document.addEventListener("keydown", ev => { if (ev.key === "Escape") menu.hidden = true; });
    }

    const tabs = document.createElement("nav");
    tabs.className = "nav-bottom m-nav shell-tabs only-m";
    tabs.setAttribute("aria-label", "Navigation principale");
    const mobileTabs = NAV.filter(n => !n.desktopOnly);
    tabs.innerHTML = mobileTabs.map(n =>
        `<a class="tab${n.id === active ? " tab-on" : ""}" href="${n.href}"${n.id === active ? ' aria-current="page"' : ""}>${icon(n.icon, 22)}${n.label}</a>`).join("") +
        (u ? `<a class="tab${active === "profil" ? " tab-on" : ""}" href="/u/${escapeHTML(u.slug)}">${avatarHTML(u, "av av-24")}Profil</a>`
           : `<a class="tab" href="${loginHref()}">${icon("user", 22)}Connexion</a>`);
    document.body.appendChild(tabs);

    for (const b of $$(".js-shell-theme")) b.addEventListener("click", () => toggleTheme());
    document.addEventListener("bp-theme", () => {
        for (const b of $$(".js-shell-theme")) {
            const svg = b.querySelector("svg");
            if (svg) svg.outerHTML = icon(currentTheme() === "clair" ? "moon" : "sun", 18);
            const label = b.querySelector("span");
            if (label) label.textContent = currentTheme() === "clair" ? "Mode sombre" : "Mode clair";
        }
        const meta = document.querySelector("meta[name=theme-color]");
        if (meta) meta.content = currentTheme() === "clair" ? "#f3eee3" : "#11100e";
    });
    fillIcons();
    return me;
}

// Empty state (e-vides): a useful message and one action.
export function emptyState({title, text, action = "", mascot = false}) {
    return `<div class="card empty-state">
        ${mascot ? `<img class="px" src="/img/farwest-x8.png" alt="" width="42" height="72">` : ""}
        <span class="fs-body b">${title}</span>
        ${text ? `<span class="fs-small t2 pretty">${text}</span>` : ""}
        ${action}
    </div>`;
}

// /login — « Mets ton chapeau »: JustBetter login (when wired), dev accounts with -devAuth.

import {$, $$, avatarHTML, escapeHTML, fillIcons} from "./ui.js";

// Sample accounts of the design (same avatars as the dev provider on the server).
const DEV_ACCOUNTS = [
    ["Brindille", "brindille"], ["Kaelen", "kaelen"], ["Zozo42", "zozo42"], ["Mamie Pixel", "mamie"],
    ["Lucie_px", "lucie"], ["Nyx", "nyx"], ["Poulpe", "poulpe"], ["Sam", "sam"], ["Tiago", "tiago"], ["Evan", "evan"],
];

function safeNext(next) {
    if (!next || next[0] !== "/" || next.startsWith("//") || /[\\\r\n]/.test(next)) return "/ace";
    return next;
}

const next = safeNext(new URLSearchParams(location.search).get("next"));

fillIcons();
for (const a of $$(".js-next")) a.href = next;
for (const input of $$("input[name=next]")) if (!input.closest(".js-logout")) input.value = next;
$(".js-logout input[name=next]").value = "/login?next=" + encodeURIComponent(next);

$(".js-dev-accounts").innerHTML = DEV_ACCOUNTS.map(([pseudo, file]) => `
    <form method="post" action="/auth/dev">
        <input type="hidden" name="pseudo" value="${escapeHTML(pseudo)}">
        <input type="hidden" name="next" value="${escapeHTML(next)}">
        <button class="login-dev-account" type="submit">
            <img class="av av-40" src="img/avatars/${file}.png" alt="">
            <span class="fs-cap b ellip">${escapeHTML(pseudo)}</span>
        </button>
    </form>`).join("");

try {
    const me = await fetch("/api/me", {cache: "no-store"}).then(r => r.json());
    const auth = me.auth ?? [];
    const justbetter = auth.includes("justbetter");
    $(".js-justbetter").hidden = !justbetter;
    $(".js-justbetter-off").hidden = justbetter;
    $(".js-justbetter-note").hidden = justbetter;
    const err = new URLSearchParams(location.search).get("erreur");
    const ERRORS = {
        identifiants: "Identifiant ou mot de passe incorrect.",
        trop: "Trop d'essais : attends quelques minutes avant de réessayer.",
        indisponible: "Le serveur de comptes JustBetter ne répond pas. Réessaie dans un instant.",
    };
    if (justbetter && ERRORS[err]) {
        const box = $(".js-error");
        box.textContent = ERRORS[err];
        box.hidden = false;
        $("#jb-pass").focus();
    }
    $(".js-dev").hidden = !auth.includes("dev");
    // no guest mode: an account is needed, the close button goes back home
    if (me.guests === false) {
        $(".js-guest").hidden = true;
        $(".login-close").href = "/home";
        $(".login-close").classList.remove("js-next");
    }
    if (!auth.length) {
        $(".js-justbetter-note").textContent = me.guests === false ? "La connexion n'est pas encore ouverte." : "La connexion n'est pas encore ouverte. En attendant, tu peux regarder le canvas en invité.";
    }
    if (me.user) {
        const u = me.user;
        const ring = u.accent || "var(--accent)";
        $(".js-me-row").innerHTML = `${avatarHTML(u, "av av-48", `box-shadow: 0 0 0 2px var(--surface-2), 0 0 0 4px ${escapeHTML(ring)}`)}
            <div class="me-text"><span class="fs-cap t3">Tu es connecté en tant que</span><span class="fs-h3 ellip">${escapeHTML(u.pseudo)}</span></div>`;
        $(".login-already").hidden = false;
        $(".js-actions").hidden = true;
    }
} catch {
    $(".js-justbetter-note").textContent = "Le serveur ne répond pas. Réessaie dans un instant.";
}

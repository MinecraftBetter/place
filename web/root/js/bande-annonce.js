// /bande-annonce — the trailer (the video sent with the site, see deploy/justbetter).

import {fillIcons} from "./ui.js";

const $ = s => document.querySelector(s);
fillIcons();
const status = await fetch("/api/status", {cache: "no-store"}).then(r => r.json()).catch(() => ({}));
const launch = status.launch ?? {};
const waiting = !!launch.active;
$(".js-back").href = waiting ? "/lancement" : "/home";
$(".js-back").textContent = waiting ? "Retour au compte à rebours" : "Retour à l'accueil";
if (!launch.video) {
    $(".js-missing").hidden = false;
} else {
    const v = $(".js-video");
    v.src = launch.video;
    if (launch.poster) v.poster = launch.poster;
    v.hidden = false;
    v.addEventListener("ended", () => { if (!waiting) $(".js-go").hidden = false; });
    v.play().catch(() => { /* the player's own button */ });
}

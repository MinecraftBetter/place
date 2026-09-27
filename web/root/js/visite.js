// /musee/visite — the guided tour: the camera flies from artwork to artwork (m-visite, d-visite).

import {$, avatarHTML, escapeHTML, fillIcons, icon, toast} from "./ui.js";
import {fetchMe, loginHref} from "./shell.js";
import {authorsOf, artView, SALLE_LABELS} from "./common.js";

const DURATION = 8000;
let tour = [], i = 0, playing = true, timer = null, started = 0, remaining = DURATION, me = null, W = 1024, H = 768;

function camera() {
    const o = tour[i];
    const vw = window.innerWidth, vh = window.innerHeight;
    const s = Math.min(vw * 0.7 / o.w, vh * 0.5 / o.h, 40);
    const cx = (o.x + o.w / 2) * s, cy = (o.y + o.h / 2) * s;
    const tx = vw / 2 - cx, ty = vh * 0.4 - cy;
    $(".js-cam").style.transform = `translate(${tx}px, ${ty}px) scale(${s})`;
}

function render() {
    const o = tour[i];
    const e = o.extra?.exposition;
    $(".js-segs").innerHTML = tour.map((_, k) => `<div class="story-seg"><div class="${k < i ? "vis-done" : k === i ? "vis-now" : ""}" style="${k === i ? `animation-duration: ${DURATION}ms` : ""}"></div></div>`).join("");
    $(".js-room").textContent = `Visite guidée · ${e ? SALLE_LABELS[e.salle] ?? "" : ""}`;
    $(".js-card").innerHTML = `<div class="anim-feed vis-text">
        <span class="fs-over">Œuvre ${i + 1} sur ${tour.length}${o.apparue_le ? " · " + new Date(o.apparue_le).getFullYear() : ""}</span>
        <h1 class="fs-h1">${escapeHTML(o.titre)}</h1>
        <div class="vis-by">${avatarHTML(o.auteurs[0]?.user, "av av-32")}<span class="fs-small t2">${escapeHTML(authorsOf(o))}</span></div>
        ${e?.mot ? `<p class="fs-small t2 pretty">« ${escapeHTML(e.mot)} »</p>` : ""}
    </div>`;
    $(".js-view").href = artView(o);
    $(".js-like").setAttribute("aria-pressed", String(!!o.extra?.liked));
    const seg = $(".vis-now");
    if (seg && !playing) seg.classList.add("paused");
    camera();
}

function schedule(ms = DURATION) {
    clearTimeout(timer);
    started = Date.now();
    remaining = ms;
    if (playing) timer = setTimeout(() => go(1), ms);
}

function go(d) {
    i = (i + d + tour.length) % tour.length;
    render();
    schedule();
}

function toggle() {
    playing = !playing;
    $(".js-play").innerHTML = icon(playing ? "pause" : "play", 20);
    $(".js-play").setAttribute("aria-label", playing ? "Pause" : "Lecture");
    $(".vis-now")?.classList.toggle("paused", !playing);
    if (playing) schedule(remaining);
    else { clearTimeout(timer); remaining -= Date.now() - started; }
}

async function main() {
    me = (await fetchMe()).user;
    const data = await fetch("/api/musee/visite", {cache: "no-store"}).then(r => r.json());
    tour = data.oeuvres;
    W = data.width; H = data.height;
    fillIcons();
    if (!tour.length) {
        $(".js-card").innerHTML = `<div class="vis-text"><h1 class="fs-h1">La visite n'a pas encore d'étapes</h1><p class="fs-small t2">Les œuvres exposées au musée apparaissent ici.</p><a class="btn btn-primary btn-px" href="/musee">Retour au musée</a></div>`;
        $(".vis-controls").hidden = true;
        return;
    }
    $(".js-cam").style.width = W + "px";
    $(".js-cam").style.height = H + "px";
    render();
    schedule();
    $(".js-prev").addEventListener("click", () => go(-1));
    $(".js-next").addEventListener("click", () => go(1));
    $(".js-play").addEventListener("click", toggle);
    $(".js-like").addEventListener("click", async ev => {
        if (!me) { location.href = loginHref(); return; }
        const o = tour[i];
        const on = !o.extra.liked;
        const r = await fetch(`/api/oeuvres/${o.id}/like`, {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({on})}).then(r => r.json());
        o.extra.liked = on;
        o.extra.likes = r.likes;
        ev.currentTarget.setAttribute("aria-pressed", String(on));
        if (on) toast(`<img class="px anim-bob" src="/img/badges/coeur.png" alt="" width="24" height="24"><span class="fs-small b">Coup de cœur pour « ${escapeHTML(o.titre)} »</span>`, {timeout: 2000});
    });
    document.addEventListener("keydown", e => {
        if (e.key === "ArrowRight") go(1);
        if (e.key === "ArrowLeft") go(-1);
        if (e.key === " ") { e.preventDefault(); toggle(); }
        if (e.key === "Escape") location.href = "/musee";
    });
    window.addEventListener("resize", camera);
}

main();

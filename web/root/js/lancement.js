// /lancement — the release countdown (the link shared on Discord). At zero the trailer
// plays, then the place opens. The server opens the site by itself at the same instant.

import {fillIcons} from "./ui.js";

const $ = s => document.querySelector(s);
fillIcons();
const LINES = [
    "Howdy ! Le nouveau place arrive…",
    "Tes pixels vont enfin porter ton nom.",
    "Tu pourras revendiquer tes œuvres d'avant, preuves à l'appui.",
    "Un musée ? Oui, un vrai musée. Avec des cadres dorés.",
    "Et ton musée à toi, avec ta musique.",
    "4 ans de captures… j'ai tout gardé, partner.",
    "Prépare tes couleurs !",
];

let offset = 0, launch = {}, soundOn = false, audioCtx = null, lastS = -1, ended = false;

const pad = n => String(n).padStart(2, "0");
const now = () => Date.now() + offset;

// admins get in before the release: a button when logged in as one, a discreet link otherwise
async function adminAccess() {
    const me = await fetch("/api/me", {cache: "no-store"}).then(r => r.json()).catch(() => null);
    if (!launch.active) return;
    if (me?.user?.role === "admin") $(".js-admin-enter").hidden = false;
    else if (!me?.user) $(".js-admin-login").hidden = false;
}

async function main() {
    const r = await fetch("/api/status", {cache: "no-store"}).catch(() => null);
    const st = r ? await r.json().catch(() => ({})) : {};
    if (st.now) offset = st.now - Date.now();
    launch = st.launch ?? {};
    adminAccess();
    $(".js-replay").hidden = !launch.video; // the trailer can be watched while waiting
    let line = 0;
    setInterval(() => { line = (line + 1) % LINES.length; if (!ended) $(".js-bulle").textContent = LINES[line]; }, 6000);
    if (!launch.at) return; // no date yet: « La date arrive très bientôt »
    if (now() >= launch.at) { opened(); return; }
    $(".js-date").innerHTML = `Ouverture <b></b>`;
    $(".js-date b").textContent = launch.label;
    $(".js-clock").hidden = false;
    const ics = $(".js-ics");
    ics.hidden = false;
    ics.href = icsURL(launch.at);
    tick();
    setInterval(tick, 200);
}

function tick() {
    if (ended) return;
    const ms = launch.at - now();
    if (ms <= 0) { zero(); return; }
    const s = Math.floor(ms / 1000);
    if (s === lastS) return;
    lastS = s;
    const parts = {d: Math.floor(s / 86400), h: Math.floor(s % 86400 / 3600), m: Math.floor(s % 3600 / 60), s: s % 60};
    for (const [k, v] of Object.entries(parts)) {
        const el = $(".js-" + k), txt = pad(v);
        if (el.textContent !== txt) { el.textContent = txt; el.classList.remove("tick"); void el.offsetWidth; el.classList.add("tick"); }
    }
    document.title = `${parts.d ? parts.d + " j " : ""}${pad(parts.h)}:${pad(parts.m)}:${pad(parts.s)} · BetterPlace`;
    // the trailer loads during the last minute, so it starts right away
    const v = $(".js-video");
    if (s < 60 && launch.trailer && launch.video && !v.src) { v.src = launch.video; if (launch.poster) v.poster = launch.poster; v.load(); }
    // the last ten seconds, full screen
    const fin = $(".js-final"), n = $(".js-final span");
    if (s < 10) {
        fin.hidden = false;
        n.textContent = String(s + 1);
        n.classList.remove("tick"); void n.offsetWidth; n.classList.add("tick");
        beep(s < 3 ? 880 : 660);
    }
}

function beep(freq) {
    if (!soundOn || !audioCtx) return;
    const o = audioCtx.createOscillator(), g = audioCtx.createGain(), t = audioCtx.currentTime;
    o.type = "square";
    o.frequency.value = freq;
    g.gain.setValueAtTime(.12, t);
    g.gain.exponentialRampToValueAtTime(.001, t + .25);
    o.connect(g).connect(audioCtx.destination);
    o.start(t);
    o.stop(t + .3);
}

function zero() {
    ended = true;
    $(".js-final").hidden = true;
    document.title = "C'est ouvert ! · BetterPlace";
    if (!launch.trailer || !launch.video) { opened(); return; }
    const box = $(".js-trailer"), v = $(".js-video");
    box.hidden = false;
    if (!v.src) { v.src = launch.video; if (launch.poster) v.poster = launch.poster; }
    const done = () => { v.pause(); $(".js-enter").hidden = false; $(".js-skip").hidden = true; $(".js-unmute").hidden = true; $(".js-play").hidden = true; };
    v.addEventListener("ended", done);
    $(".js-skip").onclick = done;
    $(".js-unmute").onclick = () => { v.muted = false; $(".js-unmute").hidden = true; };
    $(".js-play").onclick = () => { v.play(); $(".js-play").hidden = true; };
    // with sound when the visitor asked for it (a click), otherwise muted (browsers allow it)
    v.muted = !soundOn;
    v.play().then(() => { $(".js-unmute").hidden = !v.muted; }).catch(() => {
        v.muted = true;
        v.play().then(() => { $(".js-unmute").hidden = false; }).catch(() => { $(".js-play").hidden = false; });
    });
}

// the page is opened after the release
function opened() {
    ended = true;
    $(".js-title").textContent = "C'est ouvert !";
    $(".js-date").textContent = "Le nouveau place de JustBetter t'attend.";
    $(".js-bulle").textContent = "Allez, à tes pixels !";
    $(".js-clock").hidden = true;
    $(".js-ics").hidden = true;
    $(".js-sound").hidden = true;
    $(".js-open").hidden = false;
    $(".js-replay").hidden = !launch.video;
    $(".js-replay-label").textContent = "Revoir la bande-annonce";
}

function icsURL(at) {
    const d = new Date(at), z = x => x.toISOString().replace(/[-:]/g, "").replace(/\.\d{3}/, "");
    const end = new Date(at + 3600e3);
    const ics = ["BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//JustBetter//BetterPlace//FR", "BEGIN:VEVENT", `UID:lancement-${at}@place.justbetter.fr`,
        `DTSTAMP:${z(new Date())}`, `DTSTART:${z(d)}`, `DTEND:${z(end)}`, "SUMMARY:Sortie du nouveau BetterPlace",
        `DESCRIPTION:Le nouveau place de JustBetter ouvre ! ${location.origin}/lancement`, `URL:${location.origin}/lancement`, "END:VEVENT", "END:VCALENDAR"].join("\r\n");
    return "data:text/calendar;charset=utf-8," + encodeURIComponent(ics);
}

$(".js-sound").addEventListener("click", e => {
    soundOn = !soundOn;
    if (soundOn && !audioCtx) audioCtx = new (window.AudioContext || window.webkitAudioContext)();
    audioCtx?.resume();
    e.currentTarget.setAttribute("aria-pressed", String(soundOn));
    $(".js-sound-label").textContent = soundOn ? "Son activé : garde l'onglet ouvert" : "Activer le son pour le jour J";
    if (soundOn) beep(990);
});

main();

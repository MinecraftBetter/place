// Pixel inspector: hover card and pinned card on desktop, sheet on mobile.

import {colorName, parseHex, rgbToHex} from "./palette.js";
import {formatCoord, relativeTime} from "./view.js";
import {$, avatarHTML, escapeHTML, fillIcons, hideSheet, icon, showSheet} from "./ui.js";

const HOVER_DELAY = 300;
const MONTHS_LONG = ["janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."];
const dateLong = ms => { const d = new Date(ms); return `${d.getDate() === 1 ? "1er" : d.getDate()} ${MONTHS_LONG[d.getMonth()]} ${d.getFullYear()}`; };

function artRow(art, big) {
    return `<a href="/oeuvre/${art.id}" class="card-2 insp-art" style="padding: 8px">
        <span class="fs-cap t3">${big ? "Fait partie de l'œuvre" : "Œuvre"}</span><span class="fs-small b grow ellip">${escapeHTML(art.titre)}</span>${icon("chevron", 16)}</a>`;
}
const DETAIL_TTL = 10000;

export class Inspector {
    #app;
    #card = $("#d-inspector");
    #sheet = $("#sheet-inspector");
    #hoverTimer = null;
    #current = null;      // {x, y, mode: "hover" | "pinned" | "sheet"}
    #details = new Map(); // "x,y" → {at, owner, data}

    constructor(app) {
        this.#app = app;
        this.#card.addEventListener("click", ev => this.#onAction(ev));
        this.#sheet.addEventListener("click", ev => this.#onAction(ev));
    }

    get pinned() {
        return this.#current?.mode === "pinned";
    }

    get current() {
        return this.#current;
    }

    // Desktop: the mouse rests on a pixel.
    hover(px, py) {
        if (this.pinned) return;
        clearTimeout(this.#hoverTimer);
        if (this.#current?.mode === "hover" && (this.#current.x !== px || this.#current.y !== py)) this.#hideCard();
        this.#hoverTimer = setTimeout(() => this.#show(px, py, "hover"), HOVER_DELAY);
    }

    leave() {
        clearTimeout(this.#hoverTimer);
        if (this.#current?.mode === "hover") this.#hideCard();
    }

    // Desktop: I pins the card on a pixel (again: unpin).
    togglePin(px, py) {
        clearTimeout(this.#hoverTimer);
        if (this.pinned && this.#current.x === px && this.#current.y === py) {
            this.close();
            return;
        }
        this.#show(px, py, "pinned");
    }

    // Mobile: long press, or a guest's tap.
    openSheet(px, py) {
        this.#show(px, py, "sheet");
    }

    close() {
        clearTimeout(this.#hoverTimer);
        if (this.#current?.mode === "sheet") hideSheet();
        this.#hideCard();
    }

    // The canvas moved: keep the card next to its pixel.
    reposition() {
        if (this.#current && this.#current.mode !== "sheet") this.#placeCard();
    }

    // A pixel changed (WebSocket): refresh if it is the one on display.
    pixelChanged(x, y) {
        this.#details.delete(x + "," + y);
        if (this.#current && this.#current.x === x && this.#current.y === y) this.#render(true);
    }

    #hideCard() {
        if (this.#current && this.#current.mode !== "sheet") {
            this.#current = null;
            this.#app.inspectZone = null;
        }
        this.#card.hidden = true;
        this.#card.classList.remove("pinned");
        this.#app.render();
    }

    #show(x, y, mode) {
        this.#current = {x, y, mode};
        if (mode === "sheet") {
            this.#card.hidden = true;
            showSheet(this.#sheet, {onClose: () => {
                if (this.#current?.mode === "sheet") this.#current = null;
                this.#app.inspectZone = null;
                this.#app.render();
            }});
        } else {
            this.#card.hidden = false;
            this.#card.classList.toggle("pinned", mode === "pinned");
        }
        this.#render(true);
        this.#app.render();
    }

    async #render(fetchDetails) {
        const cur = this.#current;
        if (!cur) return;
        const {x, y} = cur;
        const app = this.#app;
        const hex = rgbToHex(app.gl.getColor({x, y}));
        const ownerId = app.owners.get(x, y);
        const cached = this.#details.get(x + "," + y);
        const details = cached && cached.owner === ownerId && Date.now() - cached.at < DETAIL_TTL ? cached.data : null;
        const owner = ownerId ? (app.users.peek(ownerId) ?? details?.owner ?? null) : null;
        this.#syncZone(details);

        const html = this.#html({x, y, hex, ownerId, owner, details, mode: cur.mode, loading: fetchDetails && !details});
        const target = cur.mode === "sheet" ? this.#sheet : this.#card;
        target.innerHTML = html;
        fillIcons(target);
        if (cur.mode !== "sheet") this.#placeCard();

        if (ownerId && !owner) {
            app.users.get(ownerId).then(() => this.#stillOn(x, y) && this.#render(false));
        }
        if (fetchDetails && !details) {
            try {
                const resp = await fetch(`/api/pixel?x=${x}&y=${y}`, {cache: "no-store"});
                if (!resp.ok) return;
                const data = await resp.json();
                this.#details.set(x + "," + y, {at: Date.now(), owner: data.owner?.id ?? 0, data});
                if (data.owner) app.users.put(data.owner);
                if (this.#stillOn(x, y)) this.#render(false);
            } catch { /* offline: keep the local info */ }
        }
    }

    // The frame around the artwork of the pixel: only once pinned (or in the mobile sheet),
    // never left over from another pixel.
    #syncZone(details) {
        const o = this.#current && this.#current.mode !== "hover" ? details?.oeuvre : null;
        const z = o ? {x: o.x, y: o.y, w: o.w, h: o.h, titre: o.titre} : null;
        const before = this.#app.inspectZone;
        if (before?.x === z?.x && before?.y === z?.y && before?.w === z?.w && before?.h === z?.h && before?.titre === z?.titre) return;
        this.#app.inspectZone = z;
        this.#app.render();
    }

    #stillOn(x, y) {
        return this.#current && this.#current.x === x && this.#current.y === y;
    }

    #html({x, y, hex, ownerId, owner, details, mode, loading}) {
        const name = colorName(hex);
        const big = mode !== "hover";
        const colorTitle = name
            ? `${escapeHTML(name)} <span class="mono t3" style="font-weight: 400; font-size: 13px">${hex}</span>`
            : `<span class="mono">${hex}</span>`;
        const placedAt = details?.placed_at ?? this.#app.lastPlaced(x, y);
        let out = "";

        if (mode === "sheet") {
            out += `<div class="grab"></div>
            <div class="insp-head">
                <div class="insp-color" style="width: 44px; height: 44px; border-radius: 10px; background: ${hex}"></div>
                <div class="insp-title" style="gap: 3px"><span class="fs-body b">${colorTitle}</span><span class="coord" style="align-self: flex-start">${formatCoord(x, y)}</span></div>
                <button class="btn btn-icon btn-ghost" type="button" data-act="close" aria-label="Fermer">${icon("close")}</button>
            </div>`;
        } else {
            out += `<div class="insp-head">
                <div class="insp-color" style="background: ${hex}"></div>
                <div class="insp-title"><span class="fs-small b">${colorTitle}</span><span class="mono fs-cap t3">${formatCoord(x, y)}</span></div>
                ${mode === "pinned" ? `<button class="btn btn-icon btn-sm btn-ghost" type="button" data-act="close" aria-label="Fermer (Échap)">${icon("close", 18)}</button>` : ""}
            </div>`;
        }

        const art = details?.oeuvre;
        if (ownerId) {
            const pseudo = owner ? escapeHTML(owner.pseudo) : "…";
            const when = placedAt ? (big ? "posé " : "") + relativeTime(placedAt) : "";
            if (big) {
                const ring = owner?.accent || "var(--accent)";
                out += `<a class="card-2 insp-author" href="/u/${escapeHTML(owner?.slug ?? "")}" style="padding: 12px; gap: 12px">
                    ${avatarHTML(owner, "av av-48", `box-shadow: 0 0 0 2px var(--surface-2), 0 0 0 4px ${escapeHTML(ring)}`)}
                    <div style="display: flex; flex-direction: column; flex-grow: 1; gap: 2px"><span class="fs-body b">${pseudo}</span><span class="fs-small t3">${when}</span></div>
                    <span class="fs-small tac b" style="display: flex; align-items: center; gap: 2px">Profil${icon("chevron", 16)}</span>
                </a>`;
            } else {
                out += `<a class="insp-author" href="/u/${escapeHTML(owner?.slug ?? "")}">${avatarHTML(owner, "av av-32")}<span class="fs-small grow"><b>${pseudo}</b>${when ? ` <span class="t3">· ${when}</span>` : ""}</span><span class="fs-cap tac b">Profil</span></a>`;
            }
            if (art) out += artRow(art, big);
        } else if (art) {
            // A pixel of a claimed pre-account artwork (m-inspecteur-origine).
            const authors = art.auteurs.map(a => `<a class="insp-art-author" href="/u/${escapeHTML(a.user?.slug ?? "")}">${avatarHTML(a.user, "av av-32")}<span class="fs-small b grow">${escapeHTML(a.user?.pseudo ?? "")}</span><span class="fs-cap t3">${a.role === "auteur" ? "auteur" : "co-auteur"}</span></a>`).join("");
            out += `<div class="card-2 insp-origin">
                <div style="display: flex; align-items: center; gap: 10px"><span class="pill st-neutre">Œuvre d'origine</span><span class="fs-cap t3">posé avant les comptes</span></div>
                <a href="/oeuvre/${art.id}" class="insp-art">
                    <img src="/img/badges/pionnier.png" alt="" class="px" width="${big ? 40 : 28}" height="${big ? 40 : 28}">
                    <div style="display: flex; flex-direction: column; flex-grow: 1; min-width: 0"><span class="fs-${big ? "body" : "small"} b">${escapeHTML(art.titre)}</span>
                    ${art.apparue_le ? `<span class="fs-${big ? "small" : "cap"} t2">Œuvre pionnière · là depuis le ${dateLong(art.apparue_le)}</span>` : ""}</div>
                    ${icon("chevron", 18)}
                </a>
                ${big ? `<div class="rule"></div><div class="insp-art-authors">${authors}</div>` : `<span class="fs-cap t2">par ${art.auteurs.map(a => escapeHTML(a.user?.pseudo ?? "")).join(" et ")}</span>`}
            </div>
            ${big && art.revendiquee_le ? `<p class="fs-cap t3 pretty">Revendiquée le ${dateLong(art.revendiquee_le)}${art.confirmations ? `, confirmée par ${art.confirmations} joueur${art.confirmations > 1 ? "s" : ""}` : ""}, validée par l'équipe.</p>` : ""}`;
        } else if (loading) {
            out += `<p class="fs-small t3">Recherche de son œuvre…</p>`;
        } else if (parseHex(hex) === "#FFFFFF") {
            out += `<div style="display: flex; flex-direction: column; gap: 6px"><span class="pill st-neutre" style="align-self: flex-start">Pixel libre</span><p class="fs-small t2 pretty">Personne n'a posé de pixel ici depuis l'ouverture des comptes.</p></div>`;
        } else {
            const claimHref = `/revendiquer?x=${x}&y=${y}&z=8`;
            out += mode === "sheet"
                ? `<div class="card-2 insp-origin"><div style="display: flex; align-items: center; gap: 10px"><span class="pill st-neutre">Œuvre d'origine</span><span class="fs-cap t3">posé avant les comptes</span></div><p class="fs-small t2 pretty">Cette zone n'a pas encore d'auteur.</p>
                    <a class="btn btn-sm btn-gold btn-px" href="${claimHref}" style="align-self: flex-start">C'est toi ? Revendique-la</a></div>`
                : `<div style="display: flex; flex-direction: column; gap: 8px"><span class="pill st-neutre" style="align-self: flex-start">Œuvre d'origine</span><p class="fs-small t2 pretty">Posé avant les comptes. Cette zone n'a pas encore d'auteur.</p>
                    ${mode === "pinned" ? `<a class="btn btn-sm btn-gold btn-px" href="${claimHref}" style="align-self: flex-start">C'est toi ? Revendique-la</a>` : ""}</div>`;
        }

        if (big && details?.history?.length) {
            out += `<div class="insp-history"><span class="fs-over">Historique de ce pixel</span>`;
            details.history.forEach((h, i) => {
                const prev = details.history[i + 1];
                const when = h.ts ? relativeTime(h.ts) : "";
                if (h.origin) {
                    out += `<div class="insp-hrow fs-small"><span class="dot" style="background: ${escapeHTML(h.color)}"></span><span class="pill st-neutre">Œuvre d'origine</span><span class="t3 mono when">avant les comptes</span></div>`;
                } else {
                    const verb = !prev || prev.origin || prev.user?.id === h.user?.id ? "a posé" : "a retouché";
                    out += `<div class="insp-hrow fs-small"><span class="dot" style="background: ${escapeHTML(h.color)}"></span><span class="b">${escapeHTML(h.user?.pseudo ?? "?")}</span><span class="t3">${verb}</span><span class="t3 mono when">${when}</span></div>`;
                }
            });
            out += `</div>`;
        }

        if (big) {
            const btn = mode === "sheet" ? "btn" : "btn btn-sm";
            const pick = this.#app.me ? `<button class="${btn}" type="button" data-act="pick">${icon("pipette", 18)}Prendre la couleur</button>` : "";
            const share = mode === "sheet"
                ? `<button class="${btn}" type="button" data-act="share">${icon("link", 18)}Partager</button>`
                : `<button class="${btn}" type="button" data-act="copy">${icon("link", 18)}Copier le lien</button>`;
            out += `<div class="insp-actions" style="${pick ? "" : "grid-template-columns: 1fr"}">${pick}${share}</div>`;
        } else {
            out += `<span class="fs-cap t3"><span class="kbd">I</span> épingler</span>`;
        }
        return out;
    }

    #placeCard() {
        const cur = this.#current;
        if (!cur || this.#card.hidden) return;
        const gl = this.#app.gl;
        const zoom = gl.getZoom();
        const s = gl.pixelToScreen(cur.x, cur.y);
        const w = this.#card.offsetWidth, h = this.#card.offsetHeight;
        const vw = window.innerWidth, vh = window.innerHeight;
        let left = s.x + Math.max(zoom, 4) + 16;
        let top = s.y - 22;
        if (left + w > vw - 16) left = s.x - w - 16;
        top = Math.min(Math.max(top, 92), vh - h - 90);
        left = Math.min(Math.max(left, 148), vw - w - 16);
        this.#card.style.left = left + "px";
        this.#card.style.top = top + "px";
    }

    #onAction(ev) {
        const btn = ev.target.closest("[data-act]");
        if (!btn || !this.#current) return;
        const {x, y} = this.#current;
        switch (btn.dataset.act) {
            case "close":
                this.close();
                break;
            case "pick":
                this.#app.pickColor(x, y);
                this.close();
                break;
            case "share":
                this.close();
                this.#app.openShare({x, y});
                break;
            case "copy":
                this.#app.copyLink({x, y});
                break;
        }
    }
}

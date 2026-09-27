// The "Modèle" sheet of /ace: choose an image, place it, follow it.

import {Blueprint} from "./blueprint.js";
import {$, $$, fillIcons, hideSheet, showSheet, toast} from "./ui.js";
import {formatCoord, formatNumber} from "./view.js";

export function setupBlueprint(app) {
    const bp = new Blueprint(app);
    const overlay = $("#bp-overlay");
    const ctx = overlay.getContext("2d");
    let maxSide = 128, quantize = true;

    const resize = () => {
        overlay.width = window.innerWidth;
        overlay.height = window.innerHeight;
    };
    resize();
    window.addEventListener("resize", resize);

    app.blueprint = bp;
    app.blueprintDrag = false;

    app.drawBlueprint = () => {
        ctx.clearRect(0, 0, overlay.width, overlay.height);
        if (app.loaded) bp.draw(ctx, app.gl);
    };

    const render = () => {
        $(".js-bpm-empty").hidden = bp.active;
        $(".js-bpm-active").hidden = !bp.active;
        for (const b of $$(".js-bpm-size button")) b.classList.toggle("on", Number(b.dataset.size) === maxSide);
        $(".js-bpm-quant").classList.toggle("toggle-on", quantize);
        $(".js-bpm-quant").setAttribute("aria-pressed", String(quantize));
        for (const b of $$(".js-blueprint")) b.setAttribute("aria-pressed", String(bp.active && bp.visible));
        if (!bp.active) return;
        const thumb = $(".js-bpm-thumb");
        thumb.width = bp.w;
        thumb.height = bp.h;
        const tctx = thumb.getContext("2d");
        const img = tctx.createImageData(bp.w, bp.h);
        bp.cells.forEach((v, i) => {
            if (v < 0) return;
            img.data.set([v >> 16, (v >> 8) & 255, v & 255, 255], i * 4);
        });
        tctx.putImageData(img, 0, 0);
        $(".js-bpm-name").textContent = bp.name || "Modèle";
        $(".js-bpm-dims").textContent = `${bp.w} × ${bp.h} px`;
        const {done, total} = bp.progress();
        const pct = total ? Math.floor(done / total * 100) : 0;
        $(".js-bpm-bar").style.width = pct + "%";
        $(".js-bpm-progress").textContent = total
            ? (done === total ? `Terminé : les ${formatNumber(total)} pixels sont posés !` : `${formatNumber(done)} / ${formatNumber(total)} pixels déjà bons · ${pct} %`)
            : "Le modèle est en dehors du canvas.";
        for (const b of $$(".js-bpm-mode button")) b.classList.toggle("on", b.dataset.mode === bp.mode);
        $(".js-bpm-opacity").value = Math.round(bp.opacity * 100);
        $(".js-bpm-opacity").hidden = bp.mode !== "image";
        $(".js-bpm-follow").classList.toggle("toggle-on", bp.follow);
        $(".js-bpm-follow").setAttribute("aria-pressed", String(bp.follow));
        $(".js-bpm-pos").textContent = formatCoord(bp.x, bp.y);
        $(".js-bpm-drag").setAttribute("aria-pressed", String(app.blueprintDrag));
        $(".js-bpm-hide-label").textContent = bp.visible ? "Masquer" : "Afficher";
    };
    app.renderBlueprintPanel = () => { if (!$("#sheet-blueprint").hidden) render(); };

    const changed = () => {
        bp.save();
        render();
        app.render();
    };

    for (const b of $$(".js-blueprint")) b.addEventListener("click", () => {
        if (!app.loaded) return;
        render();
        showSheet($("#sheet-blueprint"), {onClose: () => { app.blueprintDrag = false; document.body.classList.remove("bp-dragging"); }});
    });
    $(".js-bpm-size").addEventListener("click", e => {
        const b = e.target.closest("[data-size]");
        if (b) { maxSide = Number(b.dataset.size); render(); }
    });
    $(".js-bpm-quant").addEventListener("click", () => { quantize = !quantize; render(); });
    $(".js-bpm-file").addEventListener("change", async e => {
        const f = e.target.files[0];
        e.target.value = "";
        if (!f) return;
        try {
            await bp.fromFile(f, {maxSide, quantize});
            if (bp.follow) app.followBlueprint();
            changed();
        } catch (err) {
            console.error(err);
            toast(`<span class="fs-small b">Cette image ne peut pas être lue.</span>`);
        }
    });
    $(".js-bpm-mode").addEventListener("click", e => {
        const b = e.target.closest("[data-mode]");
        if (b) { bp.mode = b.dataset.mode; changed(); }
    });
    $(".js-bpm-opacity").addEventListener("input", e => { bp.opacity = Number(e.target.value) / 100; bp.save(); app.render(); });
    $(".js-bpm-follow").addEventListener("click", () => {
        bp.follow = !bp.follow;
        if (bp.follow) app.followBlueprint();
        changed();
    });
    $(".js-bpm-move-wrap")?.remove();
    for (const b of $$("[data-nudge]")) b.addEventListener("click", () => {
        const [dx, dy] = b.dataset.nudge.split(",").map(Number);
        bp.moveTo(bp.x + dx, bp.y + dy);
        changed();
    });
    $(".js-bpm-here").addEventListener("click", () => {
        const c = (app.me && app.aim) || app.centerPixel();
        bp.moveTo(c.x - Math.floor(bp.w / 2), c.y - Math.floor(bp.h / 2));
        changed();
    });
    $(".js-bpm-drag").addEventListener("click", () => {
        app.blueprintDrag = !app.blueprintDrag;
        document.body.classList.toggle("bp-dragging", app.blueprintDrag);
        if (app.blueprintDrag) {
            hideSheet();
            app.blueprintDrag = true;
            document.body.classList.add("bp-dragging");
            toast(`<span class="toast-text"><span class="fs-small b">Glisse le modèle à la souris</span><span class="fs-cap t3">Échap ou clic droit pour terminer.</span></span>`);
        }
        render();
    });
    $(".js-bpm-next").addEventListener("click", () => {
        const from = (app.me && app.aim) || app.centerPixel();
        const p = bp.nextWrong(from.x, from.y);
        if (!p) {
            toast(`<span class="fs-small b">Plus rien à poser : le modèle est terminé !</span>`);
            return;
        }
        app.goTo(p);
        hideSheet();
    });
    $(".js-bpm-hide").addEventListener("click", () => { bp.visible = !bp.visible; changed(); });
    $(".js-bpm-remove").addEventListener("click", () => {
        bp.clear();
        app.blueprintDrag = false;
        render();
        app.render();
    });
    fillIcons($("#sheet-blueprint"));
    return bp;
}

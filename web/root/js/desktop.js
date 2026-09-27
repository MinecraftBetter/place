// Desktop controls. Historical ones kept: left drag = move, middle click = pipette,
// right click = place (Ctrl + right click = pipette), wheel / + / - / PageUp / PageDown /
// numpad = zoom. Added: hover = inspector, I = pin it, G = grid, L = copy the link,
// Escape = close, arrows + Enter = aim and place with the keyboard.

import {$, $$} from "./ui.js";

export function setupDesktop(app) {
    const cvs = $("#viewport-canvas");
    let drag = null;   // {x, y, button}
    let lastMouse = null;
    let lastPainted = null; // last pixel painted while dragging with the right button

    // Paints every pixel between two points (Bresenham), so fast strokes leave no holes.
    const paintLine = (a, b) => {
        let {x: x0, y: y0} = a;
        const {x: x1, y: y1} = b;
        const dx = Math.abs(x1 - x0), dy = -Math.abs(y1 - y0);
        const sx = x0 < x1 ? 1 : -1, sy = y0 < y1 ? 1 : -1;
        let err = dx + dy;
        for (let guard = 0; guard < 2048; guard++) {
            app.placePixel(x0, y0, {source: "drag", quiet: true});
            if (x0 === x1 && y0 === y1) break;
            const e2 = 2 * err;
            if (e2 >= dy) { err += dy; x0 += sx; }
            if (e2 <= dx) { err += dx; y0 += sy; }
        }
    };

    const pixelAt = ev => app.gl.screenToPixel(ev.clientX, ev.clientY);

    cvs.addEventListener("mousedown", ev => {
        if (!app.loaded) return;
        const pos = {x: ev.clientX, y: ev.clientY};
        app.closeMenus();
        switch (ev.button) {
            case 0: {
                if (app.picking) {
                    const p = pixelAt(ev);
                    if (p) app.pickColor(p.x, p.y);
                    app.setPicking(false);
                    return;
                }
                drag = {...pos, button: 0};
                break;
            }
            case 1: {
                ev.preventDefault();
                const p = pixelAt(ev);
                if (p && app.me) app.pickColor(p.x, p.y);
                break;
            }
            case 2: {
                drag = {...pos, button: 2};
                const p = pixelAt(ev);
                if (!p) break;
                if (ev.ctrlKey) app.pickColor(p.x, p.y);
                else app.placePixel(p.x, p.y, {source: "mouse"});
                lastPainted = p;
                break;
            }
        }
    });

    document.addEventListener("mouseup", () => {
        drag = null;
        lastPainted = null;
        document.body.classList.remove("dragging");
    });

    document.addEventListener("mousemove", ev => {
        const pos = {x: ev.clientX, y: ev.clientY};
        lastMouse = pos;
        if (!app.loaded) return;
        if (drag) {
            if (drag.button === 2 || ev.buttons === 2) {
                const p = pixelAt(ev);
                if (p) {
                    if (ev.ctrlKey) app.pickColor(p.x, p.y);
                    else if (lastPainted && app.cooldown <= 0) paintLine(lastPainted, p);
                    else app.placePixel(p.x, p.y, {source: "drag", quiet: true});
                    lastPainted = p;
                }
            } else {
                app.gl.move(pos.x - drag.x, pos.y - drag.y);
                drag.x = pos.x;
                drag.y = pos.y;
                document.body.classList.add("dragging");
                app.viewChanged();
            }
        }
        if (ev.target !== cvs) {
            app.setHover(null);
            app.inspector.leave();
            return;
        }
        const p = pixelAt(ev);
        app.setHover(p);
        if (p && !drag) app.inspector.hover(p.x, p.y);
        else app.inspector.leave();
    });

    cvs.addEventListener("mouseleave", () => {
        app.setHover(null);
        app.inspector.leave();
    });

    window.addEventListener("wheel", ev => {
        if (!app.loaded || ev.target.closest?.(".sheet, .d-palette, .d-menu, .modal")) return;
        ev.preventDefault();
        app.gl.zoomAt(ev.deltaY > 0 ? 1 / 1.05 : 1.05, ev.clientX, ev.clientY);
        app.viewChanged();
    }, {passive: false});

    document.addEventListener("keydown", ev => {
        if (ev.target.closest?.("input, textarea") || ev.metaKey || ev.altKey) return;
        if (ev.ctrlKey && ev.code !== "NumpadAdd" && ev.code !== "NumpadSubtract") return;
        const step = ev.shiftKey ? 10 : 1;
        // Letters by key (layout-proof: AZERTY, BÉPO…), the rest by physical code.
        const key = ev.key.length === 1 ? ev.key.toLowerCase() : ev.code;
        switch (key) {
            case "PageDown":
            case "NumpadSubtract":
            case "-":
                ev.preventDefault();
                app.zoomBy(1 / 1.2);
                break;
            case "PageUp":
            case "NumpadAdd":
            case "+":
            case "=":
                ev.preventDefault();
                app.zoomBy(1.2);
                break;
            case "g":
                app.toggleGrid();
                break;
            case "l":
                app.copyLink();
                break;
            case "i": {
                const p = (app.keyboardAim && app.aim) || app.hover;
                if (p) app.inspector.togglePin(p.x, p.y);
                break;
            }
            case "Escape":
                app.escape();
                break;
            case "ArrowUp":
            case "ArrowDown":
            case "ArrowLeft":
            case "ArrowRight": {
                if (!app.loaded) break;
                ev.preventDefault();
                const dx = ev.code === "ArrowLeft" ? -step : ev.code === "ArrowRight" ? step : 0;
                const dy = ev.code === "ArrowUp" ? -step : ev.code === "ArrowDown" ? step : 0;
                app.moveAim(dx, dy, {follow: true});
                break;
            }
            case "Enter":
            case "NumpadEnter":
                if (app.aim && !ev.target.closest?.("button, a")) {
                    ev.preventDefault();
                    app.placePixel(app.aim.x, app.aim.y, {source: "keyboard"});
                }
                break;
        }
    });

    // Toolbar
    for (const b of $$(".js-zoom-in")) b.addEventListener("click", () => app.zoomBy(1.2));
    for (const b of $$(".js-zoom-out")) b.addEventListener("click", () => app.zoomBy(1 / 1.2));
    for (const b of $$(".js-pipette")) b.addEventListener("click", () => app.setPicking(!app.picking));
    for (const b of $$(".js-copy-link")) b.addEventListener("click", () => app.copyLink());

    return {get lastMouse() { return lastMouse; }};
}

// Touch controls (HANDOFF §5).
// Player: tap = aim (reticle + 7×7 loupe), drag one finger = refine the aim,
// "Poser" = confirm, pinch = zoom, two fingers = move, long press 500 ms = inspector.
// Guest: tap = inspector, one finger = move.

import {$} from "./ui.js";

const LONG_PRESS = 500;
const TAP_SLOP = 8;

export function setupMobile(app) {
    const cvs = $("#viewport-canvas");
    let gesture = null;

    const center = touches => {
        let x = 0, y = 0;
        for (const t of touches) { x += t.clientX; y += t.clientY; }
        return {x: x / touches.length, y: y / touches.length};
    };
    const distance = (a, b) => Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY);

    cvs.addEventListener("touchstart", ev => {
        if (!app.loaded) return;
        ev.preventDefault();
        app.closeMenus();
        const touches = ev.touches;
        clearTimeout(gesture?.longTimer);
        if (touches.length === 1) {
            const t = touches[0];
            gesture = {
                kind: "one", startX: t.clientX, startY: t.clientY, lastX: t.clientX, lastY: t.clientY,
                started: performance.now(), moved: false, longFired: false,
            };
            gesture.longTimer = setTimeout(() => {
                if (!gesture || gesture.moved) return;
                gesture.longFired = true;
                const p = app.gl.screenToPixel(gesture.startX, gesture.startY);
                if (p) {
                    navigator.vibrate?.(12);
                    app.inspector.openSheet(p.x, p.y);
                }
            }, LONG_PRESS);
        } else if (touches.length >= 2) {
            const c = center([touches[0], touches[1]]);
            gesture = {kind: "two", cx: c.x, cy: c.y, dist: distance(touches[0], touches[1])};
        }
    }, {passive: false});

    cvs.addEventListener("touchmove", ev => {
        if (!gesture || !app.loaded) return;
        ev.preventDefault();
        const touches = ev.touches;
        if (gesture.kind === "one" && touches.length === 1) {
            const t = touches[0];
            if (!gesture.moved && Math.hypot(t.clientX - gesture.startX, t.clientY - gesture.startY) > TAP_SLOP) {
                gesture.moved = true;
                clearTimeout(gesture.longTimer);
            }
            if (gesture.moved) {
                const dx = t.clientX - gesture.lastX, dy = t.clientY - gesture.lastY;
                if (app.me) {
                    app.nudgeAim(dx, dy);   // the reticle follows the finger, offset from it
                } else {
                    app.gl.move(dx, dy);
                    app.viewChanged();
                }
            }
            gesture.lastX = t.clientX;
            gesture.lastY = t.clientY;
        } else if (touches.length >= 2) {
            const c = center([touches[0], touches[1]]);
            const d = distance(touches[0], touches[1]);
            if (gesture.kind !== "two") {
                clearTimeout(gesture.longTimer);
                gesture = {kind: "two", cx: c.x, cy: c.y, dist: d};
                return;
            }
            app.gl.move(c.x - gesture.cx, c.y - gesture.cy);
            if (gesture.dist > 0 && d > 0) app.gl.zoomAt(d / gesture.dist, c.x, c.y);
            gesture.cx = c.x;
            gesture.cy = c.y;
            gesture.dist = d;
            app.viewChanged();
        }
    }, {passive: false});

    const end = ev => {
        if (!gesture) return;
        ev.preventDefault();
        clearTimeout(gesture.longTimer);
        if (gesture.kind === "one" && !gesture.moved && !gesture.longFired && ev.touches.length === 0) {
            const p = app.gl.screenToPixel(gesture.startX, gesture.startY);
            if (p) {
                if (app.me) app.setAim(p, {loupe: true});
                else app.inspector.openSheet(p.x, p.y);
            }
        }
        if (gesture.kind === "one" && gesture.moved && app.me) app.aimSettled();
        if (ev.touches.length === 0) gesture = null;
        else if (ev.touches.length === 1 && gesture.kind === "two") {
            // One finger left after a pinch: do not turn the rest into a drag.
            gesture = {kind: "idle"};
        }
    };
    cvs.addEventListener("touchend", end, {passive: false});
    cvs.addEventListener("touchcancel", end, {passive: false});

    // Poser, and aiming tools of the side column.
    $(".js-place")?.addEventListener("click", () => {
        if (app.aim) app.placePixel(app.aim.x, app.aim.y, {source: "button"});
    });
}

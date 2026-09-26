import {test} from "node:test";
import assert from "node:assert/strict";
import {parseView, viewQuery, roundZoom, formatZoom, formatCoord, formatNumber, relativeTime, formatCountdown} from "../root/js/view.js";

test("position links are read and checked against the canvas", () => {
    assert.deepEqual(parseView("?x=470&y=300&z=6", 1024, 768), {x: 470, y: 300, z: 6});
    assert.deepEqual(parseView("?x=470&y=300", 1024, 768), {x: 470, y: 300, z: null});
    assert.equal(parseView("", 1024, 768), null);
    assert.equal(parseView("?x=1024&y=0", 1024, 768), null);
    assert.equal(parseView("?x=-1&y=0", 1024, 768), null);
    assert.equal(parseView("?x=1.5&y=0", 1024, 768), null);
    assert.equal(parseView("?x=abc&y=0", 1024, 768), null);
    assert.equal(parseView("?x=1&y=2&z=999", 1024, 768).z, 60);
    assert.equal(parseView("?x=1&y=2&z=-3", 1024, 768).z, null);
});

test("position links are written", () => {
    assert.equal(viewQuery({x: 470.7, y: 300.2, z: 6}), "?x=470&y=300&z=6");
    assert.equal(viewQuery({x: 470, y: 300, z: 2.46}), "?x=470&y=300&z=2.5");
    assert.equal(viewQuery({x: 470, y: 300, z: 6}, false), "?x=470&y=300");
    assert.equal(roundZoom(12.4), 12);
});

test("French formatting", () => {
    assert.equal(formatZoom(2.5), "×2,5");
    assert.equal(formatZoom(8), "×8");
    assert.equal(formatCoord(470, 352), "x 470 · y 352");
    assert.equal(formatNumber(786432), "786 432");
    assert.equal(formatCountdown(4.2), "0:05");
    assert.equal(formatCountdown(0), "0:00");
    assert.equal(formatCountdown(75), "1:15");
});

test("relative times", () => {
    const now = Date.UTC(2026, 9, 14, 12, 0, 0);
    assert.equal(relativeTime(now - 10_000, now), "à l'instant");
    assert.equal(relativeTime(now - 3 * 60_000, now), "il y a 3 min");
    assert.equal(relativeTime(now - 2 * 3600_000, now), "il y a 2 h");
    assert.equal(relativeTime(now - 3 * 86400_000, now), "il y a 3 j");
    assert.equal(relativeTime(new Date(2022, 8, 13, 12).getTime(), now), "13 sept. 2022");
});

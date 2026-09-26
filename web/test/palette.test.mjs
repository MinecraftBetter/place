import {test} from "node:test";
import assert from "node:assert/strict";
import {PALETTE, parseHex, hexToRgb, rgbToHex, colorName, colorLabel, pushRecent} from "../root/js/palette.js";

test("the 30 historical colours keep their names", () => {
    assert.equal(PALETTE.length, 30);
    assert.equal(colorName("#5EB3FF"), "bleu evian");
    assert.equal(colorName("#00ccc0"), "bleu sale de bain");
    assert.equal(colorName("33e9f4"), "cian");
    assert.equal(PALETTE.filter(([, n]) => n === "rouge").length, 2);
});

test("free hex input is normalised like the historical field", () => {
    assert.equal(parseHex("#5eb3ff"), "#5EB3FF");
    assert.equal(parseHex("5eb"), "#55EEBB");
    assert.equal(parseHex("#12"), "#120000");
    assert.equal(parseHex("zz#12345678"), "#123456");
    assert.equal(parseHex(""), "#000000");
});

test("rgb conversions", () => {
    assert.deepEqual(Array.from(hexToRgb("#5EB3FF")), [94, 179, 255]);
    assert.equal(rgbToHex(new Uint8Array([94, 179, 255, 255])), "#5EB3FF");
});

test("labels fall back to hex for free colours", () => {
    assert.equal(colorLabel("#ff63aa"), "rose");
    assert.equal(colorLabel("#123456"), "#123456");
});

test("recent colours: most recent first, no duplicates, 6 max", () => {
    const r = pushRecent(["#000000", "#FFFFFF", "#FF63AA"], "#ff63aa");
    assert.deepEqual(r, ["#FF63AA", "#000000", "#FFFFFF"]);
    assert.equal(pushRecent(["#1", "#2", "#3", "#4", "#5", "#6"].map(parseHex), "#abcdef").length, 6);
});

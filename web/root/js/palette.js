// The 30 colours of BetterPlace and their historical names (kept as is, typos included).
export const PALETTE = [
    ["#000000", "noir"],
    ["#333434", "gris"],
    ["#d4d7d9", "gris clair"],
    ["#ffffff", "blanc"],
    ["#6d302f", "maron"],
    ["#6d001a", "rouge marronâtre"],
    ["#9c451a", "maron clair"],
    ["#be0027", "rouge"],
    ["#ff2651", "rouge clair"],
    ["#ff2d00", "rouge"],
    ["#ffa800", "orange foncé"],
    ["#ffd623", "jaune"],
    ["#fff8b8", "beige"],
    ["#7eed38", "vert clair"],
    ["#00cc4e", "vert"],
    ["#00a344", "vert foncé"],
    ["#598d5a", "vert foncé foncé"],
    ["#004b6f", "bleu sous marin"],
    ["#009eaa", "bleu marin"],
    ["#00ccc0", "bleu sale de bain"],
    ["#33E9F4", "cian"],
    ["#5eb3ff", "bleu evian"],
    ["#245aea", "bleu ciel"],
    ["#313ac1", "bleu ciel violet"],
    ["#1832a4", "ciel violet foncé"],
    ["#511e9f", "violet"],
    ["#6a5cff", "violet clair"],
    ["#b44ac0", "violet clair rose"],
    ["#ff63aa", "rose"],
    ["#e4abff", "rose clair"],
];

// Default "recent colours" of the mobile bar (rose, rose clair, violet clair rose, jaune, noir, blanc).
export const DEFAULT_RECENT = ["#FF63AA", "#E4ABFF", "#B44AC0", "#FFD623", "#000000", "#FFFFFF"];

const NAMES = new Map(PALETTE.map(([hex, name]) => [hex.toUpperCase(), name]));

/**
 * Normalises a free hex input the way the historical field did: keeps hex digits,
 * pads with 0 to 6 digits. "#abc" is read as the CSS shorthand.
 * Returns "#RRGGBB" (upper case).
 */
export function parseHex(input) {
    let hex = String(input).replace(/[^A-Fa-f0-9]/g, "").toUpperCase();
    if (hex.length === 3) hex = hex.split("").map(c => c + c).join("");
    hex = hex.substring(0, 6).padEnd(6, "0");
    return "#" + hex;
}

export function hexToRgb(hex) {
    const h = parseHex(hex);
    return new Uint8Array([parseInt(h.substring(1, 3), 16), parseInt(h.substring(3, 5), 16), parseInt(h.substring(5, 7), 16)]);
}

export function rgbToHex(rgb) {
    return "#" + Array.from(rgb.slice(0, 3), v => v.toString(16).padStart(2, "0")).join("").toUpperCase();
}

// French name of a palette colour, or null for a free colour.
export function colorName(hex) {
    return NAMES.get(parseHex(hex)) ?? null;
}

// Label shown for a colour: its name, or its hex code.
export function colorLabel(hex) {
    return colorName(hex) ?? parseHex(hex);
}

// Keeps the last used colours, most recent first, without duplicates.
export function pushRecent(recent, hex, max = 6) {
    const h = parseHex(hex);
    return [h, ...recent.map(parseHex).filter(c => c !== h)].slice(0, max);
}

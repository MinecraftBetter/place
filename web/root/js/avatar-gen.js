// Pixel avatars like the design's: a 9×9 grid, 1-cell margin, left/right symmetric,
// two colours (the colour and a dark version of it), 16 px per cell → 144 px.

export function randomSeed() {
    return Math.floor(Math.random() * 0xffffffff);
}

function rng(seed) {
    let s = seed >>> 0 || 1;
    return () => {
        s ^= s << 13; s >>>= 0;
        s ^= s >> 17;
        s ^= s << 5; s >>>= 0;
        return s / 0x100000000;
    };
}

// Returns the 9×9 grid (true = colour).
export function avatarGrid(seed) {
    const r = rng(seed);
    const g = Array.from({length: 9}, () => Array(9).fill(false));
    let filled = 0;
    for (let y = 1; y < 8; y++) {
        for (let x = 1; x <= 4; x++) {
            const on = r() < (x === 4 ? 0.35 : 0.5);
            g[y][x] = g[y][8 - x] = on;
            if (on) filled++;
        }
    }
    if (filled < 6) g[3][3] = g[3][5] = g[5][2] = g[5][6] = g[6][3] = g[6][4] = g[6][5] = true; // a face, just in case
    return g;
}

export function darken(hex, f = 0.23) {
    const n = parseInt(hex.replace("#", ""), 16);
    const c = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map(v => Math.round(v * f));
    return `rgb(${c.join(",")})`;
}

// Draws the avatar and resolves to a PNG Blob.
export function avatarBlob(seed, hex) {
    const cvs = document.createElement("canvas");
    cvs.width = cvs.height = 144;
    const ctx = cvs.getContext("2d");
    ctx.fillStyle = darken(hex);
    ctx.fillRect(0, 0, 144, 144);
    ctx.fillStyle = hex;
    avatarGrid(seed).forEach((row, y) => row.forEach((on, x) => { if (on) ctx.fillRect(x * 16, y * 16, 16, 16); }));
    return new Promise(res => cvs.toBlob(res, "image/png"));
}

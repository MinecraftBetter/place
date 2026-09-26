// Who placed each pixel: a local copy of /api/owners kept up to date by the WebSocket,
// plus a cache of player profiles.

export class OwnersMap {
    constructor(width, height) {
        this.width = width;
        this.height = height;
        this.ids = new Uint32Array(width * height);
        this.loaded = false;
    }

    // Fills the map from RGBA pixels where RGB holds a 24-bit player id.
    setFromRGBA(rgba) {
        const n = Math.min(this.ids.length, rgba.length >> 2);
        for (let i = 0; i < n; i++) {
            this.ids[i] = (rgba[i * 4] << 16) | (rgba[i * 4 + 1] << 8) | rgba[i * 4 + 2];
        }
        this.loaded = true;
    }

    get(x, y) {
        if (x < 0 || y < 0 || x >= this.width || y >= this.height) return 0;
        return this.ids[y * this.width + x];
    }

    set(x, y, id) {
        if (x < 0 || y < 0 || x >= this.width || y >= this.height) return;
        this.ids[y * this.width + x] = id;
    }
}

// Downloads /api/owners and decodes it without colour management.
export async function loadOwners(url, map) {
    const resp = await fetch(url, {cache: "no-store"});
    if (!resp.ok) throw new Error("owners: HTTP " + resp.status);
    const bitmap = await createImageBitmap(await resp.blob(), {colorSpaceConversion: "none", premultiplyAlpha: "none"});
    const cvs = document.createElement("canvas");
    cvs.width = bitmap.width;
    cvs.height = bitmap.height;
    const ctx = cvs.getContext("2d", {willReadFrequently: true});
    ctx.drawImage(bitmap, 0, 0);
    map.setFromRGBA(ctx.getImageData(0, 0, bitmap.width, bitmap.height).data);
    return map;
}

// Player profiles by id, fetched in batches from /api/users.
export class UserCache {
    #users = new Map();
    #pending = new Map();
    #fetcher;

    constructor(fetcher = ids => fetch("/api/users?ids=" + ids.join(",")).then(r => r.json())) {
        this.#fetcher = fetcher;
    }

    peek(id) {
        return this.#users.get(id) ?? null;
    }

    put(user) {
        if (user) this.#users.set(user.id, user);
    }

    async get(id) {
        if (!id) return null;
        if (this.#users.has(id)) return this.#users.get(id);
        if (!this.#pending.has(id)) {
            const p = this.#fetcher([id]).then(res => {
                const u = res.users?.[id] ?? null;
                this.#users.set(id, u);
                return u;
            }).finally(() => this.#pending.delete(id));
            this.#pending.set(id, p);
        }
        return this.#pending.get(id);
    }
}

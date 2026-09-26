// WebSocket connection to /ws: automatic reconnection (3 attempts), ping/pong,
// typed messages. The page decides what to do with each event.

const RETRY_DELAYS = [1000, 3000, 5000];
const PING_INTERVAL = 30000;

export class Connection extends EventTarget {
    #url;
    #socket = null;
    #attempt = 0;
    #retryTimer = null;
    #pingTimer = null;
    #lastPing = 0;
    #wasOpen = false;
    status = "idle"; // idle | connecting | open | retrying | failed
    latency = null;  // ms of the first handshake

    constructor(url) {
        super();
        this.#url = url;
    }

    get open() {
        return this.#socket?.readyState === WebSocket.OPEN;
    }

    connect() {
        clearTimeout(this.#retryTimer);
        if (this.#socket) return;
        this.#setStatus("connecting");
        const started = performance.now();
        const socket = new WebSocket(this.#url);
        this.#socket = socket;

        socket.addEventListener("open", () => {
            if (this.latency === null) this.latency = Math.round(performance.now() - started);
            const reconnected = this.#wasOpen;
            this.#wasOpen = true;
            this.#attempt = 0;
            this.#setStatus("open");
            clearInterval(this.#pingTimer);
            this.#pingTimer = setInterval(() => this.#ping(), PING_INTERVAL);
            this.#emit("open", {reconnected});
        });

        socket.addEventListener("message", ev => this.#onMessage(ev.data));

        socket.addEventListener("close", () => {
            if (this.#socket !== socket) return;
            this.#socket = null;
            clearInterval(this.#pingTimer);
            this.#scheduleRetry();
        });
        // "error" is always followed by "close": nothing to do here.
    }

    // Manual retry from the "Réessayer" button: a fresh series of 3 attempts.
    retryNow() {
        this.#attempt = 0;
        clearTimeout(this.#retryTimer);
        this.connect();
    }

    send(data) {
        if (!this.open) return false;
        this.#socket.send(typeof data === "string" ? data : JSON.stringify(data));
        return true;
    }

    // Sends a pixel in the historical format. color: [r, g, b].
    sendPixel(x, y, color) {
        return this.send({x, y, color: {R: color[0], G: color[1], B: color[2], A: 255}});
    }

    #ping() {
        if (this.send("ping")) this.#lastPing = performance.now();
    }

    #scheduleRetry() {
        if (this.#attempt >= RETRY_DELAYS.length) {
            this.#setStatus("failed");
            this.#emit("failed", {});
            return;
        }
        const delay = RETRY_DELAYS[this.#attempt++];
        this.#setStatus("retrying");
        this.#emit("retrying", {attempt: this.#attempt, of: RETRY_DELAYS.length, delay, at: Date.now() + delay});
        this.#retryTimer = setTimeout(() => this.connect(), delay);
    }

    #onMessage(data) {
        if (data === "pong") {
            this.#emit("pong", {ms: Math.round(performance.now() - this.#lastPing)});
            return;
        }
        let msg;
        try {
            msg = JSON.parse(data);
        } catch {
            return;
        }
        if (msg.type) this.#emit(msg.type, msg);   // stat, error, and later mode/announce/activity/alert
        else if (msg.color) this.#emit("pixel", msg);
    }

    #setStatus(s) {
        this.status = s;
        this.#emit("status", {status: s});
    }

    #emit(type, detail) {
        this.dispatchEvent(new CustomEvent(type, {detail}));
    }
}

// Downloads a file with a real progress percentage (Content-Length), like the historical place.js.
export async function downloadWithProgress(url, onProgress) {
    const resp = await fetch(url, {cache: "no-store"});
    if (!resp.ok) throw new Error(`${url}: HTTP ${resp.status}`);
    const len = parseInt(resp.headers.get("Content-Length") ?? "0", 10);
    if (!resp.body || !len) {
        const buf = new Uint8Array(await resp.arrayBuffer());
        onProgress?.(buf.length, buf.length);
        return buf;
    }
    const out = new Uint8Array(len);
    const reader = resp.body.getReader();
    let pos = 0;
    for (;;) {
        const {done, value} = await reader.read();
        if (value) {
            out.set(value.subarray(0, Math.min(value.length, len - pos)), pos);
            pos += value.length;
            onProgress?.(Math.min(pos, len), len);
        }
        if (done) break;
    }
    return out;
}

export function loadImage(bytes) {
    return new Promise((resolve, reject) => {
        const img = new Image();
        const url = URL.createObjectURL(new Blob([bytes], {type: "image/png"}));
        img.onload = () => {
            URL.revokeObjectURL(url);
            resolve(img);
        };
        img.onerror = reject;
        img.src = url;
    });
}

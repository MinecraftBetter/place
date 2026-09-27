// Little 8-bit sound engine for the players' museums: three looping tunes, the sounds
// played when passing an artwork, footsteps. Everything is synthesised (Web Audio),
// so there are no audio files to ship or moderate.

const NOTE = {C: 0, D: 2, E: 4, F: 5, G: 7, A: 9, B: 11};

function midi(tok) {
    const m = /^([A-G])(#|b)?(-?\d)$/.exec(tok);
    if (!m) return null;
    return 12 * (Number(m[3]) + 1) + NOTE[m[1]] + (m[2] === "#" ? 1 : m[2] === "b" ? -1 : 0);
}

const freq = n => 440 * Math.pow(2, (n - 69) / 12);

// "A4 - - C5 . ." → [{step, len, note}] ; "-" holds the previous note, "." is a rest.
function parse(pattern) {
    const toks = pattern.replace(/\|/g, " ").trim().split(/\s+/);
    const out = [];
    toks.forEach((t, i) => {
        if (t === "-") { if (out.length && out[out.length - 1].step + out[out.length - 1].len === i) out[out.length - 1].len++; return; }
        if (t === ".") return;
        const n = midi(t);
        if (n !== null) out.push({step: i, len: 1, note: n});
    });
    return {notes: out, length: toks.length};
}

const TUNES = {
    vagues: {
        bpm: 84,
        voices: [
            {wave: "triangle", vol: .2, release: .25, p: "A4 - - C5 - - E5 - D5 - - C5 - - A4 - | G4 - - A4 - - C5 - - - - - . . . . | E5 - - D5 - - C5 - A4 - - G4 - - E4 - | G4 - - A4 - - - - - - - - . . . ."},
            {wave: "sine", vol: .26, release: .4, p: "A2 - - - - - - - E3 - - - - - - - | F2 - - - - - - - C3 - - - - - - - | C3 - - - - - - - G2 - - - - - - - | E2 - - - - - - - A2 - - - - - - -"},
            {wave: "sine", vol: .06, release: .15, p: "A5 . E5 . C5 . E5 . A5 . E5 . C5 . E5 . | F5 . C5 . A4 . C5 . F5 . C5 . A4 . C5 . | C6 . G5 . E5 . G5 . C6 . G5 . E5 . G5 . | E5 . B4 . G#4 . B4 . E5 . B4 . G#4 . B4 ."},
        ],
        swell: [0, 32],
    },
    saloon: {
        bpm: 116,
        voices: [
            {wave: "square", vol: .07, release: .08, p: "E5 . G5 . C6 - . A5 G5 . E5 . C5 - D5 . | E5 . D5 . C5 . A4 . G4 - - - . . . . | F5 . A5 . C6 - . A5 G5 . F5 . E5 - D5 . | C5 . E5 . G5 . E5 . C5 - - - . . . ."},
            {wave: "triangle", vol: .22, release: .1, p: "C3 . G3 . E3 . G3 . C3 . G3 . E3 . G3 . | G2 . D3 . B2 . D3 . G2 . D3 . B2 . D3 . | F2 . C3 . A2 . C3 . F2 . C3 . A2 . C3 . | C3 . G3 . E3 . G3 . G2 . D3 . B2 . D3 ."},
        ],
    },
    desert: {
        bpm: 72,
        voices: [
            {wave: "sawtooth", vol: .08, release: .6, pluck: true, p: "E4 - - - F4 - G4 - - - F4 - E4 - - - | D4 - - - E4 - - - - - - - . . . . | B4 - - - C5 - B4 - - - A4 - G4 - - - | F4 - - - E4 - - - - - - - . . . ."},
            {wave: "triangle", vol: .2, release: .5, pluck: true, p: "E2 . . . E2 . . . E2 . . . E2 . B2 . | D2 . . . D2 . . . D2 . . . D2 . A2 . | C2 . . . C2 . . . C2 . . . C2 . G2 . | D2 . . . D2 . . . E2 . . . E2 . B2 ."},
        ],
        wind: true,
    },
};
for (const t of Object.values(TUNES)) for (const v of t.voices) Object.assign(v, parse(v.p));

export const TUNE_NAMES = {vagues: "Vagues 8-bit", saloon: "Saloon au piano", desert: "Désert de nuit"};

export class Chiptune {
    ctx = null;
    master = null;
    musicGain = null;
    timer = 0;
    tune = null;
    file = null;
    volume = .6;

    ensure() {
        if (this.ctx) return true;
        const AC = window.AudioContext || window.webkitAudioContext;
        if (!AC) return false;
        this.ctx = new AC();
        this.master = this.ctx.createGain();
        this.master.gain.value = 1;
        this.master.connect(this.ctx.destination);
        this.musicGain = this.ctx.createGain();
        this.musicGain.gain.value = this.volume;
        this.musicGain.connect(this.master);
        const len = this.ctx.sampleRate;
        this.noise = this.ctx.createBuffer(1, len, this.ctx.sampleRate);
        const d = this.noise.getChannelData(0);
        for (let i = 0; i < len; i++) d[i] = Math.random() * 2 - 1;
        return true;
    }

    setVolume(v) {
        this.volume = Math.max(0, Math.min(1, v));
        if (this.musicGain) this.musicGain.gain.setTargetAtTime(this.volume * (this.ducked ? .35 : 1), this.ctx.currentTime, .05);
        if (this.file) this.file.volume = this.volume * (this.ducked ? .35 : 1);
    }

    // lower the music while an artist speaks
    duck(on) {
        this.ducked = on;
        this.setVolume(this.volume);
    }

    // play a built-in tune ("vagues", "saloon", "desert") or an audio file URL
    play(name, url) {
        this.stop();
        if (url) {
            this.file = new Audio(url);
            this.file.loop = true;
            this.file.volume = this.volume;
            this.file.play().catch(() => {});
            return;
        }
        const tune = TUNES[name];
        if (!tune || !this.ensure()) return;
        this.ctx.resume();
        this.tune = tune;
        this.step = 0;
        this.stepDur = 60 / tune.bpm / 4;
        this.next = this.ctx.currentTime + .08;
        const len = tune.voices[0].length;
        this.len = len;
        if (tune.wind) this.startWind();
        const tick = () => {
            while (this.next < this.ctx.currentTime + .2) {
                this.schedule(this.step, this.next);
                this.next += this.stepDur;
                this.step = (this.step + 1) % len;
            }
        };
        tick();
        this.timer = setInterval(tick, 50);
    }

    playing() {
        return !!(this.tune || this.file);
    }

    stop() {
        clearInterval(this.timer);
        this.timer = 0;
        this.tune = null;
        if (this.file) { this.file.pause(); this.file = null; }
        if (this.wind) { try { this.wind.stop(); } catch { /* already stopped */ } this.wind = null; }
    }

    schedule(step, t) {
        for (const v of this.tune.voices) {
            for (const n of v.notes) if (n.step === step) this.tone(v, n.note, t, n.len * this.stepDur);
        }
        if (this.tune.swell?.includes(step)) this.swell(t, 3.2, .05, this.musicGain);
    }

    tone(v, note, t, dur) {
        const ctx = this.ctx, o = ctx.createOscillator(), g = ctx.createGain();
        o.type = v.wave;
        o.frequency.value = freq(note);
        let out = g;
        if (v.pluck) {
            const f = ctx.createBiquadFilter();
            f.type = "lowpass";
            f.frequency.setValueAtTime(2400, t);
            f.frequency.exponentialRampToValueAtTime(500, t + dur + v.release);
            g.connect(f);
            out = f;
        }
        out.connect(this.musicGain);
        o.connect(g);
        g.gain.setValueAtTime(0, t);
        g.gain.linearRampToValueAtTime(v.vol, t + .008);
        if (v.pluck) g.gain.exponentialRampToValueAtTime(.001, t + dur + v.release);
        else {
            g.gain.setValueAtTime(v.vol * .8, t + Math.max(.01, dur - .02));
            g.gain.exponentialRampToValueAtTime(.001, t + dur + v.release);
        }
        o.start(t);
        o.stop(t + dur + v.release + .05);
    }

    noiseSource(t, dur) {
        const s = this.ctx.createBufferSource();
        s.buffer = this.noise;
        s.loop = true;
        s.start(t, Math.random());
        s.stop(t + dur);
        return s;
    }

    swell(t, dur, vol, dest) {
        const ctx = this.ctx, s = this.noiseSource(t, dur), f = ctx.createBiquadFilter(), g = ctx.createGain();
        f.type = "bandpass";
        f.frequency.setValueAtTime(400, t);
        f.frequency.linearRampToValueAtTime(900, t + dur * .4);
        f.frequency.linearRampToValueAtTime(300, t + dur);
        f.Q.value = .7;
        g.gain.setValueAtTime(0, t);
        g.gain.linearRampToValueAtTime(vol, t + dur * .4);
        g.gain.linearRampToValueAtTime(0, t + dur);
        s.connect(f).connect(g).connect(dest);
    }

    startWind() {
        const ctx = this.ctx, s = ctx.createBufferSource(), f = ctx.createBiquadFilter(), g = ctx.createGain(), lfo = ctx.createOscillator(), lg = ctx.createGain();
        s.buffer = this.noise;
        s.loop = true;
        f.type = "bandpass";
        f.frequency.value = 700;
        f.Q.value = 1.2;
        g.gain.value = .025;
        lfo.frequency.value = .12;
        lg.gain.value = 400;
        lfo.connect(lg).connect(f.frequency);
        s.connect(f).connect(g).connect(this.musicGain);
        s.start();
        lfo.start();
        s.onended = () => lfo.stop();
        this.wind = s;
    }

    // sounds played when a visitor reaches an artwork, and footsteps
    sfx(name, url) {
        if (url) {
            const a = new Audio(url);
            a.volume = .8;
            a.play().catch(() => {});
            return;
        }
        if (!this.ensure()) return;
        this.ctx.resume();
        const ctx = this.ctx, t = ctx.currentTime + .02, out = this.master;
        const beep = (type, f, at, dur, vol, f2) => {
            const o = ctx.createOscillator(), g = ctx.createGain();
            o.type = type;
            o.frequency.setValueAtTime(f, at);
            if (f2) o.frequency.exponentialRampToValueAtTime(f2, at + dur);
            g.gain.setValueAtTime(vol, at);
            g.gain.exponentialRampToValueAtTime(.001, at + dur);
            o.connect(g).connect(out);
            o.start(at);
            o.stop(at + dur + .02);
        };
        switch (name) {
            case "vague":
                this.swell(t, 1.6, .18, out);
                break;
            case "clochette":
                for (const at of [t, t + .28]) { beep("sine", 1320, at, 1, .12); beep("sine", 1980, at, .6, .05); }
                break;
            case "piece":
                beep("square", 988, t, .08, .08);
                beep("square", 1319, t + .08, .35, .08);
                break;
            case "mouette":
                for (const at of [t, t + .42]) { beep("sine", 1500, at, .18, .09, 1000); beep("sine", 1250, at + .18, .2, .07, 1450); }
                break;
            case "pas": {
                for (const at of [t, t + .32]) {
                    const s = this.noiseSource(at, .06), f = ctx.createBiquadFilter(), g = ctx.createGain();
                    f.type = "lowpass";
                    f.frequency.value = 500;
                    g.gain.setValueAtTime(.12, at);
                    g.gain.exponentialRampToValueAtTime(.001, at + .06);
                    s.connect(f).connect(g).connect(out);
                }
                break;
            }
        }
    }
}

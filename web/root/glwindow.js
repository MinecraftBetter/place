// ***************************************************
// ***************************************************
// GLSL code for the vertex shader
// Scales and rotates the quad
//
const viewportVertexShaderSource = `
	precision mediump float;
	attribute vec2 vert;
	uniform vec2 cam;
	uniform vec2 tex_scale;
	uniform vec2 view_scale;
	uniform float zoom;
	varying vec2 uv;
	void main() {
		uv = vert + 0.5;
		vec2 pos = ((vert * tex_scale - cam) * zoom) / view_scale;
		pos += 0.5;
		pos.y = 1.0 - pos.y;
		gl_Position = vec4(pos * 2.0 - 1.0, 0.0, 1.0);
	}
`;

// ***************************************************
// ***************************************************
// GLSL code for the fragment shader
// Paints the texture onto the quad
//
const viewportFragmentShaderSource = `
precision mediump float;
uniform sampler2D tex;
uniform vec2 tex_scale;
uniform vec2 view_scale;
varying vec2 uv;
uniform float zoom;
uniform bool grid;

vec4 color = vec4(.95, .95, .95, 1.);

void main() {
    vec2 pos = uv * view_scale;
    vec2 px_size = view_scale / tex_scale;
    float nbPx = 1.;
    if(zoom < 10.) nbPx = 2.;
    if(zoom < 5.) nbPx = 5.;
    if(zoom < 2.5) nbPx = 15.;
    if(zoom < 1.5) nbPx = 50.;

    if (grid
    && (mod(pos.x, px_size.x * nbPx) < 2. / zoom
    || mod(pos.y, px_size.y * nbPx) < 2. / zoom)) {
        gl_FragColor = color;
    } else {
        gl_FragColor = texture2D(tex, uv);
    }
}
`;

export class GLWindow {
	#cvs;
	#gl;
	#program;
	#tex;
	#texFramebuffer;
	#texScale;
	#camPos;
	#zoom;
	#grid;

	#u_cam;
	#u_zoom;
	#u_grid;
	#u_tex;
	#u_view;
	#a_vert;

	constructor(cvs) {
		this.#cvs = cvs;
		this.#gl = cvs.getContext("webgl");
		if (this.#gl == null) {
			alert("Couldn't get WebGL context.");
			return;
		}

		this.#texScale = {x: 0, y: 0};
		this.#camPos = {x: 0, y: 0};
		this.#zoom = 1;
		this.#grid = false;

		const vertexShader = this.#compileShader(this.#gl.VERTEX_SHADER, viewportVertexShaderSource);
		const fragmentShader = this.#compileShader(this.#gl.FRAGMENT_SHADER, viewportFragmentShaderSource);

		this.#createProgram(vertexShader, fragmentShader);
		this.#createPosAttribute();
		this.#createUniforms();
		this.updateViewScale();
		this.#gl.clearColor(0.0,0.0,0.0,0.0);
	}

	ok() {
		return this.#gl != null;
	}

	draw() {
		this.#gl.bindFramebuffer(this.#gl.FRAMEBUFFER, null);
		this.#gl.clear(this.#gl.COLOR_BUFFER_BIT);
		this.#gl.drawArrays(this.#gl.TRIANGLES, 0, 6);
	}

	setTexture(img, keepView = false) {
		if (this.#tex) this.#gl.deleteTexture(this.#tex);
		if (this.#texFramebuffer) this.#gl.deleteFramebuffer(this.#texFramebuffer);
		this.#tex = this.#gl.createTexture();
		this.#gl.bindTexture(this.#gl.TEXTURE_2D, this.#tex);
		this.#gl.texParameteri(this.#gl.TEXTURE_2D, this.#gl.TEXTURE_WRAP_S, this.#gl.CLAMP_TO_EDGE);
		this.#gl.texParameteri(this.#gl.TEXTURE_2D, this.#gl.TEXTURE_WRAP_T, this.#gl.CLAMP_TO_EDGE);
		this.#gl.texParameteri(this.#gl.TEXTURE_2D, this.#gl.TEXTURE_MIN_FILTER, this.#gl.LINEAR);
		this.#gl.texParameteri(this.#gl.TEXTURE_2D, this.#gl.TEXTURE_MAG_FILTER, this.#gl.NEAREST);
		this.#gl.texImage2D(this.#gl.TEXTURE_2D, 0, this.#gl.RGBA, this.#gl.RGBA, this.#gl.UNSIGNED_BYTE, img);
		this.#texFramebuffer = this.#gl.createFramebuffer();
		this.#gl.bindFramebuffer(this.#gl.FRAMEBUFFER, this.#texFramebuffer);
		this.#gl.framebufferTexture2D(this.#gl.FRAMEBUFFER, this.#gl.COLOR_ATTACHMENT0, this.#gl.TEXTURE_2D, this.#tex, 0);
		this.#texScale = {x: img.width, y: img.height};
		this.#gl.uniform2f(this.#u_tex, this.#texScale.x, this.#texScale.y);
		if (keepView) {
			this.setZoom(this.#zoom);
		} else if (this.#cvs.width > this.#cvs.height) {
			this.#zoom = this.#cvs.width / this.#texScale.x;
		} else {
			this.#zoom = this.#cvs.height / this.#texScale.y;
		}
		this.setZoom(this.#zoom);
		this.setGrid(this.#grid);
	}

	setPixelColor(x, y, color) {
		let rgba = new Uint8Array(4);
		rgba[3] = 255;
		for (let i = 0; i < color.length; i++) {
			rgba[i] = color[i];
		}
		this.#gl.texSubImage2D(this.#gl.TEXTURE_2D, 0, x, y, 1, 1, this.#gl.RGBA, this.#gl.UNSIGNED_BYTE, rgba);
	}

	/**
	 * Reads a block of pixels (image coordinates), clamped to the canvas.
	 * Returns {x, y, w, h, data: Uint8Array RGBA}.
	 */
	getColors(x, y, w, h) {
		const x0 = Math.max(0, Math.floor(x)), y0 = Math.max(0, Math.floor(y));
		const x1 = Math.min(this.#texScale.x, Math.floor(x) + w), y1 = Math.min(this.#texScale.y, Math.floor(y) + h);
		const cw = Math.max(0, x1 - x0), ch = Math.max(0, y1 - y0);
		const data = new Uint8Array(cw * ch * 4);
		if (cw && ch) {
			this.#gl.bindFramebuffer(this.#gl.FRAMEBUFFER, this.#texFramebuffer);
			this.#gl.readPixels(x0, y0, cw, ch, this.#gl.RGBA, this.#gl.UNSIGNED_BYTE, data);
		}
		return {x: x0, y: y0, w: cw, h: ch, data};
	}

	getColor(pos) {
		let rgba = new Uint8Array(4);
		this.#gl.bindFramebuffer(this.#gl.FRAMEBUFFER, this.#texFramebuffer);
		this.#gl.readPixels(pos.x, pos.y, 1, 1, this.#gl.RGBA, this.#gl.UNSIGNED_BYTE, rgba);
		return rgba.slice(0,3);
	}

	scroll(ev) {
		this.#camPos = {x: ev.target.scrollLeft, y: ev.target.scrollTop};
		this.#gl.uniform2f(this.#u_cam, this.#camPos.x, this.#camPos.y);
	}

	move(x, y) {
		this.#camPos.x -= x / this.#zoom;
		this.#camPos.y -= y / this.#zoom;
		this.#gl.uniform2f(this.#u_cam, this.#camPos.x, this.#camPos.y);
	}

	setZoom(z) {
		if (z < 0.1) z = 0.1;
		if (z > 60) z = 60;
		this.#zoom = z;
		this.#gl.uniform1f(this.#u_zoom, z);
	}

	getZoom() {
		return this.#zoom;
	}

	// Camera position, in pixels from the centre of the canvas.
	getCam() {
		return {x: this.#camPos.x, y: this.#camPos.y};
	}

	setCam(x, y) {
		this.#camPos = {x, y};
		this.#gl.uniform2f(this.#u_cam, x, y);
	}

	getTexSize() {
		return {x: this.#texScale.x, y: this.#texScale.y};
	}

	getViewSize() {
		return {x: this.#cvs.width, y: this.#cvs.height};
	}

	// Centres the view on a pixel of the canvas.
	centerOn(px, py) {
		this.setCam(px + 0.5 - this.#texScale.x / 2, py + 0.5 - this.#texScale.y / 2);
	}

	// Canvas pixel shown at the centre of the screen (fractional).
	getCenterPixel() {
		return {x: this.#camPos.x + this.#texScale.x / 2, y: this.#camPos.y + this.#texScale.y / 2};
	}

	// Zooms by a factor while keeping the point under (sx, sy) in place.
	zoomAt(factor, sx, sy) {
		const before = this.screenToCanvas(sx, sy);
		this.setZoom(this.#zoom * factor);
		const after = this.screenToCanvas(sx, sy);
		this.setCam(this.#camPos.x + before.x - after.x, this.#camPos.y + before.y - after.y);
	}

	// Screen (CSS px, relative to the canvas element) → canvas coordinates (fractional, may be outside).
	screenToCanvas(sx, sy) {
		return {
			x: (sx - this.#cvs.width / 2) / this.#zoom + this.#camPos.x + this.#texScale.x / 2,
			y: (sy - this.#cvs.height / 2) / this.#zoom + this.#camPos.y + this.#texScale.y / 2,
		};
	}

	// Screen → integer pixel, or null outside the canvas.
	screenToPixel(sx, sy) {
		const p = this.screenToCanvas(sx, sy);
		const x = Math.floor(p.x), y = Math.floor(p.y);
		if (x < 0 || y < 0 || x >= this.#texScale.x || y >= this.#texScale.y) return null;
		return {x, y};
	}

	// Top-left corner of a pixel on screen.
	pixelToScreen(px, py) {
		return {
			x: (px - this.#texScale.x / 2 - this.#camPos.x) * this.#zoom + this.#cvs.width / 2,
			y: (py - this.#texScale.y / 2 - this.#camPos.y) * this.#zoom + this.#cvs.height / 2,
		};
	}

	setGrid(enable) {
		this.#grid = enable;
		this.#gl.uniform1f(this.#u_grid, enable);
	}
	getGrid() {
		return this.#grid;
	}

	updateViewScale() {
		let w = this.#cvs.clientWidth;
		let h = this.#cvs.clientHeight;
		this.#cvs.width = w;
		this.#cvs.height = h;
		this.#gl.viewport(0, 0, w, h);
		this.#gl.uniform2f(this.#u_view, w, h);
	}

	click(pos) {
		pos.x /= this.#cvs.width;
		pos.y /= this.#cvs.height;

		let a = {
			x: ((-0.5 * this.#texScale.x - this.#camPos.x) * this.#zoom) / this.#cvs.width + 0.5,
			y: ((-0.5 * this.#texScale.y - this.#camPos.y) * this.#zoom) / this.#cvs.height + 0.5,
		};

		let b = {
			x: ((0.5 * this.#texScale.x - this.#camPos.x) * this.#zoom) / this.#cvs.width + 0.5,
			y: ((0.5 * this.#texScale.y - this.#camPos.y) * this.#zoom) / this.#cvs.height + 0.5,
		};

		if (pos.x < a.x || pos.y < a.y || pos.x > b.x || pos.y > b.y) {
			return;
		}

		pos = {
			x: (pos.x - a.x) / (b.x - a.x) * this.#texScale.x,
			y: (pos.y - a.y) / (b.y - a.y) * this.#texScale.y,
		}

		return pos;
	}

	#toTexCoords(pos) {
		pos.x = Math.floor(pos.x * this.#texScale.x);
		pos.y = Math.floor(pos.y * this.#texScale.y);
		return pos;
	}

	#createProgram(vertexShader, fragmentShader) {
		this.#program = this.#gl.createProgram();
		this.#gl.attachShader(this.#program, vertexShader);
		this.#gl.attachShader(this.#program, fragmentShader);
		this.#gl.linkProgram(this.#program);
		if (!this.#gl.getProgramParameter(this.#program, this.#gl.LINK_STATUS)) {
			console.error(this.#gl.getProgramInfoLog(this.#program));
			return null;
		}
		this.#gl.useProgram(this.#program);
	}

	#compileShader(type, source) {
		let shader = this.#gl.createShader(type);
		this.#gl.shaderSource(shader, source);
		this.#gl.compileShader(shader);
		if (!this.#gl.getShaderParameter(shader, this.#gl.COMPILE_STATUS)) {
			console.error(this.#gl.getShaderInfoLog(shader));
			this.#gl.deleteShader(shader);
			return null;
		}
		return shader;
	}

	#createPosAttribute() {
		let buffer = this.#gl.createBuffer();
		this.#gl.bindBuffer(this.#gl.ARRAY_BUFFER, buffer);
		let positions = [
			-0.5,-0.5,
			 0.5,-0.5,
			 0.5, 0.5,
			-0.5,-0.5,
			 0.5, 0.5,
			-0.5, 0.5,
		];
		this.#gl.bufferData(this.#gl.ARRAY_BUFFER, new Float32Array(positions), this.#gl.STATIC_DRAW);
		this.#a_vert = this.#gl.getAttribLocation(this.#program, 'vert');
		this.#gl.vertexAttribPointer(this.#a_vert, 2, this.#gl.FLOAT, false, 0, 0);
		this.#gl.enableVertexAttribArray(this.#a_vert);
	}

	#createUniforms() {
		this.#u_cam = this.#gl.getUniformLocation(this.#program, 'cam');
		this.#u_tex = this.#gl.getUniformLocation(this.#program, 'tex_scale');
		this.#u_view = this.#gl.getUniformLocation(this.#program, 'view_scale');
		this.#u_zoom = this.#gl.getUniformLocation(this.#program, 'zoom');
		this.#u_grid = this.#gl.getUniformLocation(this.#program, 'grid');
	}
}
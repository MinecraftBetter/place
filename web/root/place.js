class Place {
	#loaded;
	#socket;
	#loadingText;
	#uiWrapper;
	#glWindow;
	mobile;

	constructor(glWindow) {
		this.#loaded = false;
		this.#socket = null;
		this.#loadingText = document.querySelector("#loading-p");
		this.#uiWrapper = document.querySelector("#ui-wrapper");
		this.#glWindow = glWindow;
		this.mobile = 'ontouchstart' in document.documentElement;
	}

	initConnection() {
		this.#loadingText.innerHTML = "Connexion en cours";
		if(this.mobile) document.body.classList.add("read-only");

		let host = window.location.host;
		let wsProt, httpProt;
		if (window.location.protocol === "http:") {
			wsProt = "ws://";
			httpProt = "http://";
		} else {
			wsProt = "wss://";
			httpProt = "https://";
		}

		this.#connect(wsProt + host + "/ws");
		this.#loadingText.innerHTML = "Téléchargement de la carte";

		fetch(httpProt + host + "/place.png")
		.then(async resp => {
			if (!resp.ok) {
				console.error("Error downloading map.");
				return null;
			}

			let buf = await this.#downloadProgress(resp);
			await this.#setImage(buf);

			this.#loaded = true;
			this.#loadingText.innerHTML = "";
			this.#uiWrapper.setAttribute("hide", true);
		});
	}

	async #downloadProgress(resp) {
		let len = parseInt(resp.headers.get("Content-Length"));
		let a = new Uint8Array(len);
		let pos = 0;
		let reader = resp.body.getReader();
		while(true) {
			let {done, value} = await reader.read();
			if (value) {
				a.set(value, pos);
				pos += value.length;
				this.#loadingText.innerHTML = "Téléchargement de la carte (" + Math.round(pos/len*100) + "%)";
			}
			if(done) break;
		}
		return a;
	}

	#connect(path) {
		this.#socket = new WebSocket(path);

		const socketMessage = async (event) => {
    		this.#handleSocketSetPixel(event.data);
		};

		const socketClose = () => {
			this.#socket = null;
		};

		const socketError = (error) => {
			console.error("Error making WebSocket connection.", error);
			alert("Erreur de connexion");
			this.#socket.close();
			this.#socket = null;
		};

		this.#socket.addEventListener("message", socketMessage);
		this.#socket.addEventListener("close", socketClose);
		this.#socket.addEventListener("error", socketError);
	}

	/**
	 * @param {int} x X coordinate
	 * @param {int} y Y coordinate
	 * @param {Uint8Array} color Pixel color
	 */
	setPixel(x, y, color) {
		if (this.#socket != null && this.#socket.readyState === 1) {
			const data = {
				"x": x,
				"y": y,
				"color": {
					"R" : color[0],
					"G" : color[1],
					"B" : color[2],
					"A" : 255
				}
			};
			this.#socket.send(JSON.stringify(data));
			this.#glWindow.setPixelColor(x, y, color);
			this.#glWindow.draw();
		} else {
			alert("Déconnecté, veuillez rafraichir la page");
			console.error("Disconnected.");
		}
	}

	#handleSocketSetPixel(b) {
		if (this.#loaded) {
			const data = JSON.parse(b);
			let x = data["x"];
			let y = data["y"];
			let color = new Uint8Array(4);
			color[0] = data["color"]["R"];
			color[1] = data["color"]["G"];
			color[2] = data["color"]["B"];
			color[3] = data["color"]["A"];
			console.log(data, x, y, color);
			this.#glWindow.setPixelColor(x, y, color);
			this.#glWindow.draw();
		}
	}

	async #setImage(data) {
		let img = new Image()
		let blob = new Blob([data], {type : "image/png"});
		img.src = URL.createObjectURL(blob);
		let promise = new Promise((resolve, reject) => {
			img.onload = () => {
				this.#glWindow.setTexture(img);
				this.#glWindow.setZoom(window.visualViewport.height * 0.75 / img.height);
				this.#glWindow.draw();
				resolve();
			};
			img.onerror = reject;
		});
		await promise;
	}
}
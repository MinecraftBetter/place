function main() {
    let cvs = document.querySelector("#viewport-canvas");
    let glWindow = new GLWindow(cvs);

    if (!glWindow.ok()) return;

    let place = new Place(glWindow);
    place.initConnection();
    GUI(cvs, glWindow, place);
}

const GUI = (cvs, glWindow, place) => {
    let color = new Uint8Array([0, 0, 0]);
    let dragdown = false;
    let lastMovePos = {x: 0, y: 0};
    let touchstartTime;

    const colorField = document.querySelector("#color-field");
    const colorPreset = document.querySelector("#color-preset");
    const colorSwatch = document.querySelector("#color-swatch");

    // ***************************************************
    // ***************************************************
    // Event Listeners
    //
    document.addEventListener("keydown", ev => {
        switch (ev.code) {
            case "PageDown":
            case "NumpadSubtract":
                ev.preventDefault();
                zoomOut(1.2);
                break;
            case "PageUp":
            case "NumpadAdd":
                ev.preventDefault();
                zoomIn(1.2);
                break;
            case "KeyP":
                const pos = glWindow.click(lastMovePos);
                console.log("Current mouse position:", parseInt(pos.x), parseInt(pos.y));
                break;
        }
    });

    window.addEventListener("wheel", ev => {
        if (ev.deltaY > 0) {
            zoomOut(1.05);
        } else {
            zoomIn(1.05);
        }
    });

    document.querySelector("#zoom-in").addEventListener("click", () => {
        zoomIn(1.2);
    });

    document.querySelector("#zoom-out").addEventListener("click", () => {
        zoomOut(1.2);
    });

    window.addEventListener("resize", ev => {
        glWindow.updateViewScale();
        glWindow.draw();
    });

    cvs.addEventListener("mousedown", (ev) => {
        const pos = {x: ev.clientX, y: ev.clientY};
        switch (ev.button) {
            case 0:
                dragdown = true;
                lastMovePos = pos;
                break;
            case 1:
                pickColor(pos);
                break;
            case 2:
                dragdown = true;
                if (ev.ctrlKey) {
                    pickColor(pos);
                } else {
                    drawPixel(pos, color);
                }
        }
    });

    document.addEventListener("mouseup", (ev) => {
        dragdown = false;
        document.body.style.cursor = "auto";
    });

    document.addEventListener("mousemove", (ev) => {
        const movePos = {x: ev.clientX, y: ev.clientY};
        if (dragdown) {
            if (ev.buttons === 2) {
                if (ev.ctrlKey) {
                    pickColor(movePos);
                } else {
                    drawPixel(movePos, color);
                }
            } else {
                glWindow.move(movePos.x - lastMovePos.x, movePos.y - lastMovePos.y);
                glWindow.draw();
                document.body.style.cursor = "grab";
            }
        }
        lastMovePos = movePos;
    });

    cvs.addEventListener("touchstart", (ev) => {
        touchstartTime = (new Date()).getTime();
        lastMovePos = {x: ev.touches[0].clientX, y: ev.touches[0].clientY};
    });

    if (!place.mobile)
        document.addEventListener("touchend", (ev) => {
            let elapsed = (new Date()).getTime() - touchstartTime;
            if (elapsed < 100) {
                drawPixel(lastMovePos, color);
            }
        });

    document.addEventListener("touchmove", (ev) => {
        let movePos = {x: ev.touches[0].clientX, y: ev.touches[0].clientY};
        glWindow.move(movePos.x - lastMovePos.x, movePos.y - lastMovePos.y);
        glWindow.draw();
        lastMovePos = movePos;
    });

    cvs.addEventListener("contextmenu", () => {
        return false;
    });

    colorField.addEventListener("change", ev => setColor(colorField.value));

    const presets = {
        "#000000": "noir",
        "#333434": "gris",
        "#d4d7d9": "gris clair",
        "#ffffff": "blanc",
        "#6d302f": "maron",
        "#6d001a": "rouge marronâtre",
        "#9c451a": "maron clair",
        "#be0027": "rouge",
        "#ff2651": "rouge clair",
        "#ff2d00": "rouge",
        "#ffa800": "orange foncé",
        "#ffd623": "jaune",
        "#fff8b8": "beige",
        "#7eed38": "vert clair",
        "#00cc4e": "vert",
        "#00a344": "vert foncé",
        "#598d5a": "vert foncé foncé",
        "#004b6f": "bleu sous marin",
        "#009eaa": "bleu marin",
        "#00ccc0": "bleu sale de bain",
        "#33E9F4": "cian",
        "#5eb3ff": "bleu evian",
        "#245aea": "bleu ciel",
        "#313ac1": "bleu ciel violet",
        "#1832a4": "ciel violet foncé",
        "#511e9f": "violet",
        "#6a5cff": "violet clair",
        "#b44ac0": "violet clair rose",
        "#ff63aa": "rose",
        "#e4abff": "rose clair",
    };
    console.log(presets);
    Object.entries(presets).forEach(([key, value]) => {
        var element = document.createElement("button");
        colorPreset.appendChild(element);
        element.setAttribute('data-color', key);
        element.setAttribute("title", value);
        element.style.backgroundColor = key;
        element.addEventListener("click", ev => setColor(ev.target.getAttribute('data-color')));
    });

    // ***************************************************
    // ***************************************************
    // Helper Functions
    //
    const setColor = (c) => {
        let hex = c.replace(/[^A-Fa-f0-9]/g, "").toUpperCase();
        hex = hex.substring(0, 6);
        while (hex.length < 6) {
            hex += "0";
        }
        color[0] = parseInt(hex.substring(0, 2), 16);
        color[1] = parseInt(hex.substring(2, 4), 16);
        color[2] = parseInt(hex.substring(4, 6), 16);
        hex = "#" + hex;
        colorField.value = hex;
        colorSwatch.style.backgroundColor = hex;
    }

    const pickColor = (pos) => {
        color = glWindow.getColor(glWindow.click(pos));
        let hex = "#";
        for (let i = 0; i < color.length; i++) {
            let d = color[i].toString(16);
            if (d.length === 1) d = "0" + d;
            hex += d;
        }
        colorField.value = hex.toUpperCase();
        colorSwatch.style.backgroundColor = hex;
    }

    const drawPixel = (pos, color) => {
        pos = glWindow.click(pos);
        if (pos) {
            const oldColor = glWindow.getColor(pos);
            for (let i = 0; i < oldColor.length; i++) {
                if (oldColor[i] !== color[i]) {
                    place.setPixel(parseInt(pos.x), parseInt(pos.y), color);
                    break;
                }
            }
        }
    }

    const zoomIn = (v) => {
        let zoom = glWindow.getZoom();
        glWindow.setZoom(zoom * v);
        glWindow.draw();
    }

    const zoomOut = (v) => {
        let zoom = glWindow.getZoom();
        if (zoom < 1) return;
        glWindow.setZoom(zoom / v);
        glWindow.draw();
    }


    setColor("#000000");
}
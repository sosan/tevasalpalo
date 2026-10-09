// Interruptor de WARP: lee el estado del servidor y lo cambia.
// Mismo patrón que proxy.js (mismo DOMContentLoaded, sin dependencias).
document.addEventListener("DOMContentLoaded", function () {
    var toggle = document.getElementById("warp-toggle");
    var status = document.getElementById("warp-status");
    if (!toggle) {
        return;
    }

    function paint(d) {
        var on = !!d.enabled;
        toggle.checked = on;
        if (toggle.disabled) {
            toggle.style.opacity = "0.5";
            toggle.style.cursor = "not-allowed";
        }
        if (!status) {
            return;
        }
        if (d.locked && on) {
            status.textContent = "WARP ON — fijado por la variable WARP del entorno";
        } else if (d.active) {
            status.textContent = "WARP ON — túnel activo en " + (d.socks || "localhost");
        } else if (on) {
            status.textContent = "WARP ON — se levantará al reproducir";
        } else {
            status.textContent = "WARP OFF — directo";
        }
    }

    function refresh() {
        return fetch("/api/warp", { method: "GET" })
            .then(function (r) { return r.json(); })
            .then(paint)
            .catch(function () { paint({ enabled: false }); });
    }

    refresh();
    // El túnel es on-demand: puede levantarse o pararse mientras miramos el
    // menú, así que el estado "activo" se refresca solo.
    setInterval(refresh, 5000);

    toggle.addEventListener("change", function () {
        var want = toggle.checked;
        fetch("/api/warp", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ enabled: want }),
        })
            .then(function (r) {
                return r.json().then(function (d) {
                    if (!r.ok) {
                        throw new Error(d.error || "error " + r.status);
                    }
                    return d;
                });
            })
            .then(paint)
            .catch(function (err) {
                if (status) {
                    status.textContent = "No se pudo guardar: " + err.message;
                }
                refresh();
            });
    });
});

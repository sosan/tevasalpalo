// Interruptor proxy multimedia: lee el estado del servidor y lo cambia.
// Sin dependencias; convive con modal.js (mismo DOMContentLoaded).
document.addEventListener("DOMContentLoaded", function () {
    var toggle = document.getElementById("proxy-media-toggle");
    var status = document.getElementById("proxy-media-status");
    if (!toggle) {
        return;
    }
    function paint(on) {
        toggle.checked = !!on;
        if (status) {
            status.textContent = on
                ? "Proxy ON — streams por Tor/xray"
                : "Proxy OFF — streams directos";
        }
    }
    fetch("/api/proxy-media", { method: "GET" })
        .then(function (r) { return r.json(); })
        .then(function (d) { paint(d.enabled); })
        .catch(function () { paint(false); });
    toggle.addEventListener("change", function () {
        var want = toggle.checked;
        paint(want);
        fetch("/api/proxy-media", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ enabled: want }),
        })
            .then(function (r) { return r.json(); })
            .then(function (d) { paint(d.enabled); })
            .catch(function () { paint(!want); });
    });
});

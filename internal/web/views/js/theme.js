// --- Fase temprana: aplicar tema al <html> antes de CSS ---
(function () {
    // let theme = getCookie("theme");
    // if (!theme) {
    //     theme = "dark"; // por defecto
    //     setCookie("theme", theme);
    // }
    // document.documentElement.classList.add(theme);
    let theme = getCookie("theme");

    // Si no hay cookie, probar tema del sistema
    if (!theme) {
        if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) {
            theme = "light";
        } else {
            theme = "dark"; // por defecto
        }
        setCookie("theme", theme);
    }

    // Aplicar inmediatamente
    document.documentElement.setAttribute("data-theme", theme);
})();


// --------------
document.addEventListener("DOMContentLoaded", function () {
    initThemeforHtmlElements();
    const toggleThemeButton = document.getElementById("toggletheme");
    if (toggleThemeButton) {
        toggleThemeButton.addEventListener("click", setToggleTheme);
    }
});

function getCookie(name) {
    const value = `; ${document.cookie}`;
    const parts = value.split(`; ${name}=`);
    if (parts.length === 2) return parts.pop().split(';').shift();
}

function setCookie(name, value) {
    const d = new Date();
    d.setFullYear(d.getFullYear() + 1);
    document.cookie = `${name}=${value};expires=${d.toUTCString()};path=/`;
}
function setToggleTheme(event) {
    if (event) {
        event.preventDefault();
    }
    // let html = document.documentElement; //document.getElementsByClassName("body")[0];
    // let logo = document.getElementsByClassName("logo")[0];

    // if (!html || !logo) return;
    // if (html.classList.contains("dark")) {
    //     html.classList.replace("dark", "light");
    //     logo.classList.replace("dark", "light")
    //     setCookie("theme", "light");
    // } else {
    //     html.classList.replace("light", "dark");
    //     logo.classList.replace("light", "dark")
    //     setCookie("theme", "dark");
    // }
    const html = document.documentElement;
    const current = html.getAttribute("data-theme");
    const next = current === "dark" ? "light" : "dark";
    html.setAttribute("data-theme", next);
    let logo = document.getElementsByClassName("logo");
    for (let i = 0; i < logo.length; i++) {
        logo[i].setAttribute("data-theme", next);
    }
    let logostv = document.getElementsByClassName("logostv");
    for (let i = 0; i < logostv.length; i++) {
        logostv[i].setAttribute("data-theme", next);
        // logostv[i].classList.replace("hidden", "visible");
    }
    setCookie("theme", next);
}

function initThemeforHtmlElements() {
    let theme = getCookie("theme");
    if (!theme) theme = "light";

    let logo = document.getElementsByClassName("logo");
    for (let i = 0; i < logo.length; i++) {
        logo[i].setAttribute("data-theme", theme);
        logo[i].classList.replace("hidden", "visible");
    }

    let logostv = document.getElementsByClassName("logostv");
    for (let i = 0; i < logostv.length; i++) {
        logostv[i].setAttribute("data-theme", theme);
        // logostv[i].classList.replace("hidden", "visible");
    }
}


// ------------
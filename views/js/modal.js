document.addEventListener("DOMContentLoaded", function () {
    loadActivityModal();
    initCompetitionPrefs();
});

const COMPETITIONS_STORAGE_KEY = "competitionsTop";

function loadCompetitionPrefs() {
    try {
        const raw = localStorage.getItem(COMPETITIONS_STORAGE_KEY);
        if (!raw) return {};
        const parsed = JSON.parse(raw);
        if (typeof parsed !== "object" || parsed === null) return {};
        return parsed;
    } catch (e) {
        console.warn("No se pudo leer competitionsTop de localStorage", e);
        return {};
    }
}

function saveCompetitionPrefs(prefs) {
    try {
        localStorage.setItem(COMPETITIONS_STORAGE_KEY, JSON.stringify(prefs));
        // Notificar a index.js sin recarga
        window.dispatchEvent(new CustomEvent("competitionsUpdated", { detail: prefs }));
    } catch (e) {
        console.warn("No se pudo guardar competitionsTop", e);
    }
}

function getEffectiveTop() {
    const serverTop = (typeof topCompetitions !== "undefined" && topCompetitions) ? topCompetitions : {};
    const userPrefs = loadCompetitionPrefs();
    // Merge: serverTop base + user overlay (Top:false elimina)
    const effective = Object.assign({}, serverTop);
    for (const [name, detail] of Object.entries(userPrefs)) {
        if (detail && detail.Top) {
            effective[name] = detail;
        } else {
            delete effective[name];
        }
    }
    return effective;
}

function isTop(competitionName) {
    const effective = getEffectiveTop();
    return !!effective[competitionName] && effective[competitionName].Top;
}

function toggleCompetition(competitionName) {
    if (!competitionName) return;
    const prefs = loadCompetitionPrefs();
    const currentlyTop = isTop(competitionName);
    if (currentlyTop) {
        prefs[competitionName] = { Top: false };
    } else {
        // Intentar preservar Titulo/Icon del servidor si existe
        const serverDetail = (typeof topCompetitions !== "undefined" && topCompetitions[competitionName]) || null;
        const allDetail = (typeof allCompetitions !== "undefined" && allCompetitions) ? findCompetitionDetail(competitionName) : null;
        const detail = serverDetail || allDetail || { Titulo: competitionName, Top: true };
        prefs[competitionName] = Object.assign({}, detail, { Top: true });
    }
    saveCompetitionPrefs(prefs);
    updateStars();
    // Si estamos en la parrilla, re-render inmediato
    if (typeof days !== "undefined" && days && typeof renderFullSchedule === "function") {
        renderFullSchedule(days);
    }
}

function findCompetitionDetail(name) {
    if (typeof allCompetitions === "undefined" || !allCompetitions) return null;
    for (const countryComps of Object.values(allCompetitions)) {
        if (countryComps[name]) return countryComps[name];
    }
    return null;
}

function updateStars() {
    const effective = getEffectiveTop();
    // Lista "Otras Competiciones" con data-competition
    document.querySelectorAll(".toggle-competition").forEach((el) => {
        const comp = el.dataset.competition;
        if (!comp) return;
        const star = el.querySelector(".icon-star");
        if (!star) return;
        const isTopNow = !!effective[comp];
        star.setAttribute("fill", isTopNow ? "#0071ea" : "none");
        const path = star.querySelector("path");
        if (path) path.setAttribute("fill", isTopNow ? "#0071ea" : "none");
    });
    // Grid Top con estrellas sin data-competition: usar texto
    document.querySelectorAll(".competitions-grid .sizebox-competition-grid").forEach((li) => {
        const titleEl = li.querySelector(".style-text-competition-content");
        if (!titleEl) return;
        const compName = titleEl.textContent.trim();
        if (!compName) return;
        const star = li.querySelector(".icon-star");
        if (!star) return;
        const isTopNow = !!effective[compName];
        star.setAttribute("fill", isTopNow ? "#0071ea" : "none");
        const path = star.querySelector("path");
        if (path) path.setAttribute("fill", isTopNow ? "#0071ea" : "none");
        // Hacer clickeable el top grid también
        const box = li.querySelector(".box-competition");
        if (box && !box.dataset.bound) {
            box.dataset.bound = "1";
            box.style.cursor = "pointer";
        }
    });
}

function initCompetitionPrefs() {
    updateStars();
    // Handler para "Otras Competiciones"
    document.querySelectorAll(".toggle-competition").forEach((el) => {
        el.addEventListener("click", (e) => {
            e.preventDefault();
            const comp = el.dataset.competition;
            if (comp) toggleCompetition(comp);
        });
    });
    // Handler para grid Top (click en la caja)
    document.querySelectorAll(".competitions-grid .box-competition").forEach((box) => {
        box.addEventListener("click", (e) => {
            e.preventDefault();
            const li = box.closest(".sizebox-competition-grid");
            if (!li) return;
            const titleEl = li.querySelector(".style-text-competition-content");
            if (!titleEl) return;
            const compName = titleEl.textContent.trim();
            if (compName) toggleCompetition(compName);
        });
    });
    // Listener para re-render externo si otra pestaña cambia
    window.addEventListener("storage", (e) => {
        if (e.key === COMPETITIONS_STORAGE_KEY) {
            updateStars();
            if (typeof days !== "undefined" && days && typeof renderFullSchedule === "function") {
                renderFullSchedule(days);
            }
        }
    });
    window.addEventListener("competitionsUpdated", () => {
        updateStars();
    });
}

// Exponer globalmente para index.js y debug
window.getEffectiveTop = getEffectiveTop;
window.toggleCompetition = toggleCompetition;
window.updateStars = updateStars;

function loadActivityModal() {
    const dialog = document.getElementById('config-menu');
    const openBtn = document.getElementById('openConfigMenuBtn');
    const closeBtn = document.querySelector('.close-dialog-config-menu');
    
    const dialogUpdating = document.getElementById('dialog-updating');
    const closeBtnUpdate = document.querySelector('.close-dialog-dialog-updating');
    
    if (closeBtnUpdate && dialogUpdating) {
        closeBtnUpdate.addEventListener('click', () => {
            dialogUpdating.close();
        });
    }
    
    if (openBtn && dialog) {
        openBtn.addEventListener('click', () => {
            updateStars();
            dialog.showModal();
        });
    }
    
    if (closeBtn && dialog) {
        closeBtn.addEventListener('click', () => {
            dialog.close();
        });
    }
    
    // Cerrar haciendo clic fuera
    if (dialog) {
        dialog.addEventListener('click', (event) => {
            if (event.target === dialog) {
                dialog.close();
            }
        });
    }

    // Botón Restablecer por defecto si existe
    const resetBtn = document.getElementById('reset-competitions-btn');
    if (resetBtn) {
        resetBtn.addEventListener('click', (e) => {
            e.preventDefault();
            localStorage.removeItem(COMPETITIONS_STORAGE_KEY);
            updateStars();
            if (typeof days !== "undefined" && days && typeof renderFullSchedule === "function") {
                renderFullSchedule(days);
            }
        });
    }
}

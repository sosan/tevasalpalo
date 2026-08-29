if (typeof COMPETITIONS_STORAGE_KEY === 'undefined') {
    var COMPETITIONS_STORAGE_KEY = "competitionsTop";
}
if (typeof COMPETITIONS_ORDER_KEY === 'undefined') {
    var COMPETITIONS_ORDER_KEY = "competitionsOrder";
}

document.addEventListener("DOMContentLoaded", function () {
    // evitar doble init si el script se carga 2 veces
    if (window.__modalInitDone) return;
    window.__modalInitDone = true;
    loadActivityModal();
    initCompetitionPrefs();
    applyCompetitionOrderToGrid();
    initCompetitionOrderSortable();
});

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
    // Grid Top: preferir data-competition (clave real), fallback a texto
    document.querySelectorAll(".competitions-grid .sizebox-competition-grid").forEach((li) => {
        const compName = (li.dataset.competition || li.querySelector(".style-text-competition-content")?.textContent.trim() || "");
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
            const compName = li.dataset.competition || li.querySelector(".style-text-competition-content")?.textContent.trim();
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

// --- Orden drag & drop ---
function loadCompetitionOrder() {
    try {
        const raw = localStorage.getItem(COMPETITIONS_ORDER_KEY);
        if (!raw) return null;
        const parsed = JSON.parse(raw);
        if (!Array.isArray(parsed)) return null;
        return parsed;
    } catch (e) {
        return null;
    }
}

function saveCompetitionOrder() {
    try {
        const grid = document.querySelector('.competitions-grid');
        if (!grid) return;
        const order = [...grid.querySelectorAll('.sizebox-competition-grid')]
            .map(li => li.dataset.competition)
            .filter(Boolean);
        localStorage.setItem(COMPETITIONS_ORDER_KEY, JSON.stringify(order));
        window.dispatchEvent(new CustomEvent('competitionsOrderChanged', { detail: order }));
    } catch (e) {
        console.warn('No se pudo guardar competitionsOrder', e);
    }
}

function applyCompetitionOrderToGrid() {
    const saved = loadCompetitionOrder();
    if (!saved || saved.length === 0) return;
    const grid = document.querySelector('.competitions-grid');
    if (!grid) return;
    const map = new Map();
    [...grid.children].forEach(li => {
        const key = li.dataset.competition;
        if (key) map.set(key, li);
    });
    // re-append en orden guardado
    saved.forEach(name => {
        const el = map.get(name);
        if (el) {
            grid.appendChild(el);
            map.delete(name);
        }
    });
    // competiciones nuevas (no estaban en el orden guardado) quedan al final
    // ya están en map, se mantienen en su posición actual al final
}

function initCompetitionOrderSortable() {
    const grid = document.querySelector('.competitions-grid');
    if (!grid) return;
    if (typeof Sortable === 'undefined') {
        console.warn('SortableJS no cargado, drag desactivado');
        return;
    }
    if (grid._sortable) return;
    grid._sortable = new Sortable(grid, {
        animation: 150,
        ghostClass: 'sortable-ghost',
        dragClass: 'sortable-drag',
        handle: '.box-competition',
        onEnd: function () {
            saveCompetitionOrder();
        }
    });
    // clicks en el grid no deben disparar drag imediatamente: Sortable maneja handle
}

function getCompetitionOrder() {
    return loadCompetitionOrder();
}

// Exponer globalmente para index.js y debug
window.getEffectiveTop = getEffectiveTop;
window.toggleCompetition = toggleCompetition;
window.updateStars = updateStars;
window.getCompetitionOrder = getCompetitionOrder;
window.applyCompetitionOrderToGrid = applyCompetitionOrderToGrid;
window.saveCompetitionOrder = saveCompetitionOrder;

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
        // Al cerrar, guardar orden (por si arrastre sin onEnd) y reordenar parrilla
        dialog.addEventListener('close', () => {
            saveCompetitionOrder();
            if (typeof days !== "undefined" && days && typeof renderFullSchedule === "function") {
                renderFullSchedule(days);
            }
        });
    }

    // Botón Restablecer por defecto si existe
    const resetBtn = document.getElementById('reset-competitions-btn');
    if (resetBtn) {
        resetBtn.addEventListener('click', (e) => {
            e.preventDefault();
            localStorage.removeItem(COMPETITIONS_STORAGE_KEY);
            localStorage.removeItem(COMPETITIONS_ORDER_KEY);
            // restaurar orden del servidor recargando grid desde DOM original? simplificar: reload
            updateStars();
            if (typeof days !== "undefined" && days && typeof renderFullSchedule === "function") {
                renderFullSchedule(days);
            }
            // opcional: recargar para restaurar orden servidor
            setTimeout(() => location.reload(), 300);
        });
    }
}

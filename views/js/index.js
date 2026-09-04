// const updateBoton = document.getElementById("updateboton");
const updateContentButton = document.getElementById("updatecontent");
const dialogUpdating = document.getElementById('dialog-updating');
const contentUpdating = document.getElementById('content-updating');
const messageWarn = document.getElementById('message-warn');

const oneHour = 60 * 60 * 1000;
let updating = false;
let updatingcheck = false;
let countPID = 1;



async function updateApp() {
    if (updating) return;
    updating = true;
    try {
        dialogUpdating.showModal();
        contentUpdating.innerText = "Actualización iniciada. Por favor, espere...";
        console.log("Actualización iniciada.");

        const request = await fetch('/update', { method: 'GET' });
        if (!request.ok) {
            throw new Error(`HTTP ${request.status} ${request.statusText}`);
        }
        await request.json(); // { sendedupdate: true } — dummy, solo confirma que el servidor aceptó

        // Poll /healthz hasta que Updated==true. Antes el código hacía clearInterval
        // incluso cuando data.ok==false (update aún en curso) y en catch abortaba
        // el polling ante un error transitorio. Ahora solo cierra en éxito.
        let attempts = 0;
        const maxAttempts = 60; // ~2 min con intervalo 2s
        const pollInterval = 2000;

        const intervalHealthz = setInterval(async () => {
            attempts++;
            try {
                const healthReq = await fetch('/healthz', { method: 'GET' });
                if (!healthReq.ok) {
                    // 5xx o 404 transitorio — seguir intentando
                    if (attempts >= maxAttempts) {
                        clearInterval(intervalHealthz);
                        updating = false;
                        contentUpdating.innerText = `Error en la actualización (healthz ${healthReq.status})`;
                        setTimeout(() => dialogUpdating.close(), 5000);
                    }
                    return;
                }
                const healthData = await healthReq.json();
                if (healthData.ok) {
                    clearInterval(intervalHealthz);
                    updating = false;
                    contentUpdating.innerText = "Actualización terminada";
                    setTimeout(() => {
                        dialogUpdating.close();
                        if (messageWarn) {
                            messageWarn.innerText = "";
                            messageWarn.classList.remove("display-block");
                            messageWarn.classList.add("display-none");
                        }
                    }, 5000);
                    return;
                }
                // healthData.ok === false → aún actualizando, seguir poll
                if (attempts >= maxAttempts) {
                    clearInterval(intervalHealthz);
                    updating = false;
                    contentUpdating.innerText = "Tiempo de espera agotado. Recargue la página.";
                    setTimeout(() => dialogUpdating.close(), 5000);
                }
            } catch (error) {
                // Error de red (servidor reiniciándose) — NO abortar, reintentar
                console.warn(`healthz intento ${attempts} falló:`, error);
                if (attempts >= maxAttempts) {
                    clearInterval(intervalHealthz);
                    updating = false;
                    contentUpdating.innerText = `Error verificando actualización`;
                    setTimeout(() => dialogUpdating.close(), 5000);
                }
            }
        }, pollInterval);
    } catch (error) {
        updating = false;
        console.error("Error al enviar la solicitud de actualización:", error);
        contentUpdating.innerText = `Error al iniciar la actualización`;
        setTimeout(() => {
            dialogUpdating.close();
        }, 5000);
    }
}



// Listener de actualización registrado una sola vez para evitar leak por setInterval
let updateListenerBound = false;
function bindUpdateListenerOnce() {
    if (updateListenerBound) return;
    const btn = document.getElementById("message-warn");
    if (!btn) return;
    btn.addEventListener("click", (event) => {
        event.preventDefault();
        updateApp();
    });
    updateListenerBound = true;
}

async function startCheckAppUpdate() {
    if (updatingcheck) return; // evitar solapamiento si el intervalo dispara antes de terminar
    if (!messageWarn) return;
    updatingcheck = true;
    try {
        console.log("Verificando actualizaciones...");
        const request = await fetch('/updateavailable', { method: 'GET' });
        if (!request.ok) {
            console.error("Error verificando actualizaciones:", request.statusText);
            return;
        }
        const data = await request.json();

        if (data.needUpdate) {
            messageWarn.innerText = "¡Nueva versión disponible! Haz clic aquí para actualizar.";
            messageWarn.classList.remove("display-none");
            messageWarn.classList.add("display-block");
            bindUpdateListenerOnce();
        } else {
            messageWarn.innerText = "";
            messageWarn.classList.remove("display-block");
            messageWarn.classList.add("display-none");
        }
    } catch (error) {
        console.error("Error verificando actualizaciones:", error);
    } finally {
        updatingcheck = false;
    }
}

function getEffectiveTopForFilter() {
    // Reusa window.getEffectiveTop si modal.js ya cargó, si no fallback local
    if (typeof window.getEffectiveTop === "function") {
        try { return window.getEffectiveTop(); } catch (e) {}
    }
    try {
        const raw = localStorage.getItem("competitionsTop");
        const userPrefs = raw ? JSON.parse(raw) : {};
        const serverTop = (typeof topCompetitions !== "undefined" && topCompetitions) ? topCompetitions : {};
        const effective = Object.assign({}, serverTop);
        for (const [name, detail] of Object.entries(userPrefs || {})) {
            if (detail && detail.Top) effective[name] = detail;
            else delete effective[name];
        }
        return effective;
    } catch (e) {
        return (typeof topCompetitions !== "undefined" && topCompetitions) ? topCompetitions : {};
    }
}

document.addEventListener("DOMContentLoaded", function () {
    if (!days || !topCompetitions) {
        console.error("No se encontraron datos de días.");
        return;
    }
    
    console.log("Datos de días cargados:", days);
    console.log("Top Competitions para filtrar (server):", topCompetitions);
    console.log("Top Competitions efectivo (server+local):", getEffectiveTopForFilter());
    updateContentButton.addEventListener("click", updateContent);
    
    // solapamiento startUpdateCheck y setinterval
    startCheckAppUpdate();
    setInterval(startCheckAppUpdate, oneHour);
    renderFullSchedule(days);

    // Re-render cuando modal cambia prefs (sin recarga)
    window.addEventListener("competitionsUpdated", () => {
        console.log("competitionsUpdated → re-render con effectiveTop", getEffectiveTopForFilter());
        renderFullSchedule(days);
    });
    window.addEventListener("competitionsOrderChanged", () => {
        console.log("competitionsOrderChanged → re-render con nuevo orden");
        renderFullSchedule(days);
    });
    window.addEventListener("storage", (e) => {
        if (e.key === "competitionsTop" || e.key === "competitionsOrder") {
            renderFullSchedule(days);
        }
    });
});


/**
 * Genera todo el HTML de la programación dentro de #daylist
 * @param {Array} daysData - El array de objetos de días con competiciones y partidos.
 */
function renderFullSchedule(daysData) {
    const container = document.getElementById('daylist');
    if (!container) {
        console.error("No se encontró el contenedor #daylist para renderizar.");
        return;
    }

    // EffectiveTop = serverTop + localStorage overlay (Plan A)
    const effectiveTop = getEffectiveTopForFilter();

    // Limpiar cualquier contenido previo
    container.innerHTML = '';

    // Iterar por cada día
    daysData.forEach(dayObj => {
        const dayItem = document.createElement('li');
        dayItem.className = 'content-item';

        // Título del día
        const titleSpan = document.createElement('span');
        titleSpan.className = 'title-section-widget';
        titleSpan.innerHTML = `<strong>${dayObj.FormattedDate}</strong>`;
        dayItem.appendChild(titleSpan);

        // Contenedor para las competiciones del día
        const tableContent = document.createElement('ol');
        tableContent.className = 'table-content';

        // Ordenar competiciones según orden arrastrado + Order del servidor
        let entries = Object.entries(dayObj.Competitions);
        // Si hay orden personalizado en localStorage, respetarlo
        const customOrder = (typeof window.getCompetitionOrder === 'function' && window.getCompetitionOrder()) || null;
        entries.sort((a, b) => {
            const nameA = a[0], nameB = b[0];
            if (customOrder) {
                const idxA = customOrder.indexOf(nameA);
                const idxB = customOrder.indexOf(nameB);
                if (idxA !== -1 && idxB !== -1) return idxA - idxB;
                if (idxA !== -1) return -1;
                if (idxB !== -1) return 1;
            }
            const ordA = (effectiveTop[nameA] && effectiveTop[nameA].Order) || 999;
            const ordB = (effectiveTop[nameB] && effectiveTop[nameB].Order) || 999;
            if (ordA !== ordB) return ordA - ordB;
            return nameA.localeCompare(nameB);
        });
        // Iterar por cada competición del día (ya ordenada)
        for (const [competitionName, matches] of entries) {

            const competitionSection = document.createElement('li');
            competitionSection.className = 'competition-section';
            competitionSection.dataset.competition = competitionName;

            // Título de la competición
            const competitionTitle = document.createElement('h3');
            competitionTitle.className = 'competition-title';
            competitionTitle.textContent = competitionName;
            competitionSection.appendChild(competitionTitle);

            // Lista de partidos
            const matchList = document.createElement('ol');

            matches.forEach((matchData, matchIndex) => {
                const filterCompetition = effectiveTop[competitionName];
                if (filterCompetition === undefined && matchData.Sport !== "Tenis" && matchData.Sport !== "Motociclismo") {
                    competitionSection.remove();
                    matchList.remove();
                    return;
                }

                const matchItem = document.createElement('li');
                matchItem.className = 'dailyevent';

                // channels puede venir null (partidos sin broadcasters mapeados) -> evitar crash
                if (!matchData.channels || !Array.isArray(matchData.channels) || matchData.channels.length === 0) {
                    return;
                }
                let atleastLink = false;
                for (let j = 0; j < matchData.channels.length; j++) {
                    const ch = matchData.channels[j];
                    if (!ch) continue;
                    if (ch.name === "APLAZADO") {
                        atleastLink = true;
                        continue;
                    }
                    if (!ch.link || ch.link.length === 0) {
                        continue;
                    }
                    atleastLink = true;
                }
                if (!atleastLink) {
                    return;
                }

                const broadcastersWithLinks = formatBroadcasters(
                    matchData.channels || [],
                    matchData.event || "",
                    matchData.competition || ""
                );

                matchItem.innerHTML = `
                    <div class="dailytime">
                        <i class="${matchData.Icon}"></i>
                        <span class="dailyday">${matchData.Sport}</span>
                        <strong class="dailyhour">${matchData.time || matchData.Match?.Time || ''}</strong>
                    </div>
                    <span class="dailycompetition multiline-truncate">${matchData.competition || matchData.Match?.Competition || ''}</span>
                    <span class="dailyteams multiline-truncate">${matchData.event || matchData.Match?.Event || ''}</span>
                    <div class="dailychannel multiline-truncate">
                        ${broadcastersWithLinks}
                    </div>
                `;
                matchList.appendChild(matchItem);
            });
            if (matchList.childElementCount > 0) {
                competitionSection.appendChild(matchList);
                tableContent.appendChild(competitionSection);
            }
        }

        dayItem.appendChild(tableContent);
        container.appendChild(dayItem);
    });
}

/**
 * Formatea la lista de canales/links para un partido.
 * @param {Array} broadcasters - Array de objetos {name: "...", link: [...]}
 * @returns {string} HTML string para los canales.
 */
function formatBroadcasters(broadcasters, eventName, competitionName) {
    if (!broadcasters || broadcasters.length === 0) {
        return '';
    }
    // dedup defensivo en frontend: si Go aún manda duplicados, colapsar por nombre
    const seen = new Map();
    const deduped = [];
    for (const b of broadcasters) {
        if (!b || !b.name || b.name.trim() === '') continue;
        const key = b.name.trim().toUpperCase();
        if (seen.has(key)) {
            // merge links si ya existe
            const existing = seen.get(key);
            const merged = [...(existing.link || existing.Links || []), ...(b.link || b.Links || [])];
            existing.link = [...new Set(merged)];
        } else {
            seen.set(key, b);
            deduped.push(b);
        }
    }
    broadcasters = deduped;
    if (broadcasters.length === 0) return '';

    return broadcasters.map((broadcaster, broadcasterIndex) => {
        const links = broadcaster.link || broadcaster.Links;
        if (links === undefined && broadcaster.name !== "APLAZADO") {
            return;
        }

        let html = '';

        html += `<span class="broadcaster-links">`;
        html += `<span class="broadcaster-name">${broadcaster.name.trim()} </span>`;

        if (links && Array.isArray(links) && links.length > 0) {
            const linksHtml = links.map((link, linkIndex) => {
                if (link && typeof link === 'string') {
                    const encriptedContent = setEncriptedContent(broadcaster.name, eventName, competitionName);
                    const btnText = `Link ${linkIndex + 1}`;
                    countPID++;
                    return `<a href="/player/index.html?link=${link}&content=${encriptedContent}&pid=${countPID}&btn=${encodeURIComponent(btnText)}" target="_blank" class="broadcaster-link">${btnText}</a>`;
                } else if (link === undefined || link === null || link === '') {
                    if (broadcaster.name && (broadcaster.name.includes("APLAZADO") || broadcaster.name.includes("POS"))) {
                        return `<span>${broadcaster.name}</span>`;
                    }
                }
                return;
            }).join('');
            html += linksHtml;
        }
        html += '</span>';
        return html;
    }).join('');
}

function setEncriptedContent(broadcasterName, eventName, competitionName) {
    const content = `${broadcasterName};${eventName};${competitionName}`;
    const encoder = new TextEncoder();
    const bytes = encoder.encode(content); // UTF-8
    let binary = '';
    bytes.forEach((b) => binary += String.fromCharCode(b));
    return btoa(binary);
}



let updatingContent = false;
async function updateContent(event) {
    if (event) event.preventDefault();
    if (updatingContent) return;
    updatingContent = true;
    try {
        dialogUpdating.showModal();
        contentUpdating.innerText = "Actualización iniciada. Por favor, espere...";
        const request = await fetch('/refresh-data', { method: 'GET' });
        if (!request.ok) {
            throw new Error(`HTTP ${request.status} ${request.statusText}`);
        }
        const data = await request.json();
        if (!data.success) {
            throw new Error(data.error || "success=false");
        }
        contentUpdating.innerText = "Actualización terminada. Recargando la página...";
        setTimeout(() => {
            window.location.reload();
        }, 1500);
    } catch (err) {
        console.error("Error cannot update content:", err);
        contentUpdating.innerText = `Error al actualizar contenido`;
        setTimeout(() => {
            dialogUpdating.close();
        }, 4000);
    } finally {
        updatingContent = false;
    }
}
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
    try {
        // updateBoton.ariaDisabled = "true";
        // updateBoton.disabled = true;
        updating = true;
        const request = await fetch('/update', { method: 'GET' });
        dialogUpdating.showModal();
        if (request.ok) {
            contentUpdating.innerText = "Actualización iniciada. Por favor, espere...";
            console.log("Actualización iniciada.");
        } else {
            // updateBoton.ariaDisabled = "false";
            // updateBoton.disabled = false;
            updating = false;
            console.error("Error al iniciar la actualización:", request.statusText);
            contentUpdating.innerText = `Error al iniciar la actualización`;
            setTimeout(() => {
                dialogUpdating.close();
            }, 5000);
            return;
        }

        const data = await request.json(); // dummy value
        const intervalHealthz = setInterval(async () => {
            try {
                const request = await fetch('/healthz', { method: 'GET' });
                if (request.ok) {
                    const data = await request.json();
                    if (data.ok) {
                        updating = false;
                        // updateBoton.ariaDisabled = "false";
                        // updateBoton.disabled = false;
                        contentUpdating.innerText = "Actualización terminada";
                    }
                    clearInterval(intervalHealthz);
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
            } catch (error) {
                updating = false;
                // updateBoton.ariaDisabled = "false";
                // updateBoton.disabled = false;
                clearInterval(intervalHealthz);
                // console.error("Error al verificar el estado de salud:", error);
                // contentUpdating.innerText = `Error en la actualización`;
                setTimeout(() => {
                    dialogUpdating.close();
                }, 5000);
            }
        }, 20_000);
    } catch (error) {
        updating = false;
        // updateBoton.ariaDisabled = "false";
        // updateBoton.disabled = false;
        console.error("Error al enviar la solicitud de actualización:", error);
        contentUpdating.innerText = `Error al iniciar la actualización`;
        setTimeout(() => {
            dialogUpdating.close();
        }, 5000);
    }
}



async function startCheckAppUpdate() {
    try {
        if (!messageWarn) {
            return;
        }
        console.log("Verificando actualizaciones...");
        updatingcheck = true;
        const request = await fetch('/updateavailable', { method: 'GET' });
        if (!request.ok) {
            updatingcheck = false;
            console.error("Error verificando actualizaciones:", request.statusText);
            return;
        }
        const data = await request.json();

        if (data.needUpdate) {
            updatingcheck = false;

            messageWarn.innerText = "¡Nueva versión disponible! Haz clic aquí para actualizar.";
            messageWarn.classList.remove("display-none");
            messageWarn.classList.add("display-block");

            const messageWarnBoton = document.getElementById("message-warn");
            if (messageWarnBoton) {
                messageWarnBoton.addEventListener("click", (event) => {
                    event.preventDefault();
                    updateApp();

                }, { once: true });
            }

        } else {
            updatingcheck = false;
            messageWarn.innerText = "";
            messageWarn.classList.remove("display-block");
            messageWarn.classList.add("display-none");
        }

    } catch (error) {
        updatingcheck = false;
        console.error("Error verificando actualizaciones:", error);
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
                    countPID++;
                    return `<a href="/player/index.html?link=${link}&content=${encriptedContent}&pid=${countPID}" target="_blank" class="broadcaster-link">Link ${linkIndex + 1}</a>`;
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



async function updateContent(event) {
    event.preventDefault();
    dialogUpdating.showModal();
    contentUpdating.innerText = "Actualización iniciada. Por favor, espere...";
    const request = await fetch('/refresh-data', { method: 'GET' });
    if (!request.ok) {
        contentUpdating.innerText = `Error al iniciar la actualización`;
        console.error("Error cannot update content:", request.statusText);
        return;
    }
    const data = await request.json();
    if (!data.success) {
        contentUpdating.innerText = `Error al iniciar la actualización`;
        console.error("Error cannot update content:", request.statusText);
        return;
    }
    contentUpdating.innerText = "Actualización terminada. Recargando la página...";
    setTimeout(() => {
        // reload
        window.location.reload();
    }, 5000);
}
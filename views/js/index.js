const updateBoton = document.getElementById("updateboton");
const dialogUpdating = document.getElementById('dialog-updating');
const contentUpdating = document.getElementById('content-updating');
const messageWarn = document.getElementById('message-warn');
const oneHour = 60 * 60 * 1000;
let updating = false;
let updatingcheck = false;

updateBoton.addEventListener("click", async function (event) {
    if (updateBoton) {
        event.preventDefault();
        if (updating) return;
        try {
            updateBoton.ariaDisabled = "true";
            updateBoton.disabled = true;
            updating = true;
            const request = await fetch('/update', { method: 'GET' });
            dialogUpdating.showModal();
            if (request.ok) {
                contentUpdating.innerText = "Actualización iniciada. Por favor, espere...";
                console.log("Actualización iniciada.");
            } else {
                updateBoton.ariaDisabled = "false";
                updateBoton.disabled = false;
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
                            updateBoton.ariaDisabled = "false";
                            updateBoton.disabled = false;
                            contentUpdating.innerText = "Actualización terminada";
                        }
                        clearInterval(intervalHealthz);
                        setTimeout(() => {
                            dialogUpdating.close();
                        }, 5000);
                        return;
                    }
                } catch (error) {
                    updating = false;
                    updateBoton.ariaDisabled = "false";
                    updateBoton.disabled = false;
                    clearInterval(intervalHealthz);
                    // console.error("Error al verificar el estado de salud:", error);
                    // contentUpdating.innerText = `Error en la actualización`;
                    setTimeout(() => {
                        dialogUpdating.close();
                    }, 5000);
                }
            }, 10_000);
        } catch (error) {
            updating = false;
            updateBoton.ariaDisabled = "false";
            updateBoton.disabled = false;
            console.error("Error al enviar la solicitud de actualización:", error);
            contentUpdating.innerText = `Error al iniciar la actualización`;
            setTimeout(() => {
                dialogUpdating.close();
            }, 5000);
        }
    }
});

async function startUpdateCheck() {
    try {
        console.log("Verificando actualizaciones...");
        updatingcheck = true;
        const request = await fetch('/updateavailable', { method: 'GET' });
        if (request.ok) {
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
                        updateBoton.click();
                    }, { once: true }); // ⚡ solo 1 vez
                }

            } else {
                updatingcheck = false;
                messageWarn.innerText = "";
                messageWarn.classList.remove("display-block");
                messageWarn.classList.add("display-none");
            }
        }
    } catch (error) {
        updatingcheck = false;
        console.error("Error verificando actualizaciones:", error);
    }
    
}

document.addEventListener("DOMContentLoaded", function () {
    console.log("Datos de días cargados:", days);
    console.log("Top Competitions para filtrar:", topCompetitions);
    // solapamiento startUpdateCheck y setinterval
    startUpdateCheck();
    setInterval(startUpdateCheck, oneHour);
    renderFullSchedule(days);
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

        // Iterar por cada competición del día
        for (const [competitionName, matches] of Object.entries(dayObj.Competitions)) {

            const competitionSection = document.createElement('li');
            competitionSection.className = 'competition-section';
            competitionSection.dataset.competition = competitionName; // Para facilitar el filtrado

            // Título de la competición
            const competitionTitle = document.createElement('h3');
            competitionTitle.className = 'competition-title';
            competitionTitle.textContent = competitionName;
            competitionSection.appendChild(competitionTitle);

            // Lista de partidos
            const matchList = document.createElement('ol');

            // Iterar por cada partido en la competición
            matches.forEach(matchData => {
                const filterCompetition = topCompetitions[competitionName];
                if (filterCompetition === undefined && matchData.Sport !== "Tenis" && matchData.Sport !== "Motociclismo") {
                    competitionSection.remove();
                    matchList.remove();
                    return;
                }

                const matchItem = document.createElement('li');
                matchItem.className = 'dailyevent';

                let atleastLink = false;
                for (let j = 0; j < matchData.channels.length; j++) {
                    if (!matchData.channels[j].link || matchData.channels[j].link.length === 0) {
                        continue;
                    }
                    if (matchData.channels[j].link || matchData.channels[j].link.length > 0) {
                        atleastLink = true;
                    }
                }
                if (!atleastLink) {
                    return;
                }

                const broadcastersWithLinks = formatBroadcasters(matchData.channels || matchData.Match?.Broadcasters || []);

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
function formatBroadcasters(broadcasters) {
    if (!broadcasters || broadcasters.length === 0) {
        return '<span>Sin links disponibles</span>';
    }

    return broadcasters.map((broadcaster, broadcasterIndex) => {
        let html = '';
        if (broadcasterIndex > 0) {
            html += '<br />';
        }
        html += `<span class="broadcaster-links">`;
        html += `<span class="broadcaster-name">${broadcaster.name || broadcaster.Name || 'Canal'}:</span>`;

        const links = broadcaster.link || broadcaster.Links; // Manejar ambos casos

        if (links && Array.isArray(links) && links.length > 0) {
            // Si hay links, crear los enlaces
            const linksHtml = links.map((link, linkIndex) => {
                if (link && typeof link === 'string') {
                    return `<a href="/player/${link}" target="_blank" class="broadcaster-link">Link</a>`;
                } else if (link === undefined || link === null || link === '') {
                    if (broadcaster.name && (broadcaster.name.includes("APLAZADO") || broadcaster.name.includes("POS"))) {
                        return `<span>${broadcaster.name}</span>`;
                    } else {
                        return '<span>Sin link</span>';
                    }
                }
                return '<span>Sin link</span>'; // Fallback
            }).join('');
            html += linksHtml;
        } else {
            // Si no hay propiedad `link`/`Links` o está vacía
            if (broadcaster.name && (broadcaster.name.includes("APLAZADO") || broadcaster.name.includes("POS"))) {
                html += `<span>${broadcaster.name}</span>`;
            } else {
                html += '<span>Sin links disponibles</span>';
            }
        }
        html += '</span>';
        return html;
    }).join('');
}


// function loadActivityModal() {
//     const dialog = document.getElementById('config-menu');
//     const openBtn = document.getElementById('openConfigMenuBtn');
//     const closeBtn = document.querySelector('.close-dialog-config-menu');

//     openBtn.addEventListener('click', () => {
//         dialog.showModal();
//     });

//     closeBtn.addEventListener('click', () => {
//         dialog.close();
//     });

//     // Opcional: Cerrar haciendo clic fuera del contenido del diálogo (solo showModal)
//     dialog.addEventListener('click', (event) => {
//         // dialog.close() se puede llamar aquí también, pero hay que tener cuidado
//         // con cerrarlo al hacer clic dentro. Una forma más robusta es comprobar el target.
//         if (event.target === dialog) {
//             dialog.close();
//         }
//     });
// }


// window.TVASApp = {
//     // filterByCompetition: filterByCompetition,
//     renderFullSchedule: renderFullSchedule // Por si se necesita re-renderizar
// };
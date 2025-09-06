document.addEventListener("DOMContentLoaded", function () {
    console.log("Datos de días cargados:", days);
    console.log("Top Competitions para filtrar:", topCompetitions);
    renderFullSchedule(days);

    // 2. Configurar los botones de filtro que ya existen en el header
    // (Generados por el template `header.html`)
    // setupFilterButtons();

    // 3. Aplicar filtro inicial, por ejemplo, mostrar todas
    // filterByCompetition('all');

    // 4. Opcional: Destacar competiciones top visualmente
    // highlightTopCompetitions();
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
                        return `<span>${broadcaster.name}</span>`; // Mostrar el nombre si indica estado
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



window.TVASApp = {
    // filterByCompetition: filterByCompetition,
    renderFullSchedule: renderFullSchedule // Por si se necesita re-renderizar
};
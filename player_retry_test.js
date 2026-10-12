// Test de la política de reintentos de red del player (views/player.html).
//
// El motor AceStream retiene los manifests a la espera de pares P2P, así que
// los timeouts de red son habituales y NO implican que el stream esté muerto.
// Antes, un levelLoadTimeOut fatal caía en el `default` y destruía HLS.js,
// obligando a recargar el stream desde cero: eso era lo que cortaba la
// reproducción de forma intermitente.
//
// Este test extrae la función real del HTML y la evalúa, de modo que un
// cambio en la lista de errores recuperables se detecte aquí y no en el
// navegador del usuario.

const fs = require("fs");
const path = require("path");
const vm = require("vm");

const playerPath = path.join(__dirname, "views", "player.html");

function cargarIsNetworkRetryable() {
    const html = fs.readFileSync(playerPath, "utf8");
    const match = html.match(/function isNetworkRetryable\(details\) \{[\s\S]*?\n {4}\}/);
    if (!match) {
        throw new Error(
            "no se encontró isNetworkRetryable en views/player.html; " +
                "si se renombró o movió, actualiza este test"
        );
    }
    const ctx = {};
    vm.createContext(ctx);
    vm.runInContext(match[0], ctx);
    return ctx.isNetworkRetryable;
}

const tests = [
    // El motor retiene las peticiones esperando pares: estos son reintentables.
    ["levelLoadTimeOut", true],
    ["manifestLoadTimeOut", true],
    ["playlistLoadTimeOut", true],
    ["fragLoadTimeOut", true],
    ["fragLoadError", true],
    ["manifestLoadError", true],
    ["playlistLoadError", true],
    // Errores de parseo: el manifest llegó pero está roto. Reintentar
    // no lo arregla, así que no deben considerarse recuperables.
    ["manifestParsingError", false],
    ["levelEmptyError", false],
    ["internalException", false],
    // Casos límite.
    ["", false],
    [undefined, false],
    [null, false],
];

let fallos = 0;

console.log(`Probando isNetworkRetryable (${tests.length} casos)`);
const fn = cargarIsNetworkRetryable();

for (const [details, esperado] of tests) {
    const obtenido = fn(details);
    if (obtenido !== esperado) {
        console.error(
            `  FALLO isNetworkRetryable(${JSON.stringify(details)}) = ${obtenido}, se esperaba ${esperado}`
        );
        fallos++;
    }
}

// Regresión concreta: un error de red recuperable nunca debe llevar a
// destruir HLS.js. Se comprueba el bloque `default:` del switch de errores
// fatales, desde el log hasta el siguiente `break`, exigiendo que la consulta a
// isNetworkRetryable aparezca ANTES de cualquier destroy().
const html = fs.readFileSync(playerPath, "utf8");
// Hay una copia comentada de este log más arriba en el archivo; la rama real
// es la última.
const iniDefault = html.lastIndexOf('console.error("HLS.js: Error fatal", data);');
// Cuerpo de la rama: va hasta el `break;` que cierra el case.
const cuerpo =
    iniDefault === -1
        ? ""
        : html.slice(iniDefault, html.indexOf("break;", iniDefault) + "break;".length);

if (iniDefault === -1) {
    console.error("  FALLO: no se encontró la rama de error fatal del player");
    fallos++;
} else {
    const posConsulta = cuerpo.indexOf("isNetworkRetryable(data.details)");
    const posDestroy = cuerpo.indexOf("hls.destroy()");
    if (posConsulta === -1) {
        console.error(
            "  FALLO: la rama de error fatal no consulta isNetworkRetryable; " +
                "vuelve a destruir HLS.js en cuanto el manifest tarda de más"
        );
        fallos++;
    } else if (posDestroy !== -1 && posDestroy < posConsulta) {
        console.error(
            "  FALLO: hls.destroy() se ejecuta antes de comprobar si el fallo " +
                "es recuperable, así que un levelLoadTimeOut sigue destruyendo el player"
        );
        fallos++;
    }
}

// El reintento debe conservar la instancia (startLoad), no crear una
    // nueva: se comprueba dentro de la misma rama del switch, que es donde
    // importa.
    if (!cuerpo.includes("hls.startLoad()")) {
        console.error(
            "  FALLO: la rama de error fatal no usa hls.startLoad(); " +
                "el reintento destruye el player y pierde el buffer"
        );
        fallos++;
    }

// Regresión: al cambiar de Link hay que destruir la instancia VIVA, que se
// guarda en window.hlsPlayer. switchToLink miraba video.hls (que nunca se
// asigna), así que el player anterior seguía vivo: seguía pidiendo sus
// manifests,reatachando MediaSource encima del <video> compartido y
// robándole el buffer al player nuevo. De ahí los "Media attached" repetidos
// y los parones al alternar entre enlaces.
const switchFn = html.match(/function switchToLink\(newLink\) \{[\s\S]*?\n {4}\}/);
if (!switchFn) {
    console.error("  FALLO: no se encontró switchToLink() en el player");
    fallos++;
} else {
    const cuerpoSwitch = switchFn[0];
    if (!cuerpoSwitch.includes("window.hlsPlayer")) {
        console.error(
            "  FALLO: switchToLink no destruye window.hlsPlayer; el HLS " +
                "anterior sigue vivo al cambiar de enlace (zombis pidiendo manifests)"
        );
        fallos++;
    }
    if (!cuerpoSwitch.includes("detachMedia()")) {
        console.error(
            "  FALLO: switchToLink no llama detachMedia(); el MediaSource " +
                "anterior queda enganchado al <video> compartido"
        );
        fallos++;
    }
}

if (fallos > 0) {
    console.error(`\n${fallos} fallo(s)`);
    process.exit(1);
}
console.log("OK: política de reintentos de red correcta");
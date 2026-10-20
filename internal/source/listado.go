// Listado vivo: fusión de listados, ordenación por disponibilidad y carga
// rápida desde la base al arrancar.
package source

import (
	"log"
	"strings"

	"tevasalpalo/internal/store"
)

// fusionarListados une dos mapas de canal -> hashes juntando los enlaces y
func fusionarListados(a, b map[string][]string) map[string][]string {
	if a == nil {
		a = make(map[string][]string, len(b))
	}
	for nombre, hashes := range b {
		a[nombre] = quitarDuplicadosStrings(append(a[nombre], hashes...))
	}
	return a
}

func quitarDuplicadosStrings(s []string) []string {
	vistos := make(map[string]bool, len(s))
	salida := make([]string, 0, len(s))
	for _, v := range s {
		if vistos[v] {
			continue
		}
		vistos[v] = true
		salida = append(salida, v)
	}
	return salida
}

// CargarListadoDesdeDisco pone el listado guardado en BroadcasterToAcestream
// para que la web pueda servir de inmediato. No sustituye a la red: cuando las
// fuentes respondan, FetchUpdatedList lo reemplaza con datos frescos.
//
// Es la pieza que quita el arranque bloqueado. Sin base de datos no hace nada
// y todo sigue como antes.
// ordenarListadoEn vivo deja, en cada canal, el hash más probable primero.
//
// Es el uso de la disponibilidad: antes el player probaba el primero de la
// lista y se comía el timeout entero si ese hash estaba muerto. Ahora prueba
// el que se sabe que funciona.
func ordenarListadoEnVivo(mapa map[string]BroadcasterInfo) {
	if store.Global == nil || len(mapa) == 0 {
		return
	}
	// El store se consulta desde varias partes: la ordenación corre con su
	// propio mutex (OrdenarHashes).
	ordenados, muertos := 0, 0
	for nombre, info := range mapa {
		// El listado trae enlaces de dos formas mezcladas: los nativos
		// "acestream://<hash>" (o hash suelto) se deben reordenar por
		// disponibilidad, y los URI-safe ";<base64>" (los produce
		// transformUriSafeBroadcasters) ya vienen tal como se quieren servir.
		// Mandarlos por OrdenarHashes los rompe: dentro hace ToLower, y el
		// base64 distingue mayúsculas, así que al decodificar en el player
		// deja de salir la URL original. Por eso se separan y solo se
		// normalizan/reordenan los nativos.
		//
		// OrdenarHashes devuelve el hash NORMALIZADO (sin esquema), así que
		// al reensamblar hay que restaurar la forma original de cada nativo:
		// un "acestream://<hash>" (camino de disco) vuelve con su esquema, un
		// hash suelto (camino vivo) vuelve suelto. Re-prefijar siempre
		// producía "acestream://acestream://<hash>" en el listado vivo.
		type nativo struct {
			pos    int    // posición original en info.Links
			limpio string // forma normalizada (la que puntúa el store)
			esq    string // "" si venía suelto; "acestream://" si con esquema
		}
		nativos := make([]nativo, 0, len(info.Links))
		uriSafe := make([]int, 0, len(info.Links))
		for i, l := range info.Links {
			if strings.HasPrefix(l, ";") {
				uriSafe = append(uriSafe, i)
				continue
			}
			esq := ""
			if strings.HasPrefix(l, "acestream") {
				esq = "acestream://"
			}
			nativos = append(nativos, nativo{pos: i, limpio: store.NormalizarHash(l), esq: esq})
		}
		if len(nativos) < 2 {
			continue
		}
		hashes := make([]string, len(nativos))
		for j, n := range nativos {
			hashes[j] = n.limpio
		}
		nuevos, descartados := store.Global.OrdenarHashes(hashes)
		muertos += descartados
		// Si el orden no cambió respecto al de los nativos (y no hubo
		// descartes), no tocamos el canal: los URI-safe tampoco se reensamblan.
		distintos := len(nuevos) != len(hashes)
		if !distintos {
			for j, h := range nuevos {
				if hashes[j] != h {
					distintos = true
					break
				}
			}
		}
		if !distintos {
			continue
		}
		// Compactar: supervivientes nativos al frente (en el orden que devuelve
		// OrdenarHashes), URI-safe detrás en el orden relativo que ya tenían.
		// Intentar preservar la posición original de los URI-safe mezclados con
		// los nativos obligaría a los supervivientes a quedarse en su posición
		// antigua, anulando el reordenado por disponibilidad. El player prueba
		// Link 1 primero, así que el nativo con más disponibilidad queda al
		// alcance del primer clic; los URI-safe (que son los fallbacks "p;"
		// resueltos) se mantienen al final sin perder su orden.
		posDeHash := make(map[string]int, len(nativos))
		for j, n := range nativos {
			posDeHash[n.limpio] = j
		}
		enlaces := make([]string, 0, len(nuevos)+len(uriSafe))
		for _, h := range nuevos {
			enlaces = append(enlaces, nativos[posDeHash[h]].esq+h)
		}
		for _, idx := range uriSafe {
			enlaces = append(enlaces, info.Links[idx])
		}
		info.Links = enlaces
		mapa[nombre] = info
		ordenados++
	}
	if ordenados > 0 || muertos > 0 {
		log.Printf("🎯 Hashes reordenados por disponibilidad en %d canales (%d enlaces muertos descartados)",
			ordenados, muertos)
	}
}

// CargarListadoDesdeDisco rehidrata el listado de la base (arranque rapido,
// sin esperar al refresco de las fuentes) y lo ordena igual que la version viva.
func CargarListadoDesdeDisco() {
	if store.Global == nil {
		return
	}
	guardados := store.Global.CargarCanales()
	if len(guardados) == 0 {
		return
	}

	// La base guarda el listado completo (sin pasar por el gateway estricto,
	// que deja fuera la mayoría), así que no se puede reutilizar
	// updateBroadcasterMapWithGatewayTolerant: eso filtraría otra vez y el
	// arranque rápido serviría 121 canales de 1651.
	//
	// Se carga tal cual, con el mismo formato de enlaces que produce
	// transformUriSafeBroadcasters.
	tmp := make(map[string]BroadcasterInfo, len(guardados))
	for nombre, hashes := range guardados {
		enlaces := make([]string, len(hashes))
		for i, h := range hashes {
			enlaces[i] = "acestream://" + h
		}
		tmp[nombre] = BroadcasterInfo{Name: nombre, Links: enlaces}
	}
	ordenarListadoEnVivo(tmp)
	publicarBroadcaster(tmp)

	log.Printf("⚡ Listado cargado desde disco: %d canales (se refrescará con las fuentes)",
		len(tmp))
}

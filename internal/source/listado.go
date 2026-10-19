// Listado vivo: fusión de listados, ordenación por disponibilidad y carga
// rápida desde la base al arrancar.
package source

import (
	"log"

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
		if len(info.Links) < 2 {
			continue
		}
		nuevos, descartados := store.Global.OrdenarHashes(info.Links)
		muertos += descartados
		if len(nuevos) == len(info.Links) {
			distintos := false
			for i := range nuevos {
				if nuevos[i] != info.Links[i] {
					distintos = true
					break
				}
			}
			if !distintos {
				continue
			}
		}
		enlaces := make([]string, len(nuevos))
		for i, h := range nuevos {
			enlaces[i] = "acestream://" + h
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

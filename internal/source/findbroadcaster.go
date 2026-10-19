package source

import (
	"strings"
)

func FindBroadcaster(name string, competitionName, sport string) BroadcasterInfo {
	// Coincidencia exacta
	// quizas pasarlo a minisculas
	nameUpper := strings.ToUpper(strings.TrimSpace(name))
	// Normalizar abreviatura "M+ L." -> "M+ LIGA" (usuario solicita m+ l. de campeones)
	nameUpper = strings.ReplaceAll(nameUpper, "M+ L.", "M+ LIGA")
	nameUpper = strings.ReplaceAll(nameUpper, "M+L.", "M+ LIGA")
	// Colapsar espacios dobles por si acaso
	nameUpper = strings.Join(strings.Fields(nameUpper), " ")
	// Normalización para futbolenvivoargentina.com (DSports / ESPN Argentina / TNT)
	if strings.Contains(nameUpper, "DSPORTS") {
		nameUpper = "DS SPORT"
	} else if strings.Contains(nameUpper, "ESPN PREMIUM") {
		nameUpper = "ESPN ARGENTINA"
	} else if nameUpper == "ESPN 4" || strings.Contains(nameUpper, "ESPN 4") {
		nameUpper = "ESPN ARGENTINA 4"
	} else if nameUpper == "ESPN 3" || strings.Contains(nameUpper, "ESPN 3") {
		nameUpper = "ESPN ARGENTINA 3"
	} else if nameUpper == "ESPN 2" || strings.Contains(nameUpper, "ESPN 2") {
		nameUpper = "ESPN ARGENTINA 2"
	} else if strings.Contains(nameUpper, "TNT SPORTS PREMIUM") {
		nameUpper = "TNT SPORTS"
	} else if strings.Contains(nameUpper, "TNT SPORTS") {
		nameUpper = "TNT SPORTS"
	}
	if competitionName == "Bundesliga" && nameUpper == "SKY SPORTS" {
		nameUpper = "SKY SPORTS BUNDESLIGA"
	}
	if competitionName == "LaLiga" && nameUpper == "SKY SPORTS" {
		nameUpper = "SKY SPORTS LALIGA"
	}
	if sport == "Baloncesto" {
		if nameUpper == "DAZN" {
			nameUpper = "DAZN BALONCESTO"
		}
		if competitionName == "Copa del Rey Baloncesto" {
			nameUpper = "DAZN BALONCESTO"
		}
	}

	// UFC en Paramount+ (2026+) — normalizar variantes Paramount+ / CBS
	// UFC dejó ESPN+ y pasó a Paramount+ en US/LatAm/Australia desde 01/01/2026
	// Solo mapear a UFC si hay indicio UFC (en nombre o en competición), si no filtrar (ej: Paramount+ en LaLiga no es Acestream)
	if strings.Contains(nameUpper, "PARAMOUNT") {
		if strings.Contains(nameUpper, "UFC") {
			if dataAce, exists := BroadcasterToAcestream["PARAMOUNT+ UFC"]; exists && len(dataAce.Links) > 0 {
				return dataAce
			}
			if dataAce, exists := BroadcasterToAcestream["UFC"]; exists {
				return dataAce
			}
		}
		if competitionName == "UFC" {
			if dataAce, exists := BroadcasterToAcestream["UFC"]; exists {
				return dataAce
			}
			// Alias genérico Paramount+ -> mapear a UFC solo si es UFC
			nameUpper = "PARAMOUNT+ UFC"
			if dataAce, exists := BroadcasterToAcestream[nameUpper]; exists {
				return dataAce
			}
			nameUpper = "PARAMOUNT+"
			if dataAce, exists := BroadcasterToAcestream[nameUpper]; exists {
				return dataAce
			}
			if dataAce, exists := BroadcasterToAcestream["UFC"]; exists {
				return dataAce
			}
		}
		// Para LaLiga u otras competiciones, Paramount+ sin UFC no tiene pool Acestream → filtrar para no colar UFC en LaLiga
		return BroadcasterInfo{}
	}
	if strings.Contains(nameUpper, "CBS") && competitionName == "UFC" {
		if dataAce, exists := BroadcasterToAcestream["UFC"]; exists {
			return dataAce
		}
	}

	// LALIGA TV M1-M4 solo aplica a Hypermotion — evitar contaminación en Serie A Italiana
	// Ej: futbolenlatv.es lista "LaLiga TV M3" para Genoa-Como (Serie A) que NO es Hypermotion
	switch nameUpper {
	case "LALIGA TV M2", "LALIGA TV M3":
		if strings.Contains(strings.ToUpper(competitionName), "HYPERMOTION") {
			nameUpper = "LALIGA HYPERMOTION"
		} else {
			// Para Serie A y otras competiciones, M3 es canal BAR sin acestream fiable -> filtrar
			return BroadcasterInfo{}
		}
	case "LALIGA TV M4":
		if strings.Contains(strings.ToUpper(competitionName), "HYPERMOTION") {
			nameUpper = "LALIGA HYPERMOTION 2"
		} else {
			return BroadcasterInfo{}
		}
	case "LALIGA TV M1":
		if strings.Contains(strings.ToUpper(competitionName), "HYPERMOTION") {
			nameUpper = "LALIGA HYPERMOTION 3"
		} else {
			return BroadcasterInfo{}
		}
	}

	// Unificar variantes LaLiga Hypermotion al mismo canal (incluye 4 y 5 nuevos)
	switch nameUpper {
	case "LALIGA TV HYPERMOTION":
		nameUpper = "LALIGA HYPERMOTION"
	case "LALIGA TV HYPERMOTION 2":
		nameUpper = "LALIGA HYPERMOTION 2"
	case "LALIGA TV HYPERMOTION 3":
		nameUpper = "LALIGA HYPERMOTION 3"
	case "LALIGA TV HYPERMOTION 4", "LALIGA TV HYPERMOTION 5", "LALIGA HYPERMOTION 4", "LALIGA HYPERMOTION 5", "HYPERMOTION 4", "HYPERMOTION 5":
		// normalizar a LALIGA HYPERMOTION 4/5 según número
		if strings.HasSuffix(nameUpper, "5") {
			nameUpper = "LALIGA HYPERMOTION 5"
		} else {
			nameUpper = "LALIGA HYPERMOTION 4"
		}
	}

	// M+ BALONCESTO / DAZN BALONCESTO — respetar prefijo (usuario añadió M+ BALONCESTO)
	if strings.Contains(nameUpper, "BALONCESTO") {
		isMPlus := strings.Contains(nameUpper, "M+") || strings.Contains(nameUpper, "MOVISTAR")
		if isMPlus {
			if strings.Contains(nameUpper, "2") {
				nameUpper = "M+ BALONCESTO 2"
			} else if strings.Contains(nameUpper, "3") {
				nameUpper = "M+ BALONCESTO 2" // no hay M+ 3, fallback a 2
			} else {
				nameUpper = "M+ BALONCESTO"
			}
			if dataAce, exists := BroadcasterToAcestream[nameUpper]; exists && len(dataAce.Links) > 0 {
				return dataAce
			}
			// fallback a DAZN si M+ aún está vacío
		}
		if strings.Contains(nameUpper, "2") {
			nameUpper = "DAZN BALONCESTO 2"
		} else if strings.Contains(nameUpper, "3") {
			nameUpper = "DAZN BALONCESTO 3"
		} else {
			nameUpper = "DAZN BALONCESTO 1"
		}
		if dataAce, exists := BroadcasterToAcestream[nameUpper]; exists {
			return dataAce
		}
	}

	// FOX UFC / UFC Fight Pass -> UFC (Paramount+ era)
	if strings.Contains(nameUpper, "UFC") {
		if strings.Contains(nameUpper, "FOX") || strings.Contains(nameUpper, "FIGHT PASS") || strings.Contains(nameUpper, "PREMIUM") {
			if dataAce, exists := BroadcasterToAcestream["UFC"]; exists {
				return dataAce
			}
		}
	}

	if dataAce, exists := BroadcasterToAcestream[nameUpper]; exists {
		return dataAce
	}
	return BroadcasterInfo{}
}

// // findLinkForBroadcaster busca un enlace para un nombre de broadcaster.
// // Prioriza la coincidencia exacta, luego parcial.
// func findLinkForBroadcaster(name string, competitionName string) []string {
// 	// Coincidencia exacta
// 	// quizas pasarlo a minisculas
// 	nameUpper := strings.ToUpper(name)
// 	if competitionName == "Bundesliga" && nameUpper == "SKY SPORTS" {
// 		nameUpper = "SKY SPORTS BUNDESLIGA"
// 	}
// 	if dataAce, exists := BroadcasterToAcestream[nameUpper]; exists {
// 		return dataAce.Links
// 	}

// 	// Coincidencia parcial (como antes)
// 	// nameUpper := strings.ToUpper(name)
// 	for key, dataAce := range BroadcasterToAcestream {
// 		baseKey := strings.Split(key, " [")[0]
// 		if strings.Contains(nameUpper, strings.ToUpper(baseKey)) {
// 			// Preferir coincidencia exacta de base si es posible
// 			if nameUpper == strings.ToUpper(baseKey) {
// 				return dataAce.Links
// 			}
// 			// Si no hay exacta, esta es una candidata (la última encontrada)
// 			// Para hacerlo más robusto, podrías tener lógica para elegir la mejor parcial
// 		}
// 	}
// 	// Si no se encontró parcial, devolver vacío
// 	return []string{}
// }

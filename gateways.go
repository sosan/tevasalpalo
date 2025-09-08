package main

var broadcasterGatewayMap = map[string][]string{
	// `Primera Federacion "RFEF" (FHD)`:    {"Primera Federacion \"RFEF\""},
	"DAZN 1 (FHD)":                       {"DAZN 1", "DAZN"},
	"DAZN 2 (FHD)":                       {"DAZN 2"},
	"DAZN 3 (FHD)":                       {"DAZN 3"},
	"DAZN 4 (FHD)":                       {"DAZN 4"},
	"DAZN F1 (FHD)":                      {"DAZN F1"},
	"DAZN LaLiga (FHD)":                  {"DAZN LALIGA", "DAZN LALIGA TV"},
	"DAZN LaLiga 2 (FHD)":                {"DAZN LALIGA 2"},
	"LaLiga Hypermotion (FHD)":           {"LALIGA HYPERMOTION", "LALIGA TV HYPERMOTION"},
	"LaLiga Hypermotion 2 (FHD)":         {"LALIGA HYPERMOTION 2"},
	"LaLiga Hypermotion 3 (FHD)":         {"LALIGA HYPERMOTION 3"},
	"Movistar LaLiga (FHD)":              {"M+ LALIGA", "M+ LALIGA TV"},
	"Movistar LaLiga 2 (FHD)":            {"M+ LALIGA 2", "M+ LALIGA 2 TV"},
	"Movistar Plus (FHD)":                {"Movistar Plus", "Movistar Plus+", "M+"},
	"Movistar Plus 2 (FHD)":              {"Movistar Plus+ 2"},
	"Movistar Liga de Campeones (FHD)":   {"M+ LIGA DE CAMPEONES"},
	"Movistar Liga de Campeones 2 (FHD)": {"M+ LIGA DE CAMPEONES 2"},
	"Movistar Liga de Campeones 3 (FHD)": {"M+ LIGA DE CAMPEONES 3"},
	"Movistar Liga de Campeones 4 (FHD)": {"M+ LIGA DE CAMPEONES 4"},
	"Movistar Deportes (FHD)":            {"M+ DEPORTES"},
	"Movistar Deportes 2 (FHD)":          {"M+ DEPORTES 2"},
	"Movistar Deportes 3 (FHD)":          {"M+ DEPORTES 3"},
	"Movistar Deportes 4 (FHD)":          {"M+ DEPORTES 4"},
	"Movistar Deportes 5 (FHD)":          {"M+ DEPORTES 5"},
	"Movistar Deportes 6 (FHD)":          {"M+ DEPORTES 6"},
	"Movistar Vamos (FHD)":               {"M+ VAMOS"},
	"Movistar Golf (FHD)":                {"M+ GOLF", "Movistar Golf"},
	"Eurosport (FHD)":                    {"EUROSPORT 1"},
	"Eurosport 2 (FHD)":                  {"EUROSPORT 2"},
}

func updateBroadcasterMapWithGateway(existingMap map[string]BroadcasterInfo, newData map[string][]string) {
	for extractedName, links := range newData {
		mappedKeys, exists := broadcasterGatewayMap[extractedName]
		if !exists {
			continue
		}

		for _, mappedKey := range mappedKeys {
			if mappedKey == "" {
				continue
			}
			if info, ok := existingMap[mappedKey]; ok {
				info.Links = removeDuplicates(append(info.Links, links...))
				existingMap[mappedKey] = info
			} else {
				existingMap[mappedKey] = BroadcasterInfo{Links: links}
			}
		}
	}
}

func removeDuplicates(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}
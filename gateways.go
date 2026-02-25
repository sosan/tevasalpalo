package main

var broadcasterGatewayMap = map[string][]string{
	"RFEF TV":                            {"Primera Federacion \"RFEF\""},
	"PRIMERA FEDERACIÓN":                 {"Primera Federacion \"RFEF\""},
	`Primera Federacion "RFEF" (FHD)`:    {"Primera Federacion \"RFEF\""},
	"Canal 1":                            {"Primera Federacion \"RFEF\""},
	"Canal 2":                            {"Primera Federacion 2\"RFEF\""},
	"REAL MADRID TV":                     {"REAL MADRID TV"},
	"DAZN 1 (FHD)":                       {"DAZN 1"},
	"DAZN 2 (FHD)":                       {"DAZN 2"},
	"DAZN 3 (FHD)":                       {"DAZN 3"},
	"DAZN 3":                             {"DAZN 3"},
	"DAZN 4 (FHD)":                       {"DAZN 4"},
	"DAZN 1":                             {"DAZN 1"},
	"DAZN 2":                             {"DAZN 2"},
	"DAZN 4":                             {"DAZN 4"},
	"DAZN F1 (FHD)":                      {"DAZN F1"},
	"DAZN F1":                            {"DAZN F1"},
	"DAZN LaLiga (FHD)":                  {"DAZN LALIGA", "DAZN LALIGA TV"},
	"DAZN LA LIGA 1":                     {"DAZN LALIGA", "DAZN LALIGA TV"},
	"DAZN LA LIGA 2":                     {"DAZN LALIGA 2", "DAZN LALIGA TV"},
	"DAZN LaLiga 2 (FHD)":                {"DAZN LALIGA 2"},
	"LaLiga Hypermotion (FHD)":           {"LALIGA HYPERMOTION", "LALIGA TV HYPERMOTION"},
	"LaLiga Hypermotion 2 (FHD)":         {"LALIGA HYPERMOTION 2"},
	"LaLiga Hypermotion 3 (FHD)":         {"LALIGA HYPERMOTION 3"},
	"M+ LALIGA 1":                        {"M+ LALIGA 1"},
	"M+ LALIGA 2":                        {"M+ LALIGA 2"},
	"M+ LALIGA 3":                        {"M+ LALIGA 3", "LALIGA TV 2"},
	"M+ LALIGA 4":                        {"M+ LALIGA 4"},
	"Movistar LaLiga (FHD)":              {"M+ LALIGA", "LALIGA TV"},
	"Movistar LaLiga 2 (FHD)":            {"M+ LALIGA 2", "LALIGA TV 2"},
	"MOVISTAR PLUS":                      {"MOVISTAR PLUS", "MOVISTAR PLUS+", "M+"},
	"Movistar Plus (FHD)":                {"MOVISTAR PLUS", "MOVISTAR PLUS+", "M+"},
	"Movistar Plus 2 (FHD)":              {"MOVISTAR PLUS+ 2"},
	"LIGA DE CAMPEONES":                  {"M+ LIGA DE CAMPEONES"},
	"Movistar Liga de Campeones (FHD)":   {"M+ LIGA DE CAMPEONES"},
	"LIGA DE CAMPEONES 2":                {"M+ LIGA DE CAMPEONES  2"},
	"LIGA DE CAMPEONES 3":                {"M+ LIGA DE CAMPEONES  3"},
	"LIGA DE CAMPEONES 4":                {"M+ LIGA DE CAMPEONES  4"},
	"Movistar Liga de Campeones 2 (FHD)": {"M+ LIGA DE CAMPEONES 2"},
	"Movistar Liga de Campeones 3 (FHD)": {"M+ LIGA DE CAMPEONES 3"},
	"Movistar Liga de Campeones 4 (FHD)": {"M+ LIGA DE CAMPEONES 4"},
	"Movistar Liga de Campeones 5 (FHD)": {"M+ LIGA DE CAMPEONES 5"},
	"Movistar Liga de Campeones 6 (FHD)": {"M+ LIGA DE CAMPEONES 6"},
	"Movistar Liga de Campeones 7 (FHD)": {"M+ LIGA DE CAMPEONES 7"},
	"Movistar Liga de Campeones 8 (FHD)": {"M+ LIGA DE CAMPEONES 8"},
	"Movistar Liga de Campeones 9 (FHD)": {"M+ LIGA DE CAMPEONES 9"},
	"MOVISTAR DEPORTES 3":                {"M+ DEPORTES 3"},
	"Movistar Deportes (FHD)":            {"M+ DEPORTES"},
	"Movistar Deportes 2 (FHD)":          {"M+ DEPORTES 2"},
	"Movistar Deportes 3 (FHD)":          {"M+ DEPORTES 3"},
	"Movistar Deportes 4 (FHD)":          {"M+ DEPORTES 4"},
	"Movistar Deportes 5 (FHD)":          {"M+ DEPORTES 5"},
	"Movistar Deportes 6 (FHD)":          {"M+ DEPORTES 6"},
	"MOVISTAR DEPORTES 6":                {"M+ DEPORTES 6"},
	"MOVISTAR VAMOS":                     {"M+ VAMOS"},
	"Movistar Vamos (FHD)":               {"M+ VAMOS"},
	"Movistar Golf (FHD)":                {"M+ GOLF", "Movistar Golf"},
	"Eurosport (FHD)":                    {"EUROSPORT 1"},
	"Eurosport 2 (FHD)":                  {"EUROSPORT 2"},
	"M.LaLiga2":                          {"M+ LALIGA 2", "LALIGA TV 2"},
	"DAZNLaLiga":                         {"DAZN LALIGA", "DAZN LALIGA TV"},
	"Eurosport2":                         {"EUROSPORT 2"},
	"DAZNF1":                             {"DAZN F1"},
	"LaLigaSmartbank":                    {"LALIGA HYPERMOTION", "LALIGA TV HYPERMOTION"},
	"LaLigaSmartbank3":                   {"LALIGA HYPERMOTION 3"},
	"MOVISTAR DEPORTES 2":                {"M+ DEPORTES 2"},
	"Deporte2":                           {"M+ DEPORTES 2"},
	"MovistarPlus":                       {"MOVISTAR PLUS", "MOVISTAR PLUS+", "M+"},
	"Dazn1":                              {"DAZN 1"},
	"Deporte":                            {"M+ DEPORTES"},
	"Deporte3":                           {"M+ DEPORTES 3"},
	"Deporte4":                           {"M+ DEPORTES 4"},
	"Campeones":                          {"M+ LIGA DE CAMPEONES"},
	"Campeones2":                         {"M+ LIGA DE CAMPEONES 2"},
	"Campeones3":                         {"M+ LIGA DE CAMPEONES 3"},
	"Campeones4":                         {"M+ LIGA DE CAMPEONES 4"},
	"Campeones5":                         {"M+ LIGA DE CAMPEONES 5"},
	"Campeones6":                         {"M+ LIGA DE CAMPEONES 6"},
	"Campeones7":                         {"M+ LIGA DE CAMPEONES 7"},
	"Campeones8":                         {"M+ LIGA DE CAMPEONES 8"},
	"LaLigaSmartbank2":                   {"LALIGA HYPERMOTION 2"},
	"Vamos":                              {"M+ VAMOS"},
	"Deporte5":                           {"M+ DEPORTES 5"},
	"M.LaLiga":                           {"M+ LALIGA", "M+ LALIGA TV"},
	"M.LaLiga3":                          {"M+ LALIGA 3", "LALIGA TV 3"},
	"Eurosport":                          {"EUROSPORT 1"},
	"Dazn2":                              {"DAZN 2"},
	// ----
	"Fox Sports 2 COL":      {"FOX SPORTS 2"},
	"FOX Sports 2":          {"FOX Sports 2"},
	"MOVISTAR DEPORTES":     {"M+ DEPORTES"},
	"Eurosport360 6":        {"EUROSPORT 6"},
	"Eurosport360 2":        {"EUROSPORT 2"},
	"Eurosport360 1":        {"EUROSPORT 1"},
	"BEIN SPORTS 1":         {"BEIN SPORTS 1"},
	"beIN SPORTS 1":         {"BEIN SPORTS 1"},
	"BEIN SPORTS 2":         {"BEIN SPORTS 2"},
	"beIN SPORTS 2":         {"BEIN SPORTS 2"},
	"beIN SPORTS 3":         {"BEIN SPORTS 3"},
	"BEIN SPORTS 3":         {"BEIN SPORTS 3"},
	"beIN SPORTS 4":         {"BEIN SPORTS 4"},
	"BEIN SPORTS 4":         {"BEIN SPORTS 4"},
	"SKY SPORTS FOOTBALL":   {"SKY SPORTS FOOTBALL"},
	"DAZN BALONCESTO":       {"DAZN BALONCESTO 1"},
	"HYPERMOTION":           {"LALIGA HYPERMOTION", "LALIGA TV HYPERMOTION"},
	"HYPERMOTION 2":         {"LALIGA HYPERMOTION 2"},
	"RALLY TV":              {"RALLY TV"},
	"MIXED+":                {"MIXED+"},
	"CANAL MOTOR":           {"CANAL MOTOR"},
	"ONETORO":               {"ONETORO"},
	"SKY SPORTS MAIN EVENT": {"SKY SPORTS MAIN EVENT"},
	"BT Sport 1":            {"BT SPORT 1"},
	"ESPN1":                 {"ESPN1"},
	"ESPN2":                 {"ESPN2"},
	"ESPN3":                 {"ESPN3"},
	"SPORT1+":               {"SPORT1+"},
	"Esport3":               {"ESPORT3"},
	"ESPORTS 3":             {"ESPORT3"},
	"Esport3 (Cataluña)":    {"ESPORT3"},
	"ELEVEN DAZN 1":         {"DAZN 1 PT"},
	"ELEVEN DAZN 2":         {"DAZN 2 PT"},
	"ELEVEN DAZN 3":         {"DAZN 3 PT"},
	"ELEVEN DAZN 4":         {"DAZN 4 PT"},
	"ELEVEN DAZN 5":         {"DAZN 5 PT"},
	"RETROVISION MOTOR":     {"RETROVISION MOTOR"},
	"CANAL+ Sport 1":        {"CANAL+ Sport 1"},
	"CANAL+ Sport 2":        {"CANAL+ Sport 2"},
	"CANAL+ Sport 3":        {"CANAL+ Sport 3"},
	"ACB EVENTO 01":         {"DAZN BALONCESTO 1"},
	"ACB EVENTO 02":         {"DAZN BALONCESTO 2"},
	"ACB EVENTO 03":         {"DAZN BALONCESTO 3"},
	"CALCIO EVENTOS":        {"CALCIO EVENTOS"},
	"ESPN ARGENTINA 1":      {"ESPN ARGENTINA 1"},
	"ESPN ARGENTINA 2":      {"ESPN ARGENTINA 2"},
	"ESPN ARGENTINA 3":      {"ESPN ARGENTINA 3"},
	"ESPN ARGENTINA 4":      {"ESPN ARGENTINA 4"},
}

func updateBroadcasterMapWithGateway(existingMap map[string]BroadcasterInfo, newData map[string][]string) map[string]BroadcasterInfo {
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
	return existingMap
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

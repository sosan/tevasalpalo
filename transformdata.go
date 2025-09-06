package main

func transformCompetitionsToTop(allCompe AllCompetitions) map[string]CompetitionDetail {
	topCompetitions := make(map[string]CompetitionDetail)
	for _, countryComps := range allCompe {
		for name, detail := range countryComps {
			if detail.Top {
				topCompetitions[name] = detail
			}
		}
	}
	return topCompetitions
}

package main

import "sort"

type OrderedTopCompetition struct {
	Name   string
	Detail CompetitionDetail
}

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

func getOrderedTopCompetitions(top map[string]CompetitionDetail) []OrderedTopCompetition {
	ordered := make([]OrderedTopCompetition, 0, len(top))
	for name, detail := range top {
		ordered = append(ordered, OrderedTopCompetition{Name: name, Detail: detail})
	}
	sort.Slice(ordered, func(i, j int) bool {
		oi, oj := ordered[i].Detail.Order, ordered[j].Detail.Order
		// Order == 0 -> al final, ordenados alfabéticamente
		if oi == 0 && oj == 0 {
			return ordered[i].Name < ordered[j].Name
		}
		if oi == 0 {
			return false
		}
		if oj == 0 {
			return true
		}
		if oi == oj {
			return ordered[i].Name < ordered[j].Name
		}
		return oi < oj
	})
	return ordered
}

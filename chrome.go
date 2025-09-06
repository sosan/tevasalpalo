package main

import (
	"context"
	"fmt"
	"log"

	// "regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

// isRelevant checks if the competition is one we are interested in.
// You need to define your own logic here.
// func isRelevant(competition string) bool {
// 	// --- Define your relevant competitions ---
// 	// Example: Check for specific leagues or keywords in Spanish
// 	relevantLeagues := []string{
// 		"España - LaLiga", "Inglaterra - Premier League", "Alemania - Bundesliga", "Italia - Serie A", "Francia - Ligue 1",
// 		"Champions League", "Europa League", "Copa del Rey",
// 	}
// 	for _, league := range relevantLeagues {
// 		if strings.Contains(competition, "Premier League ") || strings.Contains(competition, "Italia - Serie A ") || strings.Contains(competition, "España - LaLiga ") {
// 			return false
// 		}

// 		if strings.Contains(competition, league) {
// 			return true
// 		}
// 	}
// 	return false
// }

// func isRelevant(competition string) bool {
// 	// --- Define your relevant competitions ---
// 	// Use the exact full names as they appear on the website for the main leagues
// 	// Focus on top-tier European leagues and major cups
// 	relevantLeagues := []string{
// 		// Use the specific full names found in the HTML dump (livescoretv.com.txt)
// 		// Spain
// 		"España - LaLiga", // This should match "España - LaLiga" but not "España - LaLiga SmartBank"

// 		// England - Be very specific to get only the top tier
// 		"Inglaterra - Premier League", // Should match exactly this

// 		// Germany
// 		"Alemania - Bundesliga", // Should match "Alemania - Bundesliga" but not "Alemania - 2. Bundesliga"

// 		// Italy
// 		"Italia - Serie A", // Should match exactly this

// 		// France
// 		"Francia - Ligue 1", // Should match exactly this

// 		// Other major competitions
// 		"Liga de Campeones de la UEFA", // Champions League
// 		"Liga Europa de la UEFA",       // Europa League
// 		// Add other top-tier leagues/cups you are interested in
// 		// "Brasil - Série A",
// 		// "México - Liga MX",
// 		// "Argentina - Primera División",
// 		// etc.
// 	}

// 	// Normalize the input competition name for comparison
// 	normalizedComp := strings.TrimSpace(competition)

// 	for _, league := range relevantLeagues {
// 		// Check for an exact match first (e.g., "Copa del Rey")
// 		if normalizedComp == league {
// 			return true
// 		}

// 		// Check if the competition name starts with the relevant league name
// 		// and is followed by a non-letter character (like space, '(', etc.)
// 		// This prevents matching "Inglaterra - Premier League 2" when looking for "Inglaterra - Premier League"
// 		// Example pattern for "Inglaterra - Premier League":
// 		// ^Inglaterra - Premier League($|[^a-zA-Z])
// 		// ^ : Start of string
// 		// ($|[^a-zA-Z]) : End of string OR any character that is NOT a letter
// 		pattern := "^" + regexp.QuoteMeta(league) + `($|[^a-zA-Z])`
// 		matched, err := regexp.MatchString(pattern, normalizedComp)
// 		if err != nil {
// 			// Handle regex error if necessary, though QuoteMeta should prevent it
// 			// For simplicity, we'll just log and continue
// 			// log.Printf("Regex error for pattern '%s': %v", pattern, err)
// 			continue
// 		}
// 		if matched {
// 			return true
// 		}
// 	}
// 	return false
// }

// func isRelevant(competition string) bool {
// 	// --- Define your relevant competitions ---
// 	// Use names that match the structure found in the HTML dump.
// 	// Be as specific as needed for the top tier.
// 	relevantLeagues := []string{
// 		// Spain
// 		"España-LaLiga", // Normalized version of "España - LaLiga"

// 		// England - Specific to top tier
// 		"Inglaterra-PremierLeague", // Normalized version of "Inglaterra - Premier League"

// 		// Germany
// 		"Alemania-Bundesliga", // Normalized version of "Alemania - Bundesliga"

// 		// Italy
// 		"Italia-SerieA", // Normalized version of "Italia - Serie A"

// 		// France
// 		"Francia-Ligue1", // Normalized version of "Francia - Ligue 1"

// 		// Major Cups (adjust normalization as needed based on actual HTML text)
// 		"LigadeCampeonesdelaUEFA", // Normalized version of "Liga de Campeones de la UEFA"
// 		"LigaEuropadelaUEFA",      // Normalized version of "Liga Europa de la UEFA"
// 		"CopadelRey",              // Normalized version of "Copa del Rey"

// 		// Add more top-tier leagues/cups you are interested in, in their normalized form
// 		// Example:
// 		// "Brasil-SérieA", // Note the accented 'é'
// 		// "México-LigaMX",
// 		// "Argentina-PrimeraDivisión",
// 		// etc.
// 	}

// 	// Normalize the input competition name
// 	normalizedComp := normalizeForComparison(competition)
// 	// fmt.Printf("DEBUG: Checking '%s' (normalized: '%s')\n", competition, normalizedComp) // Optional debug print

// 	// Check if the normalized competition name matches any normalized relevant league
// 	for _, league := range relevantLeagues {
// 		// fmt.Printf("DEBUG:   Against '%s'\n", league) // Optional debug print
// 		if normalizedComp == league {
// 			return true // Exact match found after normalization
// 		}
// 	}
// 	return false // No match found
// }

// func normalizeForComparison(s string) string {
// 	return strings.ToLower(strings.ReplaceAll(s, " ", ""))
// }

// normalizeCompetition standardizes competition names if needed.
// func normalizeCompetition(competition string) string {
// 	// Add your normalization rules here if necessary
// 	// e.g., replace long names with shorter ones
// 	return strings.TrimSpace(competition)
// }

// cleanText removes extra whitespace.
func cleanText(text string) string {
	return strings.Join(strings.Fields(text), " ") // Normalize whitespace
}

// scrapeLiveSoccerTV scrapes the schedule for a given date.
func scrapeLiveSoccerTV(dateStr string) ([]Match, error) {
	// Create a context with a timeout (e.g., 60 seconds)
	// opts := append(chromedp.DefaultExecAllocatorOptions[:],
	// 	chromedp.Flag("headless", false), // Uncomment for debugging
	// 	// chromedp.WindowSize(1920, 1080),   // Set window size if needed
	// )

	// allocCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	// defer cancel()

	// allocCtx, cancel = chromedp.NewExecAllocator(allocCtx, opts...)
	// defer cancel()

	// // Create a new browser context.
	// ctx, cancel := chromedp.NewContext(allocCtx)
	// defer cancel()

	// // Navigate to the page
	// url := fmt.Sprintf("https://www.livesoccertv.com/es/schedules/%s/", dateStr) // e.g., "2025-08-23"
	// fmt.Printf("Navigating to: %s\n", url)

	// var htmlContent string
	// err := chromedp.Run(ctx,
	// 	chromedp.Navigate(url),
	// 	chromedp.WaitVisible(`h1`, chromedp.ByQuery),
	// 	chromedp.WaitVisible(`div.r_livecomp`, chromedp.ByQuery), // Wait for competition names
	// 	chromedp.Sleep(10*time.Second),
	// 	chromedp.OuterHTML("html", &htmlContent, chromedp.ByQuery),
	// )
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to navigate and get HTML: %w", err)
	// }

	// fmt.Println("Page loaded successfully. Processing HTML with goquery...")

	// // --- Processing the HTML using goquery ---
	// doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to parse HTML with goquery: %w", err)
	// }
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // Uncomment for debugging
		// chromedp.WindowSize(1920, 1080),   // Set window size if needed
	)

	allocCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second) // Increased timeout
	defer cancel()

	allocCtx, cancel = chromedp.NewExecAllocator(allocCtx, opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	url := fmt.Sprintf("https://www.livesoccertv.com/es/schedules/%s/", dateStr)
	fmt.Printf("Navigating to: %s\n", url)

	// 1. Navigate and wait for basic structure
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`h1`, chromedp.ByQuery),             // Wait for main title
		chromedp.WaitVisible(`div.r_livecomp`, chromedp.ByQuery), // Wait for initial competition names
		// chromedp.Sleep(2*time.Second), // Optional small delay after initial load
	)
	if err != nil {
		return nil, fmt.Errorf("failed to navigate and wait for initial load: %w", err)
	}

	fmt.Println("Initial page loaded. Scrolling to load more content...")

	// --- 2. Scroll down the page to trigger lazy loading ---
	// We can scroll multiple times or scroll to the bottom.
	// Scrolling to the bottom is often effective.
	// We'll scroll in steps to give time for content to load.
	scrollSteps := 5               // Number of times to scroll
	scrollDelay := 1 * time.Second // Delay between scrolls

	for i := 0; i < scrollSteps; i++ {
		fmt.Printf("   -> Scroll step %d/%d\n", i+1, scrollSteps)
		err := chromedp.Run(ctx,
			// Scroll to the bottom of the page
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
			// Wait a bit for content to load
			chromedp.Sleep(scrollDelay),
		)
		if err != nil {
			log.Printf("Warning: Error during scroll step %d: %v", i+1, err)
			// Continue anyway
		}
	}

	// Optional: One final scroll to absolute bottom after a longer wait
	// err = chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
	// if err == nil {
	//     chromedp.Run(ctx, chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil))
	// }

	fmt.Println("Scrolling complete. Retrieving final HTML...")
	// --- 3. Get the final HTML after scrolling ---
	var htmlContent string
	err = chromedp.Run(ctx,
		chromedp.OuterHTML("html", &htmlContent, chromedp.ByQuery),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get final HTML after scrolling: %w", err)
	}

	// --- 4. Process the HTML as before ---
	fmt.Println("Page loaded successfully (after scrolling). Processing HTML with goquery...")

	// --- INSPECCIÓN DEPURACIÓN: Imprime todas las competiciones encontradas ---
	fmt.Println("--- INICIO Depuración Competencias (Después de Scroll) ---")
	tempDoc, _ := goquery.NewDocumentFromReader(strings.NewReader(htmlContent)) // Crea un doc temporal para esta depuración
	foundSpain := false
	tempDoc.Find("div.r_livecomp").Each(func(i int, compDiv *goquery.Selection) {
		competitionName := cleanText(compDiv.Find("span.r_compname").Text())
		parentDiv := compDiv.Parent()
		onclickAttr, _ := parentDiv.Attr("onclick")
		var compIdStr string
		parts := strings.Split(onclickAttr, "'")
		if len(parts) >= 2 {
			compIdStr = parts[1]
		}
		fmt.Printf("Comp %d: ID=%s, Nombre='%s'\n", i+1, compIdStr, competitionName)
		// Verifica específicamente si aparece España
		if strings.Contains(competitionName, "España") {
			fmt.Printf("  -> COMPETICIÓN DE ESPAÑA ENCONTRADA: %s\n", competitionName)
			foundSpain = true
		}
	})
	if !foundSpain {
		fmt.Println("  -> No se encontró ninguna competición de España después del scroll.")
	}
	fmt.Println("--- FIN Depuración Competencias (Después de Scroll) ---")
	// --- FIN INSPECCIÓN DEPURACIÓN ---

	var matches []Match
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML with goquery: %w", err)
	}

	// Find competition rows
	doc.Find("div.r_livecomp").Each(func(i int, compDiv *goquery.Selection) {
		// --- Extract Competition Name ---
		competitionName := cleanText(compDiv.Find("span.r_compname").Text())
		fmt.Printf("Found competition div %d: %s\n", i+1, competitionName) // Debug

		// --- Check if the competition is relevant ---
		// if !isRelevant(competitionName) {
		// 	// fmt.Printf("Skipping non-relevant competition: %s\n", competitionName) // Optional debug
		// 	return
		// }
		fmt.Printf(" -> Relevant competition found: %s\n", competitionName) // Debug print

		// --- Extract Competition ID from parent div's onclick attribute ---
		// The parent div should have the onclick attribute with the ID
		parentDiv := compDiv.Parent()
		if parentDiv.Length() == 0 {
			log.Printf("Warning: Could not find parent div for competition: %s\n", competitionName)
			return
		}
		onclickAttr, exists := parentDiv.Attr("onclick")
		if !exists {
			log.Printf("Warning: Parent div missing 'onclick' attribute for competition: %s\n", competitionName)
			return
		}
		// Parse ID from onclick: showMatches('ID',...
		var compIdStr string
		parts := strings.Split(onclickAttr, "'")
		if len(parts) >= 2 {
			compIdStr = parts[1]
		} else {
			log.Printf("Warning: Could not parse competition ID from onclick for %s: %s\n", competitionName, onclickAttr)
			return
		}

		// --- Find the corresponding matches container (span#cXXX) ---
		matchesContainerSelector := fmt.Sprintf("span#c%s", compIdStr)
		// fmt.Printf("Looking for matches in container: %s\n", matchesContainerSelector) // Debug print

		// --- Iterate through matches within this competition's container ---
		doc.Find(matchesContainerSelector + " tr.matchrow").Each(func(j int, matchRow *goquery.Selection) {
			fmt.Printf("  Processing match %d in %s\n", j+1, competitionName) // Debug print

			// --- Extract Match Details ---
			eventName := cleanText(matchRow.Find("td#match a").Text())
			// fmt.Printf("    Event Name: '%s'\n", eventName) // Debug print

			// --- Extract Time ---
			var time string
			timeCell := matchRow.Find("td.timecol div.meta span.livecell")
			if timeCell.Length() > 0 {
				if title, exists := timeCell.Attr("title"); exists && (strings.Contains(strings.ToLower(title), "vivo") || strings.Contains(strings.ToLower(title), "live")) {
					time = "EN VIVO"
				} else {
					time = cleanText(timeCell.Text())
				}
			} else {
				time = cleanText(matchRow.Find("td.timecol div.meta").First().Text())
			}
			// fmt.Printf("    Time: '%s'\n", time) // Debug print

			// --- Extract Broadcasters ---
			var broadcasters []string
			matchRow.Find("td#channels div.mchannels a").Each(func(k int, channelLink *goquery.Selection) {
				broadcaster := cleanText(channelLink.Text())
				if broadcaster != "" {
					broadcasters = append(broadcasters, broadcaster)
				}
			})
			fmt.Printf("    Broadcasters: %v\n", broadcasters) // Debug print

			// --- Add Match ---
			if eventName != "" {
				matches = append(matches, Match{
					Date:         dateStr,
					Competition:  normalizeCompetition(competitionName),
					Time:         time,
					Event:        eventName,
					Broadcasters: []BroadcasterInfo{},
				})
				// fmt.Printf("    -> Added match: %s vs %s\n", eventName, time) // Debug print
			} else {
				// fmt.Printf("    -> Skipped match (no event name)\n") // Debug print
			}
		})
	})

	return matches, nil
}

// normalizeCompetition standardizes competition names if needed.
func normalizeCompetition(competition string) string {
	// Add your normalization rules here if necessary
	// e.g., replace long names with shorter ones
	return strings.TrimSpace(competition)
}

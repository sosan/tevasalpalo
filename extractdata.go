package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/grafov/m3u8"
)

var filterList = []string{
	// "ESPN HD",
	// "ESPN 2 HD",
	// "ESPN 3 HD",
	// "ESPN 4 HD",
	// "ESPN 5 HD",
	// "ESPN 6 HD",
	// "ESPN 7 HD",
	"ESPN ARGENTINA",
	"DAZN",
}

const (
	shickatWeb = "https://shickat.me/"
	elcanoWeb  = "https://ipfs.io/ipns/elcano.top"
	listaplana = "https://ipfs.io/ipns/k2k4r8oqlcjxsritt5mczkcn4mmvcmymbqw7113fz2flkrerfwfps004/data/listas/listaplana.txt"
)

func FetchUpdatedList() error {
	var extractedData map[string][]string
	var err error
	for range 10 {
		body, err := FetchWebData(listaplana, false)
		if err != nil || len(body) == 0 {
			log.Print("cannot read lista plana")
			time.Sleep(1 * time.Second)
			continue
		}
		extractedData = extractDataFromWebTxtRaw(body)
		break
		// if err == nil {
		// 	// extractedData = extractDataFromWebShitkat(body)
		// }

	}
	broadcasterToAcestream = updateBroadcasterMapWithGateway(broadcasterToAcestream, extractedData)
	broadcasterToAcestream = transformUriSafeBroadcasters(broadcasterToAcestream)

	// body, err = FetchWebData(elcanoWeb)
	// if err != nil {
	// 	return err
	// }
	// extractedData, err = extractDataFromWebElCano(body)
	// if err != nil {
	// 	return err
	// }
	// broadcasterToAcestream = updateBroadcasterMapWithGateway(broadcasterToAcestream, extractedData)

	// body, err = FetchWebData("https://gist.githubusercontent.com/GUAPACHA/ff9cf6435b379c4c550913fdadc8edc4/raw/563943f091669fc21d96de72d0f9937a9b10221c/the%2520beatles")
	// body, err = FetchWebData("https://raw.githubusercontent.com/Icastresana/lista1/refs/heads/main/peticiones", true)
	// if err != nil {
	// 	return err
	// }

	// extractedData, err = extractDataFromM3U8(body, filterList)
	// if err != nil {
	// 	return err
	// }
	// broadcasterToAcestream = updateBroadcasterMapWithGateway(broadcasterToAcestream, extractedData)

	// programacion espn, skysports
	// 'https://api.acestream.me/all?api_version=1&api_key=test_api_key
	// check active link
	// broadcasterToAcestream = checkActiveLinks(broadcasterToAcestream)
	log.Print("Filtrando....")
	// transform uri links to base64 uri safe

	topCompetitions = transformCompetitionsToTop(allCompetitions)

	if err := preloadProgramationTVData(); err != nil {
		log.Printf("⚠️  Advertencia: No se pudieron pre-cargar datos: %v", err)
	}

	startTVProgramationDataRefresh()
	return err
}

func extractDataFromWebElCano(body []byte) (map[string][]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("error al parsear el documento HTML: %w", err)
	}

	// Buscar el bloque <script> que contiene el JSON
	var scriptContent string
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		scriptText := s.Text()
		if strings.Contains(scriptText, "const linksData") {
			scriptContent = scriptText
			return
		}
	})

	if scriptContent == "" {
		return nil, fmt.Errorf("no se encontró el bloque <script> con el JSON")
	}

	splitted := strings.Split(scriptContent, "\n        const linksData =")
	faseA := strings.Split(splitted[1], "const linksList = document.getElementById('linksList');")
	faseB := strings.Split(faseA[0], ";")
	faseC := strings.ReplaceAll(faseB[0], "acestream://", "")
	jsonStr := faseC

	// Parsear el JSON extraído
	var linksData struct {
		Links []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"links"`
	}
	err = json.Unmarshal([]byte(jsonStr), &linksData)
	if err != nil {
		return nil, fmt.Errorf("error al parsear el JSON: %w", err)
	}

	replacer := strings.NewReplacer(
		"720", "",
		"1080P", "",
		"1080", "",
		"(Fórmula 1)", "",
		" ", "",
	)

	// Construir el mapa resultante
	extractedData := make(map[string][]string)
	for _, link := range linksData.Links {
		if link.URL != "" {
			if strings.Contains(strings.ToUpper(link.Name), "UHD") || strings.Contains(strings.ToUpper(link.Name), "MULTIAUDIO") {
				continue
			}
			name := strings.TrimSpace(replacer.Replace(link.Name))

			// name := link.Name
			// name = strings.ReplaceAll(name, "720", "")
			// name = strings.ReplaceAll(name, "1080P", "")
			// name = strings.ReplaceAll(name, "1080", "")
			// name = strings.ReplaceAll(name, "(Fórmula 1)", "")
			// name = strings.ReplaceAll(name, " ", "")

			if name == "Dedporte2" {
				name = "Deporte2"
			}
			extractedData[name] = append(extractedData[name], link.URL)
		}
	}

	return extractedData, nil
}

func extractDataFromWebShitkat(body []byte) map[string][]string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	extractedData := make(map[string][]string)
	doc.Find(".canal-card").Each(func(i int, card *goquery.Selection) {
		nombre := card.Find(".canal-nombre").Text()
		acestreamLink := strings.TrimSpace(card.Find(".acestream-link").Text()) //.AttrOr("href", "")
		extractedData[nombre] = append(extractedData[nombre], acestreamLink)
	})
	return extractedData
}

func extractDataFromWebTxtRaw(body []byte) map[string][]string {
	extractedData := make(map[string][]string)
	lines := strings.Split(string(body), "\n")
	for i := 0; i < len(lines); i += 2 {
		if i+1 >= len(lines) {
			break
		}
		
		nombre := normalizeChannelName(lines[i])
		if nombre == "ACB EVENTO 01" {
			nombre = "DAZN BALONCESTO 1"
		}
		if nombre == "ACB EVENTO 02" {
			nombre = "DAZN BALONCESTO 2"
		}
		acestreamLink := strings.TrimSpace(lines[i+1])

		if nombre == "" || acestreamLink == "" {
			continue
		}

		extractedData[nombre] = append(extractedData[nombre], acestreamLink)
	}

	return extractedData
}

var (
	reArrow      = regexp.MustCompile(`\s*-->.*$`)
	reParens     = regexp.MustCompile(`\([^)]*\)`)
	reBrackets   = regexp.MustCompile(`\[[^]]*\]`)
	reStars      = regexp.MustCompile(`\*+`)
	reQuality    = regexp.MustCompile(`(?i)\b(4K|UHD|FHDp|FHD|HDp|HD|SDp|SD|720p|1080p|2160p)\b`)
	reMultiSpace = regexp.MustCompile(`\s+`)
)

// NormalizeChannelName limpia y normaliza el nombre del canal
func normalizeChannelName(input string) string {

	s := input

	// 1. eliminar todo después de -->
	s = reArrow.ReplaceAllString(s, "")

	// 2. eliminar (...) y [...]
	s = reParens.ReplaceAllString(s, "")
	s = reBrackets.ReplaceAllString(s, "")

	// 3. eliminar *
	s = reStars.ReplaceAllString(s, "")

	// 4. eliminar etiquetas de calidad
	s = reQuality.ReplaceAllString(s, "")

	// 5. trim
	s = strings.TrimSpace(s)

	// 6. normalizar espacios
	s = reMultiSpace.ReplaceAllString(s, " ")

	return s
}

func transformUriSafeBroadcasters(broadcasterToAcestream map[string]BroadcasterInfo) map[string]BroadcasterInfo {
	redirectClient := IinitializeRedirectClients()
	for key := range broadcasterToAcestream {
		for i := 0; i < len(broadcasterToAcestream[key].Links); i++ {
			if strings.Contains(broadcasterToAcestream[key].Links[i], "p;") {
				initialUri := strings.Split(broadcasterToAcestream[key].Links[i], "p;")[1]
				finalURL, _, _, _ := resolveFinalManifestURL(initialUri, redirectClient)
				// fmt.Println(string(manifestContent) + "...")
				// fmt.Printf("%v", headers)
				// fmt.Println(string(finalURL) + "...")
				broadcasterToAcestream[key].Links[i] = finalURL
			}
			if strings.Contains(broadcasterToAcestream[key].Links[i], ":") {
				encoded := changeLinkToUriSafe(broadcasterToAcestream[key].Links[i])
				broadcasterToAcestream[key].Links[i] = fmt.Sprintf(";%s", encoded)
			}
		}
	}
	StopRedirectClient(redirectClient)
	return broadcasterToAcestream
}

func changeLinkToUriSafe(url string) string {
	encodedRaw := base64.RawURLEncoding.EncodeToString([]byte(url))
	return encodedRaw
}

func extractDataFromM3U8(body []byte, filterList []string) (map[string][]string, error) {
	p, listType, err := m3u8.Decode(*bytes.NewBuffer(body), false)
	if err != nil {
		return nil, err
	}
	var mediapl *m3u8.MediaPlaylist
	// var masterpl *m3u8.MasterPlaylist
	switch listType {
	case m3u8.MEDIA:
		mediapl = p.(*m3u8.MediaPlaylist)
		fmt.Printf("%+v\n", mediapl)
	case m3u8.MASTER:
		return nil, fmt.Errorf("Not playlist")
		// masterpl = p.(*m3u8.MasterPlaylist)
		// fmt.Printf("%+v\n", masterpl)
	}
	extractedData := make(map[string][]string)
	for i := 0; i < len(mediapl.Segments); i++ {
		if mediapl.Segments[i] == nil {
			continue
		}
		name := mediapl.Segments[i].Title
		link := mediapl.Segments[i].URI
		extractedData[name] = append(extractedData[name], link)
	}
	return extractedData, nil
}

func resolveFinalManifestURL(initialURL string, redirectClient *http.Client) (finalURL string, finalHeaders http.Header, manifestBody []byte, err error) {
	return fetchWithRedirects(initialURL, redirectClient)
}

func checkActiveLinks(broadcasters map[string]BroadcasterInfo) map[string]BroadcasterInfo {
	log.Printf(" 🔍 Comprobando enlaces activos...")
	for key := range broadcasters {
		log.Printf(" 🔍 Comprobando %s...", key)
		for i := len(broadcasters[key].Links) - 1; i >= 0; i-- {
			if strings.Contains(broadcasters[key].Links[i], ";") {
				// es un enlace codificado, no se puede comprobar
				log.Printf("Link codificado, no se puede comprobar: %s - %s", key, broadcasters[key].Links[i])
				continue
			}

			boolean, err := checkActiveLink(broadcasters[key].Links[i])
			if err != nil || !boolean {
				log.Printf("Link no activo: %s - %s", key, broadcasters[key].Links[i])
				currentBroadcaster := broadcasters[key]
				currentBroadcaster.Links = append(currentBroadcaster.Links[:i], currentBroadcaster.Links[i+1:]...)
				broadcasters[key] = currentBroadcaster
			} else {
				log.Printf("Link activo: %s - %s", key, broadcasters[key].Links[i])
			}
		}
	}
	return broadcasters
}

func checkActiveLink(initialURL string) (bool, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return http.ErrUseLastResponse
		},
		Timeout: timeTimeout,
	}

	currentURL := initialURL
	redirectCount := 0

	for {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return false, err
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Connection", "keep-alive")

		resp, err := client.Do(req)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			redirectCount++
			if redirectCount > 10 {
				return false, err
			}

			location := resp.Header.Get("Location")
			if location == "" {
				return false, err
			}
			currentURL = location
			continue
		}

		if resp.StatusCode >= 400 {
			return false, nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}
		log.Printf("Manifest body: %s", string(body))
		return true, err
	}

}

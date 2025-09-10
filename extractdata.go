package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/grafov/m3u8"
)

var filterList = []string {
	"ESPN HD",
	"ESPN 2 HD",
	"ESPN 3 HD",
	"ESPN 4 HD",
	"ESPN 5 HD",
	"ESPN 6 HD",
	"ESPN 7 HD",

}

func fetchUpdatedList() error {
	body, err := FetchWebData("https://shickat.me/")
	if err != nil {
		return err
	}
	extractedData := extractDataFromWebShitkat(body)
	broadcasterToAcestream = updateBroadcasterMapWithGateway(broadcasterToAcestream, extractedData)

	body, err = FetchWebData("https://ipfs.io/ipns/elcano.top")
	if err != nil {
		return err
	}
	extractedData, err = extractDataFromWebElCano(body)
	if err != nil {
		return err
	}
	broadcasterToAcestream = updateBroadcasterMapWithGateway(broadcasterToAcestream, extractedData)

	// body, err = FetchWebData("https://gist.githubusercontent.com/GUAPACHA/ff9cf6435b379c4c550913fdadc8edc4/raw/563943f091669fc21d96de72d0f9937a9b10221c/the%2520beatles")
	// if err != nil {
	// 	return err
	// }

	// extractedData, err = extractDataFromM3U8(body, filterList)
	// if err != nil {
	// 	return err
	// }
	// broadcasterToAcestream = updateBroadcasterMapWithGateway(broadcasterToAcestream, extractedData)

	// transform uri links to base64 uri safe
	broadcasterToAcestream = transformUriSafeBroadcasters(broadcasterToAcestream)
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
			extractedData[name] = append(extractedData[name], link.URL )
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

func transformUriSafeBroadcasters(broadcasterToAcestream map[string]BroadcasterInfo) map[string]BroadcasterInfo {
	for key := range broadcasterToAcestream {
		for i := 0; i < len(broadcasterToAcestream[key].Links); i++ {
			if strings.Contains(broadcasterToAcestream[key].Links[i], ":") {
				encoded := changeLinkToUriSafe(broadcasterToAcestream[key].Links[i])
				broadcasterToAcestream[key].Links[i] = fmt.Sprintf(";%s", encoded)
			}
		}
	}
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
	var mediapl  *m3u8.MediaPlaylist
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
		name := mediapl.Segments[i].Title
		link := mediapl.Segments[i].URI
		extractedData[name] = append(extractedData[name], link )
	}
	return extractedData, nil
}
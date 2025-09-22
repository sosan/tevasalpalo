package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/grafov/m3u8"
)

var filterList = []string{
	"ESPN HD",
	"ESPN 2 HD",
	"ESPN 3 HD",
	"ESPN 4 HD",
	"ESPN 5 HD",
	"ESPN 6 HD",
	"ESPN 7 HD",
}

const (
	shickatWeb = "https://shickat.me/"
	elcanoWeb = "https://ipfs.io/ipns/elcano.top"
) 

func FetchUpdatedList() error {
	body, err := FetchWebData(shickatWeb)
	if err != nil {
		return err
	}
	extractedData := extractDataFromWebShitkat(body)
	broadcasterToAcestream = updateBroadcasterMapWithGateway(broadcasterToAcestream, extractedData)

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
	// if err != nil {
	// 	return err
	// }

	// extractedData, err = extractDataFromM3U8(body, filterList)
	// if err != nil {
	// 	return err
	// }
	// broadcasterToAcestream = updateBroadcasterMapWithGateway(broadcasterToAcestream, extractedData)

	// programacion espn, skysports
	

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

func transformUriSafeBroadcasters(broadcasterToAcestream map[string]BroadcasterInfo) map[string]BroadcasterInfo {
	for key := range broadcasterToAcestream {
		for i := 0; i < len(broadcasterToAcestream[key].Links); i++ {
			if strings.Contains(broadcasterToAcestream[key].Links[i], "p;") {
				initialUri := strings.Split(broadcasterToAcestream[key].Links[i], "p;")[1]
				finalURL, _, _, _ := resolveFinalManifestURL(initialUri)
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
		name := mediapl.Segments[i].Title
		link := mediapl.Segments[i].URI
		extractedData[name] = append(extractedData[name], link)
	}
	return extractedData, nil
}

func extractDataForDazn() {
	
}

func resolveFinalManifestURL(initialURL string) (finalURL string, finalHeaders http.Header, manifestBody []byte, err error) {
	client := &http.Client{
		// No seguir redirecciones automáticamente con el cliente principal
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Detenerse en 10 redirecciones para evitar bucles infinitos
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			// Indicar que no siga la redirección automáticamente
			return http.ErrUseLastResponse
		},
		Timeout: 30 * time.Second, // Timeout razonable para la resolución inicial
	}

	// Empezamos con la URL inicial
	currentURL := initialURL
	redirectCount := 0

	// Bucle para seguir redirecciones manualmente
	for {
		// Crear una nueva solicitud
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return "", nil, nil, fmt.Errorf("failed to create request for %s: %w", currentURL, err)
		}

		// Agregar headers típicos para solicitudes de manifiesto
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Connection", "keep-alive")

		// Hacer la solicitud
		resp, err := client.Do(req)
		if err != nil {
			return "", nil, nil, fmt.Errorf("failed to fetch %s: %w", currentURL, err)
		}

		// Es crucial cerrar el cuerpo, incluso si luego lo leemos.
		// Usamos defer para asegurarnos de que se cierre siempre al salir de la función o el bucle.
		defer resp.Body.Close() 

		// Verificar si es una redirección
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			redirectCount++
			if redirectCount > 10 {
				return "", nil, nil, fmt.Errorf("too many redirects (>10)")
			}

			location := resp.Header.Get("Location")
			if location == "" {
				return "", nil, nil, fmt.Errorf("redirect status %d received but no Location header found for URL %s", resp.StatusCode, currentURL)
			}
			// Actualizar la URL actual para la próxima iteración
			currentURL = location
			fmt.Printf("Redirect #%d: %s -> %s\n", redirectCount, req.URL.String(), location)
			// Continuar con el bucle para seguir la próxima redirección
			continue
		}

		// Si llegamos aquí, no es una redirección (o es un error 4xx/5xx, que manejamos como final)
		// Leer el cuerpo del manifiesto
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", nil, nil, fmt.Errorf("failed to read manifest body from %s: %w", currentURL, err)
		}

		// Devolver la URL final, los headers y el cuerpo
		// Nota: resp.Request.URL contiene la URL de la solicitud *real* que se hizo
		// (la última antes de obtener una respuesta no-redirect).
		// Esta es la URL final resuelta.
		return resp.Request.URL.String(), resp.Header, body, nil
	}
}
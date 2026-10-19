// Resolución de enlaces p; de broadcasters, cache de hosts muertos, prueba de
// enlaces activos y transformación a URI-safe.
package source

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"encoding/base64"
	"github.com/grafov/m3u8"
	"io"
	"tevasalpalo/internal/transport"
)

type hostMuerto struct {
	mu    sync.Mutex
	hosts map[string]bool
}

func newHostMuerto() *hostMuerto {
	return &hostMuerto{hosts: make(map[string]bool)}
}

func (h *hostMuerto) markDead(host string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hosts[host] = true
}

func (h *hostMuerto) isDead(host string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hosts[host]
}

// pLink es un enlace "p;" pendiente de resolver: el broadcaster al que
// pertenece y la URL inicial.
type pLink struct {
	broacasterKey string
	linkIndex     int
	initialURI    string
}

// transformUriSafeBroadcasters resuelve los enlaces "p;" y codifica todos los
// enlaces en base64 URI-safe.
//
// La resolución va en paralelo (pLinkWorkers) y con un timeout menor que el
// global, porque aquí no hace falta el manifiesto entero: solo la URL final
// tras las redirecciones. Además, cuando un host falla se marca como muerto
// para el resto de la pasada, así que N enlaces del mismo servidor muerto
// cuestan un solo timeout en vez de N.
func transformUriSafeBroadcasters(BroadcasterToAcestream map[string]BroadcasterInfo) map[string]BroadcasterInfo {
	isDev := os.Getenv("ENV") == "dev"

	// Fase 1: recoger los enlaces "p;" sin tocar la red.
	type broadcasterJob struct {
		key      string
		original []string
		indices  []int
		initial  []string
	}
	var jobs []broadcasterJob
	for key, info := range BroadcasterToAcestream {
		job := broadcasterJob{key: key}
		for i, link := range info.Links {
			if strings.TrimSpace(link) == "" {
				continue
			}
			if strings.Contains(link, "p;") {
				job.indices = append(job.indices, i)
				job.initial = append(job.initial, strings.Split(link, "p;")[1])
			}
		}
		job.original = info.Links
		jobs = append(jobs, job)
	}

	// Fase 2: resolver en paralelo.
	resolved := map[pLink]string{}
	var pending []pLink
	for _, job := range jobs {
		for idx, initial := range job.initial {
			pending = append(pending, pLink{broacasterKey: job.key, linkIndex: job.indices[idx], initialURI: initial})
		}
	}
	if len(pending) > 0 {
		muertos := newHostMuerto()
		var muResolved sync.Mutex
		sem := make(chan struct{}, pLinkWorkers)
		var wg sync.WaitGroup
		for _, p := range pending {
			wg.Add(1)
			sem <- struct{}{}
			go func(p pLink) {
				defer wg.Done()
				defer func() { <-sem }()

				host := hostDeURL(p.initialURI)
				if host != "" && muertos.isDead(host) {
					if isDev {
						log.Printf("⏭️  [%s] host ya caído, se mantiene p; original %s", p.broacasterKey, p.initialURI)
					}
					return
				}

				client := transport.IinitializeRedirectClients()
				finalURL, _, _, err := resolveFinalManifestURL(p.initialURI, client)
				transport.StopRedirectClient(client)

				if err != nil || strings.TrimSpace(finalURL) == "" {
					if host != "" {
						muertos.markDead(host)
					}
					log.Printf("⚠️  [%s] no se pudo resolver p; link %s (err=%v) - manteniendo original", p.broacasterKey, p.initialURI, err)
					return
				}
				muResolved.Lock()
				resolved[p] = finalURL
				muResolved.Unlock()
			}(p)
		}
		wg.Wait()
	}

	// Fase 3: ensamblar los enlaces finales por broadcaster.
	for _, job := range jobs {
		newLinks := make([]string, 0, len(job.original))
		for i, link := range job.original {
			if strings.TrimSpace(link) == "" {
				continue
			}
			if strings.Contains(link, "p;") {
				initialURI := strings.Split(link, "p;")[1]
				if finalURL, ok := resolved[pLink{broacasterKey: job.key, linkIndex: i, initialURI: initialURI}]; ok && strings.TrimSpace(finalURL) != "" {
					link = finalURL
				} else {
					link = initialURI
				}
			}
			if strings.TrimSpace(link) == "" {
				continue
			}
			if strings.Contains(link, ":") {
				encoded := changeLinkToUriSafe(link)
				link = fmt.Sprintf(";%s", encoded)
			}
			if strings.TrimSpace(link) != "" {
				newLinks = append(newLinks, link)
			}
		}
		info := BroadcasterToAcestream[job.key]
		info.Links = removeDuplicates(newLinks)
		BroadcasterToAcestream[job.key] = info
		// Solo avisar si tenía links y se quedó sin ninguno tras transformar (evita ruido de broadcasters que ya nacen vacíos como HYPERMOTION alias o DAZN LALIGA 3 recién creado)
		if isDev && len(job.original) > 0 && len(newLinks) == 0 {
			log.Printf("⚠️  [%s] quedó sin links válidos tras transformUriSafe (tenía %d, ahora 0)", job.key, len(job.original))
		}
	}
	return BroadcasterToAcestream
}

// pLinkWorkers: nº de resoluciones "p;" simultáneas. Acota los conexiones a
// los servidores directos sin arrastrar una lluvia de peticiones.
const pLinkWorkers = 8

// hostDeURL extrae host:port de una URL para la cache de hosts caídos.
func hostDeURL(u string) string {
	parsed, err := url.Parse(strings.TrimSpace(u))
	if err != nil {
		return ""
	}
	return parsed.Host
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
	return transport.FetchWithRedirects(initialURL, redirectClient)
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
		Timeout: transport.TimeTimeout,
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

// fusionarListados une dos mapas de canal -> hashes juntando los enlaces y
// quitando duplicados. Se usa para tener el listado completo de todas las
// fuentes, por encima del filtrado por gateway.

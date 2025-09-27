package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

type CompetitionRequest struct {
	URL     string
	Proxied bool
	Name    string
}

const (
	timeTimeout = 20 * time.Second
)


// FetchWebData se encarga únicamente de obtener los datos en bruto de la URL especificada.
// Devuelve el cuerpo de la respuesta como un slice de bytes o un error si ocurre.
func FetchWebData(url string, proxied bool) ([]byte, error) {
	var err error
	client := &http.Client{
		Timeout: timeTimeout,
	}

	if proxied {
		client, err = createSOCKS5Client()
		if err != nil {
			client, err = createSOCKS5Client()
			if err != nil {
				return nil, fmt.Errorf("error al crear el cliente SOCKS5: %w", err)
			}
		}
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error al crear la solicitud: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.8")
	req.Header.Set("Referer", "https://www.google.com/")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error al realizar la solicitud HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error al leer el cuerpo de la respuesta: %w", err)
	}

	return body, nil
}

func createSOCKS5Client() (*http.Client, error) {
	listSocksAddr, err := getSockList()
	if err != nil {
		return nil, err
	}

	for _, currentAddr := range listSocksAddr {
		if !testSOCKS5Proxy(currentAddr) {
			continue
		}

		sockURL, err := url.Parse("socks5://" + currentAddr)
		if err != nil {
			continue
		}
		dialer, err := proxy.FromURL(sockURL, proxy.Direct)
		if err != nil {
			continue
		}
		return &http.Client{
			Transport: &http.Transport{
				Dial: dialer.Dial,
			},
			Timeout: timeTimeout,
		}, nil
	}
	return nil, fmt.Errorf("no se pudo crear un cliente SOCKS5 válido")
}

func getSockList() ([]string, error) {
	proxyListURL := "https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/protocols/socks5/data.txt"
	body, err := FetchWebData(proxyListURL, false)
	strBody := strings.ReplaceAll(string(body), "socks5://", "")
	proxiesNotChecked := strings.Split(strBody, "\n")

	if len(proxiesNotChecked) == 0 {
		fmt.Println("No se encontraron proxies en la lista.")
		return nil, fmt.Errorf("error al obtener la lista de proxies: %w", err)
	}

	fmt.Printf("Se encontraron %d proxies. Probando...\n", len(proxiesNotChecked))
	return proxiesNotChecked, nil
	// var proxies []string
	// for _, proxyAddr := range proxiesNotChecked {
	// 	proxies = append(proxies, proxyAddr)
	// 	// if testSOCKS5Proxy(proxyAddr) {
	// 	// 	// return proxyAddr, nil
	// 	// }
	// }

	// return proxies, nil
}

// func testSOCKS5Proxy(proxyAddr string) bool {
// 	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
// 	if err != nil {
// 		return false
// 	}

// 	// ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 	// defer cancel()
	
// 	conn, err := dialer.Dial("tcp", "httpbin.org:80")
// 	if err != nil {
// 		return false
// 	}
// 	conn.Close()
// 	return true
// }

func testSOCKS5Proxy(proxyAddr string) bool {
    dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
    if err != nil {
        return false
    }

    result := make(chan bool, 1)
    go func() {
        conn, err := dialer.Dial("tcp", "httpbin.org:80")
        if err != nil {
            result <- false
            return
        }
        defer conn.Close()

        request := "GET / HTTP/1.0\r\nHost: httpbin.org\r\n\r\n"
        _, err = conn.Write([]byte(request))
        if err != nil {
            result <- false
            return
        }

        response := make([]byte, 100) // Leer solo los primeros 100 bytes
        _, err = conn.Read(response)
        if err != nil {
            result <- false
            return
        }
        // fmt.Printf("Proxy %s responded with: %s\n", proxyAddr, string(response[:n])) // Opcional: para depuración

        result <- true
    }()

    select {
    case success := <-result:
        return success
    case <-time.After(timeTimeout): // Usar el timeout aquí también
        return false
    }
}

func FetchCompetitionsParallel(
	requests []CompetitionRequest,
	getFunc func(url string, proxied bool) ([]DayView, error),
) map[string][]DayView {
	results := make(map[string][]DayView)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, req := range requests {
		wg.Add(1)
		go func(req CompetitionRequest) {
			defer wg.Done()
			events, err := getFunc(req.URL, req.Proxied)
			if err != nil {
				log.Printf("❌ Error en %s: %v", req.Name, err)
				return
			}
			mu.Lock()
			results[req.Name] = events
			mu.Unlock()
		}(req)
	}

	wg.Wait()
	return results
}
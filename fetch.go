package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// FetchWebData se encarga únicamente de obtener los datos en bruto de la URL especificada.
// Devuelve el cuerpo de la respuesta como un slice de bytes o un error si ocurre.
func FetchWebData(url string, proxied bool) ([]byte, error) {
	var err error
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	if proxied {
		client, err = createSOCKS5Client()
		if err != nil {
			log.Printf("error al crear el cliente SOCKS5: %v", err)
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
		dialer, err := proxy.FromURL(sockURL, proxy.Direct)
		if err != nil {
			return nil, err
		}
		return &http.Client{
			Transport: &http.Transport{
				Dial: dialer.Dial,
			},
			Timeout: 10 * time.Second,
		}, nil
	}
	return nil, fmt.Errorf("no se pudo crear un cliente SOCKS5 válido")
}

func getSockList() ([]string, error) {
	proxyListURL := "https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/protocols/socks5/data.txt"
	body, err := FetchWebData(proxyListURL, false)
	// resp, err := http.Get(proxyListURL)
	// if err != nil {
	// 	return nil, fmt.Errorf("error al obtener la lista de proxies: %w", err)
	// }
	// defer resp.Body.Close()

	// Leer los proxiesNotChecked
	// var proxiesNotChecked []string
	strBody := strings.ReplaceAll(string(body), "socks5://", "")
	proxiesNotChecked := strings.Split(strBody, "\n")
	// for string(body) != "" {
	// 	line := strings.TrimSpace(scanner.Text())
	// 	if line != "" {
	// 		proxies = append(proxies, line)
	// 	}
	// }

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
	// var conn net.Conn
	go func() {
		conn, err := dialer.Dial("tcp", "httpbin.org:80")
		if err != nil {
			result <- false
			return
		}
		defer conn.Close()
		result <- true
	}()

	select {
	case success := <-result:
		return success
	case <-time.After(10 * time.Second):
		return false
	}
}
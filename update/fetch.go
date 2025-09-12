package update

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func GetRequestRaw(uri string, token string) (*io.ReadCloser, int) {
	clientHttp := http.Client{Timeout: time.Duration(60) * time.Second}

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Add("Accept", `application/json`)
	if token != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	}
	req.Header.Add("Content-type", "application/json")

	resp, err := clientHttp.Do(req)

	if err != nil {
		log.Fatal(err)
		return nil, 404
	}
	// defer resp.Body.Close()

	return &resp.Body, resp.StatusCode
}


func GetRequest(uri string, token string) ([]byte, int) {

	clientHttp := http.Client{Timeout: time.Duration(60) * time.Second}

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Add("Accept", `application/json`)
	if token != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	}
	req.Header.Add("Content-type", "application/json")

	resp, err := clientHttp.Do(req)

	if err != nil {
		log.Fatal(err)
		return nil, 404
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error %s", err)
		return nil, 404
	}

	return body, resp.StatusCode

}
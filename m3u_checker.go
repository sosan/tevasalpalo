//go:build ignore

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	reHash40    = regexp.MustCompile(`(?i)\b[a-f0-9]{40}\b`)
	reAcestream = regexp.MustCompile(`(?i)acestream://([a-f0-9]{40})`)
)

type Link struct {
	File string
	Line int
	Raw  string
	Type string
	URL  string
	Name string
}

type Result struct {
	Link     Link
	OK       bool
	Status   int
	Err      string
	Duration time.Duration
}

func parseFile(path string) ([]Link, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var links []Link
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	lineNum := 0
	var pendingName string
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF") {
			if idx := strings.LastIndex(line, ","); idx != -1 {
				pendingName = strings.TrimSpace(line[idx+1:])
			} else {
				pendingName = line
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if m := reAcestream.FindStringSubmatch(line); m != nil {
			hash := strings.ToLower(m[1])
			links = append(links, Link{File: path, Line: lineNum, Raw: line, Type: "ace_hash", URL: fmt.Sprintf("http://127.0.0.1:6878/ace/manifest.m3u8?id=%s", hash), Name: pendingName})
			pendingName = ""
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "http") {
			if strings.Contains(line, "/ace/getstream") || strings.Contains(line, "/ace/manifest") {
				if m := reHash40.FindString(line); m != "" {
					hash := strings.ToLower(m)
					u := fmt.Sprintf("http://127.0.0.1:6878/ace/manifest.m3u8?id=%s", hash)
					links = append(links, Link{File: path, Line: lineNum, Raw: line, Type: "ace_url", URL: u, Name: pendingName})
				} else {
					links = append(links, Link{File: path, Line: lineNum, Raw: line, Type: "http", URL: line, Name: pendingName})
				}
			} else {
				clean := strings.Trim(line, `",`)
				links = append(links, Link{File: path, Line: lineNum, Raw: line, Type: "http", URL: clean, Name: pendingName})
			}
			pendingName = ""
			continue
		}
		if reHash40.MatchString(line) {
			m := reHash40.FindString(line)
			hash := strings.ToLower(m)
			links = append(links, Link{File: path, Line: lineNum, Raw: line, Type: "ace_hash", URL: fmt.Sprintf("http://127.0.0.1:6878/ace/manifest.m3u8?id=%s", hash), Name: pendingName})
			pendingName = ""
			continue
		}
		pendingName = line
	}
	return links, scanner.Err()
}

func checkLink(client *http.Client, l Link) Result {
	start := time.Now()
	req, err := http.NewRequest("GET", l.URL, nil)
	if err != nil {
		return Result{Link: l, OK: false, Err: err.Error(), Duration: time.Since(start)}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Checker)")
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return Result{Link: l, OK: false, Err: err.Error(), Duration: time.Since(start)}
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 1024)
	dur := time.Since(start)
	if resp.StatusCode == 200 || resp.StatusCode == 206 || resp.StatusCode == 302 || resp.StatusCode == 301 {
		return Result{Link: l, OK: true, Status: resp.StatusCode, Duration: dur}
	}
	if resp.StatusCode >= 400 {
		return Result{Link: l, OK: false, Status: resp.StatusCode, Err: fmt.Sprintf("HTTP %d", resp.StatusCode), Duration: dur}
	}
	return Result{Link: l, OK: true, Status: resp.StatusCode, Duration: dur}
}

func main() {
	dir := flag.String("dir", "/mnt/1E341780341759DB/javascript_proyectos/tele/m3u8", "carpeta con m3u/m3u8")
	workers := flag.Int("workers", 20, "concurrencia")
	timeout := flag.Duration("timeout", 8*time.Second, "timeout por link")
	aceCheck := flag.Bool("ace", false, "comprobar hashes ace via engine 127.0.0.1:6878")
	aceOnly := flag.Bool("ace-only", false, "solo acestream (implica -ace)")
	httpOnly := flag.Bool("http-only", false, "solo http")
	outOK := flag.String("out", "", "prefijo para archivos filtrados OK")
	format := flag.String("format", "m3u", "formato salida: m3u o txt (txt = broadcaster\\nace_stream)")
	combined := flag.Bool("combined", false, "si -out, genera un solo archivo combinado")
	flag.Parse()

	if *aceOnly {
		*aceCheck = true
	}
	// http-only y ace-only son excluyentes
	if *aceOnly && *httpOnly {
		log.Fatal("usa solo uno: -http-only o -ace-only, no ambos")
	}

	absDir, _ := filepath.Abs(*dir)
	fmt.Printf("📂 Escaneando %s\n", absDir)
	fmt.Printf("⚙️  workers=%d timeout=%s aceCheck=%v aceOnly=%v httpOnly=%v format=%s\n", *workers, *timeout, *aceCheck, *aceOnly, *httpOnly, *format)

	var allFiles []string
	filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".m3u" || ext == ".m3u8" {
			allFiles = append(allFiles, path)
		}
		return nil
	})
	fmt.Printf("📄 Encontrados %d archivos\n", len(allFiles))
	var allLinks []Link
	for _, f := range allFiles {
		links, err := parseFile(f)
		if err != nil {
			log.Printf("WARN %s: %v", f, err)
			continue
		}
		allLinks = append(allLinks, links...)
	}
	fmt.Printf("🔗 Total links extraídos: %d\n", len(allLinks))
	var toCheck []Link
	for _, l := range allLinks {
		isAce := strings.HasPrefix(l.Type, "ace")
		if *aceOnly {
			if !isAce {
				continue
			}
		} else if *httpOnly {
			if isAce {
				continue
			}
		} else if !*aceCheck {
			if isAce {
				continue
			}
		}
		toCheck = append(toCheck, l)
	}
	if !*aceCheck && !*aceOnly && !*httpOnly {
		fmt.Printf("⏭️  Saltando %d ace (usa -ace o -ace-only para comprobarlos)\n", len(allLinks)-len(toCheck))
	}
	if *aceOnly {
		fmt.Printf("🎯 Modo ace-only: solo %d ace a comprobar\n", len(toCheck))
	}
	fmt.Printf("✅ A comprobar: %d\n", len(toCheck))
	if len(toCheck) == 0 {
		byFile := make(map[string]int)
		for _, l := range allLinks {
			byFile[filepath.Base(l.File)]++
		}
		fmt.Println("\nResumen por archivo (total incluye ace):")
		for f, c := range byFile {
			fmt.Printf("  %s: %d\n", f, c)
		}
		return
	}
	client := &http.Client{
		Timeout: *timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
		Transport: &http.Transport{MaxIdleConns: 100, IdleConnTimeout: 30 * time.Second},
	}
	jobs := make(chan Link)
	results := make(chan Result)
	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for l := range jobs {
				results <- checkLink(client, l)
			}
		}()
	}
	go func() {
		for _, l := range toCheck {
			jobs <- l
		}
		close(jobs)
	}()
	go func() { wg.Wait(); close(results) }()

	var okCount, failCount int
	byFileOK := make(map[string][]Result)
	byFileFail := make(map[string][]Result)
	var allOK []Result
	for r := range results {
		base := filepath.Base(r.Link.File)
		if r.OK {
			okCount++
			byFileOK[base] = append(byFileOK[base], r)
			allOK = append(allOK, r)
			fmt.Printf("✅ OK %d %s [%s] %s -> %s (%v)\n", r.Status, base, r.Link.Type, r.Link.Name, r.Link.URL, r.Duration.Round(time.Millisecond))
		} else {
			failCount++
			byFileFail[base] = append(byFileFail[base], r)
			fmt.Printf("❌ FAIL %s [%s] %s -> %s err:%s (%v)\n", base, r.Link.Type, r.Link.Name, r.Link.URL, r.Err, r.Duration.Round(time.Millisecond))
		}
	}
	fmt.Printf("\n📊 OK=%d FAIL=%d Total=%d\n", okCount, failCount, okCount+failCount)
	fmt.Println("\nPor archivo:")
	for _, f := range allFiles {
		base := filepath.Base(f)
		ok := len(byFileOK[base])
		fail := len(byFileFail[base])
		total := 0
		for _, l := range allLinks {
			if filepath.Base(l.File) == base {
				isAce := strings.HasPrefix(l.Type, "ace")
				if *aceOnly && !isAce {
					continue
				}
				if *httpOnly && isAce {
					continue
				}
				if !*aceCheck && !*aceOnly && isAce {
					continue
				}
				total++
			}
		}
		if total > 0 {
			fmt.Printf("  %s: OK %d / FAIL %d / TOTAL %d\n", base, ok, fail, total)
		}
	}
	if *outOK != "" {
		if *combined {
			outPath := filepath.Join(absDir, *outOK+"_combined."+*format)
			if *format == "txt" {
				outPath = filepath.Join(absDir, *outOK+"_combined.txt")
			}
			f, _ := os.Create(outPath)
			defer f.Close()
			if *format == "txt" {
				for _, r := range allOK {
					name := r.Link.Name
					if name == "" {
						name = r.Link.Raw
					}
					acestream := r.Link.Raw
					if r.Link.Type == "ace_hash" {
						h := reHash40.FindString(r.Link.Raw)
						if h != "" {
							acestream = "acestream://" + strings.ToLower(h)
						}
					}
					// si Raw ya es http ace getstream, dejarlo tal cual o convertir a acestream://
					if r.Link.Type == "ace_url" {
						h := reHash40.FindString(r.Link.Raw)
						if h != "" {
							acestream = "acestream://" + strings.ToLower(h)
						} else {
							acestream = r.Link.Raw
						}
					}
					fmt.Fprintln(f, name)
					fmt.Fprintln(f, acestream)
				}
			} else {
				fmt.Fprintln(f, "#EXTM3U")
				for _, r := range allOK {
					if r.Link.Name != "" {
						fmt.Fprintf(f, "#EXTINF:-1,%s\n", r.Link.Name)
					}
					fmt.Fprintln(f, r.Link.Raw)
				}
			}
			fmt.Printf("💾 Generado %s con %d links OK (formato %s)\n", outPath, len(allOK), *format)
		} else {
			for base, list := range byFileOK {
				ext := *format
				outPath := filepath.Join(absDir, *outOK+"_"+base+"."+ext)
				if *format == "txt" {
					outPath = filepath.Join(absDir, *outOK+"_"+strings.TrimSuffix(base, filepath.Ext(base))+".txt")
				}
				f, err := os.Create(outPath)
				if err != nil {
					continue
				}
				if *format == "txt" {
					for _, r := range list {
						name := r.Link.Name
						if name == "" {
							name = filepath.Base(r.Link.File)
						}
						acestream := r.Link.Raw
						if r.Link.Type == "ace_hash" {
							h := reHash40.FindString(r.Link.Raw)
							if h != "" {
								acestream = "acestream://" + strings.ToLower(h)
							}
						}
						if r.Link.Type == "ace_url" {
							h := reHash40.FindString(r.Link.Raw)
							if h != "" {
								acestream = "acestream://" + strings.ToLower(h)
							}
						}
						fmt.Fprintln(f, name)
						fmt.Fprintln(f, acestream)
					}
				} else {
					fmt.Fprintln(f, "#EXTM3U")
					for _, r := range list {
						if r.Link.Name != "" {
							fmt.Fprintf(f, "#EXTINF:-1,%s\n", r.Link.Name)
						}
						fmt.Fprintln(f, r.Link.Raw)
					}
				}
				f.Close()
				fmt.Printf("💾 %s -> %d OK (formato %s)\n", outPath, len(list), *format)
			}
		}
	}
}

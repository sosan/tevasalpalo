package source

import (
	"sync"
	"testing"
	"time"
)

// TestFetchUpdatedListConcurrenteNoMutaMapasPublicados verifica la invariante
// copy-on-write del listado (ver vars.go): mientras el refresco de fuentes
// construye su mapa privado, los lectores concurrentes (handlers de fiber,
// snapshots de la web, BroadcasterVivo) no deben verse nunca un mapa a medio
// escribir ni perder misma la reasignación de la variable. Corre con -race en
// CI: cualquier regresión que mute un mapa publicado revienta aquí.
func TestFetchUpdatedListConcurrenteNoMutaMapasPublicados(t *testing.T) {
	origFetch, origSources := fetchWebData, sources
	origGateway := normGateway
	origCallback := OnListaRefrescada
	t.Cleanup(func() {
		fetchWebData, sources, normGateway, OnListaRefrescada = origFetch, origSources, origGateway, origCallback
	})
	// Los mapas vivos se restituyen con sus helpers de publicación, que son
	// los mismos que usa el refresco: así tampoco este test expone carreras.
	origBroadcasters, origTop := SnapshotBroadcasters(), SnapshotTopCompetitions()
	t.Cleanup(func() {
		publicarBroadcaster(origBroadcasters)
		publicarTop(origTop)
	})

	// Consumir el sync.Once del gateway ANTES de fijar el nuestro: si no,
	// FetchUpdatedList repoblaría el baked y el override se perdería.
	ensureNormGateway()
	normGateway = map[string][]string{"PRUEBA": {"PRUEBA"}}
	BroadcasterToAcestream = map[string]BroadcasterInfo{"PRUEBA": {Name: "PRUEBA"}}
	TopCompetitions = nil
	OnListaRefrescada = nil

	fetchWebData = func(_ string, _ bool) ([]byte, error) {
		// Latencia mínima: agranda la ventana de carrera sin ralentizar la suite.
		time.Sleep(2 * time.Millisecond)
		return []byte("#EXTINF:-1,PRUEBA\nacestream://0123456789abcdef0123456789abcdef01234567\n"), nil
	}
	sources = []Source{{Name: "fake", URL: "probe://fake", Type: SourceM3U, Proxied: false}}

	// Lectores concurrentes tipo web: iteración completa + lookup puntual
	stop := make(chan struct{})
	var lecturas sync.WaitGroup
	for i := 0; i < 4; i++ {
		lecturas.Add(1)
		go func() {
			defer lecturas.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, info := range SnapshotBroadcasters() {
					for range info.Links {
					}
				}
				for k, v := range SnapshotTopCompetitions() {
					_, _ = k, v
				}
				_, _ = BroadcasterVivo("PRUEBA")
			}
		}()
	}

	err := FetchUpdatedList()
	close(stop)
	lecturas.Wait()
	if err != nil {
		t.Fatalf("FetchUpdatedList: %v", err)
	}

	// El canal debe haber entrado en el listado publicado (hit del gateway)
	vivo, _ := BroadcasterVivo("PRUEBA")
	if len(vivo.Links) != 1 {
		t.Fatalf("PRUEBA debería tener 1 link tras el refresco, tiene %d", len(vivo.Links))
	}
}

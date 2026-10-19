// Estado de datos servidos por la web: cache con mutex de la vista, precarga
// tras cada refresco de fuentes y el ticker programado. Lo que la web lee de
// source pasa SIEMPRE por snapshots (invariante copy-on-write).

package web

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"encoding/json"
	"html/template"

	"tevasalpalo/internal/source"
)

var (
	cachedData     CachedData
	dataMutex      sync.RWMutex
	lastDataUpdate time.Time
)

type CachedData struct {
	Days                   []source.DayView
	DaysJSON               template.JS
	TopCompetitionsJSON    template.JS
	AllCompetitions        source.AllCompetitions
	TopCompetitions        map[string]source.CompetitionDetail
	TopCompetitionsOrdered []source.OrderedTopCompetition
	Broadcasters           map[string]source.BroadcasterInfo
	BroadcastersOrdered    []source.BroadcasterInfo
}

func preloadProgramationTVData() error {
	log.Println("📡 Obteniendo programacion TV...")
	days, daysJSON, topCompetitionsJSON, err := fetchEvents()
	if err != nil {
		return fmt.Errorf("error preloading data: %v", err)
	}

	broadcastersMap := source.SnapshotBroadcasters()
	broadcastersSlice := make([]source.BroadcasterInfo, 0, len(broadcastersMap))

	for _, info := range broadcastersMap {
		if info.ShowListChannels {
			broadcastersSlice = append(broadcastersSlice, info)
		}
	}

	// based in order value
	sort.Slice(broadcastersSlice, func(i, j int) bool {
		if broadcastersSlice[i].Order == 0 && broadcastersSlice[j].Order != 0 {
			return false
		}
		if broadcastersSlice[j].Order == 0 && broadcastersSlice[i].Order != 0 {
			return true
		}
		return broadcastersSlice[i].Order < broadcastersSlice[j].Order
	})

	topSnapshot := source.SnapshotTopCompetitions()
	topOrdered := source.GetOrderedTopCompetitions(topSnapshot)

	dataMutex.Lock()
	cachedData = CachedData{
		Days:                   days,
		DaysJSON:               *daysJSON,
		TopCompetitionsJSON:    *topCompetitionsJSON,
		AllCompetitions:        source.AllCompetitionsData,
		TopCompetitions:        topSnapshot,
		TopCompetitionsOrdered: topOrdered,
		Broadcasters:           broadcastersMap,
		BroadcastersOrdered:    broadcastersSlice,
	}
	lastDataUpdate = time.Now()
	dataMutex.Unlock()

	log.Println("✅ Datos pre-cargados en memoria")
	return nil
}

// Refrescar datos periódicamente
var refrescoProgramadoOnce sync.Once

func startTVProgramationDataRefresh() {
	// Solo un ticker: OnListaRefrescada dispara este arranque en cada
	// refresco de fuentes; sin el once, cada uno añadiría otro ticker (y
	// otra goroutine) para siempre.
	refrescoProgramadoOnce.Do(func() {
		ticker := time.NewTicker(6 * time.Hour)
		go func() {
			for range ticker.C {
				// Un fallo puntual no mata la proceso: la web sigue sirviendo
				// con lo que hay en cachedData y en 6h se reintenta. Un
				// log.Fatal aquí mataba la app en producción sin cierre
				// limpio (ni Tor/xray/engine ni flush de telemetría) justo
				// por una descarga fallida.
				if err := preloadProgramationTVData(); err != nil {
					log.Printf("⚠️ refresco de programación fallido (se reintenta en 6h): %v", err)
				}
			}
		}()
	})
}

func fetchEvents() ([]source.DayView, *template.JS, *template.JS, error) {
	// TODO: retry si da err 520, retry
	var err error
	var days []source.DayView
	for i := 1; i < 3; i++ {
		days, err = source.FetchScheduleMatchesFutbolEnCasa()
		if err != nil {
			log.Printf("Error fetching schedule: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		break
	}

	if err != nil {
		log.Printf("Error fetching schedule: %v", err)
		return nil, nil, nil, fmt.Errorf("1 Error al obtener la programación")
	}

	daysJSONBytes, err := json.Marshal(days)
	if err != nil {
		log.Printf("Error marshaling days to JSON: %v", err)
		return nil, nil, nil, fmt.Errorf("2 Error al obtener la programación")
	}
	daysJSON := template.JS(daysJSONBytes)

	topCompetitionsBytes, err := json.Marshal(source.SnapshotTopCompetitions())
	if err != nil {
		log.Printf("Error marshaling days to JSON: %v", err)
		return nil, nil, nil, fmt.Errorf("3 Error al obtener la programación")
	}
	topCompetitionsJSON := template.JS(topCompetitionsBytes)
	return days, &daysJSON, &topCompetitionsJSON, nil
}

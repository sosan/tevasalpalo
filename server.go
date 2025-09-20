package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"sync"

	"log"
	"main/update"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"

	// "github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/gofiber/template/html/v2"
)

//go:embed views/*
//go:embed views/*/*
//go:embed views/css/*.*
//go:embed views/*.html
var viewsFS embed.FS

var (
	cachedData     CachedData
	dataMutex      sync.RWMutex
	lastDataUpdate time.Time
)

type CachedData struct {
	Days                []DayView
	DaysJSON            template.JS
	TopCompetitionsJSON template.JS
	AllCompetitions     AllCompetitions
	TopCompetitions     map[string]CompetitionDetail
	Broadcasters        map[string]BroadcasterInfo
}

// Pre-cargar datos al iniciar
func preloadData() error {
	days, daysJSON, topCompetitionsJSON, err := fetchEvents()
	if err != nil {
		return fmt.Errorf("error preloading data: %v", err)
	}

	dataMutex.Lock()
	cachedData = CachedData{
		Days:                days,
		DaysJSON:            *daysJSON,
		TopCompetitionsJSON: *topCompetitionsJSON,
		AllCompetitions:     allCompetitions,
		TopCompetitions:     topCompetitions,
		Broadcasters:        broadcasterToAcestream,
	}
	lastDataUpdate = time.Now()
	dataMutex.Unlock()

	log.Println("✅ Datos pre-cargados en memoria")
	return nil
}

// Refrescar datos periódicamente
func startDataRefresh() {
	ticker := time.NewTicker(6 * time.Hour)
	go func() {
		for range ticker.C {
			if err := preloadData(); err != nil {
				log.Printf("⚠️  Error refrescando datos: %v", err)
			}
		}
	}()
}

func StartWebServer() (*fiber.App, error) {
	topCompetitions = transformCompetitionsToTop(allCompetitions)

	if err := preloadData(); err != nil {
		log.Printf("⚠️  Advertencia: No se pudieron pre-cargar datos: %v", err)
	}

	startDataRefresh()

	engine := html.NewFileSystem(http.FS(viewsFS), ".html")

	app := fiber.New(fiber.Config{
		Views: engine,
	})

	app.Use("/css", filesystem.New(filesystem.Config{
		Root:       http.FS(viewsFS),
		PathPrefix: "/views/css",
		Browse:     false,
	}))

	app.Use("/images", filesystem.New(filesystem.Config{
		Root:       http.FS(viewsFS),
		PathPrefix: "/views/images",
		Browse:     false,
	}))

	app.Use("/player/js", filesystem.New(filesystem.Config{
		Root:       http.FS(viewsFS),
		PathPrefix: "/views/js",
		Browse:     false,
	}))

	app.Use("/player/css", filesystem.New(filesystem.Config{
		Root:       http.FS(viewsFS),
		PathPrefix: "/views/css",
		Browse:     false,
	}))

	app.Use("/js", filesystem.New(filesystem.Config{
		Root:       http.FS(viewsFS),
		PathPrefix: "/views/js",
		Browse:     false,
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		// days, daysJSON, topCompetitionsJSON, err := fetchEvents()
		// if err != nil {
		// 	return c.Status(fiber.StatusInternalServerError).SendString("Error al obtener la programación")
		// }
		// return c.Render("index", fiber.Map{
		// 	"Days":                days,
		// 	"allCompetitions":     allCompetitions,
		// 	"topCompetitions":     topCompetitions,
		// 	"DaysJSON":            daysJSON,
		// 	"topCompetitionsJSON": topCompetitionsJSON,
		// })
		dataMutex.RLock()
		data := cachedData
		dataMutex.RUnlock()
		return c.Render("index", fiber.Map{
			"Days":                data.Days,
			"allCompetitions":     data.AllCompetitions,
			"topCompetitions":     data.TopCompetitions,
			"DaysJSON":            data.DaysJSON,
			"topCompetitionsJSON": data.TopCompetitionsJSON,
		})
	})

	app.Get("/broadcasters", func(c *fiber.Ctx) error {
		// days, daysJSON, topCompetitionsJSON, err := fetchEvents()
		// if err != nil {
		// 	return c.Status(fiber.StatusInternalServerError).SendString("Error al obtener la programación")
		// }
		// return c.Render("views/broadcasters", fiber.Map{
		// 	"Broadcasters":        broadcasterToAcestream,
		// 	"DaysJSON":            daysJSON,
		// 	"Days":                days,
		// 	"allCompetitions":     allCompetitions,
		// 	"topCompetitions":     topCompetitions,
		// 	"topCompetitionsJSON": topCompetitionsJSON,
		// })
		dataMutex.RLock()
		data := cachedData
		dataMutex.RUnlock()

		return c.Render("views/broadcasters", fiber.Map{
			"Broadcasters":        data.Broadcasters,
			"DaysJSON":            data.DaysJSON,
			"Days":                data.Days,
			"allCompetitions":     data.AllCompetitions,
			"topCompetitions":     data.TopCompetitions,
			"topCompetitionsJSON": data.TopCompetitionsJSON,
		})
	})

	app.Get("/refresh-data", func(c *fiber.Ctx) error {
		if err := preloadData(); err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(fiber.Map{
			"success": true,
			"message": "Datos actualizados correctamente",
		})
	})

	app.Get("/player/:id", func(c *fiber.Ctx) error {
		acestreamId := c.Params("id")
		if acestreamId == "" {
			fmt.Printf("Error obteniendo content_id para stream_id %s\n", acestreamId)
			return c.Status(500).SendString("No se pudo obtener el content ID")
		}

		return c.Render("views/player", fiber.Map{
			"AcestreamId": acestreamId,
			"Error":       nil,
		})
	})

	app.Get("/update", func(c *fiber.Ctx) error {
		go func() {
			err := update.ForceUpdate()
			if err != nil {
				log.Print("ERROR | No es posible actualizarse")
				return
			}
			time.Sleep(3 * time.Second)
			shutdownChan <- struct{}{}
		}()

		return c.JSON(fiber.Map{
			"sendedupdate": true,
		})
	})

	app.Get("/healthz", func(c *fiber.Ctx) error {
		if update.Updated {
			return c.JSON(fiber.Map{
				"ok": true,
			})
		}
		return c.JSON(fiber.Map{
			"ok": false,
		})
	})

	app.Get("/updateavailable", func(c *fiber.Ctx) error {
		needUpdate := update.GetNeedUpdate()
		return c.JSON(fiber.Map{
			"needUpdate": needUpdate,
		})
	})

	go func() {
		if err := app.Listen("0.0.0.0:3000"); err != nil {
			log.Printf("❌ Error en servidor web: %v", err)
		}
	}()

	return app, nil
}

func fetchEvents() ([]DayView, *template.JS, *template.JS, error) {
	// TODO: retry si da err 520, retry
	var err error
	var days []DayView
	for i := 1; i < 10; i++ {
		days, err = fetchScheduleMatchesFutbolEnCasa()
		if err != nil {
			// Loggear el error completo para depuración
			log.Printf("Error fetching schedule: %v", err)
			time.Sleep(3 * time.Second)
			continue
			// Devolver un error al cliente
			// return nil, nil, nil, fmt.Errorf("Error al obtener la programación")
		}
		if err == nil {
			break
		}
	}

	if err != nil {
		// Loggear el error completo para depuración
		log.Printf("Error fetching schedule: %v", err)
		// Devolver un error al cliente
		return nil, nil, nil, fmt.Errorf("Error al obtener la programación")
	}

	daysJSONBytes, err := json.Marshal(days)
	if err != nil {
		log.Printf("Error marshaling days to JSON: %v", err)
		return nil, nil, nil, fmt.Errorf("Error al obtener la programación")
	}
	daysJSON := template.JS(daysJSONBytes)

	topCompetitionsBytes, err := json.Marshal(topCompetitions)
	if err != nil {
		log.Printf("Error marshaling days to JSON: %v", err)
		return nil, nil, nil, fmt.Errorf("Error al obtener la programación")
	}
	topCompetitionsJSON := template.JS(topCompetitionsBytes)
	return days, &daysJSON, &topCompetitionsJSON, nil
}

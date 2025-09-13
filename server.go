package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"main/update"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/template/html/v2"
)

//go:embed views/*
//go:embed views/*/*
//go:embed views/css/*.*
//go:embed views/*.html
var viewsFS embed.FS

type Match struct {
	Competition  string            `json:"competition"`
	Date         string            `json:"date"`
	Time         string            `json:"time"`
	Event        string            `json:"event"`
	Broadcasters []BroadcasterInfo `json:"channels"`
	Country      string            `json:"-"`
	Sport        string            `json:"-"`
}

type BroadcasterInfo struct {
	Name  string   `json:"name"`
	Logo  string   `json:"logo"`
	Links []string `json:"link,omitempty"`
}

func StartWebServer() error {
	topCompetitions = transformCompetitionsToTop(allCompetitions)

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

	app.Use("/js", filesystem.New(filesystem.Config{
		Root:       http.FS(viewsFS),
		PathPrefix: "/views/js",
		Browse:     false,
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		days, daysJSON, topCompetitionsJSON, err := fetchEvents()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Error al obtener la programación")
		}
		return c.Render("index", fiber.Map{
			"Days":                days,
			"allCompetitions":     allCompetitions,
			"topCompetitions":     topCompetitions,
			"DaysJSON":            daysJSON,
			"topCompetitionsJSON": topCompetitionsJSON,
		})
	})

	app.Get("/broadcasters", func(c *fiber.Ctx) error {
		days, daysJSON, topCompetitionsJSON, err := fetchEvents()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Error al obtener la programación")
		}
		return c.Render("views/broadcasters", fiber.Map{
			"Broadcasters":        broadcasterToAcestream,
			"DaysJSON":            daysJSON,
			"Days":                days,
			"allCompetitions":     allCompetitions,
			"topCompetitions":     topCompetitions,
			"topCompetitionsJSON": topCompetitionsJSON,
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
		go update.ForceUpdate()

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

	return app.Listen("0.0.0.0:3000")
}

func fetchEvents() ([]DayView, *template.JS, *template.JS, error) {
	// TODO: si da err 520, retry
	days, err := fetchScheduleMatchesFutbolEnCasa()
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

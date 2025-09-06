package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
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

func startWebServer() error {
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
		// TODO: si da err 520, retry
		days, err := fetchScheduleMatchesFutbolEnCasa()
		if err != nil {
			// Loggear el error completo para depuración
			log.Printf("Error fetching schedule: %v", err)
			// Devolver un error al cliente
			return c.Status(fiber.StatusInternalServerError).SendString("Error al obtener la programación")
		}

		daysJSONBytes, err := json.Marshal(days)
		if err != nil {
			log.Printf("Error marshaling days to JSON: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Error al procesar los datos para JS")
		}
		daysJSON := template.JS(daysJSONBytes)

		topCompetitionsBytes, err := json.Marshal(topCompetitions)
		if err != nil {
			log.Printf("Error marshaling days to JSON: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Error al procesar los datos para JS")
		}
		topCompetitionsJSON := template.JS(topCompetitionsBytes)
		
		return c.Render("index", fiber.Map{
			"DaysJSON":        daysJSON,
			"Days":            days,
			"allCompetitions": allCompetitions,
			"topCompetitions": topCompetitions,
			"topCompetitionsJSON": topCompetitionsJSON,
		})
	})

	app.Get("/broadcasters", func(c *fiber.Ctx) error {
		return c.Render("views/broadcasters", fiber.Map{
			"Broadcasters": broadcasterToAcestream,
		})
	})

	app.Get("/player/:id", func(c *fiber.Ctx) error {
		// Obtener el ID del parámetro de la URL
		acestreamId := c.Params("id")

		if acestreamId == "" {
			// Manejar el error, por ejemplo, loguearlo y devolver un error al cliente
			fmt.Printf("Error obteniendo content_id para stream_id %s\n", acestreamId)
			// Opcional: Devolver el stream_id original como fallback si es un content_id válido
			// Pero normalmente si falla aquí, es mejor mostrar un error.
			// Para depurar, puedes pasar el stream_id y manejarlo en JS
			return c.Status(500).Render("player", fiber.Map{
				"AcestreamId": acestreamId, // Pasar el original
				"Error":       fmt.Sprintf("No se pudo obtener el content ID: %v", acestreamId),
			})
		}

		// 3. Renderizar la plantilla con el content_id obtenido
		return c.Render("views/player", fiber.Map{
			"AcestreamId": acestreamId,
			"Error":       nil,
		})
	})

	return app.Listen("0.0.0.0:3000")
}

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
	Links []string `json:"link,omitempty"`
}

package app

// Base sembrada: una lista de canales de AceStream que viaja dentro del
// binario para que una instalación nueva no arranque con cero.
//
// El motor no ofrece ningún endpoint de listado (todo lo de /ace/server/* y
// /ace/get responde 404), así que no existe la manera de leer los enlaces
// primigenios en vivo: el motor solo conoce lo que se le pide. Lo único que se
// puede guardar es una instantánea, y esa instantánea es justamente lo que
// hace falta para que el primer arranque tenga algo que mostrar antes de que
// terminen las siete fuentes.
//
// La base no se queda quieta: los canales que una fuente real publica
// adoptan los hashes de esa fuente, y los que nadie publica se apartan solos
// por la puntuación de disponibilidad. Lo que no se hace nunca es retirarlos,
// porque para eso están.

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// semillaBase es el fichero embebido. Va comprimido porque son unos 2.000
// canales y sin comprimir serían unos 200 KB de los cuales casi todo son
// nombres repetidos.
//
//go:embed semilla/base.json.gz
var semillaBase embed.FS

const (
	// rutaSemilla es la ruta del fichero DENTRO del embed FS (relativa al
	// directorio del paquete, donde está este fichero).
	rutaSemilla = "semilla/base.json.gz"
	// rutaSemillaDisco es donde el generador (--genera-semilla, ejecutado
	// desde la raíz del repo) escribe el embed fuente para el siguiente
	// build.
	rutaSemillaDisco = "internal/app/semilla/base.json.gz"
	nombreBaseSrc    = "base"
)

// semillaCanal es un canal tal como viaja en la instantánea. Solo nombre y
// hashes: el resto de campos (fechas, fuentes, visto) los pone la base al
// sembrar, porque son cosas del momento de sembrar y no del contenido.
type semillaCanal struct {
	Nombre string   `json:"nombre"`
	Hashes []string `json:"hashes"`
}

type semillaDocumento struct {
	Generado time.Time      `json:"generado"`
	Canales  []semillaCanal `json:"canales"`
}

// leerSemilla descomprime y parsea la base embebida.
func leerSemilla() (*semillaDocumento, error) {
	raw, err := semillaBase.ReadFile(rutaSemilla)
	if err != nil {
		return nil, fmt.Errorf("leer la base embebida: %w", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("la base embebida no está comprimida: %w", err)
	}
	defer zr.Close()
	cuerpo, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("descomprimir la base embebida: %w", err)
	}
	var doc semillaDocumento
	if err := json.Unmarshal(cuerpo, &doc); err != nil {
		return nil, fmt.Errorf("la base embebida no es un JSON válido: %w", err)
	}
	return &doc, nil
}

// sembrarBaseVacia carga la base embebida en una base que no tiene canales.
//
// Solo cuando el bucket está vacío: en cuanto hay un canal, la base es de
// esa instalación y la instantánea ya no debe tocar nada.
// Insertar en cada arranque machacaría lo que hayan traido las fuentes.
func sembrarBaseVacia() (int, error) {
	if storeGlobal == nil || storeGlobal.db == nil {
		return 0, nil
	}
	vacia := false
	err := storeGlobal.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChannels))
		if b == nil {
			return fmt.Errorf("no hay bucket de canales")
		}
		if b.Stats().KeyN == 0 {
			vacia = true
		}
		return nil
	})
	if err != nil || !vacia {
		return 0, err
	}

	doc, err := leerSemilla()
	if err != nil {
		// Una base corrupta no debe impedir arrancar: el listado estará tras el
		// refresco de fuentes. Se avisa y se sigue.
		log.Printf("⚠️  no se pudo sembrar la base: %v (seguimos con las fuentes)", err)
		return 0, nil
	}

	ahora := time.Now().UTC()
	insertados := 0
	err = storeGlobal.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChannels))
		for _, c := range doc.Canales {
			hashes := normalizarHashes(c.Hashes)
			if c.Nombre == "" || len(hashes) == 0 {
				continue
			}
			raw, err := json.Marshal(canalRegistro{
				Nombre:     c.Nombre,
				Hashes:     hashes,
				Fuentes:    []string{nombreBaseSrc},
				PrimeraVez: ahora,
				UltimaVez:  ahora,
				Visto:      true,
				Permanente: true,
			})
			if err != nil {
				return err
			}
			if err := b.Put([]byte(c.Nombre), raw); err != nil {
				return err
			}
			insertados++
		}
		return nil
	})
	if err != nil {
		log.Printf("⚠️  no se pudo escribir la base sembrada: %v", err)
		return 0, err
	}
	// Sin eventos de "nuevo" a propósito: sembrar 2.000 canales no son 2.000
	// altas nuevas, y si lo fueran el histórico dejaría de significar nada.
	log.Printf("🌱 Base sembrada con %d canales de la instantánea (generada el %s)",
		insertados, doc.Generado.Format("2006-01-02"))
	return insertados, nil
}

// normalizarHashes limpia una lista de enlaces tal como vienen de las fuentes:
// minúsculas y sin el esquema delante.
func normalizarHashes(hashes []string) []string {
	salida := make([]string, 0, len(hashes))
	vistos := map[string]bool{}
	for _, h := range hashes {
		n := normalizarHash(h)
		if n == "" || !esHashAceStream(n) {
			continue
		}
		if vistos[n] {
			continue
		}
		vistos[n] = true
		salida = append(salida, n)
	}
	return salida
}

// generarSemilla escribe la instantánea desde la base actual. Es la forma de
// renovar la base: se ejecuta con la app parada o sin tráfico, y el resultado
// se compila dentro del binario siguiente.
func generarSemilla() error {
	if storeGlobal == nil || storeGlobal.db == nil {
		return fmt.Errorf("no hay base de datos abierta")
	}
	doc := semillaDocumento{Generado: time.Now().UTC().Truncate(time.Hour)}
	err := storeGlobal.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChannels))
		if b == nil {
			return fmt.Errorf("no hay bucket de canales")
		}
		return b.ForEach(func(_, v []byte) error {
			var reg canalRegistro
			if err := json.Unmarshal(v, &reg); err != nil {
				return nil
			}
			hashes := normalizarHashes(reg.Hashes)
			if reg.Nombre == "" || len(hashes) == 0 {
				return nil
			}
			doc.Canales = append(doc.Canales, semillaCanal{Nombre: reg.Nombre, Hashes: hashes})
			return nil
		})
	})
	if err != nil {
		return err
	}
	if len(doc.Canales) == 0 {
		return fmt.Errorf("la base no tiene canales: no hay nada que guardar")
	}

	cuerpo, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := zw.Write(cuerpo); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(rutaSemillaDisco), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(rutaSemillaDisco, buf.Bytes(), 0o644); err != nil {
		return err
	}
	log.Printf("✅ Semilla escrita: %d canales, %d KB comprimidos (sin comprimir %d KB)",
		len(doc.Canales), buf.Len()/1024, len(cuerpo)/1024)
	return nil
}

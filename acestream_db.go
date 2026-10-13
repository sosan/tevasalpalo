package main

// Base de datos local de los AceStream.
//
// Qué resuelve: hasta ahora el listado de canales vivía solo en memoria y se
// reconstruía desde 7 fuentes remotas en cada arranque, bloqueando el arranque
// entre 27 y 70 segundos (medido). Aquí el listado se guarda y al arrancar se
// carga de golpe, así que la app sirve en cuanto puede y refresca después en
// segundo plano.
//
// Además guarda dos cosas que antes se perdían: qué canal aparece y desaparece
// con el tiempo, y qué aporta cada fuente por separado. Eso último permite
// decidir si una fuente sobra comparando su solapamiento con el resto.
//
// Elección: bbolt (go.etcd.io/bbolt). Es Go puro, así que funciona con
// CGO_ENABLED=0, que es como se compila este proyecto. SQLite no vale: el
// driver con cgo queda descartado y el puro son ~40 MB de código generado y
// minutos de build en un binario que ya pesa 180 MB y se compila para 6
// plataformas.
//
// Esquema, un bucket por:
//
//	channels  clave = nombre normalizado  -> registro del canal
//	sources   clave = nombre de la fuente -> estado y aporte de cada fuente
//	history   clave = timestamp#canal     -> evento (nuevo, desaparecido, vuelve)
//	meta      clave = versión del esquema  -> comprobación al abrir
//
// El arranque lee el bucket channels entero: son ~2.000 claves, microsegundos.
// No hace falta un snapshot aparte, así que no hay dos verdades que puedan
// desincronizarse.

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	bucketChannels = "channels"
	bucketSources  = "sources"
	bucketHistory  = "history"
	bucketMeta     = "meta"

	// versionEsquema sube si cambia la forma de los registros. Al abrir un
	// fichero de otra versión se descarta en vez de intentar migrar a ciegas:
	// los datos son cachés y se regeneran solos en el siguiente arranque.
	versionEsquema = "1"

	// nombreDB es el fichero junto al ejecutable.
	nombreDB = "acestream.db"

	// maxEventosHistorial acota el bucket history. Sin tope, una fuente que
	// desapareciera y volviera una vez por minuto llenaría el disco en meses.
	maxEventosHistorial = 20000
)

// canalRegistro es lo que se guarda por canal.
type canalRegistro struct {
	Nombre string   `json:"nombre"`
	Hashes []string `json:"hashes"`
	// Fuentes es qué fuentes lo publican. Sirve para ver de dónde sale cada
	// uno y para medir el solapamiento entre fuentes.
	Fuentes []string `json:"fuentes"`
	// Primera vez que apareció y última vez que se vio. Con ambos se puede
	// saber si un canal es nuevo o lleva tiempo ahí.
	PrimeraVez time.Time `json:"primera_vez"`
	UltimaVez  time.Time `json:"ultima_vez"`
	// Visto es si apareció en el último refresco. Un canal que no aparece
	// no se borra: sigue en la base como histórico y con Visto=false, para
	// poder saying "estaba aquí y ahora no" en vez de perderlo en silencio.
	Visto bool `json:"visto"`
}

// fuenteRegistro es lo que se guarda por fuente.
type fuenteRegistro struct {
	Nombre      string    `json:"nombre"`
	UltimoOK    time.Time `json:"ultimo_ok"`
	UltimoError string    `json:"ultimo_error"`
	Entradas    int       `json:"entradas"`
	Hashes      int       `json:"hashes"`
	// SoloAporta son los canales que esta fuente publica y ninguna otra.
	SoloAporta int `json:"solo_aporta"`
	// Coincidentes con los que también da alguna otra fuente.
	Coincidentes int   `json:"coincidentes"`
	DuracionMs   int64 `json:"duracion_ms"`
	Intentos     int   `json:"intentos"`
	// Ciclo de vida. Una fuente que no aporta ningún canal exclusivo se pausa
	// sola en vez de desaparecer: así sigue vigilándose de vez en cuando y se
	// nota ella sola si revive. Si se borrara del código habría que volver a
	// añadirla en cuanto volviera a servir de algo.
	IntervaloMin         int       `json:"intervalo_min"`
	CiclosSinAportar     int       `json:"ciclos_sin_aportar"`
	UltimaConsulta       time.Time `json:"ultima_consulta"`
	ProximaConsulta      time.Time `json:"proxima_consulta"`
	ExclusivosHistoricos int       `json:"exclusivos_historicos"`
}

// Cadencia de la pausa adaptativa, en minutos. El 0 es "en cada refresco".
// Sube sola tras varios ciclos sin aportar nada exclusivo y vuelve a bajar en
// cuanto la fuente aporta de nuevo.
var nivelesIntervalo = []int{0, 30, 180, 720, 1440}

// nivelIntervaloPara devuelve el intervalo según los ciclos sin aportar.
func nivelIntervaloPara(ciclos int) int {
	if ciclos < 0 {
		ciclos = 0
	}
	// Dos ciclos de gracia antes de pausar: una fuente puede fallar un
	// domingo y no por eso dejar de mirarse.
	if ciclos < 2 {
		return 0
	}
	idx := ciclos - 1
	if idx >= len(nivelesIntervalo) {
		idx = len(nivelesIntervalo) - 1
	}
	return nivelesIntervalo[idx]
}

// debeConsultarse dice si toca preguntar por esta fuente ahora mismo.
func (r fuenteRegistro) debeConsultarse(ahora time.Time) bool {
	if r.ProximaConsulta.IsZero() || r.UltimaConsulta.IsZero() {
		return true
	}
	return !ahora.Before(r.ProximaConsulta)
}

// eventoHistorial es una entrada del histórico.
type eventoHistorial struct {
	Fecha   time.Time `json:"fecha"`
	Tipo    string    `json:"tipo"`
	Canal   string    `json:"canal"`
	Fuentes []string  `json:"fuentes,omitempty"`
	Hashes  int       `json:"hashes"`
}

// tipos de evento del histórico
const (
	eventoNuevo      = "nuevo"
	eventoVuelve     = "vuelve"
	eventoDesaparece = "desaparece"
)

type store struct {
	mu sync.Mutex
	db *bolt.DB
	// rutaOverride lo usan los tests para no escribir el fichero junto al
	// binario de pruebas.
	rutaOverride string
}

var storeGlobal *store

// abrirStoreEn abre la base en una ruta concreta. La usa abrirStore y los
// tests; fuera de los tests no hay razón para llamarla.
func abrirStoreEn(ruta string) *store {
	s, err := prepararStore(ruta)
	if err != nil {
		log.Printf("⚠️  base de datos: no se pudo abrir %s: %v (se sigue sin persistencia)", ruta, err)
		return nil
	}
	storeGlobal = s
	return s
}

// abrirStore abre (o crea) la base. Devuelve nil sin error si no se puede: la
// app tiene que funcionar igual sin base, solo que sin arranque rápido ni
// histórico. Perder la persistencia no puede impedir ver el vídeo.
func abrirStore() *store {
	ruta := nombreDB
	if p := storePath(); p != "" {
		ruta = p
	}
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		log.Printf("⚠️  base de datos: no se pudo crear el directorio: %v", err)
		return nil
	}

	s, err := prepararStore(ruta)
	if err != nil {
		return nil
	}
	storeGlobal = s
	return s
}

// prepararStore hace el trabajo de abrir la base y preparar los buckets,
// sin tocar el global. Separado para que los tests puedan abrir una base en un
// temporal sin interferir con la de producción.
func prepararStore(ruta string) (*store, error) {
	db, err := bolt.Open(ruta, 0o600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		// Timeout quiere decir que otra instancia tiene el fichero abierto.
		// Es normal si el usuario lanzó dos veces; no es un error grave.
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range []string{bucketChannels, bucketSources, bucketHistory, bucketMeta} {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}
		meta := tx.Bucket([]byte(bucketMeta))
		vers := meta.Get([]byte("esquema"))
		if vers == nil {
			return meta.Put([]byte("esquema"), []byte(versionEsquema))
		}
		if string(vers) != versionEsquema {
			// Esquema distinto: se vacía. Los datos son caché y se regeneran.
			log.Printf("⚠️  base de datos: esquema %s distinto del esperado %s, se regenera",
				vers, versionEsquema)
			for _, b := range []string{bucketChannels, bucketSources, bucketHistory} {
				if err := tx.DeleteBucket([]byte(b)); err != nil {
					return err
				}
				if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
					return err
				}
			}
			return meta.Put([]byte("esquema"), []byte(versionEsquema))
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &store{db: db}, nil
}

// storePath es la ruta de la base, junto al ejecutable. La respeta si ya hay
// un store abierto, para que los tests puedan trabajar en un temporal.
func storePath() string {
	if storeGlobal != nil && storeGlobal.rutaOverride != "" {
		return storeGlobal.rutaOverride
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), nombreDB)
}

func cerrarStore() {
	if storeGlobal == nil || storeGlobal.db == nil {
		return
	}
	storeGlobal.db.Close()
	storeGlobal = nil
}

// cargarCanales lee el bucket entero. Es lo que hace rápido el arranque: una
// pasada por ~2.000 claves, sin red y sin parsear las 7 fuentes.
func (s *store) cargarCanales() map[string][]string {
	salida := make(map[string][]string)
	if s == nil || s.db == nil {
		return salida
	}
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChannels))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var reg canalRegistro
			if err := json.Unmarshal(v, &reg); err != nil {
				return nil // registro ilegible: se salta, no rompe la carga
			}
			// Solo los vistos en el último refresco. Un canal que desapareció
			// se queda en la base (para el histórico y las consultas), pero
			// no debe volver a la lista de reproducción: sus hashes están
			// caducados y ofrecerlos es ofrecer algo que no va a funcionar.
			if reg.Visto && len(reg.Hashes) > 0 {
				salida[reg.Nombre] = reg.Hashes
			}
			return nil
		})
	})
	if err != nil {
		log.Printf("⚠️  base de datos: no se pudo leer (%v)", err)
	}
	return salida
}

// guardarRefresco escribe el resultado de un ciclo completo de fuentes:
// fusiona el listado, calcula altas y bajas, y registra el aporte de cada
// fuente. Todo en una transacción para que no queden estados intermedios.
func (s *store) guardarRefresco(
	fusionado map[string][]string,
	porFuente map[string]map[string][]string,
	estado map[string]string,
	intentos map[string]int,
	duracionMs map[string]int64,
	enPausa map[string]bool,
) (altas, bajas int, err error) {
	if s == nil || s.db == nil {
		return 0, 0, nil
	}
	ahora := time.Now().UTC()

	err = s.db.Update(func(tx *bolt.Tx) error {
		ch := tx.Bucket([]byte(bucketChannels))
		hs := tx.Bucket([]byte(bucketHistory))
		if ch == nil || hs == nil {
			return fmt.Errorf("buckets ausentes")
		}

		// Cuál fuente publica cada canal, para el registro de fuentes.
		fuentesDe := map[string]map[string]bool{}
		for nombre, canales := range porFuente {
			for c := range canales {
				if fuentesDe[c] == nil {
					fuentesDe[c] = map[string]bool{}
				}
				fuentesDe[c][nombre] = true
			}
		}

		// Se lee todo antes de escribir nada. Dos motivos: bbolt no permite
		// modificar un bucket mientras se recorre con ForEach (el Put lanzaba
		// error y el marcado se quedaba a medias), y sobre todo hace falta
		// el estado PREVIO para distinguir "nuevo" de "vuelve" y no fills el
		// histórico de falsos retornos en cada refresco.
		previos := map[string]canalRegistro{}
		if err := ch.ForEach(func(k, v []byte) error {
			var reg canalRegistro
			if err := json.Unmarshal(v, &reg); err != nil {
				return nil
			}
			previos[reg.Nombre] = reg
			return nil
		}); err != nil {
			return err
		}

		for canonico, hashes := range fusionado {
			clave := []byte(canonico)
			previa, existia := previos[canonico]

			reg := canalRegistro{
				Nombre:     canonico,
				Hashes:     hashes,
				Fuentes:    clavesOrdenadas(fuentesDe[canonico]),
				UltimaVez:  ahora,
				PrimeraVez: ahora,
				Visto:      true,
			}

			switch {
			case !existia:
				altas++
				if err := escribirEvento(hs, eventoHistorial{
					Fecha: ahora, Tipo: eventoNuevo, Canal: canonico,
					Fuentes: reg.Fuentes, Hashes: len(hashes),
				}); err != nil {
					return err
				}
			case !previa.Visto:
				// Estaba en la base pero no en el ciclo anterior: vuelve.
				altas++
				reg.PrimeraVez = previa.PrimeraVez
				if err := escribirEvento(hs, eventoHistorial{
					Fecha: ahora, Tipo: eventoVuelve, Canal: canonico,
					Fuentes: reg.Fuentes, Hashes: len(hashes),
				}); err != nil {
					return err
				}
			default:
				reg.PrimeraVez = previa.PrimeraVez
			}

			raw, err := json.Marshal(reg)
			if err != nil {
				return err
			}
			if err := ch.Put(clave, raw); err != nil {
				return err
			}
		}

		// Los que estaban y no han vuelto en este ciclo. Se recorren los
		// previos, no el bucket, por lo mismo de arriba.
		//
		// Solo se registra la baja la primera vez: un canal desaparecido
		// sigue desaparecido en los ciclos siguientes y no debe ensuciar el
		// histórico con una línea por refresco.
		//
		// Y solo si además alguna de sus fuentes respondió. Que una fuente se
		// caiga no significa que el canal haya desaparecido: significa que no
		// lo sabemos. Marcarlo como baja vaciaría la lista cada vez que una
		// de las 7 fuentes falla, que es a menudo.
		vistos := map[string]bool{}
		for c := range fusionado {
			vistos[c] = true
		}
		for nombre, reg := range previos {
			if vistos[nombre] || !reg.Visto {
				continue
			}
			if !algunaFuenteRespondio(reg.Fuentes, estado) {
				// Todas las que lo publicaban fallaron: se mantiene tal
				// cual, con su Visto anterior, para no perder el canal por
				// un problema de red ajeno.
				continue
			}
			bajas++
			reg.Visto = false
			raw, err := json.Marshal(reg)
			if err != nil {
				return err
			}
			if err := ch.Put([]byte(nombre), raw); err != nil {
				return err
			}
			if err := escribirEvento(hs, eventoHistorial{
				Fecha: ahora, Tipo: eventoDesaparece, Canal: nombre,
				Fuentes: reg.Fuentes, Hashes: len(reg.Hashes),
			}); err != nil {
				return err
			}
		}

		if err := podarHistorial(hs); err != nil {
			return err
		}
		return registrarFuentes(tx, porFuente, estado, intentos, duracionMs, ahora, enPausa)
	})
	if err != nil {
		log.Printf("⚠️  base de datos: no se pudo guardar el refresco: %v", err)
	}
	return altas, bajas, err
}

// registrarFuentes guarda el estado y el aporte de cada fuente.
func registrarFuentes(tx *bolt.Tx, porFuente map[string]map[string][]string,
	estado map[string]string, intentos map[string]int, duracion map[string]int64,
	ahora time.Time, enPausa map[string]bool) error {

	b := tx.Bucket([]byte(bucketSources))
	if b == nil {
		return nil
	}

	// Cuántos canales publica cada fuente y cuántos son exclusivos suyos.
	cuentaPorFuente := map[string]int{}
	paraFuente := map[string]map[string]bool{}
	for nombre, canales := range porFuente {
		for c := range canales {
			cuentaPorFuente[nombre]++
			if paraFuente[c] == nil {
				paraFuente[c] = map[string]bool{}
			}
			paraFuente[c][nombre] = true
		}
	}

	exclusivos := map[string]int{}
	for _, fs := range paraFuente {
		if len(fs) == 1 {
			for nombre := range fs {
				exclusivos[nombre]++
			}
		}
	}

	// Se parte del registro anterior para conservar UltimoOK: si una fuente
	// lleva tres días caída, hay que seguir sabiendo cuándo fue la última vez
	// que trajo algo, no perder ese dato en el primer fallo.
	anteriores := map[string]fuenteRegistro{}
	if err := b.ForEach(func(k, v []byte) error {
		var r fuenteRegistro
		if err := json.Unmarshal(v, &r); err == nil {
			anteriores[string(k)] = r
		}
		return nil
	}); err != nil {
		return err
	}

	for nombre, canales := range porFuente {
		totalHashes := 0
		for _, hs := range canales {
			totalHashes += len(hs)
		}
		prev := anteriores[nombre]
		reg := fuenteRegistro{
			Nombre:               nombre,
			Entradas:             len(canales),
			Hashes:               totalHashes,
			SoloAporta:           exclusivos[nombre],
			Coincidentes:         len(canales) - exclusivos[nombre],
			DuracionMs:           duracion[nombre],
			Intentos:             intentos[nombre],
			UltimoError:          estado[nombre],
			UltimoOK:             prev.UltimoOK,
			CiclosSinAportar:     prev.CiclosSinAportar,
			ExclusivosHistoricos: prev.ExclusivosHistoricos,
		}

		// Una fuente en pausa no se ha consultado: su cadencia se queda como
		// estaba. Si se recalculara con cero entradas subiría un nivel más en
		// cada refresco y nunca podría volver a entrar por su cuenta.
		if enPausa[nombre] {
			reg.IntervaloMin = prev.IntervaloMin
			reg.CiclosSinAportar = prev.CiclosSinAportar
			reg.ProximaConsulta = prev.ProximaConsulta
			reg.UltimaConsulta = prev.UltimaConsulta
			reg.UltimoOK = prev.UltimoOK
			rawPausa, err := json.Marshal(reg)
			if err != nil {
				return err
			}
			return b.Put([]byte(nombre), rawPausa)
		}

		// Cadencia: solo se penaliza el silencio, no el error. Una fuente que
		// responde bien pero todo lo que da ya está en otras no aporta nada y
		// no vale la pena bajarla cada ciclo; una que falla no se sabe si
		// aportaría, así que no se penaliza hasta que responda.
		if reg.UltimoError == "" {
			if reg.SoloAporta > 0 {
				reg.CiclosSinAportar = 0
				reg.ExclusivosHistoricos = reg.SoloAporta
			} else {
				reg.CiclosSinAportar++
			}
			reg.IntervaloMin = nivelIntervaloPara(reg.CiclosSinAportar)
			reg.UltimaConsulta = ahora
			reg.UltimoOK = ahora
			reg.ProximaConsulta = ahora.Add(time.Duration(reg.IntervaloMin) * time.Minute)
		} else {
			// Falló: se reintenta en el siguiente refresco, sin pausa.
			reg.UltimaConsulta = ahora
			reg.IntervaloMin = 0
			reg.ProximaConsulta = ahora
		}
		raw, err := json.Marshal(reg)
		if err != nil {
			return err
		}
		if err := b.Put([]byte(nombre), raw); err != nil {
			return err
		}
	}
	return nil
}

// algunaFuenteRespondio dice si alguna de las fuentes que publicaban un canal
// traerlo en este ciclo. Si ninguna respondió, su ausencia no significa nada:
// la fuente se cayó, el canal no desapareció.
func algunaFuenteRespondio(fuentes []string, estado map[string]string) bool {
	// Sin datos de fuentes no se puede afirmar que todas fallaron: se asume
	// que el ciclo fue bueno. Tratar "lo desconozco" como "falló" vaciaría la
	// lista en el arranque, que es justo cuando aún no se sabe nada.
	if len(fuentes) == 0 || len(estado) == 0 {
		return true
	}
	for _, f := range fuentes {
		if estado[f] == "" {
			return true
		}
	}
	return false
}

// escribirEvento añade una entrada al histórico. La clave lleva el timestamp
// primero para que el orden natural de bbolt sea el cronológico, y el canal
// detrás para desempatar eventos del mismo instante.
func escribirEvento(b *bolt.Bucket, ev eventoHistorial) error {
	clave := fmt.Sprintf("%s#%s", ev.Fecha.UTC().Format(time.RFC3339Nano), ev.Canal)
	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return b.Put([]byte(clave), raw)
}

// podarHistorial borra los eventos más antiguos cuando se supera el tope.
func podarHistorial(b *bolt.Bucket) error {
	exceso := contarClaves(b) - maxEventosHistorial
	if exceso <= 0 {
		return nil
	}
	// El cursor se crea una vez: crear uno nuevo en cada vuelta del bucle
	// reinicia la iteración y no se avanza nada.
	c := b.Cursor()
	for k, _ := c.First(); k != nil && exceso > 0; k, _ = c.Next() {
		if err := b.Delete(k); err != nil {
			return err
		}
		exceso--
	}
	return nil
}

// contarClaves cuenta las claves de un bucket.
//
// No se usa b.Stats().KeyN: dentro de una transacción de escritura no refleja
// las claves pendientes, así que el conteo salía bajo y la poda no hacía nada.
// Recorrer con cursor siempre da el número real.
func contarClaves(b *bolt.Bucket) int {
	n := 0
	c := b.Cursor()
	for k, _ := c.First(); k != nil; k, _ = c.Next() {
		n++
	}
	return n
}

// listarFuentes devuelve el estado de cada fuente, ordenado por lo que aporta.
func (s *store) listarFuentes() []fuenteRegistro {
	if s == nil || s.db == nil {
		return nil
	}
	var salida []fuenteRegistro
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSources))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var reg fuenteRegistro
			if err := json.Unmarshal(v, &reg); err != nil {
				return nil
			}
			salida = append(salida, reg)
			return nil
		})
	})
	sort.Slice(salida, func(i, j int) bool {
		return salida[i].SoloAporta > salida[j].SoloAporta
	})
	return salida
}

// buscarCanales busca por subcadena del nombre, usando el índice de claves del
// bucket en vez de recorrer el mapa entero. El prefijo se quita antes de
// filtrar para no perder las mayúsculas del nombre real.
func (s *store) buscarCaneles(texto string, limite int) []string {
	if s == nil || s.db == nil {
		return nil
	}
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return nil
	}
	if limite <= 0 {
		limite = 50
	}
	busqueda := strings.ToLower(texto)

	var salida []string
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChannels))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var reg canalRegistro
			if err := json.Unmarshal(v, &reg); err != nil {
				return nil
			}
			if strings.Contains(strings.ToLower(reg.Nombre), busqueda) {
				salida = append(salida, reg.Nombre)
			}
			return nil
		})
	})
	sort.Strings(salida)
	if len(salida) > limite {
		salida = salida[:limite]
	}
	return salida
}

// eventosRecientes devuelve los últimos eventos del histórico.
func (s *store) eventosRecientes(n int) []eventoHistorial {
	if s == nil || s.db == nil || n <= 0 {
		return nil
	}
	var salida []eventoHistorial
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHistory))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, v := c.Last(); k != nil && n > 0; k, v = c.Prev() {
			var ev eventoHistorial
			if err := json.Unmarshal(v, &ev); err != nil {
				continue
			}
			salida = append(salida, ev)
			n--
		}
		return nil
	})
	return salida
}

// resumen es el estado global para la UI y /api/logs.
// resumen es el estado global para la UI y /api/logs.
func (s *store) resumen() map[string]any {
	if s == nil || s.db == nil {
		return map[string]any{"disponible": false}
	}
	var canales, vistos, soloUnaFuente int
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChannels))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var reg canalRegistro
			if err := json.Unmarshal(v, &reg); err != nil {
				return nil
			}
			canales++
			if reg.Visto {
				vistos++
			}
			if len(reg.Fuentes) == 1 {
				soloUnaFuente++
			}
			return nil
		})
	})
	return map[string]any{
		"disponible":    true,
		"canales":       canales,
		"vistos":        vistos,
		"desaparecidos": canales - vistos,
		"de_una_fuente": soloUnaFuente,
		"eventos":       s.contarEventos(),
		"bytes":         tamanoFichero(s.rutaActual()),
	}
}

// contarEventos cuenta las entradas del histórico, que es lo que acota el
// disco. db.Stats().KeyN daría todas las claves de todos los buckets, que no
// es lo que se quiere mostrar.
func (s *store) contarEventos() int {
	if s == nil || s.db == nil {
		return 0
	}
	total := 0
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHistory))
		if b == nil {
			return nil
		}
		total = contarClaves(b)
		return nil
	})
	return total
}

func (s *store) rutaActual() string {
	if s == nil {
		return ""
	}
	if s.rutaOverride != "" {
		return s.rutaOverride
	}
	return storePath()
}

func clavesOrdenadas(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	salida := make([]string, 0, len(m))
	for k := range m {
		salida = append(salida, k)
	}
	sort.Strings(salida)
	return salida
}

// cargarListadoDesdeDisco pone el listado guardado en broadcasterToAcestream
// para que la web pueda servir de inmediato. No sustituye a la red: cuando las
// fuentes respondan, FetchUpdatedList lo reemplaza con datos frescos.
//
// Es la pieza que quita el arranque bloqueado. Sin base de datos no hace nada
// y todo sigue como antes.
func cargarListadoDesdeDisco() {
	if storeGlobal == nil {
		return
	}
	guardados := storeGlobal.cargarCanales()
	if len(guardados) == 0 {
		return
	}

	// La base guarda el listado completo (sin pasar por el gateway estricto,
	// que deja fuera la mayoría), así que no se puede reutilizar
	// updateBroadcasterMapWithGatewayTolerant: eso filtraría otra vez y el
	// arranque rápido serviría 121 canales de 1651.
	//
	// Se carga tal cual, con el mismo formato de enlaces que produce
	// transformUriSafeBroadcasters.
	tmp := make(map[string]BroadcasterInfo, len(guardados))
	for nombre, hashes := range guardados {
		enlaces := make([]string, len(hashes))
		for i, h := range hashes {
			enlaces[i] = "acestream://" + h
		}
		tmp[nombre] = BroadcasterInfo{Name: nombre, Links: enlaces}
	}
	broadcasterToAcestream = tmp

	log.Printf("⚡ Listado cargado desde disco: %d canales (se refrescará con las fuentes)",
		len(broadcasterToAcestream))
}

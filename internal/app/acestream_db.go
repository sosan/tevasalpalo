package app

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
	"math"
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
	bucketHashes   = "hashes"
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

	// Permanente es para los canales de la base sembrada (ver semilla_base.go).
	// Existen para que una instalación nueva no arranque con la lista vacía, y
	// por eso no se retiran nunca: si ninguna fuente real los publica, siguen
	// ahí como base. La capa de calidad de hashes es la que se encarga de
	// apartar los que están muertos.
	Permanente bool `json:"permanente,omitempty"`
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

// hashRegistro es lo que se sabe de un infohash concreto.
//
// De aquí sale la decisión que más tiempo ahorra: qué hash se le pasa al
// player. Antes probaba el primero de la lista y, si estaba muerto, esperaba
// el timeout entero (hasta 2 minutos) antes de saberlo. Con disponibilidad y
// con el propio histórico de intentos, los hashes muertos se pueden poner al
// final o directamente dejar de ofrecer.
type hashRegistro struct {
	Hash string `json:"hash"`

	// Disponibilidad que publica la API de AceStream. Va de 0 a 1 y es el dato
	// de campo: el motor dice si ese stream tiene peers ahora mismo. -1 o
	// desconocido se representa con Conocida=false.
	Disponibilidad float64   `json:"disponibilidad"`
	Conocida       bool      `json:"conocida"`
	MedidaEn       time.Time `json:"medida_en"`

	// Lo que sabemos nosotros, que no depende de que haya API. Un stream puede
	// estar disponible y aun así caerse por el camino: aquí se ve.
	Intentos      int       `json:"intentos"`
	OK            int       `json:"ok"`
	Fallos        int       `json:"fallos"`
	FallosSeguros int       `json:"fallos_seguros"`
	UltimaOK      time.Time `json:"ultima_ok"`
	UltimoFallo   time.Time `json:"ultimo_fallo"`
	UltimoIntento time.Time `json:"ultimo_intento"`

	// Evidencia propia: lo que el proxy ha visto pasar con este enlace, de
	// -1 (claramente roto) a +1 (funcionando a pleno). Es nuestra sustituta de
	// la availability de la API cuando esa no está disponible, y además mide
	// algo que la API no mide: no solo si hay pares ahora, sino si este
	// stream funciona desde esta máquina.
	//
	// Va separada de Disponibilidad a propósito: una es una medida nuestra con
	// antigüedad, la otra un número de fuera. availabilityEfectiva() las
	// resuelve.
	Propia   float64   `json:"propia"`
	PropiaEn time.Time `json:"propia_en"`

	// Contadores por tipo de señal, solo para poder explicar una puntuación.
	PlaylistsOK  int `json:"playlists_ok"`
	PlaylistsVnc int `json:"playlists_vacias"`
	SegmentosOK  int `json:"segmentos_ok"`
	SegmentosErr int `json:"segmentos_err"`
}

// Señales que el proxy puede observar sobre un enlace, y lo que pesa cada una.
//
// El caso importante es el que NO pesa: un manifest retenido (el engine no
// responde hasta que aparecen pares) queda en cero. No distingue "arrancando en
// frío" de "canal muerto", y contar los timeouts como fallo es lo que dejó
// esta capa muerta: mataba hashes que funcionaban y, para no hacerlo, no se
// anotaba nada en absoluto.
const (
	evPlaylistOK    = "playlist_ok"
	evPlaylistVacia = "playlist_vacia"
	evSegmentoOK    = "segmento_ok"
	evSegmentoErr   = "segmento_err"
)

var pesoEvidencia = map[string]float64{
	evPlaylistOK: 0.6,
	// Un manifest sin una sola URL no tiene nada que reproducir. Pesa poco a
	// propósito: solo uno no demuestra nada, pero cuatro seguidos sí.
	evPlaylistVacia: -0.15,
	// Llegó media: la prueba más fuerte de que el stream está vivo.
	evSegmentoOK: 0.9,
	// El engine no pudo servir un segmento concreto. Menos de la mitad que el
	// acierto porque un 404 aislado también es ruido de red.
	evSegmentoErr: -0.45,
}

const (
	// vidaMediaEvidencia es cada cuánto se mitad la señal propia. Un enlace que
	// funcionó hace un día dice muy poco de cómo está ahora, así que su valor
	// tiende a 0, que es "no se sabe", no "está mal".
	vidaMediaEvidencia = 6 * time.Hour

	// disponibilidadMuerte es el corte en la escala común 0..1. Por debajo se
	// considera muerto: dos fallos de segmento seguidos dejan la evidencia en
	// -0.9, es decir 0.05 en esta escala.
	disponibilidadMuerte = 0.2

	// maximoEvidenciaAcumulada satura la suma para que un enlace que lleva
	// mucho tiempo bien no bloquee a los que llegan después.
	maximoEvidenciaAcumulada = 1.0
)

// fallosParaDarPorMuerto es el número de fallos seguidos a partir del cual un
// hash se considera muerto. Tres strikes: uno puede ser un fallo de red, y a
// partir del segundo casi siempre es que el stream ya no emite.
const fallosParaDarPorMuerto = 3

// enEscala01 convierte la evidencia bruta (-1..1) a la escala de la API
// (0..1), donde 0 significa "sin peers ahora mismo" y 1 "a pleno rendimiento".
// Mezclar las dos en un solo número es lo que hace falta para que "la API dice
// 0" y "nuestros fallos se acumulan" signifiquen lo mismo.
func enEscala01(e float64) float64 {
	v := (e + 1) / 2
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// evidenciaVigente es la evidencia propia descontando el tiempo transcurrido.
// Devuelve false si nunca hemos observado nada: sin datos no se puede afirmar
// nada, y un hash recién visto tiene que contar como vivo.
func (r hashRegistro) evidenciaVigente(ahora time.Time) (float64, bool) {
	if r.PropiaEn.IsZero() {
		return 0, false
	}
	edad := ahora.Sub(r.PropiaEn)
	if edad < 0 {
		edad = 0
	}
	decay := math.Pow(0.5, edad.Seconds()/vidaMediaEvidencia.Seconds())
	return r.Propia * decay, true
}

// disponibilidadEfectiva devuelve la disponibilidad unificada en 0..1 y de
// dónde sale. Gana la propia si es más reciente: está medida sobre este stream
// y desde esta máquina, no copiada de otra parte.
func (r hashRegistro) disponibilidadEfectiva(ahora time.Time) (float64, bool, string) {
	propia, hayPropia := r.evidenciaVigente(ahora)
	propia01 := enEscala01(propia)
	switch {
	case hayPropia && r.Conocida:
		if r.PropiaEn.After(r.MedidaEn) {
			return propia01, true, "propia"
		}
		return r.Disponibilidad, true, "api"
	case hayPropia:
		return propia01, true, "propia"
	case r.Conocida:
		return r.Disponibilidad, true, "api"
	}
	return 0, false, ""
}

// vivo indica si el hash merece la pena. Se apoya en tres señales, de la más
// fiable a la menos:
//
//   - fallos seguidos: si acaba de fallar tres veces, está muerto y da igual
//     lo que diga la API;
//   - la disponibilidad unificada, la nuestra o la de la API, cuando hay;
//   - nuestra propia tasa de acierto, cuando hay intentos suficientes.
//
// Un hash sin ningún dato (recién visto, nunca probado) se considera vivo:
// no se puede descartar un stream solo por no saber nada de él.
func (r hashRegistro) vivo() bool {
	return r.vivoEn(time.Now())
}

func (r hashRegistro) vivoEn(ahora time.Time) bool {
	if r.FallosSeguros >= fallosParaDarPorMuerto {
		return false
	}
	if v, conocida, _ := r.disponibilidadEfectiva(ahora); conocida && v <= disponibilidadMuerte {
		return false
	}
	return true
}

// puntuar ordena los hashes de mejor a peor. A igualdad de todo, el orden es
// el de entrada, que viene de las fuentes y es el que conoce mejor el
// proveedor.
func (r hashRegistro) puntuar() float64 {
	return r.puntuarEn(time.Now())
}

// puntuarEn es puntuar con el instante explícito, para poder comprobar cómo
// decae una puntuación vieja sin tener que esperar un día.
func (r hashRegistro) puntuarEn(ahora time.Time) float64 {
	p := 0.5 // sin datos: neutro, ni sube ni baja
	if v, conocida, _ := r.disponibilidadEfectiva(ahora); conocida {
		p = v
	}
	if r.Intentos >= 3 {
		exito := float64(r.OK) / float64(r.Intentos)
		// El dato propio pesa lo mismo que el de la API: es el que de verdad
		// ha visto funcionar este hash desde esta máquina.
		p = (p + exito) / 2
	}
	// Los fallos seguidos hunden a los que acaban de morir, sin llegar a
	// descartar: la caída se comprueba al final.
	p -= float64(r.FallosSeguros) * 0.1
	if p < 0 {
		p = 0
	}
	return p
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
		for _, b := range []string{bucketChannels, bucketSources, bucketHistory, bucketHashes, bucketMeta} {
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
			for _, b := range []string{bucketChannels, bucketSources, bucketHistory, bucketHashes} {
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
			// Un canal de la base sembrada que además publica alguna fuente
			// real pasa a mandar esa fuente, pero no pierde el respaldo de la
			// base: si dejara de aparecer, no se retira.
			if previa.Permanente {
				reg.Permanente = true
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
			// La base sembrada no se retira nunca: existe justo para que
			// siempre haya algo que ofrecer. Si un enlace suyo está muerto,
			// lo aparta la puntuación por disponibilidad, no esta regla.
			if reg.Permanente {
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
// ordenarListadoEn vivo deja, en cada canal, el hash más probable primero.
//
// Es el uso de la disponibilidad: antes el player probaba el primero de la
// lista y se comía el timeout entero si ese hash estaba muerto. Ahora prueba
// el que se sabe que funciona.
func ordenarListadoEnVivo(mapa map[string]BroadcasterInfo) {
	if storeGlobal == nil || len(mapa) == 0 {
		return
	}
	// Se usa el mutex del store y no el de la carga de fuentes: aquí ya
	// han terminado todas las goroutines, pero el store se consulta desde
	// varias partes y no se quiere que se solapen.
	storeGlobal.mu.Lock()
	defer storeGlobal.mu.Unlock()
	ordenados, muertos := 0, 0
	for nombre, info := range mapa {
		if len(info.Links) < 2 {
			continue
		}
		nuevos, descartados := storeGlobal.ordenarHashesPuntuados(info.Links)
		muertos += descartados
		if len(nuevos) == len(info.Links) {
			distintos := false
			for i := range nuevos {
				if nuevos[i] != info.Links[i] {
					distintos = true
					break
				}
			}
			if !distintos {
				continue
			}
		}
		enlaces := make([]string, len(nuevos))
		for i, h := range nuevos {
			enlaces[i] = "acestream://" + h
		}
		info.Links = enlaces
		mapa[nombre] = info
		ordenados++
	}
	if ordenados > 0 || muertos > 0 {
		log.Printf("🎯 Hashes reordenados por disponibilidad en %d canales (%d enlaces muertos descartados)",
			ordenados, muertos)
	}
}

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
	ordenarListadoEnVivo(broadcasterToAcestream)

	log.Printf("⚡ Listado cargado desde disco: %d canales (se refrescará con las fuentes)",
		len(broadcasterToAcestream))
}

// registrarHashResultado anota que un hash funcionó o falló. Es la fuente de
// datos que no depende de ninguna API: el propio proxy la alimenta cada vez que
// el player pide un stream.
//
// No bloquea el arranque del vídeo: si la escritura falla, el playback sigue
// igual. Perder una anotación no puede tumbar la reproducción.
// registrarEvidencia anota una observación sobre un hash. Es la única vía por
// la que la base aprende cómo son los enlaces, y la que hace que ordenar y
// descartar hashes dejen de ser un adorno: sin ella el bucket se queda vacío
// y el.player sigue probando el primer hash de la lista a ciegas.
//
// Un manifiesto retenido (timeout con respuesta 204) NO se anota: no sabemos
// si es que aún no han entrado pares o que el canal está muerto. Anotarlo
// como fallo era el error que motivó excluir los manifests, y excluirlos
// fue lo que dejó este código sin usar.
func (s *store) registrarEvidencia(hash, evidencia string) {
	peso, ok := pesoEvidencia[evidencia]
	if !ok {
		return
	}
	hash = normalizarHash(hash)
	if s == nil || s.db == nil || hash == "" {
		return
	}
	ahora := time.Now().UTC()
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHashes))
		if b == nil {
			return nil
		}
		clave := []byte(hash)
		reg := hashRegistro{Hash: hash}
		if v := b.Get(clave); v != nil {
			if err := json.Unmarshal(v, &reg); err != nil {
				reg = hashRegistro{Hash: hash}
			}
		}
		reg.Intentos++
		reg.UltimoIntento = ahora

		// La evidencia es una suma que satura: una racha larga de aciertos o de
		// fallos acerca el valor al extremo sin pasarse nunca.
		reg.Propia += peso
		if reg.Propia > maximoEvidenciaAcumulada {
			reg.Propia = maximoEvidenciaAcumulada
		}
		if reg.Propia < -maximoEvidenciaAcumulada {
			reg.Propia = -maximoEvidenciaAcumulada
		}
		reg.PropiaEn = ahora

		switch evidencia {
		case evPlaylistOK:
			reg.PlaylistsOK++
			reg.OK++
			reg.FallosSeguros = 0
			reg.UltimaOK = ahora
		case evPlaylistVacia:
			reg.PlaylistsVnc++
			reg.Fallos++
			reg.FallosSeguros++
			reg.UltimoFallo = ahora
		case evSegmentoOK:
			reg.SegmentosOK++
			reg.OK++
			reg.FallosSeguros = 0
			reg.UltimaOK = ahora
		case evSegmentoErr:
			reg.SegmentosErr++
			reg.Fallos++
			reg.FallosSeguros++
			reg.UltimoFallo = ahora
		}

		// Un enlace que acaba de entregar algo no puede figurar como caído por
		// una medición vieja de la API.
		if peso > 0 && reg.Conocida && reg.Disponibilidad <= 0 {
			reg.Disponibilidad = 0.5
		}

		raw, err := json.Marshal(reg)
		if err != nil {
			return err
		}
		return b.Put(clave, raw)
	})
	if err != nil {
		log.Printf("⚠️  base de datos: no se pudo registrar evidencia de %s: %v", hash[:8], err)
	}
}

// registrarHashResultado conserva la vía anterior (éxito/fallo de una petición
// de datos) y la traduce a evidencia, para no perder llamadas ya hechas.
func (s *store) registrarHashResultado(hash string, ok bool) {
	if ok {
		s.registrarEvidencia(hash, evSegmentoOK)
		return
	}
	s.registrarEvidencia(hash, evSegmentoErr)
}

// normalizarHash deja el infohash en minúsculas y sin acestream:// delante, que
// es como llega desde unas rutas y desde otras.
func normalizarHash(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	for _, pre := range []string{"acestream://", "p; acestream://", "p;", "acestream:"} {
		h = strings.TrimPrefix(h, pre)
	}
	h = strings.TrimPrefix(h, "/")
	if i := strings.IndexAny(h, "?#"); i >= 0 {
		h = h[:i]
	}
	return strings.TrimSpace(h)
}

// importarDisponibilidad carga el array JSON de la API de AceStream
// (infohash, name, availability, availability_updated_at, categories) y lo
// mezcla con lo que ya sabíamos. Es el formato de parser/input.txt.
//
// Es idempotente: importar el mismo fichero dos veces no cambia nada. Una
// medición más antigua que la que ya tenemos no pisa la nueva, porque el
// timestamp de la API va en segundos y puede venir de una copia guardada.
func (s *store) importarDisponibilidad(cuerpo []byte) (nuevos, actualizados, ignorados int, err error) {
	if s == nil || s.db == nil {
		return 0, 0, 0, fmt.Errorf("sin base de datos")
	}
	var entrada []struct {
		Infohash     string   `json:"infohash"`
		Name         string   `json:"name"`
		Availability *float64 `json:"availability"`
		Actualizado  int64    `json:"availability_updated_at"`
		Categories   []string `json:"categories"`
	}
	if err := json.Unmarshal(cuerpo, &entrada); err != nil {
		return 0, 0, 0, fmt.Errorf("el JSON no tiene el formato esperado: %w", err)
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHashes))
		if b == nil {
			return nil
		}
		for _, it := range entrada {
			hash := normalizarHash(it.Infohash)
			if hash == "" {
				ignorados++
				continue
			}
			clave := []byte(hash)
			reg := hashRegistro{Hash: hash}
			existente := b.Get(clave)
			if existente != nil {
				if err := json.Unmarshal(existente, &reg); err != nil {
					reg = hashRegistro{Hash: hash}
				}
			}

			medida := time.Time{}
			if it.Actualizado > 0 {
				medida = time.Unix(it.Actualizado, 0).UTC()
			}

			switch {
			case it.Availability == nil:
				ignorados++
				continue
			case reg.Conocida && !medida.IsZero() && !reg.MedidaEn.IsZero() && medida.Before(reg.MedidaEn):
				// Medición más antigua que la que ya tenemos: la API se
				// actualiza porScrape y esto puede ser una copia vieja.
				ignorados++
				continue
			}

			antes := reg.Disponibilidad
			reg.Conocida = true
			reg.Disponibilidad = *it.Availability
			reg.MedidaEn = medida

			raw, err := json.Marshal(reg)
			if err != nil {
				return err
			}
			if err := b.Put(clave, raw); err != nil {
				return err
			}
			if existente == nil {
				nuevos++
			} else if antes != reg.Disponibilidad {
				actualizados++
			}
		}
		return nil
	})
	return nuevos, actualizados, ignorados, err
}

// ordenarHashesPuntuados devuelve los hashes de un canal ordenados de mejor a
// peor, y descarta los que la base considera muertos. Devuelve además cuántos
// ha quitado, para poder decirlo en el log en vez de dejar un contador que
// siempre valía cero.
//
// Es el uso principal de todo esto: en vez de probar el primero de la lista y
// comerse el timeout, se prueba el que se sabe que funciona.
//
// El descarte solo ocurre si queda al menos un hash vivo. Un canal con todos
// sus enlaces muertos se deja como estaba: el player puede probarlos y suena
// un aviso de que no hay nada que probar, que es distinto de no tener ningún
// enlace con el que responder.
func (s *store) ordenarHashesPuntuados(hashes []string) ([]string, int) {
	if s == nil || s.db == nil || len(hashes) <= 1 {
		return hashes, 0
	}
	type conPuntos struct {
		hash  string
		punto float64
		pos   int
		vivo  bool
	}
	items := make([]conPuntos, 0, len(hashes))

	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHashes))
		if b == nil {
			return nil
		}
		for i, h := range hashes {
			reg := hashRegistro{Hash: h}
			if v := b.Get([]byte(normalizarHash(h))); v != nil {
				if err := json.Unmarshal(v, &reg); err != nil {
					reg = hashRegistro{Hash: h}
				}
			}
			items = append(items, conPuntos{hash: h, punto: reg.puntuar(), pos: i, vivo: reg.vivo()})
		}
		return nil
	})

	salida := make([]string, 0, len(items))
	muertos := 0
	// Stable sort por puntos: a igualdad mantiene el orden de las fuentes.
	sort.SliceStable(items, func(a, b int) bool {
		return items[a].punto > items[b].punto
	})
	for _, it := range items {
		if !it.vivo {
			muertos++
			continue
		}
		salida = append(salida, it.hash)
	}
	if len(salida) == 0 {
		return hashes, 0
	}
	return salida, muertos
}

// hashesDeCanal devuelve los hashes de un canal tal como están guardados, para
// poder explicar en detalle por qué se prueban en ese orden.
func (s *store) hashesDeCanal(nombre string) []string {
	if s == nil || s.db == nil || nombre == "" {
		return nil
	}
	var hashes []string
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChannels))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(nombre))
		if v == nil {
			return nil
		}
		var reg canalRegistro
		if err := json.Unmarshal(v, &reg); err != nil {
			return nil
		}
		hashes = reg.Hashes
		return nil
	})
	return hashes
}

// hashesMuertos lista los hashes que la base considera muertos, para poder
// revisarlos y para vaciar de la lista los que sobran.
//
// Usa vivo() y solo vivo(): cuando esta función exigía además un número de
// fallos seguidos, decía que un enlace estaba muerto por un camino y el
// ranking lo descartaba por otro. "Muerto" tiene que significar una sola cosa,
// que es la que decide qué se ofrece.
func (s *store) hashesMuertos() []string {
	if s == nil || s.db == nil {
		return nil
	}
	var salida []string
	ahora := time.Now().UTC()
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHashes))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var reg hashRegistro
			if err := json.Unmarshal(v, &reg); err != nil {
				return nil
			}
			if !reg.vivoEn(ahora) {
				salida = append(salida, reg.Hash)
			}
			return nil
		})
	})
	sort.Strings(salida)
	return salida
}

// hash devuelve el registro de un infohash, o uno vacío si no se sabe nada.
// Nunca devuelve nil, para que el llamante no tenga que comprobarlo.
func (s *store) hash(h string) hashRegistro {
	h = normalizarHash(h)
	reg := hashRegistro{Hash: h}
	if s == nil || s.db == nil || h == "" {
		return reg
	}
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHashes))
		if b == nil {
			return nil
		}
		if v := b.Get([]byte(h)); v != nil {
			if err := json.Unmarshal(v, &reg); err != nil {
				reg = hashRegistro{Hash: h}
			}
		}
		return nil
	})
	return reg
}

// actualizarDisponibilidad fija la disponibilidad de un hash. Es la operación
// que usa internamente la importación por lotes.
func (s *store) actualizarDisponibilidad(h string, disponibilidad float64, medida time.Time) error {
	h = normalizarHash(h)
	if s == nil || s.db == nil || h == "" {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHashes))
		if b == nil {
			return nil
		}
		clave := []byte(h)
		reg := hashRegistro{Hash: h}
		if v := b.Get(clave); v != nil {
			if err := json.Unmarshal(v, &reg); err != nil {
				reg = hashRegistro{Hash: h}
			}
		}
		reg.Conocida = true
		reg.Disponibilidad = disponibilidad
		reg.MedidaEn = medida
		raw, err := json.Marshal(reg)
		if err != nil {
			return err
		}
		return b.Put(clave, raw)
	})
}

// resumenHashes da el estado global para /api/db.
// resumenHashes cuenta cómo está la capa de calidad de los enlaces. Es lo que
// dice si el bucket hashes está aprendiendo de verdad o si lleva meses vacío,
// que era el estado en el que estaba: la base parecía saber de enlaces y no
// sabía nada, porque nada la alimentaba.
func (s *store) resumenHashes() map[string]any {
	if s == nil || s.db == nil {
		return map[string]any{"disponible": false}
	}
	ahora := time.Now().UTC()
	var (
		total, conocidos, muertos, vivos, conFallos int
		conPropia, soloPropia, soloAPI              int
		playlists, segmentos                        int
	)
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHashes))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var reg hashRegistro
			if err := json.Unmarshal(v, &reg); err != nil {
				return nil
			}
			total++
			if reg.Conocida {
				conocidos++
			}
			if reg.FallosSeguros > 0 {
				conFallos++
			}
			if _, hayPropia := reg.evidenciaVigente(ahora); hayPropia {
				conPropia++
			}
			_, _, origen := reg.disponibilidadEfectiva(ahora)
			switch origen {
			case "propia":
				soloPropia++
			case "api":
				soloAPI++
			}
			playlists += reg.PlaylistsOK
			segmentos += reg.SegmentosOK
			if reg.vivoEn(ahora) {
				vivos++
			} else {
				muertos++
			}
			return nil
		})
	})
	return map[string]any{
		"disponible":    true,
		"total":         total,
		"con_medida":    conocidos,
		"con_evidencia": conPropia,
		"solo_propia":   soloPropia,
		"solo_api":      soloAPI,
		"vivos":         vivos,
		"muertos":       muertos,
		"con_fallos":    conFallos,
		"playlists_ok":  playlists,
		"segmentos_ok":  segmentos,
		"umbral_muerto": disponibilidadMuerte,
		"vida_media_h":  int(vidaMediaEvidencia / time.Hour),
	}
}

// hashesOrdenadosPara devuelve los hashes de un canal ya puntuados, para
// consultar por qué se pone uno primero. Explica el desempate y de dónde sale
// cada dato, que es la diferencia entre "está mal puesto" y "no hay dato".
func (s *store) hashesOrdenadosPara(hashes []string) []map[string]any {
	ahora := time.Now().UTC()
	out := make([]map[string]any, 0, len(hashes))
	for _, h := range hashes {
		reg := s.hash(h)
		efectiva, conocida, origen := reg.disponibilidadEfectiva(ahora)
		out = append(out, map[string]any{
			"hash":             reg.Hash,
			"puntuacion":       reg.puntuar(),
			"vivo":             reg.vivoEn(ahora),
			"disponibilidad":   efectiva,
			"origen":           origen,
			"con_medida":       conocida,
			"evidencia":        reg.Propia,
			"evidencia_en":     reg.PropiaEn,
			"api":              reg.Disponibilidad,
			"api_conocida":     reg.Conocida,
			"intentos":         reg.Intentos,
			"ok":               reg.OK,
			"fallos":           reg.Fallos,
			"fallos_seguros":   reg.FallosSeguros,
			"playlists_ok":     reg.PlaylistsOK,
			"playlists_vacias": reg.PlaylistsVnc,
			"segmentos_ok":     reg.SegmentosOK,
			"segmentos_err":    reg.SegmentosErr,
		})
	}
	return out
}

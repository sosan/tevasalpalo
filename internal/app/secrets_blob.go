package app

// Contenedor cifrado para la configuración de secretos que se incrusta en el
// binario.
//
// Para qué: poder meter las credenciales de Infisical dentro de portable.exe sin
// que aparezcan en claro. Al arrancar se descifran en memoria, se usan para
// traer el resto de variables (los tokens de Grafana, por ejemplo) y se
// descartan.
//
// Lo que esto SÍ protege, y es real:
//
//   - El binario no contiene el token en claro: `strings portable.exe` no lo
//     encuentra. Tampoco aparece en un grep del repo, ni en los logs de
//     Actions, ni en un volcado de memoria.
//   - AES-256-GCM es autenticado: si alguien edita el contenedor, el descifrado
//     falla en vez de devolver basura.
//   - La clave no está en el código fuente, se mete al compilar.
//
// Lo que NO protege, y conviene no confundirse: la clave viaja en el mismo
// binario, así que quien se tome la molestia de invertir el descifrado la
// obtiene. Esto evita la filtración accidental, no el análisis de un atacante.
// Para una instalación seria, el token debe ir en el entorno o en
// settings.json.
//
// Formato: "v1:" + base64(salt || nonce || ciphertext||tag). El versionado
// permite cambiar de algoritmo más adelante sin romper los binarios viejos.
//
// Generación del contenedor:
//	CLAVE_SECRETOS=... INFISICAL_TOKEN=st.... INFISICAL_PROJECT_ID=... \
//	INFISICAL_ENV=prod INFISICAL_PATH=/streaming \
//	./portable --genera-secretos
// que imprime la línea de -ldflags lista para el build.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// versionContenedor identifica el formato. Si algún día se cambia el algoritmo,
// esta constante cambia y los binarios viejos siguen siendo legibles con su
// versión.
const versionContenedor = "v1"

// iteracionesPBKDF2 es alto a propósito: descifrar cuesta unos cientos de
// milisegundos una vez al arrancar, y una clave derivada rápido se sabe partir
// offline.
const iteracionesPBKDF2 = 210000

const (
	longitudClave = 32
	longitudSalt  = 16
	longitudNonce = 12
)

// embebidoSecretos es el contenedor cifrado, inyectado al compilar con:
//
//	-ldflags "-X 'tevasalpalo/internal/app.embebidoSecretos=v1:BASE64'"
var embebidoSecretos = ""

// claveSecretos es la contraseña de la que se deriva la clave. También se
// inyecta al compilar:
//
//	-ldflags "-X 'tevasalpalo/internal/app.claveSecretos=CLAVE'"
var claveSecretos = ""

// configInfisical es lo que se guarda cifrado dentro del binario.
type configInfisical struct {
	Token     string `json:"token"`
	ProjectID string `json:"project_id"`
	Env       string `json:"env"`
	Path      string `json:"path"`
	// URL solo para self-hosted; vacío significa el de Infisical Cloud.
	URL string `json:"url,omitempty"`
	// Variables extra que se aplican al entorno, por si prefieres no
	// depender de que estén guardadas en Infisical.
	Variables map[string]string `json:"variables,omitempty"`
	// Salt propio del hash de IP, si quieres que no dependa de Infisical.
	HashSalt string `json:"hash_salt,omitempty"`
}

// descifrarContenedor abre el contenedor y devuelve la configuración.
func descifrarContenedor(blob, clave string) (*configInfisical, error) {
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return nil, errors.New("no hay contenedor cifrado en el binario")
	}

	version, resto, ok := strings.Cut(blob, ":")
	if !ok {
		return nil, errors.New(`formato inválido: se esperaba "v1:base64"`)
	}
	if version != versionContenedor {
		return nil, fmt.Errorf("versión de contenedor desconocida: %q", version)
	}
	crudo, err := base64.StdEncoding.DecodeString(resto)
	if err != nil {
		return nil, fmt.Errorf("el contenedor no es base64 válido: %w", err)
	}
	if len(crudo) < longitudSalt+longitudNonce+16 {
		return nil, errors.New("el contenedor está truncado")
	}

	salt := crudo[:longitudSalt]
	nonce := crudo[longitudSalt : longitudSalt+longitudNonce]
	cifrado := crudo[longitudSalt+longitudNonce:]

	if clave == "" {
		return nil, errors.New("falta la clave de descifrado (main.claveSecretos)")
	}

	gcm, err := nuevoGCM(salt, clave)
	if err != nil {
		return nil, err
	}
	// GCM verifica la etiqueta: si alguien tocó el contenedor, esto falla.
	enclaro, err := gcm.Open(nil, nonce, cifrado, []byte(version))
	if err != nil {
		// No se distingue "clave equivocada" de "contenedor manipulado" a
		// propósito: dar el detalle ayudaría a ajustar un ataque.
		return nil, errors.New("no se pudo descifrar: la clave no es correcta o el contenedor fue modificado")
	}

	var cfg configInfisical
	if err := json.Unmarshal(enclaro, &cfg); err != nil {
		return nil, fmt.Errorf("el contenido descifrado no es la configuración esperada: %w", err)
	}
	borra(enclaro)
	return &cfg, nil
}

// borra un slice en memoria. Es mejor que nada (Go no garantiza el borrado, el
// GC puede copiar), pero evita que el token siga en el heap por casualidad.
func borra(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// nuevoGCM deriva la clave y construye el descifrador. Derivar en cada llamada
// es lo correcto: la clave nunca se guarda.
func nuevoGCM(salt []byte, clave string) (cipher.AEAD, error) {
	derivada, err := pbkdf2.Key(sha256.New, clave, salt, iteracionesPBKDF2, longitudClave)
	if err != nil {
		return nil, fmt.Errorf("no se pudo derivar la clave: %w", err)
	}
	defer borra(derivada)

	bloque, err := aes.NewCipher(derivada)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(bloque)
}

// cifrarContenedor es la operación inversa: la que usa --genera-secretos.
func cifrarContenedor(clave string, cfg configInfisical) (string, error) {
	if clave == "" {
		return "", errors.New("falta la clave")
	}
	salt := make([]byte, longitudSalt)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("no se pudo generar el salt: %w", err)
	}
	gcm, err := nuevoGCM(salt, clave)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, longitudNonce)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("no se pudo generar el nonce: %w", err)
	}

	enclaro, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	cifrado := gcm.Seal(nil, nonce, enclaro, []byte(versionContenedor))
	borra(enclaro)

	empaquetado := make([]byte, 0, longitudSalt+longitudNonce+len(cifrado))
	empaquetado = append(empaquetado, salt...)
	empaquetado = append(empaquetado, nonce...)
	empaquetado = append(empaquetado, cifrado...)

	return versionContenedor + ":" + base64.StdEncoding.EncodeToString(empaquetado), nil
}

// aplicarSecretosIncrustados descifra el contenedor y aplica sus variables al
// entorno sin pisar lo que ya esté definido: el contenedor rellena huecos, no
// decide. Así el entorno, o una rotación en Infisical, siguen mandando.
//
// Devuelve false si el binario no lleva secretos o si no se pudo abrir. En ese
// último caso la app arranca igual, solo que sin telemetría central.
func aplicarSecretosIncrustados() bool {
	if strings.TrimSpace(embebidoSecretos) == "" {
		return false
	}

	cfg, err := descifrarContenedor(embebidoSecretos, claveSecretos)
	if err != nil {
		// Un contenedor ilegible no puede impedir arrancar la app: se pierde la
		// telemetría, no el vídeo.
		fmt.Fprintf(os.Stderr, "⚠️  secretos incrustados: %v\n", err)
		return false
	}

	if cfg.Token != "" {
		setenvSiVacio("INFISICAL_TOKEN", cfg.Token)
	}
	if cfg.ProjectID != "" {
		setenvSiVacio("INFISICAL_PROJECT_ID", cfg.ProjectID)
	}
	if cfg.Env != "" {
		setenvSiVacio("INFISICAL_ENV", cfg.Env)
	}
	if cfg.Path != "" {
		setenvSiVacio("INFISICAL_PATH", cfg.Path)
	}
	if cfg.URL != "" {
		setenvSiVacio("INFISICAL_URL", cfg.URL)
	}
	if cfg.HashSalt != "" {
		setenvSiVacio("TELEMETRY_HASH_SALT", cfg.HashSalt)
	}
	for k, v := range cfg.Variables {
		setenvSiVacio(k, v)
	}
	return true
}

// setenvSiVacio define una variable solo si no está ya.
func setenvSiVacio(nombre, valor string) {
	if valor == "" {
		return
	}
	if _, ya := os.LookupEnv(nombre); ya {
		return
	}
	_ = os.Setenv(nombre, valor)
}

// construirSnippetBuild monta el bloque de shell que se imprime para compilar
// en local. Va en su propia función para poder testearlo: esto ya ha fallado
// dos veces por cosas que solo se ven al ejecutar el bloque de verdad.
func construirSnippetBuild(valor string) string {
	// El + inicial no es opcional: sin él, date dice "fecha no válida" y
	// VERSION_BUILD acaba vacío, que es peor que no ponerlo porque el binario
	// compila igual pero la comprobación de actualizaciones se queda tonta.
	const formatoFecha = "+%Y-%m-%dT%H:%M:%SZ"

	return `   VERSION_BUILD=$(date -u "` + formatoFecha + `")
   TELEMETRY_LDFLAGS="` + valor + `"
   go build -ldflags="-X 'tevasalpalo/update.VersionBuild=${VERSION_BUILD}' ${TELEMETRY_LDFLAGS}" -o ./build/portable ./cmd/portable
`
}

// generarSecretosIncrustados implementa la bandera --genera-secretos: lee la
// configuración del entorno, la cifra y escribe por stdout la línea de -ldflags
// para el build.
//
// Está en el propio ejecutable y no en una herramienta aparte a propósito:
// así quien tenga el portable puede rotar los secretos sin necesidad del
// código fuente, y la bandera viaja con la versión que produjo el contenedor
// (mismo formato que espera el build).
func generarSecretosIncrustados() error {
	clave := os.Getenv("CLAVE_SECRETOS")
	if clave == "" {
		return errors.New("falta CLAVE_SECRETOS")
	}

	cfg := configInfisical{
		Token:     os.Getenv("INFISICAL_TOKEN"),
		ProjectID: os.Getenv("INFISICAL_PROJECT_ID"),
		Env:       os.Getenv("INFISICAL_ENV"),
		Path:      os.Getenv("INFISICAL_PATH"),
		URL:       os.Getenv("INFISICAL_URL"),
		HashSalt:  os.Getenv("TELEMETRY_HASH_SALT"),
	}
	if v := os.Getenv("TELEMETRY_VARIABLES"); v != "" {
		cfg.Variables = map[string]string{}
		// Formato: CLAVE=valor,OTRA=valor2
		for _, par := range strings.Split(v, ",") {
			if k, val, ok := strings.Cut(par, "="); ok {
				cfg.Variables[k] = val
			}
		}
	}

	if cfg.Token == "" || cfg.ProjectID == "" || cfg.Env == "" {
		return errors.New("hacen falta al menos INFISICAL_TOKEN, INFISICAL_PROJECT_ID e INFISICAL_ENV")
	}

	blob, err := cifrarContenedor(clave, cfg)
	if err != nil {
		return err
	}

	// Esto es literalmente el valor del secret TELEMETRY_LDFLAGS, sin
	// envoltorio de -ldflags ni comillas: copiarlo tal cual entre los
	// marcadores evita el error clásico de dejar una comilla pegada.
	valor := fmt.Sprintf("-X 'tevasalpalo/internal/app.embebidoSecretos=%s' -X 'tevasalpalo/internal/app.claveSecretos=%s'", blob, clave)

	fmt.Println("1) Valor del secret de GitHub Actions (TELEMETRY_LDFLAGS):")
	fmt.Println()
	fmt.Println("┌" + strings.Repeat("─", 76) + "┐")
	fmt.Println("│ " + valor)
	fmt.Println("└" + strings.Repeat("─", 76) + "┘")
	fmt.Println()
	fmt.Println("2) Build local:")
	fmt.Println()
	// OJO: aquí no se pone $(date ...) dentro de las comillas de -X. El
	// parser de -ldflags de Go trata '...' como un único token, así que
	// cualquier espacio dentro (y `date -u +%Y` tiene) rompe el definición y
	// el linker se queja con el volcado de su usage.
	snippetBuild := construirSnippetBuild(valor)

	// El valor de TELEMETRY_LDFLAGS va entre comillas dobles porque él mismo
	// lleva comillas simples dentro; con simples, la asignación se cortaría
	// por la mitad.
	//
	// WriteString y no Print, con la fecha en una constante: el %d del formato
	// no es una directiva de formato, pero vet lo toma por una y obligaría a
	// escaparlo.
	os.Stdout.WriteString(snippetBuild)
	fmt.Println()
	fmt.Println("   En CI ya está hecho: buidandtest.yaml concatena ${TELEMETRY_LDFLAGS} a los -ldflags.")

	if avisos := revisarPlaceholders(clave, cfg); len(avisos) > 0 {
		fmt.Println()
		fmt.Println("⚠️  Revisa antes de usar esto:")
		for _, a := range avisos {
			fmt.Println("   - " + a)
		}
	}

	fmt.Println()
	fmt.Println("Recuerda: la clave va dentro del binario. Esto evita la filtración")
	fmt.Println("accidental (strings, grep, logs de CI), no el análisis de alguien")
	fmt.Println("que se lo tome en serio.")
	return nil
}

// revisarPlaceholders busca valores de relleno que se cuelan sin querer. Un
// token o una clave de ejemplo siguen funcionando: cifran, compilan y auth
// fallan más tarde con un error que no señala la causa.
func revisarPlaceholders(clave string, cfg configInfisical) []string {
	marcas := []string{
		"TU-", "TU_", "pega-aqui", "PEGA_AQUI", "cambia-esto", "CAMBIAR",
		"changeme", "changethis", "EXAMPLE", "example", "TODO", "<", "xxx", "XXXX",
	}
	var avisos []string

	revisa := func(nombre, valor string) {
		for _, m := range marcas {
			if strings.Contains(valor, m) {
				avisos = append(avisos, nombre+" parece un valor de relleno (contiene \""+m+"\"): no funcionará tal cual")
				return
			}
		}
		if len(valor) < 16 && valor != "" {
			avisos = append(avisos, nombre+" es muy corto ("+strconv.Itoa(len(valor))+" caracteres)")
		}
	}

	revisa("CLAVE_SECRETOS", clave)
	if cfg.Token != "" {
		revisa("INFISICAL_TOKEN", cfg.Token)
	}
	if cfg.HashSalt != "" {
		revisa("TELEMETRY_HASH_SALT", cfg.HashSalt)
	}
	return avisos
}

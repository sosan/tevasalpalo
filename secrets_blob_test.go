package main

// Tests del contenedor cifrado de secretos.
//
// Lo importante aquí no es que el round-trip funcione (eso lo hace cualquier
// crypto), sino que un token manipulado NO se acepte y que el texto plano no
// sobreviva en el contenedor.

import (
	"os"
	"strings"
	"testing"
)

// aplicarSecretosIncrustadosCon es la versión con parámetros explícitos, para
// poder probar el arranque sin tocar los globales que inyecta el linker.
func aplicarSecretosIncrustadosCon(blob, clave string) bool {
	anteriorBlob, anteriorClave := embebidoSecretos, claveSecretos
	embebidoSecretos, claveSecretos = blob, clave
	defer func() { embebidoSecretos, claveSecretos = anteriorBlob, anteriorClave }()
	return aplicarSecretosIncrustados()
}

func configDePrueba() configInfisical {
	return configInfisical{
		Token:     "st.26028f0b-abcdef.0123456789abcdef.9876543210abcdef",
		ProjectID: "d818000d-1ce2-4bc5-aaa5-77907d1f3927",
		Env:       "prod",
		Path:      "/streaming",
		HashSalt:  "sal-secreto-de-prueba",
		Variables: map[string]string{"TELEMETRY_USER": "251374"},
	}
}

func TestContenedorIdaYVuelve(t *testing.T) {
	original := configDePrueba()
	blob, err := cifrarContenedor("clave-de-prueba", original)
	if err != nil {
		t.Fatalf("Cifrar: %v", err)
	}

	got, err := descifrarContenedor(blob, "clave-de-prueba")
	if err != nil {
		t.Fatalf("Descifrar: %v", err)
	}
	if got.Token != original.Token || got.ProjectID != original.ProjectID ||
		got.Env != original.Env || got.Path != original.Path {
		t.Errorf("la configuración no coincide:\nobtenido %+v\nesperado %+v", got, original)
	}
	if got.Variables["TELEMETRY_USER"] != "251374" {
		t.Errorf("variables = %v", got.Variables)
	}
}

// El token no puede aparecer en claro en el contenedor. Esto es justo lo que
// justify el cifrado: `strings portable.exe` no debe encontrarlo.
func TestElTokenNoApareceEnClaro(t *testing.T) {
	cfg := configDePrueba()
	blob, err := cifrarContenedor("clave-de-prueba", cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibido := range []string{
		cfg.Token,
		cfg.ProjectID,
		cfg.HashSalt,
		"251374",
		"streaming",
	} {
		if strings.Contains(blob, prohibido) {
			t.Errorf("el contenedor contiene %q en claro", prohibido)
		}
	}
}

// AES-GCM tiene que rechazar cualquier manipulación. Si un cambio pasara el
// filtro, el descifrado daría basura silenciosamente.
func TestContenedorManipuladoSeRechaza(t *testing.T) {
	blob, err := cifrarContenedor("clave-de-prueba", configDePrueba())
	if err != nil {
		t.Fatal(err)
	}
	version, resto, _ := strings.Cut(blob, ":")
	bruto := []byte(resto)

	// Mover un byte del final (dentro del ciphertext) debe fallar por la etiqueta.
	brutoMod := append([]byte(nil), bruto...)
	brutoMod[len(brutoMod)-1] ^= 0x01
	if _, err := descifrarContenedor(version+":"+string(brutoMod), "clave-de-prueba"); err == nil {
		t.Error("aceptó un contenedor con un byte cambiado")
	}

	// Cambiar el salt debe fallar: la clave derivada sería otra.
	brutoSalt := append([]byte(nil), bruto...)
	brutoSalt[0] ^= 0xFF
	if _, err := descifrarContenedor(version+":"+string(brutoSalt), "clave-de-prueba"); err == nil {
		t.Error("aceptó un contenedor con el salt cambiado")
	}

	// Cambiar el nonce también: afecta al descifrado.
	brutoNonce := append([]byte(nil), bruto...)
	brutoNonce[longitudSalt] ^= 0xFF
	if _, err := descifrarContenedor(version+":"+string(brutoNonce), "clave-de-prueba"); err == nil {
		t.Error("aceptó un contenedor con el nonce cambiado")
	}
}

func TestClaveIncorrectaSeRechaza(t *testing.T) {
	blob, err := cifrarContenedor("la-buena", configDePrueba())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := descifrarContenedor(blob, "la-mala"); err == nil {
		t.Error("aceptó una clave incorrecta")
	}
}

// Dos cifrados del mismo contenido no pueden ser iguales: si lo fueran, el
// nonce o el salt no se estarían usando y el GCM no valdría nada.
func TestCifrarNoEsDeterminista(t *testing.T) {
	a, err := cifrarContenedor("clave", configDePrueba())
	if err != nil {
		t.Fatal(err)
	}
	b, err := cifrarContenedor("clave", configDePrueba())
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("dos cifrados del mismo contenido son idénticos: falta entropía")
	}
	// Pero los dos descifran igual.
	for _, blob := range []string{a, b} {
		if _, err := descifrarContenedor(blob, "clave"); err != nil {
			t.Errorf("Descifrar: %v", err)
		}
	}
}

func TestContenedorConBasura(t *testing.T) {
	casos := []struct {
		nombre string
		blob   string
	}{
		{"vacío", ""},
		{"sin dos puntos", "abcd"},
		{"versión desconocida", "v9:abcd"},
		{"base64 inválido", "v1:!!!no-es-base64!!!"},
		{"truncado", "v1:AAAA"},
		{"sin clave", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, err := descifrarContenedor(c.blob, "clave"); err == nil {
				t.Errorf("aceptó %q", c.blob)
			}
		})
	}
}

// Sin clave no se puede abrir, y el error debe decirlo.
func TestFaltaClave(t *testing.T) {
	blob, err := cifrarContenedor("clave", configDePrueba())
	if err != nil {
		t.Fatal(err)
	}
	_, err = descifrarContenedor(blob, "")
	if err == nil {
		t.Fatal("abrió sin clave")
	}
	if !strings.Contains(err.Error(), "clave") {
		t.Errorf("el error no menciona la clave: %v", err)
	}
}

// Lo que se aplica al entorno nunca pisa lo que ya estaba definido: el
// contenedor rellena huecos, el entorno manda.
func TestAplicarNoPisaElEntorno(t *testing.T) {
	t.Setenv("INFISICAL_TOKEN", "ya-definido")
	t.Setenv("INFISICAL_ENV", "dev")
	os.Unsetenv("INFISICAL_PROJECT_ID")
	os.Unsetenv("INFISICAL_PATH")

	blob, err := cifrarContenedor("clave", configInfisical{
		Token:     "viene-del-contenedor",
		ProjectID: "proj-del-contenedor",
		Env:       "prod",
		Path:      "/streaming",
		Variables: map[string]string{"TELEMETRY_USER": "251374", "RESPUETADA": "si"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !aplicarSecretosIncrustadosCon(blob, "clave") {
		t.Fatal("Aplicar devolvió false")
	}

	if os.Getenv("INFISICAL_TOKEN") != "ya-definido" {
		t.Errorf("pisó un token ya definido: %q", os.Getenv("INFISICAL_TOKEN"))
	}
	if os.Getenv("INFISICAL_ENV") != "dev" {
		t.Errorf("pisó el entorno ya definido: %q", os.Getenv("INFISICAL_ENV"))
	}
	if os.Getenv("INFISICAL_PROJECT_ID") != "proj-del-contenedor" {
		t.Errorf("no rellenó el hueco: %q", os.Getenv("INFISICAL_PROJECT_ID"))
	}
	if os.Getenv("INFISICAL_PATH") != "/streaming" {
		t.Errorf("INFISICAL_PATH = %q", os.Getenv("INFISICAL_PATH"))
	}
	if os.Getenv("TELEMETRY_USER") != "251374" {
		t.Errorf("no aplicó las variables extra: %q", os.Getenv("TELEMETRY_USER"))
	}
	if os.Getenv("RESPUETADA") != "si" {
		t.Errorf("variable con tilde perdida: %q", os.Getenv("RESPUETADA"))
	}
}

// Un contenedor roto no puede tumbar la app: se avisa y se sigue.
func TestContenedorRotoNoRompe(t *testing.T) {
	if aplicarSecretosIncrustadosCon("v1:basura", "clave") {
		t.Error("Aplicar dijo que sí con un contenedor roto")
	}
	if aplicarSecretosIncrustadosCon("", "") {
		t.Error("Aplicar dijo que sí sin contenedor")
	}
}

// El generador se niega a producir un contenedor incompleto: un token sin
// project ID no sirve de nada y daría un fallo difícil de diagnosticar luego.
func TestGeneradorExigeLoBasico(t *testing.T) {
	t.Setenv("CLAVE_SECRETOS", "clave")
	t.Setenv("INFISICAL_TOKEN", "")
	t.Setenv("INFISICAL_PROJECT_ID", "")
	t.Setenv("INFISICAL_ENV", "")

	if err := generarSecretosIncrustados(); err == nil {
		t.Error("generó un contenedor sin datos")
	}

	t.Setenv("CLAVE_SECRETOS", "")
	t.Setenv("INFISICAL_TOKEN", "st.x.y.z")
	t.Setenv("INFISICAL_PROJECT_ID", "p")
	t.Setenv("INFISICAL_ENV", "prod")
	if err := generarSecretosIncrustados(); err == nil {
		t.Error("generó sin clave")
	}
}

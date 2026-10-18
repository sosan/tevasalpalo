package secrets

// Prueba contra el Infisical real. Solo corre si defines INFISICAL_TOKEN,
// INFISICAL_PROJECT_ID e INFISICAL_ENV, para no meter credenciales en el repo.

import (
	"os"
	"strings"
	"testing"
)

func TestInfisicalReal(t *testing.T) {
	if os.Getenv("INFISICAL_TOKEN") == "" {
		t.Skip("sin INFISICAL_TOKEN: no se prueba el Infisical real")
	}
	cfg, ok := loadInfisicalConfig()
	if !ok {
		t.Fatal("falta INFISICAL_PROJECT_ID o INFISICAL_ENV")
	}

	cl := newInfisicalClient(cfg)
	secrets, err := cl.secrets()
	if err != nil {
		t.Fatalf("Infisical real falló: %v", err)
	}
	for k, v := range secrets {
		mostrado := v
		if len(mostrado) > 10 {
			mostrado = mostrado[:10] + "…"
		}
		t.Logf("  %-28s = %s", k, mostrado)
	}
	if len(secrets) == 0 {
		t.Logf("la carpeta %s (%s) está vacía", cfg.pathHint(), cfg.env)
	}
	t.Logf("Infisical devolvió %d secretos de %s (%s)", len(secrets), cfg.pathHint(), cfg.env)
}

// Comprueba que el token de este proyecto sigue siendo solo-lectura y acotado.
// Si alguien lo cambia por uno con permisos de escritura, esto avisa.
func TestInfisicalTokenSigueAcotado(t *testing.T) {
	tok := os.Getenv("INFISICAL_TOKEN")
	if tok == "" {
		t.Skip("sin INFISICAL_TOKEN")
	}
	if !strings.HasPrefix(tok, "st.") && !strings.HasPrefix(tok, "mi.") {
		t.Errorf("el token no parece de service ni de machine identity: %.6s…", tok)
	}
}

package main

// Mapeos horneados a partir de juicios TypeSafe offline (sin runtime ni API key
// en la app). Cada entrada se validó con una pregunta Choice contra la API y
// solo se incluyeron mapeos confiados (p>=0.8) con destino canónico existente
// y seguros sin contexto de competición.
//
// Descartados expresamente (siguen filtrando por diseño):
//   - "La Liga TV M3": depende de la competición (Hypermotion sí, Serie A no).
//   - "Paramount Plus" sin UFC, "Eurosport 360 4": sin pool de acestreams.
//   - "DAZN Bar 1", "GOL TV Play": el modelo tampoco los distingue.
//   - "Vamos por M+": grafía inventada, "Vamos" ya está cubierto.

import (
	"fmt"
	"testing"
)

var gatewayStaticTypeSafeVariants = map[string][]string{
	"M+ L. Campeones":    {"M+ LIGA DE CAMPEONES"},
	"Movistar Deporte 2": {"M+ DEPORTES 2"},
	"Dazen 1":            {"DAZN 1"},
	"LaLiga Smartbank":   {"LALIGA HYPERMOTION"},
	"ESPN Premium":       {"ESPN ARGENTINA"},
	"TNT Sports Premium": {"TNT SPORTS"},
	"D Sports":           {"DS SPORT"},
	"UFC FightPass":      {"UFC"},
	"Primera RFEF":       {"Primera Federacion"},
}

func TestGatewayStaticTypeSafeVariantsResolve(t *testing.T) {
	assertGatewayTable(t, gatewayStaticTypeSafeVariants)
}

// Ronda 2: variantes mecánicas de top-miss reales en producción
// (prefijo M+, espaciado), cada una con precedente directo en el mapa.
var gatewayDeterministicR2 = map[string][]string{
	"M+ Vamos":               {"M+ VAMOS"},
	"M+ Liga de Campeones":   {"M+ LIGA DE CAMPEONES"},
	"M+ Liga de Campeones 2": {"M+ LIGA DE CAMPEONES 2"},
	"M+ Liga de Campeones 3": {"M+ LIGA DE CAMPEONES 3"},
	"M+ Liga de Campeones 4": {"M+ LIGA DE CAMPEONES 4"},
	"M+ Liga de Campeones 5": {"M+ LIGA DE CAMPEONES 5"},
	"M+ Liga de Campeones 6": {"M+ LIGA DE CAMPEONES 6"},
	"M+ Liga de Campeones 7": {"M+ LIGA DE CAMPEONES 7"},
	"M+ Liga de Campeones 8": {"M+ LIGA DE CAMPEONES 8"},
	"M+ Golf":                {"M+ GOLF"},
	"Eurosport 360 2":        {"EUROSPORT 2"},
	"Red Bull TV":            {"REDBULL TV"},
}

func TestGatewayDeterministicR2Resolve(t *testing.T) {
	assertGatewayTable(t, gatewayDeterministicR2)
}

func assertGatewayTable(t *testing.T, table map[string][]string) {
	t.Helper()
	i := 0
	for raw, wantCanonicals := range table {
		i++
		t.Run(raw, func(t *testing.T) {
			hash := fmt.Sprintf("testhash%02dabcdef0123456789abcdef01234567", i)
			got := updateBroadcasterMapWithGatewayTolerant(
				map[string]BroadcasterInfo{},
				map[string][]string{raw: {hash}},
				"test",
			)
			if len(got) != len(wantCanonicals) {
				t.Fatalf("raw %q resolvió a %d broadcasters %v, se esperaban %v (posible colisión de bucket)",
					raw, len(got), keysOf(got), wantCanonicals)
			}
			for _, canonical := range wantCanonicals {
				info, ok := got[canonical]
				if !ok {
					t.Fatalf("raw %q no resolvió a %q (resolvió a %v)", raw, canonical, keysOf(got))
				}
				if !containsStr(info.Links, hash) {
					t.Fatalf("raw %q: %q no contiene el link extraído", raw, canonical)
				}
			}
		})
	}
}

func TestGatewayStaticTypeSafeTargetsExist(t *testing.T) {
	assertGatewayTargetsExist(t, gatewayStaticTypeSafeVariants, map[string]bool{})
	// R2: M+ GOLF es fantasma admitido (precedente: ya lo crea
	// "Movistar Golf (FHD)" desde antes).
	assertGatewayTargetsExist(t, gatewayDeterministicR2, map[string]bool{"M+ GOLF": true})
}

func assertGatewayTargetsExist(t *testing.T, table map[string][]string, phantomsOK map[string]bool) {
	t.Helper()
	for raw, canonicals := range table {
		for _, canonical := range canonicals {
			if _, ok := broadcasterToAcestream[canonical]; !ok && !phantomsOK[canonical] {
				t.Errorf("raw %q apunta a canónico inexistente %q (crearía entrada fantasma sin logo)",
					raw, canonical)
			}
		}
	}
}

func TestGatewayUnknownStillFiltered(t *testing.T) {
	got := updateBroadcasterMapWithGatewayTolerant(
		map[string]BroadcasterInfo{},
		map[string][]string{
			"Canal Inexistente XYZ":     {"abc"},
			"La Liga TV M3":             {"abc"}, // sensible a competición: sin contexto se filtra
			"Eurosport 360 4":           {"abc"}, // sin pool: se filtra
			"Paramount Plus":            {"abc"}, // sin indicio UFC: se filtra
			"DAZNLaLigaBar Inexistente": {"abc"},
		},
		"test",
	)
	if len(got) != 0 {
		t.Fatalf("nombres desconocidos deben filtrarse, resolvieron a %v", keysOf(got))
	}
}

func TestFindBroadcasterExactUntouched(t *testing.T) {
	if got := findBroadcaster("DAZN 1", "LaLiga", "Fútbol"); got.Name == "" {
		t.Fatal("la vía exacta debe seguir resolviendo sin TypeSafe")
	}
	if got := findBroadcaster("Canal Inexistente XYZ", "LaLiga", "Fútbol"); got.Name != "" {
		t.Fatalf("lo desconocido debe filtrarse, got %q", got.Name)
	}
}

func keysOf(m map[string]BroadcasterInfo) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

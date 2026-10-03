package main

// Tests de las mejoras deterministas validadas offline con TypeSafe (13/13).
// Sin red, sin API key: las decisiones del modelo se usaron solo en diseño
// y aquí se fijan como comportamiento esperado del código.

import (
	"testing"
	"time"
)

func TestSameFootballMatch(t *testing.T) {
	cases := []struct {
		name     string
		a, b     string
		wantSame bool
	}{
		// Casos validados contra juicios Noul de TypeSafe.
		{"sufijos CF/FC", "Valencia - Barcelona", "Valencia CF - FC Barcelona", true},
		{"Real implícito", "Getafe - Betis", "Getafe - Real Betis", true},
		{"orden invertido y tilde", "Alavés - Real Madrid", "Real Madrid - Alaves", true},
		{"sufijo Club", "Athletic - Girona", "Athletic Club - Girona", true},
		{"nombres cortos", "Barcelona - Madrid", "FC Barcelona - Real Madrid", true},
		{"prefijo AC", "Inter - Milan", "Inter - AC Milan", true},
		{"filial no fusiona", "Real Madrid - Barcelona", "Real Madrid Castilla - Barcelona", false},
		{"filial B no fusiona", "Real Sociedad - Alavés", "Real Sociedad B - Alaves", false},
		{"rival distinto", "Barcelona - Madrid", "Barcelona - Atlético Madrid", false},
		{"sanity distinto", "Sevilla - Betis", "Sevilla - Valencia", false},
		{"evento único igual", "Gran Premio de España", "Gran Premio de España", true},
		{"evento único distinto", "Gran Premio de España", "Gran Premio de Italia", false},
		// Limitación conocida y aceptada: no se recorta UNITED/CITY para no
		// fusionar Manchester City con Manchester United (el modelo dice same).
		{"Leeds conservador", "Leeds - Chelsea", "Leeds United - Chelsea", false},
		// Comportamiento base.
		{"exacto", "Real Madrid - Barcelona", "Real Madrid - Barcelona", true},
		{"caja y espacios", "  real madrid - BARCELONA ", "Real Madrid - Barcelona", true},
		{"equipo con guion", "Paris Saint-Germain - Marseille", "Paris Saint-Germain - Marseille", true},
		{"prefijo CA", "Osasuna - Mallorca", "CA Osasuna - Mallorca", true},
		{"prefijo RCD", "RCD Espanyol - Girona", "Espanyol - Girona", true},
		{"vacío no matchea", "", "", false},
		{"malformado no matchea", "Getafe -", "Getafe - Betis", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameFootballMatch(tc.a, tc.b); got != tc.wantSame {
				t.Fatalf("sameFootballMatch(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.wantSame)
			}
			// Simetría: el orden de las fuentes no debe importar.
			if got := sameFootballMatch(tc.b, tc.a); got != tc.wantSame {
				t.Fatalf("sameFootballMatch(%q, %q) = %v, want %v (asimétrico)", tc.b, tc.a, got, tc.wantSame)
			}
		})
	}
}

func TestCleanDate(t *testing.T) {
	if got := cleanDate("  Partidos del Lunes, 26/08/2025  "); got != "26-08-2025" {
		t.Fatalf("cleanDate válido = %q, want 26-08-2025", got)
	}
	// Fallback documentado: ante cabecera sin fecha usa hoy (y ahora lo loguea).
	wantToday := time.Now().Format("02-01-2006")
	if got := cleanDate("cabecera rota sin coma"); got != wantToday {
		t.Fatalf("cleanDate roto = %q, want hoy %q", got, wantToday)
	}
}

func TestExtractHashFromLink(t *testing.T) {
	const hash = "da9038e51a4897659f7bb1bce52cab65dd4a8cf8"
	cases := []struct {
		name, link, want string
	}{
		{"acestream", "acestream://" + hash, hash},
		{"acestream con params", "acestream://" + hash + "?x=1&y=2", hash},
		{"hash puro mayúsculas", "DA9038E51A4897659F7BB1BCE52CAB65DD4A8CF8", hash},
		{"url con id", "http://127.0.0.1:6878/ace/manifest.m3u8?id=" + hash, hash},
		{"url con ID mayúscula", "http://127.0.0.1:6878/ace/manifest.m3u8?ID=" + hash, hash},
		{"hash embebido", "xx" + hash + "yy", hash},
		{"basura", "not a hash", ""},
		{"vacío", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractHashFromLink(tc.link); got != tc.want {
				t.Fatalf("extractHashFromLink(%q) = %q, want %q", tc.link, got, tc.want)
			}
		})
	}
}

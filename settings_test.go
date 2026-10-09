package main

// Tests de los ajustes persistentes (settings.json).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func tempSettingsPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	settingsMu.Lock()
	settingsPathOverride = p
	settingsLoaded = false
	settings = appSettings{}
	settingsMu.Unlock()
	t.Cleanup(func() {
		settingsMu.Lock()
		settingsPathOverride = ""
		settingsLoaded = false
		settings = appSettings{}
		settingsMu.Unlock()
	})
	return p
}

func TestSettingsRoundTrip(t *testing.T) {
	p := tempSettingsPath(t)
	if loadSettings().WarpEnabled {
		t.Fatal("sin fichero no debe haber nada activado")
	}
	if err := setWarpEnabled(true); err != nil {
		t.Fatalf("setWarpEnabled: %v", err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("no se escribió settings.json: %v", err)
	}
	var s appSettings
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !s.WarpEnabled {
		t.Fatalf("settings.json = %s", raw)
	}
	// Recargar desde disco (como si reiniciaras la app).
	settingsMu.Lock()
	settingsLoaded = false
	settings = appSettings{}
	settingsMu.Unlock()
	if !loadSettings().WarpEnabled {
		t.Fatal("el ajuste no sobrevive a la recarga")
	}
}

func TestSettingsIgnoresCorruptFile(t *testing.T) {
	p := tempSettingsPath(t)
	if err := os.WriteFile(p, []byte("{esto no es json"), 0600); err != nil {
		t.Fatal(err)
	}
	// Un fichero roto no debe impedir arrancar: se queda con los valores por defecto.
	if loadSettings().WarpEnabled {
		t.Fatal("un settings.json corrupto no debe activar nada")
	}
}

func TestSettingsMissingFile(t *testing.T) {
	tempSettingsPath(t)
	if loadSettings().WarpEnabled {
		t.Fatal("sin fichero, todo desactivado")
	}
}

func TestWarpEnabledPriority(t *testing.T) {
	tempSettingsPath(t)

	// Guardado en la UI = habilitado.
	t.Setenv("WARP", "")
	t.Setenv("WARP_AUTO", "")
	if err := setWarpEnabled(true); err != nil {
		t.Fatal(err)
	}
	if !warpEnabled() {
		t.Fatal("el ajuste guardado debe habilitar WARP")
	}

	// WARP=0 en el entorno manda sobre lo guardado.
	t.Setenv("WARP", "0")
	if warpEnabled() {
		t.Fatal("WARP=0 debe ganar sobre settings.json")
	}

	// WARP=1 también.
	t.Setenv("WARP", "1")
	if !warpEnabled() {
		t.Fatal("WARP=1 debe habilitar")
	}

	// Desguardado y sin entorno: desactivado.
	if err := setWarpEnabled(false); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WARP", "")
	if warpEnabled() {
		t.Fatal("tras desguardar debe quedar desactivado")
	}

	// WARP_AUTO=1 sin guardado: habilitado.
	t.Setenv("WARP_AUTO", "1")
	if !warpEnabled() {
		t.Fatal("WARP_AUTO=1 debe habilitar")
	}
}

func TestWarpStatusShape(t *testing.T) {
	tempSettingsPath(t)
	t.Setenv("WARP", "")
	t.Setenv("WARP_AUTO", "")
	if err := setWarpEnabled(true); err != nil {
		t.Fatal(err)
	}
	s := warpStatus()
	for _, k := range []string{"enabled", "active", "socks", "locked"} {
		if _, ok := s[k]; !ok {
			t.Fatalf("warpStatus sin clave %q: %v", k, s)
		}
	}
	if s["enabled"] != true {
		t.Fatalf("enabled = %v", s["enabled"])
	}
	if s["active"] != false {
		t.Fatalf("active = %v, sin streams no hay túnel", s["active"])
	}
	// Con WARP=1 en el entorno, la UI debe saber que está fijada.
	t.Setenv("WARP", "1")
	if warpStatus()["locked"] != true {
		t.Fatalf("locked = %v, con WARP=1 debe ser true", warpStatus()["locked"])
	}
}

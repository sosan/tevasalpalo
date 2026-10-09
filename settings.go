package main

// Ajustes persistentes de la app, en settings.json junto al ejecutable.
//
// Antes los interruptores eran solo variables de entorno, así que cada reinicio
// los perdía y había que exportarlos a mano. Esto guarda en disco lo que se
// cambia desde la UI web.
//
// Prioridad de lectura: la variable de entorno manda sobre el fichero, para
// poder forzar un valor desde consola o desde el workflow sin tocar lo
// guardado.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const settingsFile = "settings.json"

// appSettings son los valores guardables. Solo se persisten los que la UI
// cambia; el resto de la config sigue viniendo del entorno.
type appSettings struct {
	// WarpEnabled activa el túnel Cloudflare WARP (ver warp.go).
	WarpEnabled bool `json:"warp_enabled"`
}

var (
	settingsMu     sync.RWMutex
	settingsLoaded bool
	settings       appSettings
)

// settingsPathOverride lo usan los tests para no escribir settings.json junto
// al binario de pruebas. En producción está vacío.
var settingsPathOverride string

// settingsPath es el fichero de ajustes, junto al ejecutable.
func settingsPath() (string, error) {
	if settingsPathOverride != "" {
		return settingsPathOverride, nil
	}
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exePath), settingsFile), nil
}

// loadSettings lee settings.json una vez. Si no existe o está corrupto, se
// deja el valor por defecto: un fichero de ajustes roto no debe impedir
// arrancar la app.
func loadSettings() appSettings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	if settingsLoaded {
		return settings
	}
	settingsLoaded = true
	path, err := settingsPath()
	if err != nil {
		return settings
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return settings
	}
	var s appSettings
	if err := json.Unmarshal(raw, &s); err != nil {
		return settings
	}
	settings = s
	return settings
}

// saveSettings persiste los ajustes y devuelve error si no pudo escribir, para
// que la UI avise en vez de fingir que se guardó.
func saveSettings(s appSettings) error {
	settingsMu.Lock()
	settings = s
	settingsLoaded = true
	settingsMu.Unlock()

	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0600)
}

// setWarpEnabled guarda el estado de WARP.
func setWarpEnabled(v bool) error {
	return saveSettings(appSettings{WarpEnabled: v})
}

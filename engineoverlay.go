package main

// Capa de parches sobre el motor extraído: hooks de red del APK y plugins.
//
// El APK de Ace Stream Pro 3.2.22.3 parchea el motor en Python
// (assets/engine/main.py) para dos cosas: fallo rápido de DNS de hosts que no
// resuelven y respuestas vacías para VAST y notificaciones, con lo que se
// evita esperar 14-15 s por petición a un servidor inalcanzable. Aquí se
// replica como sitecustomize del intérprete embebido del motor de Linux
// (engine_overlay/sitecustomize.py), que se importa al arrancar sin tocar el
// motor.
//
// Los plugins del APK (youtube.py con matchers de canal/embed y __init__.py)
// llevan su firma #-plugin-sig:, que el motor valida contra su propia clave y
// versión: copiarlos desde 3.2.22.3 a un motor 3.2.8 puede que el motor los
// rechace. Por eso NO se empaquetan aquí: el usuario apunta ACE_PLUGIN_DIR a
// una carpeta con los plugins que quiera probar y se copian sobre el motor.

import (
	"embed"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

//go:embed engine_overlay/sitecustomize.py
var engineOverlayFS embed.FS

// overlayDirName es la carpeta del overlay dentro del runtime.
const overlayDirName = "overlay"

func overlayDir(runtimePath string) string {
	return filepath.Join(runtimePath, overlayDirName)
}

// pluginDirEnv apunta a una carpeta con plugins (.py) para sobrescribir los
// del motor. Vacío = no tocar plugins.
const pluginDirEnv = "ACE_PLUGIN_DIR"

// hooksEnabled permite desactivar los hooks de red sin recompilar.
const hooksEnabledEnv = "ACE_ENGINE_HOOKS"

// engineHooksEnabled decide si se instala sitecustomize.
func engineHooksEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(hooksEnabledEnv))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// applyEngineOverlay deja el runtime listo: escribe sitecustomize.py en
// runtime/overlay (para el motor de Linux) y copia los plugins indicados por
// ACE_PLUGIN_DIR sobre data/plugins. Nunca es fatal: un fallo aquí solo deja el
// motor con su configuración original.
func applyEngineOverlay(runtimePath string, spec engineSpec) {
	if err := writeAceConf(runtimePath, spec); err != nil {
		log.Printf("⚠️  No se pudo escribir %s: %v", aceConfName, err)
	}
	if engineHooksEnabled() {
		if err := writeEngineHooks(runtimePath); err != nil {
			log.Printf("⚠️  No se pudieron instalar los hooks del motor: %v", err)
		}
	}
	if err := overlayPlugins(runtimePath, spec); err != nil {
		log.Printf("⚠️  No se pudieron aplicar los plugins: %v", err)
	}
}

// writeEngineHooks extrae engine_overlay/sitecustomize.py al directorio del
// overlay. Solo tiene efecto en el motor de Linux (interpreta Python embebido
// con PYTHONPATH); en Windows se copia igualmente, pero ace_console.exe no lo
// importa.
func writeEngineHooks(runtimePath string) error {
	src, err := engineOverlayFS.Open("engine_overlay/sitecustomize.py")
	if err != nil {
		return err
	}
	defer src.Close()

	dst := filepath.Join(overlayDir(runtimePath), "sitecustomize.py")
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// overlayPlugins copia los .py de ACE_PLUGIN_DIR sobre data/plugins del motor.
// Los que ya existen se respaldan en plugins.bak una sola vez, para poder
// volver al original si el motor rechaza una firma.
func overlayPlugins(runtimePath string, spec engineSpec) error {
	srcDir := strings.TrimSpace(os.Getenv(pluginDirEnv))
	if srcDir == "" {
		return nil
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	dstDir := filepath.Join(runtimePath, spec.dataDir, "plugins")
	backupDir := filepath.Join(runtimePath, spec.dataDir, "plugins.bak")
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".py") {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(dstDir, e.Name())
		if fileExists(dst) && !fileExists(filepath.Join(backupDir, e.Name())) {
			if err := os.MkdirAll(backupDir, 0755); err != nil {
				return err
			}
			if err := copyFile(dst, filepath.Join(backupDir, e.Name())); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			return err
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
		log.Printf("🧩 Plugin aplicado: %s", e.Name())
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

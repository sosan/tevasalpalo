package main

// Tests del overlay del motor: hooks de red y plugins.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteEngineHooks(t *testing.T) {
	runtime := t.TempDir()
	if err := writeEngineHooks(runtime); err != nil {
		t.Fatalf("writeEngineHooks: %v", err)
	}
	dst := filepath.Join(overlayDir(runtime), "sitecustomize.py")
	body, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("no se escribió sitecustomize.py: %v", err)
	}
	// El contenido tiene que ser el hook de DNS/VAST del APK, no un fichero vacío.
	for _, want := range []string{
		"getaddrinfo", "torrentstream.org", "router.acestream.me",
		"urlOpenTimeout", "VAST",
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("sitecustomize.py no menciona %q", want)
		}
	}
}

func TestEngineHooksEnabled(t *testing.T) {
	for val, want := range map[string]bool{
		"":      true,
		"1":     true,
		"on":    true,
		"0":     false,
		"false": false,
		"off":   false,
	} {
		t.Setenv(hooksEnabledEnv, val)
		if got := engineHooksEnabled(); got != want {
			t.Fatalf("%s=%q -> %v, want %v", hooksEnabledEnv, val, got, want)
		}
	}
}

func TestOverlayPluginsNoEnvIsNoop(t *testing.T) {
	t.Setenv(pluginDirEnv, "")
	if err := overlayPlugins(t.TempDir(), aceEngineSpec("/tmp/rt")); err != nil {
		t.Fatalf("sin ACE_PLUGIN_DIR no debe hacer nada: %v", err)
	}
}

func TestOverlayPluginsCopiesAndBacksUp(t *testing.T) {
	runtime := t.TempDir()
	spec := aceEngineSpec(runtime)

	// Plugin existente del motor y plugin nuevo, más un fichero que se ignora.
	plugins := filepath.Join(runtime, spec.dataDir, "plugins")
	if err := os.MkdirAll(plugins, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugins, "youtube.py"), []byte("viejo"), 0644); err != nil {
		t.Fatal(err)
	}

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "youtube.py"), []byte("nuevo"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nuevo.py"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "leeme.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(pluginDirEnv, src)

	if err := overlayPlugins(runtime, spec); err != nil {
		t.Fatalf("overlayPlugins: %v", err)
	}
	// El plugin existente se sobrescribe y su versión previa queda respaldada.
	got, err := os.ReadFile(filepath.Join(plugins, "youtube.py"))
	if err != nil || string(got) != "nuevo" {
		t.Fatalf("youtube.py = %q (%v)", got, err)
	}
	backup, err := os.ReadFile(filepath.Join(runtime, spec.dataDir, "plugins.bak", "youtube.py"))
	if err != nil || string(backup) != "viejo" {
		t.Fatalf("respaldo = %q (%v)", backup, err)
	}
	// El nuevo se copia sin respaldo (no existía).
	if _, err := os.Stat(filepath.Join(plugins, "nuevo.py")); err != nil {
		t.Fatalf("nuevo.py no se copió: %v", err)
	}
	// Los .txt y los directorios se ignoran.
	if _, err := os.Stat(filepath.Join(plugins, "leeme.txt")); err == nil {
		t.Fatal("los .txt no deben copiarse")
	}
	if _, err := os.Stat(filepath.Join(plugins, "subdir")); err == nil {
		t.Fatal("los directorios no deben copiarse")
	}

	// Segunda pasada: el respaldo no se pisa con la versión ya sustituida.
	if err := overlayPlugins(runtime, spec); err != nil {
		t.Fatal(err)
	}
	backup, err = os.ReadFile(filepath.Join(runtime, spec.dataDir, "plugins.bak", "youtube.py"))
	if err != nil || string(backup) != "viejo" {
		t.Fatalf("respaldo sobrescrito: %q (%v)", backup, err)
	}
}

func TestOverlayPluginsMissingSourceDir(t *testing.T) {
	t.Setenv(pluginDirEnv, filepath.Join(t.TempDir(), "no-existe"))
	if err := overlayPlugins(t.TempDir(), aceEngineSpec("/tmp/rt")); err == nil {
		t.Fatal("se esperaba error con ACE_PLUGIN_DIR inexistente")
	}
}

func TestApplyEngineOverlayNeverPanics(t *testing.T) {
	runtime := t.TempDir()
	spec := aceEngineSpec(runtime)
	t.Setenv(pluginDirEnv, "")
	t.Setenv(hooksEnabledEnv, "1")
	applyEngineOverlay(runtime, spec)
	if _, err := os.Stat(filepath.Join(overlayDir(runtime), "sitecustomize.py")); err != nil {
		t.Fatalf("applyEngineOverlay no instaló los hooks: %v", err)
	}
}

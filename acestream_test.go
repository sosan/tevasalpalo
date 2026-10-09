package main

// Tests del arranque del motor: selección de asset por plataforma y
// extracción segura. El asset de Linux se lee del embed real solo cuando
// existe; si no, se prueba el generador de rutas contra rutas sintéticas.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runtimeIsWindows() bool { return runtime.GOOS == "windows" }

func TestAceEngineSpecWindows(t *testing.T) {
	// runtime.GOOS decide la spec; el test solo comprueba la forma de cada
	// rama según el SO en el que se compila el binario de test.
	spec := aceEngineSpec("/opt/app/runtime")
	if runtimeIsWindows() {
		if spec.asset != "assets/"+aceAssetNameWin {
			t.Fatalf("asset = %q", spec.asset)
		}
		if spec.binary != filepath.Join("engine", "ace_console.exe") {
			t.Fatalf("binary = %q", spec.binary)
		}
		if spec.workDir != "engine" {
			t.Fatalf("workDir = %q", spec.workDir)
		}
		if spec.marker != filepath.Join("/opt/app/runtime", "engine", "ace_console.exe") {
			t.Fatalf("marker = %q", spec.marker)
		}
		return
	}
	// Linux: binario ELF en la raíz del runtime, no el ace_console.exe de Win.
	if spec.asset != "assets/"+aceAssetNameLinux {
		t.Fatalf("asset = %q", spec.asset)
	}
	if spec.binary != "acestreamengine" {
		t.Fatalf("binary = %q", spec.binary)
	}
	if spec.workDir != "." {
		t.Fatalf("workDir = %q", spec.workDir)
	}
	if strings.Contains(spec.asset, "windows") {
		t.Fatal("el motor de Linux no debe usar el asset de Windows")
	}
}

func TestAceEngineSpecArgs(t *testing.T) {
	spec := aceEngineSpec("/tmp/runtime")
	want := []string{"--live-buffer", "60", "--vod-buffer", "10", "--client-console"}
	if len(spec.args) != len(want) {
		t.Fatalf("args = %v", spec.args)
	}
	for i := range want {
		if spec.args[i] != want[i] {
			t.Fatalf("arg %d = %q, want %q", i, spec.args[i], want[i])
		}
	}
}

func TestSafeJoin(t *testing.T) {
	base := t.TempDir()
	ok, err := safeJoin(base, "lib/acestreamengine/Core.so")
	if err != nil {
		t.Fatalf("ruta legítima rechazada: %v", err)
	}
	if !strings.HasPrefix(ok, base) {
		t.Fatalf("safeJoin = %q, fuera de %q", ok, base)
	}
	// Saltos fuera del destino (Zip Slip / Tar Slip).
	for _, evil := range []string{
		"../escape.txt",
		"../../etc/passwd",
		"lib/../../escape",
		"a/b/../../../escape",
	} {
		if _, err := safeJoin(base, evil); err == nil {
			t.Fatalf("%q: se esperaba error", evil)
		}
	}
	// Una ruta absoluta se interpreta relativa a base (como Join), nunca
	// escapa: el resultado sigue dentro del destino.
	abs, err := safeJoin(base, "/etc/passwd")
	if err != nil {
		t.Fatalf("ruta absoluta rechazada: %v", err)
	}
	if !strings.HasPrefix(abs, base) {
		t.Fatalf("ruta absoluta escapó: %q", abs)
	}
}

func TestExtractRuntimeRejectsUnknownAsset(t *testing.T) {
	// Un asset que no existe en el embed da error claro, no panic.
	if err := extractRuntime(t.TempDir(), "assets/no-existe.zip"); err == nil {
		t.Fatal("se esperaba error con asset inexistente")
	}
}

func TestOpenAssetFindsBothEmbeds(t *testing.T) {
	// Regresión: extractRuntime abre el motor desde aceEngineAsset y Tor desde
	// runtimeZip. Cuando solo se miraba aceEngineAsset, Tor fallaba al arrancar
	// en Windows con "no se pudo abrir el ZIP embebido: file does not exist",
	// porque el bundle de Tor nunca estuvo en ese embed.
	torAsset := "assets/" + torAssetNameWin
	if _, err := os.Stat(filepath.Join("assets", torAssetNameWin)); err != nil {
		t.Skipf("asset %s no presente", torAssetNameWin)
	}
	f, err := openAsset(torAsset)
	if err != nil {
		t.Fatalf("el bundle de Tor debe abrirse desde runtimeZip: %v", err)
	}
	info, err := f.Stat()
	f.Close()
	if err != nil {
		t.Fatalf("Stat del bundle de Tor: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("el bundle de Tor está vacío")
	}
}

func TestExtractTarGzRealAsset(t *testing.T) {
	// El motor de Linux es el único tar.gz embebido, y solo se embebe en builds
	// linux (ver aceasset_linux.go). En otro SO el test no aplica.
	if _, err := os.Stat(filepath.Join("assets", aceAssetNameLinux)); err != nil {
		t.Skipf("asset %s no presente", aceAssetNameLinux)
	}
	if _, err := aceEngineAsset.Open("assets/" + aceAssetNameLinux); err != nil {
		t.Skipf("el asset de linux no está embebido en este build: %v", err)
	}
	dir := t.TempDir()
	if err := extractRuntime(dir, "assets/"+aceAssetNameLinux); err != nil {
		t.Fatalf("extractRuntime: %v", err)
	}
	// El binario del motorLinux debe quedar con bit +x.
	bin := filepath.Join(dir, "acestreamengine")
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("no se extrajo acestreamengine: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Fatalf("acestreamengine sin permiso de ejecución: %v", info.Mode())
	}
	// Y los módulos nativos del motor.
	if _, err := os.Stat(filepath.Join(dir, "lib", "acestreamengine", "Core.so")); err != nil {
		t.Fatalf("no se extrajeron los módulos del motor: %v", err)
	}
}

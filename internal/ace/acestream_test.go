package ace

// Tests del arranque del motor: selección de asset por plataforma y
// extracción segura. El asset de Linux se lee del embed real solo cuando
// existe; si no, se prueba el generador de rutas contra rutas sintéticas.

import (
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

package main

// Tests del acestream.conf del motor.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAceConfFlagsBase(t *testing.T) {
	flags := aceConfFlags("base")
	if len(flags) != 2 {
		t.Fatalf("flags base = %+v", flags)
	}
	if flags[0].flag != "--stats-report-peers" || flags[1].flag != "--service-remote-access" {
		t.Fatalf("flags base = %+v", flags)
	}
	// El perfil por defecto no debe traer flags del APK.
	for _, f := range flags {
		if strings.HasPrefix(f.flag, "--allow-user-config") {
			t.Fatal("el perfil base no debe incluir flags del APK")
		}
	}
}

func TestAceConfFlagsAPK(t *testing.T) {
	flags := aceConfFlags("apk")
	want := map[string]string{
		"--slots-manager-use-cpu-limit":        "1",
		"--core-dlr-periodic-check-interval":   "5",
		"--check-live-pos-interval":            "5",
		"--core-skip-have-before-playback-pos": "1",
		"--webrtc-allow-outgoing-connections":  "1",
		"--log-debug":                          "355",
		"--log-modules":                        "root:D",
		"--log-max-size":                       "15000000",
		"--log-backup-count":                   "1",
		"--vod-drop-max-age":                   "120",
		"--allow-user-config":                  "",
		"--disable-sentry":                     "",
		"--debug-memory-monitor":               "",
		"--debug-epg":                          "",
		"--debug-filter-aircast":               "",
		"--debug-client-transporter":           "",
	}
	got := map[string]string{}
	for _, f := range flags {
		got[f.flag] = f.value
	}
	for flag, value := range want {
		v, ok := got[flag]
		if !ok {
			t.Fatalf("falta %s", flag)
		}
		if v != value {
			t.Fatalf("%s = %q, want %q", flag, v, value)
		}
	}
	if len(flags) != len(want) {
		t.Fatalf("flags apk = %d, want %d: %+v", len(flags), len(want), flags)
	}
}

func TestRenderAceConf(t *testing.T) {
	out := renderAceConf([]confFlag{
		{flag: "--solo"},
		{flag: "--con-valor", value: "42"},
	})
	if out != "--solo\n--con-valor\n42\n" {
		t.Fatalf("render = %q", out)
	}
}

func TestAceConfProfile(t *testing.T) {
	t.Setenv("ACE_CONF", "")
	if got := aceConfProfile(); got != "base" {
		t.Fatalf("perfil por defecto = %q", got)
	}
	t.Setenv("ACE_CONF", "apk")
	if got := aceConfProfile(); got != "apk" {
		t.Fatalf("perfil = %q", got)
	}
}

func TestWriteAceConfBaseLeavesAssetConf(t *testing.T) {
	runtime := t.TempDir()
	spec := aceEngineSpec(runtime)
	dir := filepath.Join(runtime, spec.workDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// El conf que trae el asset de Linux, que el perfil base no debe tocar.
	shipped := "--stats-report-interval\n1\n--stats-report-peers\n--log-file\nacestream.log\n"
	if err := os.WriteFile(filepath.Join(dir, aceConfName), []byte(shipped), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACE_CONF", "")
	if err := writeAceConf(runtime, spec); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, aceConfName))
	if string(got) != shipped {
		t.Fatalf("el perfil base reescribió el conf del asset: %q", got)
	}
	if fileExists(filepath.Join(dir, aceConfBackup)) {
		t.Fatal("el perfil base no debe respaldar nada")
	}
}

func TestWriteAceConfBaseCreatesIfMissing(t *testing.T) {
	runtime := t.TempDir()
	spec := aceEngineSpec(runtime)
	t.Setenv("ACE_CONF", "")
	if err := writeAceConf(runtime, spec); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(runtime, spec.workDir, aceConfName))
	if err != nil {
		t.Fatalf("no se creó el conf: %v", err)
	}
	if string(got) != renderAceConf(aceConfFlags("base")) {
		t.Fatalf("conf = %q", got)
	}
}

func TestWriteAceConfCreatesAndBacksUp(t *testing.T) {
	runtime := t.TempDir()
	spec := aceEngineSpec(runtime)
	dir := filepath.Join(runtime, spec.workDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// Conf que trae el asset.
	shipped := "--solo-del-asset\n"
	if err := os.WriteFile(filepath.Join(dir, aceConfName), []byte(shipped), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ACE_CONF", "apk")
	if err := writeAceConf(runtime, spec); err != nil {
		t.Fatalf("writeAceConf: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, aceConfName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "--log-debug") {
		t.Fatalf("conf escrito = %q", got)
	}
	backup, err := os.ReadFile(filepath.Join(dir, aceConfBackup))
	if err != nil {
		t.Fatalf("no se respaldó el conf original: %v", err)
	}
	if string(backup) != shipped {
		t.Fatalf("respaldo = %q, want %q", backup, shipped)
	}

	// Segunda escritura: el respaldo no se pisa.
	if err := os.WriteFile(filepath.Join(dir, aceConfName), []byte("otro\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeAceConf(runtime, spec); err != nil {
		t.Fatal(err)
	}
	backup, _ = os.ReadFile(filepath.Join(dir, aceConfBackup))
	if string(backup) != shipped {
		t.Fatalf("respaldo sobrescrito = %q", backup)
	}
}

func TestWriteAceConfIdempotent(t *testing.T) {
	runtime := t.TempDir()
	spec := aceEngineSpec(runtime)
	t.Setenv("ACE_CONF", "apk")
	if err := writeAceConf(runtime, spec); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runtime, spec.workDir, aceConfName)
	first, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAceConf(runtime, spec); err != nil {
		t.Fatal(err)
	}
	second, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !first.ModTime().Equal(second.ModTime()) {
		t.Fatal("una escritura sin cambios no debe reescribir el fichero")
	}
	// Y sin conf previo tampoco debe crear respaldo.
	if fileExists(filepath.Join(runtime, spec.workDir, aceConfBackup)) {
		t.Fatal("no debe respaldar si no habia conf previo")
	}
}

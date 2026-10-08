package main

// acestream.conf: flags del motor.
//
// El runtime de Windows (motor 3.2.8) trae un conf mínimo con dos flags
// (--stats-report-peers y --service-remote-access); el APK de Android
// (motor 3.2.22.3) trae una lista mucho más larga, extraída de su
// data/acestream.conf: límites de CPU, intervalos de chequeo, rotación de
// logs, política de VOD, etc.
//
// No se mezclan por defecto: los flags vienen de un motor tres versiones mayor
// y no se puede comprobar aquí que el 3.2.8 los conozca (la tabla de flags
// está dentro de los .pyd ofuscados, no en texto plano). Si el motor rechaza
// un flag desconocido, se quedaría sin arrancar. Por eso el perfil completo
// es opt-in con ACE_CONF=apk, y el conf original siempre queda respaldado en
// acestream.conf.orig.

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

const aceConfName = "acestream.conf"
const aceConfBackup = "acestream.conf.orig"

// confFlag es un par flag/valor. Los flags sin valor (--foo) usan value "".
type confFlag struct {
	flag  string
	value string
}

// aceConfFlags devuelve los flags del perfil pedido.
func aceConfFlags(profile string) []confFlag {
	base := []confFlag{
		{flag: "--stats-report-peers"},
		{flag: "--service-remote-access"},
	}
	if !strings.EqualFold(strings.TrimSpace(profile), "apk") {
		return base
	}
	// Traducción literal del conf del APK (motor 3.2.22.3).
	return []confFlag{
		{flag: "--slots-manager-use-cpu-limit", value: "1"},
		{flag: "--core-dlr-periodic-check-interval", value: "5"},
		{flag: "--check-live-pos-interval", value: "5"},
		{flag: "--core-skip-have-before-playback-pos", value: "1"},
		{flag: "--webrtc-allow-outgoing-connections", value: "1"},
		{flag: "--allow-user-config"},
		{flag: "--debug-client-transporter"},
		{flag: "--debug-epg"},
		{flag: "--log-debug", value: "355"},
		{flag: "--log-modules", value: "root:D"},
		{flag: "--log-max-size", value: "15000000"},
		{flag: "--log-backup-count", value: "1"},
		{flag: "--debug-filter-aircast"},
		{flag: "--vod-drop-max-age", value: "120"},
		{flag: "--debug-memory-monitor"},
		{flag: "--disable-sentry"},
	}
}

// aceConfProfile decide el perfil: ACE_CONF=apk activa el completo.
func aceConfProfile() string {
	p := strings.TrimSpace(os.Getenv("ACE_CONF"))
	if p == "" {
		return "base"
	}
	return p
}

// renderAceConf serializa los flags: una línea por flag y, si tiene valor,
// otra con el valor.
func renderAceConf(flags []confFlag) string {
	var b strings.Builder
	for _, f := range flags {
		b.WriteString(f.flag)
		b.WriteString("\n")
		if f.value != "" {
			b.WriteString(f.value)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// writeAceConf escribe acestream.conf en el directorio de trabajo del motor.
//
// En el perfil base NO se toca el conf que trae el asset: cada motor trae el
// suyo y es lo que funciona (Windows: --stats-report-peers y
// --service-remote-access; Linux: añade --stats-report-interval y --log-file).
//
// En el perfil apk (ACE_CONF=apk) se sustituye por la lista larga del APK. Si
// el conf que trae el asset no coincide con lo que se escribe, se guarda como
// acestream.conf.orig una sola vez, para poder volver atrás. Si el contenido ya
// es el correcto, no se toca el fichero.
func writeAceConf(runtimePath string, spec engineSpec) error {
	dir := filepath.Join(runtimePath, spec.workDir)
	path := filepath.Join(dir, aceConfName)
	current, readErr := os.ReadFile(path)

	if !strings.EqualFold(strings.TrimSpace(os.Getenv("ACE_CONF")), "apk") {
		// Perfil base: el conf del asset manda.
		if readErr == nil {
			return nil
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(renderAceConf(aceConfFlags("base"))), 0644)
	}

	want := renderAceConf(aceConfFlags("apk"))
	if readErr == nil {
		if string(current) == want {
			return nil
		}
		backup := filepath.Join(dir, aceConfBackup)
		if !fileExists(backup) {
			if err := os.WriteFile(backup, current, 0644); err != nil {
				return err
			}
			log.Printf("📄 Conf original del motor respaldado en %s", aceConfBackup)
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(want), 0644)
}

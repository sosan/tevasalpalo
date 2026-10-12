package main

// Cola de eventos persistente en disco.
//
// Por qué: la cola en memoria se pierde al reiniciar. Si el central está caído
// justo cuando el usuario cierra el portátil, los eventos se pierden. Con un
// spool en disco sobreviven al reinicio y se envían cuando vuelva la red.
//
// Formato: JSON Lines. Cada línea es un registro tal cual lo produce
// buildRecord. Se lee entero al arrancar y se va truncando por delante a medida
// que se envían, así que no hay que reescribirlo entero cada vez: se copia a un
// fichero temporal y se renombra (atómico en el mismo sistema de ficheros).
//
// Si el spool crece por encima de maxSpoolBytes se descarta lo más antiguo, que
// es lo mismo que hace la cola en memoria pero con un techo en disco en vez de
// en RAM.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type diskSpool struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	bytes    int64
}

func newDiskSpool(path string, maxBytes int64) (*diskSpool, error) {
	if maxBytes <= 0 {
		maxBytes = 16 << 20
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	s := &diskSpool{path: path, maxBytes: maxBytes}
	// Si quedó una cola a medias de un envío anterior, se recupera: son
	// líneas completas, porque solo se hace rename tras escribir entero.
	s.bytes = tamanoFichero(path)
	return s, nil
}

func tamanoFichero(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// append añade un registro y devuelve cuántos bytes escribió.
func (s *diskSpool) append(rec map[string]any) (int, error) {
	b, err := json.Marshal(rec)
	if err != nil {
		return 0, err
	}
	b = append(b, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	n, err := f.Write(b)
	f.Close()
	if err != nil {
		return n, err
	}
	s.bytes += int64(n)
	s.trimLocked()
	return n, nil
}

// trimLocked recorta lo más antiguo cuando se pasa del tope. Cuenta mal en el
// mejor caso y de más en el peor: no merece la pena paginar un fichero de logs
// para estar exacto al byte.
func (s *diskSpool) trimLocked() {
	if s.bytes <= s.maxBytes {
		return
	}
	objetivo := s.maxBytes / 2
	b, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	// Nos quedamos con la cola final: retrocedemos desde el final hasta
	// llegar a objetivo, siempre en un salto de línea.
	ini := len(b) - int(objetivo)
	if ini < 0 {
		ini = 0
	}
	for ini < len(b) && b[ini] != '\n' {
		ini++
	}
	if ini < len(b) {
		ini++
	}
	s.reemplazarLocked(b[ini:])
}

// take devuelve hasta n registros y los elimina del spool. Si no se han
// confirmado con el central, el llamante debe devolverlos con requeue.
func (s *diskSpool) take(n int) []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var (
		out     []map[string]any
		restado []byte
		sc      = bufio.NewScanner(f)
	)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		linea := strings.TrimSpace(sc.Text())
		if linea == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(linea), &rec); err != nil {
			// Línea corrupta: se descarta, no bloquea el resto de la cola.
			continue
		}
		if len(out) < n {
			out = append(out, rec)
			continue
		}
		restado = append(restado, []byte(linea+"\n")...)
	}
	if err := sc.Err(); err != nil && len(out) == 0 {
		return nil
	}
	if len(restado) > 0 {
		s.reemplazarLocked(restado)
	} else if _, err := os.Stat(s.path); err == nil {
		// Todo se ha tomado: se vacía.
		_ = os.WriteFile(s.path, nil, 0o600)
		s.bytes = 0
	}
	return out
}

// requeue devuelve eventos al principio del spool. Se usa cuando el envío
// falla, para no perderlos.
func (s *diskSpool) requeue(recs []map[string]any) {
	if len(recs) == 0 {
		return
	}
	var buf strings.Builder
	for _, r := range recs {
		if b, err := json.Marshal(r); err == nil {
			buf.Write(b)
			buf.WriteByte('\n')
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	resto, err := os.ReadFile(s.path)
	if err != nil || len(resto) == 0 {
		_ = os.WriteFile(s.path, []byte(buf.String()), 0o600)
		s.bytes = int64(buf.Len())
		return
	}
	_ = os.WriteFile(s.path, []byte(buf.String()+string(resto)), 0o600)
	s.bytes = int64(buf.Len()) + int64(len(resto))
	s.trimLocked()
}

// reemplazarLocked sobrescribe el fichero. Se usa rename para que un corte de
// luz a mitad no deje un spool corrupto.
func (s *diskSpool) reemplazarLocked(contenido []byte) {
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, contenido, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return
	}
	s.bytes = int64(len(contenido))
}

func (s *diskSpool) pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	f, err := os.Open(s.path)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			n++
		}
	}
	return n
}

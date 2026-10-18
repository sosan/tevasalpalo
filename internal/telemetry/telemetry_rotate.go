package telemetry

// Escritura en fichero con rotación por tamaño, sin dependencias externas.
//
// Cuando el fichero alcanza maxSize se renombra a .1, .2... hasta maxFiles y
// se empieza uno nuevo. El renombrado se hace de forma que .maxFiles-1 se
// elimina (el más antiguo) para no crecer sin límite. La escritura es
// bloqueante pero trivial (escribir una línea a disco); el coste real está en
// la red, que va aparte y es asíncrona.

import (
	"fmt"
	"os"
	"sync"
)

type rotatingWriter struct {
	mu       sync.Mutex
	path     string
	maxSize  int64
	maxFiles int
	f        *os.File
	written  int64
}

func newRotatingWriter(path string, maxSize int64, maxFiles int) (*rotatingWriter, error) {
	if maxSize <= 0 {
		maxSize = 8 << 20
	}
	if maxFiles < 1 {
		maxFiles = 1
	}
	rw := &rotatingWriter{path: path, maxSize: maxSize, maxFiles: maxFiles}
	if err := rw.open(); err != nil {
		return nil, err
	}
	return rw, nil
}

func (rw *rotatingWriter) open() error {
	f, err := os.OpenFile(rw.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	rw.f = f
	if st, err := f.Stat(); err == nil {
		rw.written = st.Size()
	}
	return nil
}

func (rw *rotatingWriter) Write(p []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.f == nil {
		if err := rw.open(); err != nil {
			return len(p), nil // no romper el log por un fichero
		}
	}
	n, err := rw.f.Write(p)
	rw.written += int64(n)
	if rw.written >= rw.maxSize {
		rw.rotateLocked()
	}
	return n, err
}

// WriteLine escribe un registro JSON como línea propia (para ficheros de
// eventos estructurados, no texto plano del log).
func (rw *rotatingWriter) WriteLine(p []byte) (int, error) {
	line := append(append([]byte{}, p...), '\n')
	return rw.Write(line)
}

func (rw *rotatingWriter) rotateLocked() {
	if rw.f != nil {
		rw.f.Close()
		rw.f = nil
	}
	// Elimina el más antiguo si ya hay maxFiles.
	if rw.maxFiles > 0 {
		oldest := fmt.Sprintf("%s.%d", rw.path, rw.maxFiles-1)
		os.Remove(oldest)
	}
	// Desplaza .(n-1) -> .n, de mayor a menor para no pisar.
	for i := rw.maxFiles - 2; i >= 0; i-- {
		from := fmt.Sprintf("%s.%d", rw.path, i)
		to := fmt.Sprintf("%s.%d", rw.path, i+1)
		if _, err := os.Stat(from); err == nil {
			os.Rename(from, to)
		}
	}
	// El principal pasa a .0 y se abre uno nuevo.
	os.Rename(rw.path, rw.path+".0")
	_ = rw.open()
}

// Close cierra el descriptor (llamar en defer si se quiere liberar ya).
func (rw *rotatingWriter) Close() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.f == nil {
		return nil
	}
	err := rw.f.Close()
	rw.f = nil
	return err
}

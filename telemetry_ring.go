package main

// Buffer circular en memoria: guarda las últimas N líneas para poder
// descargarlas desde /api/logs o para el diagnóstico de errores sin tener que
// tocar disco. Cuando se llena descarta la más antigua.

import "sync"

type ringBuffer struct {
	mu    sync.Mutex
	buf   []string
	next  int
	count int
}

func newRingBuffer(capacity int) *ringBuffer {
	if capacity <= 0 {
		capacity = 1
	}
	return &ringBuffer{buf: make([]string, capacity)}
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.next] = string(p)
	r.next = (r.next + 1) % len(r.buf)
	if r.count < len(r.buf) {
		r.count++
	}
	return len(p), nil
}

func (r *ringBuffer) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// tail devuelve las últimas limit líneas en orden cronológico.
func (r *ringBuffer) tail(limit int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 || limit > r.count {
		limit = r.count
	}
	out := make([]string, 0, limit)
	start := r.next - limit
	if start < 0 {
		start += len(r.buf)
	}
	for i := 0; i < limit; i++ {
		idx := (start + i) % len(r.buf)
		out = append(out, r.buf[idx])
	}
	return out
}

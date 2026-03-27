package daemon

import (
	"bytes"
	"io"
	"sync"
)

// LogStore holds captured stdout/stderr per container.
// Thread-safe for concurrent read/write.
type LogStore struct {
	mu   sync.RWMutex
	logs map[string]*ContainerLog
}

// ContainerLog holds the buffered output for a container.
type ContainerLog struct {
	mu     sync.Mutex
	stdout bytes.Buffer
	stderr bytes.Buffer
}

// NewLogStore creates a log store.
func NewLogStore() *LogStore {
	return &LogStore{logs: make(map[string]*ContainerLog)}
}

// Create allocates a log buffer for a container. Returns writers that
// tee to both the buffer and the provided destination.
func (s *LogStore) Create(name string, origStdout, origStderr io.Writer) (stdout, stderr io.Writer) {
	cl := &ContainerLog{}
	s.mu.Lock()
	s.logs[name] = cl
	s.mu.Unlock()

	stdout = io.MultiWriter(origStdout, &logWriter{cl: cl, isStderr: false})
	stderr = io.MultiWriter(origStderr, &logWriter{cl: cl, isStderr: true})
	return stdout, stderr
}

// Get returns the captured output for a container.
func (s *LogStore) Get(name string) (stdout, stderr string, ok bool) {
	s.mu.RLock()
	cl, exists := s.logs[name]
	s.mu.RUnlock()
	if !exists {
		return "", "", false
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.stdout.String(), cl.stderr.String(), true
}

// Remove deletes a container's log buffer.
func (s *LogStore) Remove(name string) {
	s.mu.Lock()
	delete(s.logs, name)
	s.mu.Unlock()
}

// logWriter is a thread-safe writer that appends to a ContainerLog buffer.
type logWriter struct {
	cl       *ContainerLog
	isStderr bool
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.cl.mu.Lock()
	defer w.cl.mu.Unlock()
	if w.isStderr {
		return w.cl.stderr.Write(p)
	}
	return w.cl.stdout.Write(p)
}

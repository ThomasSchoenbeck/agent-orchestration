package workflow

import (
	"fmt"
	"sync"
)

// PortPool manages a bounded pool of TCP port numbers for agent task servers.
// Ports are allocated on claim and released when the agent transitions out of
// an execution state. The pool is in-process only — it resets on server restart,
// which is acceptable because ports are also stored on the task row and released
// by the maintenance loop.
type PortPool struct {
	mu   sync.Mutex
	free []int
}

// NewPortPool creates a pool covering [start, start+size).
func NewPortPool(start, size int) *PortPool {
	ports := make([]int, size)
	for i := range ports {
		ports[i] = start + i
	}
	return &PortPool{free: ports}
}

// Acquire returns a free port, or an error if the pool is exhausted.
func (p *PortPool) Acquire() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.free) == 0 {
		return 0, fmt.Errorf("port pool exhausted")
	}
	port := p.free[0]
	p.free = p.free[1:]
	return port, nil
}

// Release returns a port to the pool. It is a no-op for port 0.
func (p *PortPool) Release(port int) {
	if port == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.free = append(p.free, port)
}

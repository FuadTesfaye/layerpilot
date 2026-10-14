package engine

import (
	"context"
	"sync"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

// Registry manages the set of available compatibility engines
type Registry struct {
	mu      sync.RWMutex
	engines map[string]Engine
}

// NewRegistry creates an empty engine registry
func NewRegistry() *Registry {
	return &Registry{
		engines: make(map[string]Engine),
	}
}

// Register adds an engine to the registry
func (r *Registry) Register(e Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engines[e.ID()] = e
}

// Get returns an engine by its ID
func (r *Registry) Get(id string) (Engine, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.engines[id]
	return e, ok
}

// List returns all registered engines
func (r *Registry) List() []Engine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Engine, 0, len(r.engines))
	for _, e := range r.engines {
		list = append(list, e)
	}
	return list
}

// AvailableEngineIDs returns the IDs of engines that successfully probe as available
func (r *Registry) AvailableEngineIDs(ctx context.Context, host *domain.HostProfile) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var available []string
	for id, eng := range r.engines {
		rep, err := eng.Probe(ctx, host)
		if err == nil && rep != nil && rep.Available {
			available = append(available, id)
		}
	}
	return available
}

// ProbeAll returns capability reports for all registered engines
func (r *Registry) ProbeAll(ctx context.Context, host *domain.HostProfile) map[string]*domain.CapabilityReport {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reports := make(map[string]*domain.CapabilityReport)
	for id, eng := range r.engines {
		rep, err := eng.Probe(ctx, host)
		if err != nil {
			reports[id] = &domain.CapabilityReport{
				EngineID:   id,
				EngineName: eng.Name(),
				Available:  false,
				DiagnosticDetails: map[string]string{
					"error": err.Error(),
				},
			}
		} else {
			reports[id] = rep
		}
	}
	return reports
}

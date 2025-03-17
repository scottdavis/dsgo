package agents

import (
	"fmt"
	"sync"

	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/errors"
)

// ModuleRegistry provides a thread-safe registry for modules
type ModuleRegistry struct {
	modules map[string]core.Module
	mu      sync.RWMutex
}

// NewModuleRegistry creates a new ModuleRegistry
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		modules: make(map[string]core.Module),
	}
}

// Register adds a module to the registry with the given name
func (r *ModuleRegistry) Register(name string, module core.Module) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modules[name] = module
}

// Get retrieves a module by name
func (r *ModuleRegistry) Get(name string) (core.Module, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	module, exists := r.modules[name]
	if !exists {
		return nil, errors.WithFields(
			errors.New(errors.ResourceNotFound, "module not found in registry"),
			errors.Fields{"name": name},
		)
	}
	
	return module, nil
}

// GetBySignature retrieves a module by its signature string
func (r *ModuleRegistry) GetBySignature(signature string) (core.Module, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// First try direct lookup
	module, exists := r.modules[signature]
	if exists {
		return module, nil
	}
	
	// Try to match by module type name (from %T)
	for _, module := range r.modules {
		typeName := fmt.Sprintf("%T", module)
		if typeName == signature {
			return module, nil
		}
	}
	
	return nil, errors.WithFields(
		errors.New(errors.ResourceNotFound, "module not found for signature"),
		errors.Fields{"signature": signature},
	)
}

// List returns all registered module names
func (r *ModuleRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	
	return names
}

// Count returns the number of registered modules
func (r *ModuleRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.modules)
}

// Unregister removes a module from the registry
func (r *ModuleRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.modules, name)
}

// Clear removes all modules from the registry
func (r *ModuleRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modules = make(map[string]core.Module)
} 
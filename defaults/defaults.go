package defaults

import "context"

// Generator with no action
type Generator struct {
	enabled bool
}

// NewGenerator new renewer
func NewGenerator() *Generator {
	return &Generator{
		enabled: false,
	}
}

// Get always ""
func (g *Generator) Get(context.Context) (string, error) {
	return "", nil
}

// Enabled false
func (g *Generator) Enabled() bool {
	return false
}

// Run do nothing
func (g *Generator) Run(context.Context) {
}

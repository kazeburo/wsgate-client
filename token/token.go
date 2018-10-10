package token

import "context"

// Generator interface
type Generator interface {
	Get(context.Context) (string, error)
	Enabled() bool
	Run(context.Context)
}

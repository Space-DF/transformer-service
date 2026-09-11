package common

import "fmt"

// ParserRegistry is implemented by components that accept device parsers.
type ParserRegistry interface {
	RegisterParser(model, manufacturer string, parser Parser)
}

// RegisterParser validates and registers a parser with r.
func RegisterParser(r ParserRegistry, model, manufacturer string, parser Parser) error {
	if model == "" {
		return fmt.Errorf("device model name must not be empty")
	}
	r.RegisterParser(model, manufacturer, parser)
	return nil
}

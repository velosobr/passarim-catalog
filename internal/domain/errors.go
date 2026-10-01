package domain

import "errors"

// ErrNotFound indica que o item pedido não existe.
// Camadas de fora traduzem isso para NOT_FOUND (gRPC) ou 404 (HTTP).
var ErrNotFound = errors.New("não encontrado")

// InvalidArgumentError indica que um dado de entrada está errado.
// Field diz QUAL campo, para que o erro seja útil para quem chamou.
type InvalidArgumentError struct {
	Field  string
	Reason string
}

func (e *InvalidArgumentError) Error() string { return e.Field + ": " + e.Reason }

func invalid(field, reason string) error { return &InvalidArgumentError{Field: field, Reason: reason} }

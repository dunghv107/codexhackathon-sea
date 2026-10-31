package repository

import "context"

// Transactor runs fn in one DB transaction; repositories called with the ctx passed to fn join it.
type Transactor interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

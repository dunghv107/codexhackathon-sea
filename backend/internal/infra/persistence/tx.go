package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
)

type txKey struct{}

// dbFrom returns the transaction stored in ctx, else the pool.
func dbFrom(ctx context.Context, base *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return base.WithContext(ctx)
}

type gormTransactor struct{ db *gorm.DB }

// NewTransactor creates a GORM-backed Transactor
func NewTransactor(db *gorm.DB) repository.Transactor { return &gormTransactor{db: db} }

func (t *gormTransactor) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return fn(ctx) // already inside a transaction
	}
	return dbFrom(ctx, t.db).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

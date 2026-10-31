package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/query"
)

// BaseRepositoryImpl provides generic CRUD operations
type BaseRepositoryImpl[T any] struct {
	DB            *gorm.DB
	AllowedFields map[string]bool
	EntityName    string
}

// NewBaseRepository creates a new base repository
func NewBaseRepository[T any](db *gorm.DB, allowedFields map[string]bool, entityName string) *BaseRepositoryImpl[T] {
	return &BaseRepositoryImpl[T]{
		DB:            db,
		AllowedFields: allowedFields,
		EntityName:    entityName,
	}
}

// Create creates a new entity
func (r *BaseRepositoryImpl[T]) Create(ctx context.Context, entity *T) error {
	if err := dbFrom(ctx, r.DB).Create(entity).Error; err != nil {
		return wrapCreateError(err, r.EntityName)
	}
	return nil
}

// FindByID finds an entity by ID
func (r *BaseRepositoryImpl[T]) FindByID(ctx context.Context, id uint) (*T, error) {
	var entity T
	if err := dbFrom(ctx, r.DB).First(&entity, id).Error; err != nil {
		return nil, wrapFindError(err, r.EntityName)
	}
	return &entity, nil
}

// Update updates the given columns of an entity, or the whole row when none are given
func (r *BaseRepositoryImpl[T]) Update(ctx context.Context, entity *T, columns ...string) error {
	db := dbFrom(ctx, r.DB)
	var err error
	if len(columns) > 0 {
		err = db.Model(entity).Select(columns).Updates(entity).Error
	} else {
		err = db.Save(entity).Error
	}
	if err != nil {
		return wrapUpdateError(err, r.EntityName)
	}
	return nil
}

// Delete soft-deletes an entity
func (r *BaseRepositoryImpl[T]) Delete(ctx context.Context, id uint) error {
	var entity T
	if err := dbFrom(ctx, r.DB).Delete(&entity, id).Error; err != nil {
		return wrapDeleteError(err, r.EntityName)
	}
	return nil
}

// List lists entities with query options
func (r *BaseRepositoryImpl[T]) List(ctx context.Context, offset, limit int, opts query.QueryOptions) ([]T, int64, error) {
	var entities []T
	var total int64

	q := dbFrom(ctx, r.DB).Model(new(T))

	// Apply filters
	q = q.Scopes(query.ApplyFilters(opts, r.AllowedFields))

	// Count total
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, wrapListError(err, r.EntityName)
	}

	// Apply sort and pagination
	if err := q.Scopes(
		query.ApplySort(opts, r.AllowedFields),
		query.ApplyDefaultSort(opts, "created_at", true),
	).Offset(offset).Limit(limit).Find(&entities).Error; err != nil {
		return nil, 0, wrapListError(err, r.EntityName)
	}

	return entities, total, nil
}

// Exists checks if an entity exists
func (r *BaseRepositoryImpl[T]) Exists(ctx context.Context, id uint) (bool, error) {
	var count int64
	if err := dbFrom(ctx, r.DB).Model(new(T)).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, wrapFindError(err, r.EntityName)
	}
	return count > 0, nil
}

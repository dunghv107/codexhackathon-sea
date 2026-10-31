package persistence

import (
	"context"
	"errors"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/query"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"testing"
)

func TestRepositoryErrorsRetainCauseAndClassifyConflict(t *testing.T) {
	for _, msg := range []string{"duplicate key value", "Duplicate entry", "UNIQUE constraint failed", "violates unique constraint"} {
		e := errors.New(msg)
		for _, wrap := range []func(error, string) error{wrapCreateError, wrapUpdateError} {
			var app *apperror.AppError
			got := wrap(e, "item")
			if !errors.As(got, &app) || app.HTTPStatus != 409 || app.Err != e {
				t.Fatal(got)
			}
		}
	}
	if isDuplicateKeyError(nil) || isDuplicateKeyError(errors.New("timeout")) {
		t.Fatal("nonconflict classified as duplicate")
	}
	e := errors.New("database unavailable")
	for _, wrap := range []func(error, string) error{wrapFindError, wrapCreateError, wrapUpdateError, wrapDeleteError, wrapListError} {
		var app *apperror.AppError
		got := wrap(e, "item")
		if !errors.As(got, &app) || app.HTTPStatus != 500 || app.Err != e {
			t.Fatal(got)
		}
	}
	var app *apperror.AppError
	if !errors.As(wrapFindError(gorm.ErrRecordNotFound, "item"), &app) || app.HTTPStatus != 404 {
		t.Fatal(app)
	}
}
func TestCanceledQueriesNeverSucceed(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable connect_timeout=1"}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := NewBaseRepository[entity.User](db, map[string]bool{"id": true}, "user")
	user := &entity.User{ID: 1}
	checks := []func() error{func() error { return repo.Create(ctx, user) }, func() error { return repo.Update(ctx, user) }, func() error { return repo.Update(ctx, user, "username") }, func() error { return repo.Delete(ctx, 1) }, func() error { _, e := repo.FindByID(ctx, 1); return e }, func() error { _, e := repo.Exists(ctx, 1); return e }, func() error { _, _, e := repo.List(ctx, 0, 10, query.NewQueryOptions()); return e }}
	for _, check := range checks {
		if err := check(); err == nil {
			t.Fatal("canceled operation succeeded")
		}
	}
}

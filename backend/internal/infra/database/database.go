package database

import (
	"net/url"
	"strconv"
	"sync"

	"github.com/thienel/tlog"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/config"
)

var (
	db   *gorm.DB
	once sync.Once
)

// Init initializes the database connection
func Init(cfg *config.DatabaseConfig) error {
	var initErr error

	once.Do(func() {
		// search_path: domain tables live in schema sfd, users stays in public
		dsn := connectionString(cfg)

		gormConfig := &gorm.Config{
			Logger: tlog.NewGormLogger(),
		}

		db, initErr = gorm.Open(postgres.Open(dsn), gormConfig)
		if initErr != nil {
			return
		}

		sqlDB, err := db.DB()
		if err != nil {
			initErr = err
			return
		}

		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)

		tlog.Info("Database connection established",
			zap.String("host", cfg.Host),
			zap.Int("port", cfg.Port),
			zap.String("database", cfg.DBName),
		)
	})

	return initErr
}

// GetDB returns the database instance
func GetDB() *gorm.DB {
	return db
}

// Close closes the database connection
func Close() error {
	if db != nil {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// Encode each value so empty passwords and spaces cannot consume adjacent DSN fields.
func connectionString(cfg *config.DatabaseConfig) string {
	values := url.Values{
		"host": {cfg.Host}, "port": {strconv.Itoa(cfg.Port)}, "user": {cfg.User},
		"password": {cfg.Password}, "dbname": {cfg.DBName}, "sslmode": {cfg.SSLMode},
		"timezone": {cfg.TimeZone}, "search_path": {"sfd,public"},
	}
	return "postgresql://?" + values.Encode()
}

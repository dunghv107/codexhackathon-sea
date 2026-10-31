package database

import (
	"github.com/jackc/pgx/v5"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/config"
	"testing"
)

func TestConnectionStringPreservesValues(t *testing.T) {
	for _, password := range []string{"", "space quote' and \\ @?=&"} {
		cfg := config.DatabaseConfig{Host: "localhost", Port: 55439, User: "demo", Password: password, DBName: "olist demo", SSLMode: "disable", TimeZone: "Asia/Ho_Chi_Minh"}
		parsed, err := pgx.ParseConfig(connectionString(&cfg))
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Password != password || parsed.Database != cfg.DBName || parsed.Host != cfg.Host || parsed.Port != 55439 || parsed.RuntimeParams["search_path"] != "sfd,public" {
			t.Fatal("connection values changed during encoding")
		}
	}
}

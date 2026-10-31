package config

import (
	"os"
	"reflect"
	"testing"
)

func TestEnvironmentConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	old := AppConfig
	t.Cleanup(func() { AppConfig = old })
	t.Setenv("PORT", "9001")
	t.Setenv("ENV", "production")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("LOG_ENABLE_CONSOLE", "false")
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://one.test, ,https://two.test ")
	c, e := Load()
	if e != nil || c.Server.Port != "9001" || c.Database.Port != 6543 || c.Log.EnableConsole || !c.IsProduction() || c.IsDevelopment() || GetConfig() != c {
		t.Fatalf("configuration: %+v %v", c, e)
	}
	if !reflect.DeepEqual(c.CORSAllowedOrigins, []string{"https://one.test", "https://two.test"}) {
		t.Fatal(c.CORSAllowedOrigins)
	}
	c.Server.Env = "development"
	if !c.IsDevelopment() {
		t.Fatal("development not recognized")
	}
	for _, v := range []string{"redis://user:password@host:6380/1", "host:6380", ":%"} {
		c.RedisURL = v
		want := v
		if v[0] == 'r' {
			want = "host:6380"
		}
		if c.GetRedisAddr() != want {
			t.Fatal(c.GetRedisAddr())
		}
	}
	t.Setenv("CHECK_INT", "bad")
	if getEnvInt("CHECK_INT", 9) != 9 {
		t.Fatal("invalid integer")
	}
	t.Setenv("CHECK_BOOL", "bad")
	if !getEnvBool("CHECK_BOOL", true) {
		t.Fatal("invalid boolean")
	}
	t.Setenv("CHECK_MISSING", "")
	os.Unsetenv("CHECK_MISSING")
	if getEnv("CHECK_MISSING", "fallback") != "fallback" || getEnvInt("CHECK_MISSING", 7) != 7 || getEnvBool("CHECK_MISSING", false) {
		t.Fatal("missing defaults")
	}
	if len(parseCSV("")) != 0 {
		t.Fatal("empty CSV")
	}
	if err := os.WriteFile(".env", []byte("TEST_DOTENV_MARKER=loaded\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_DOTENV_MARKER", "")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}

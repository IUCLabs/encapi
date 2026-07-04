package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	for _, k := range []string{"LISTEN_ADDR", "DBHOST", "DBPORT", "DBUSER", "DBPASS", "DBNAME", "DBSSL", "ENCAPI_WRITE_TOKEN", "ENCAPI_DISTRO_API_URL"} {
		t.Setenv(k, "")
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ListenAddr != ":8000" {
		t.Errorf("ListenAddr = %q, want :8000", c.ListenAddr)
	}
	if c.DBPort != 5432 {
		t.Errorf("DBPort = %d, want 5432", c.DBPort)
	}
	if c.WriteToken != "" {
		t.Errorf("WriteToken = %q, want empty", c.WriteToken)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("LISTEN_ADDR", ":9000")
	t.Setenv("DBPORT", "6543")
	t.Setenv("DBHOST", "pg.example")
	t.Setenv("ENCAPI_WRITE_TOKEN", "s3cret")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ListenAddr != ":9000" || c.DBPort != 6543 || c.DBHost != "pg.example" || c.WriteToken != "s3cret" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestLoadBadPort(t *testing.T) {
	t.Setenv("DBPORT", "notanumber")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for non-numeric DBPORT")
	}
}

func TestDatabaseDSN(t *testing.T) {
	c := &Config{DBUser: "u", DBPass: "p", DBHost: "h", DBPort: 5432, DBName: "n", DBSSL: "require"}
	want := "postgres://u:p@h:5432/n?sslmode=require"
	if got := c.DatabaseDSN(); got != want {
		t.Errorf("DSN = %q, want %q", got, want)
	}
}

// Package testsupport starts a throwaway Postgres container for
// integration-style unit tests. It is only imported from *_test.go files, so
// it never reaches the production binary. Tests skip themselves when Docker is
// unavailable.
package testsupport

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func init() {
	// The Ryuk reaper container cannot start in every environment; callers get
	// an explicit terminate func for cleanup instead.
	if _, ok := os.LookupEnv("TESTCONTAINERS_RYUK_DISABLED"); !ok {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

// StartPostgres launches postgres:17-alpine and returns its DSN plus a
// terminate func.
func StartPostgres(ctx context.Context) (dsn string, terminate func(), err error) {
	c, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("encapi"),
		tcpostgres.WithUsername("encapi"),
		tcpostgres.WithPassword("encapi123"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return "", nil, err
	}
	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "5432/tcp")
	dsn = fmt.Sprintf("postgres://encapi:encapi123@%s:%s/encapi?sslmode=disable", host, port.Port())
	return dsn, func() { _ = c.Terminate(ctx) }, nil
}

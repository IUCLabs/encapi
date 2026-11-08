package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/IUCLabs/encapi/pkg/models"
)

// ErrInvalidReference is returned when a node names a role or environment that
// does not exist.
var ErrInvalidReference = errors.New("unknown role or environment")

// ErrNotInTesting is returned when restoring a node that is not in testing.
var ErrNotInTesting = errors.New("node is not in testing")

// ErrNoPreviousEnvironment is returned when restoring a testing node that has
// no recorded environment to return to.
var ErrNoPreviousEnvironment = errors.New("node has no previous environment to restore")

const nodeCols = `certname, role, environment, params, COALESCE(previous_environment, '')`

// prevEnv computes previous_environment for a move to newEnv: entering testing
// remembers the current environment, staying keeps it, leaving clears it.
func prevEnv(newEnv string) string {
	return `CASE WHEN ` + newEnv + ` <> 'testing' THEN NULL
		WHEN nodes.environment = 'testing' THEN nodes.previous_environment
		ELSE nodes.environment END`
}

func scanNode(row pgx.Row) (*models.Node, error) {
	var (
		n      models.Node
		params []byte
	)
	if err := row.Scan(&n.Certname, &n.Role, &n.Environment, &params, &n.PreviousEnvironment); err != nil {
		return nil, err
	}
	var err error
	if n.Params, err = unmarshalParams(params); err != nil {
		return nil, fmt.Errorf("decode params for node %q: %w", n.Certname, err)
	}
	return &n, nil
}

// fkErr maps a foreign-key violation to ErrInvalidReference.
func fkErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrInvalidReference
	}
	return err
}

// UpsertNode creates or updates a host-to-role assignment. The referenced role
// and environment must already exist (enforced by foreign keys).
func (db *DB) UpsertNode(ctx context.Context, n *models.Node) error {
	params, err := marshalParams(n.Params)
	if err != nil {
		return fmt.Errorf("marshal params for node %q: %w", n.Certname, err)
	}
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO nodes (certname, role, environment, params)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (certname) DO UPDATE
		SET role = EXCLUDED.role,
		    environment = EXCLUDED.environment,
		    params = EXCLUDED.params,
		    previous_environment = `+prevEnv("EXCLUDED.environment")+`,
		    updated_at = NOW()
		RETURNING COALESCE(previous_environment, '')
	`, n.Certname, n.Role, n.Environment, params).Scan(&n.PreviousEnvironment)
	if err != nil {
		return fmt.Errorf("upsert node %q: %w", n.Certname, fkErr(err))
	}
	return nil
}

// SetNodeEnvironment changes only a node's environment in a single statement and
// returns the updated node, ErrNotFound, or ErrInvalidReference.
func (db *DB) SetNodeEnvironment(ctx context.Context, certname, environment string) (*models.Node, error) {
	n, err := scanNode(db.Pool.QueryRow(ctx, `
		UPDATE nodes SET environment = $2,
		    previous_environment = `+prevEnv("$2::text")+`,
		    updated_at = NOW()
		WHERE certname = $1
		RETURNING `+nodeCols, certname, environment))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("set environment for node %q: %w", certname, fkErr(err))
	}
	return n, nil
}

// RestoreFromTesting moves a testing node back to its previous environment and
// returns it, or ErrNotFound, ErrNotInTesting or ErrNoPreviousEnvironment.
func (db *DB) RestoreFromTesting(ctx context.Context, certname string) (*models.Node, error) {
	n, err := scanNode(db.Pool.QueryRow(ctx, `
		UPDATE nodes SET environment = previous_environment,
		    previous_environment = NULL,
		    updated_at = NOW()
		WHERE certname = $1 AND environment = 'testing' AND previous_environment IS NOT NULL
		RETURNING `+nodeCols, certname))
	if errors.Is(err, pgx.ErrNoRows) {
		cur, gerr := db.GetNode(ctx, certname)
		switch {
		case gerr != nil:
			return nil, gerr
		case cur.Environment != "testing":
			return nil, ErrNotInTesting
		default:
			return nil, ErrNoPreviousEnvironment
		}
	}
	if err != nil {
		return nil, fmt.Errorf("restore environment for node %q: %w", certname, err)
	}
	return n, nil
}

// GetNode returns a single node or ErrNotFound.
func (db *DB) GetNode(ctx context.Context, certname string) (*models.Node, error) {
	n, err := scanNode(db.Pool.QueryRow(ctx, `SELECT `+nodeCols+` FROM nodes WHERE certname = $1`, certname))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get node %q: %w", certname, err)
	}
	return n, nil
}

// ListNodes returns all nodes ordered by certname.
func (db *DB) ListNodes(ctx context.Context) ([]models.Node, error) {
	rows, err := db.Pool.Query(ctx, `SELECT `+nodeCols+` FROM nodes ORDER BY certname`)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()

	out := []models.Node{}
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// DeleteNode removes a host assignment.
func (db *DB) DeleteNode(ctx context.Context, certname string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM nodes WHERE certname = $1`, certname)
	if err != nil {
		return fmt.Errorf("delete node %q: %w", certname, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

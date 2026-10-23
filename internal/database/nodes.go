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
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO nodes (certname, role, environment, params)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (certname) DO UPDATE
		SET role = EXCLUDED.role,
		    environment = EXCLUDED.environment,
		    params = EXCLUDED.params,
		    updated_at = NOW()
	`, n.Certname, n.Role, n.Environment, params)
	if err != nil {
		return fmt.Errorf("upsert node %q: %w", n.Certname, fkErr(err))
	}
	return nil
}

// SetNodeEnvironment changes only a node's environment in a single statement and
// returns the updated node, ErrNotFound, or ErrInvalidReference.
func (db *DB) SetNodeEnvironment(ctx context.Context, certname, environment string) (*models.Node, error) {
	var (
		n      models.Node
		params []byte
	)
	err := db.Pool.QueryRow(ctx, `
		UPDATE nodes SET environment = $2, updated_at = NOW()
		WHERE certname = $1
		RETURNING certname, role, environment, params
	`, certname, environment).Scan(&n.Certname, &n.Role, &n.Environment, &params)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("set environment for node %q: %w", certname, fkErr(err))
	}
	if n.Params, err = unmarshalParams(params); err != nil {
		return nil, fmt.Errorf("decode params for node %q: %w", certname, err)
	}
	return &n, nil
}

// GetNode returns a single node or ErrNotFound.
func (db *DB) GetNode(ctx context.Context, certname string) (*models.Node, error) {
	var (
		n      models.Node
		params []byte
	)
	err := db.Pool.QueryRow(ctx,
		`SELECT certname, role, environment, params FROM nodes WHERE certname = $1`, certname,
	).Scan(&n.Certname, &n.Role, &n.Environment, &params)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get node %q: %w", certname, err)
	}
	if n.Params, err = unmarshalParams(params); err != nil {
		return nil, fmt.Errorf("decode params for node %q: %w", certname, err)
	}
	return &n, nil
}

// ListNodes returns all nodes ordered by certname.
func (db *DB) ListNodes(ctx context.Context) ([]models.Node, error) {
	rows, err := db.Pool.Query(ctx, `SELECT certname, role, environment, params FROM nodes ORDER BY certname`)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()

	out := []models.Node{}
	for rows.Next() {
		var (
			n      models.Node
			params []byte
		)
		if err := rows.Scan(&n.Certname, &n.Role, &n.Environment, &params); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		if n.Params, err = unmarshalParams(params); err != nil {
			return nil, fmt.Errorf("decode params: %w", err)
		}
		out = append(out, n)
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

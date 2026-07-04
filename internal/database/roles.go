package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/IUCLabs/encapi/pkg/models"
)

// UpsertRole creates or updates a role and its inheritable default params.
func (db *DB) UpsertRole(ctx context.Context, r *models.Role) error {
	params, err := marshalParams(r.DefaultParams)
	if err != nil {
		return fmt.Errorf("marshal default_params for role %q: %w", r.Name, err)
	}
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO roles (name, description, default_params)
		VALUES ($1, $2, $3)
		ON CONFLICT (name) DO UPDATE
		SET description = EXCLUDED.description,
		    default_params = EXCLUDED.default_params,
		    updated_at = NOW()
	`, r.Name, r.Description, params)
	if err != nil {
		return fmt.Errorf("upsert role %q: %w", r.Name, err)
	}
	return nil
}

// GetRole returns a single role or ErrNotFound.
func (db *DB) GetRole(ctx context.Context, name string) (*models.Role, error) {
	var (
		r      models.Role
		params []byte
	)
	err := db.Pool.QueryRow(ctx,
		`SELECT name, description, default_params FROM roles WHERE name = $1`, name,
	).Scan(&r.Name, &r.Description, &params)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get role %q: %w", name, err)
	}
	if r.DefaultParams, err = unmarshalParams(params); err != nil {
		return nil, fmt.Errorf("decode default_params for role %q: %w", name, err)
	}
	return &r, nil
}

// ListRoles returns all roles ordered by name.
func (db *DB) ListRoles(ctx context.Context) ([]models.Role, error) {
	rows, err := db.Pool.Query(ctx, `SELECT name, description, default_params FROM roles ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	out := []models.Role{}
	for rows.Next() {
		var (
			r      models.Role
			params []byte
		)
		if err := rows.Scan(&r.Name, &r.Description, &params); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		if r.DefaultParams, err = unmarshalParams(params); err != nil {
			return nil, fmt.Errorf("decode default_params: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRole removes a role. It fails if any node still references it.
func (db *DB) DeleteRole(ctx context.Context, name string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM roles WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("delete role %q: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// marshalParams renders a params map to JSONB bytes, treating nil as {}.
func marshalParams(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

// unmarshalParams decodes JSONB bytes into a params map, treating empty as {}.
func unmarshalParams(b []byte) (map[string]any, error) {
	m := map[string]any{}
	if len(b) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/IUCLabs/encapi/pkg/models"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// UpsertStatus creates or updates a status (Puppet environment).
func (db *DB) UpsertStatus(ctx context.Context, s *models.Status) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO statuses (name, description)
		VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE
		SET description = EXCLUDED.description, updated_at = NOW()
	`, s.Name, s.Description)
	if err != nil {
		return fmt.Errorf("upsert status %q: %w", s.Name, err)
	}
	return nil
}

// GetStatus returns a single status or ErrNotFound.
func (db *DB) GetStatus(ctx context.Context, name string) (*models.Status, error) {
	var s models.Status
	err := db.Pool.QueryRow(ctx,
		`SELECT name, description FROM statuses WHERE name = $1`, name,
	).Scan(&s.Name, &s.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get status %q: %w", name, err)
	}
	return &s, nil
}

// ListStatuses returns all statuses ordered by name.
func (db *DB) ListStatuses(ctx context.Context) ([]models.Status, error) {
	rows, err := db.Pool.Query(ctx, `SELECT name, description FROM statuses ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list statuses: %w", err)
	}
	defer rows.Close()

	out := []models.Status{}
	for rows.Next() {
		var s models.Status
		if err := rows.Scan(&s.Name, &s.Description); err != nil {
			return nil, fmt.Errorf("scan status: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteStatus removes a status. It fails if any node still references it
// (enforced by the nodes.environment foreign key).
func (db *DB) DeleteStatus(ctx context.Context, name string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM statuses WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("delete status %q: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

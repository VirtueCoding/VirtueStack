package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type Setting struct {
	Key         string
	Value       string
	Description string
}

type SettingsRepository struct {
	db DB
}

func NewSettingsRepository(db DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) List(ctx context.Context) ([]Setting, error) {
	const q = `
		SELECT key,
		       COALESCE(value #>> '{}', ''),
		       COALESCE(description, '')
		FROM system_settings
		ORDER BY key ASC`

	settings, err := ScanRows(ctx, r.db, q, nil, func(rows pgx.Rows) (Setting, error) {
		var s Setting
		err := rows.Scan(&s.Key, &s.Value, &s.Description)
		return s, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing settings: %w", err)
	}

	return settings, nil
}

func (r *SettingsRepository) Get(ctx context.Context, key string) (*Setting, error) {
	const q = `
		SELECT key,
		       COALESCE(value #>> '{}', ''),
		       COALESCE(description, '')
		FROM system_settings
		WHERE key = $1`

	setting, err := ScanRow(ctx, r.db, q, []any{key}, func(row pgx.Row) (Setting, error) {
		var s Setting
		err := row.Scan(&s.Key, &s.Value, &s.Description)
		return s, err
	})
	if err != nil {
		return nil, fmt.Errorf("getting setting %s: %w", key, err)
	}

	return &setting, nil
}

func (r *SettingsRepository) Upsert(ctx context.Context, key, value string) error {
	const q = `
		INSERT INTO system_settings (key, value, updated_at)
		VALUES ($1, to_jsonb($2::text), NOW())
		ON CONFLICT (key)
		DO UPDATE SET value = to_jsonb($2::text), updated_at = NOW()`

	if _, err := r.db.Exec(ctx, q, key, value); err != nil {
		return fmt.Errorf("upserting setting %s: %w", key, err)
	}

	return nil
}

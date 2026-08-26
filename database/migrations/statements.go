package migrations

import (
	"context"

	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
)

func statements(ctx context.Context, conn hesapemigrations.Connection, queries ...string) error {
	for _, query := range queries {
		if _, err := conn.Statement(ctx, query, nil); err != nil {
			return err
		}
	}
	return nil
}

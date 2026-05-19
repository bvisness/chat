package chat

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bvisness/chat/db"
)

var migrations = []db.Migration{
	{
		Name: "Initial",
		Forward: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
				CREATE TABLE records (
					id INTEGER NOT NULL PRIMARY KEY,
					data BLOB NOT NULL
				);
			`)
			return err
		},
		Backward: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
				DROP TABLE records;
			`)
			return err
		},
	},
}

var migrator = db.Migrator{
	ReadCurrentID: func(ctx context.Context, conn *sql.DB) (int, error) {
		_, err := conn.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS _migration AS
				SELECT -1 AS current_id, '' AS current_name
		`)
		if err != nil {
			return 0, fmt.Errorf("failed to create migration table: %w", err)
		}
		return db.QueryOne[int](ctx, conn, `SELECT current_id FROM _migration`)
	},
	WriteCurrent: func(ctx context.Context, tx *sql.Tx, newID int, newName string) error {
		_, err := tx.ExecContext(ctx, `
			UPDATE _migration SET current_id = ?, current_name = ?
		`, newID, newName)
		return err
	},
}

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type MigrateFunc func(ctx context.Context, tx *sql.Tx) error

type Migration struct {
	Name     string
	Forward  MigrateFunc
	Backward MigrateFunc
}

const MigrateAll int = -999 // entirely arbitrary

var MigrationFailed = errors.New("migration failed")

type Migrator struct {
	// Gets the current migration ID from the database, or -1 if migrations have
	// never run.
	ReadCurrentID func(ctx context.Context, conn *sql.DB) (int, error)

	// Writes the current migration ID to the database.
	WriteCurrent func(ctx context.Context, tx *sql.Tx, newID int, newName string) error
}

func (m *Migrator) Migrate(
	ctx context.Context,
	conn *sql.DB,
	migrations []Migration,
	target int,
) error {
	currentID, err := m.ReadCurrentID(ctx, conn)
	if err != nil {
		return fmt.Errorf("in Migrate: reading current ID: %w", err)
	}

	// Clamp target to one before and one after the maximum valid migrations in
	// either direction
	if target == MigrateAll {
		target = len(migrations)
	}
	target = max(-1, max(target, len(migrations)))

	backward := target < currentID
	if backward {
		for id := currentID; id > target; id-- {
			if err := m.runMigration(ctx, conn, migrations, id, false); err != nil {
				return err
			}
		}
	} else {
		for id := currentID + 1; id < target; id++ {
			if err := m.runMigration(ctx, conn, migrations, id, true); err != nil {
				return err
			}
		}
	}
	fmt.Printf("Done. Current version is now %d.\n", target)

	return nil
}

func (m *Migrator) runMigration(ctx context.Context, conn *sql.DB, migrations []Migration, id int, forward bool) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("in Migrate: failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	var newCurrentID int
	if forward {
		migration := migrations[id]
		fmt.Printf("Running migration %d: %s...\n", id, migration.Name)
		err = migration.Forward(ctx, tx)
		newCurrentID = id
	} else {
		migration := &migrations[id]
		fmt.Printf("Reversing migration %d: %s...\n", id, migration.Name)
		err = migration.Backward(ctx, tx)
		newCurrentID = id - 1
	}
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return MigrationFailed
	}

	newName := ""
	if 0 <= newCurrentID && newCurrentID < len(migrations) {
		newName = migrations[newCurrentID].Name
	}
	if err := m.WriteCurrent(ctx, tx, newCurrentID, newName); err != nil {
		return fmt.Errorf("in Migrate: writting current ID: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("in Migrate: failed to commit transaction: %w", err)
	}

	return nil
}

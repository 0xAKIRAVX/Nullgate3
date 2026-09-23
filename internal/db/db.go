package db

import (
        "context"
        "embed"
        "fmt"
        "io/fs"
        "sort"

        "github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect creates a pgx connection pool and verifies it with a ping.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
        cfg, err := pgxpool.ParseConfig(url)
        if err != nil {
                return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
        }
        cfg.MaxConns = 8
        pool, err := pgxpool.NewWithConfig(ctx, cfg)
        if err != nil {
                return nil, err
        }
        if err := pool.Ping(ctx); err != nil {
                return nil, fmt.Errorf("ping postgres: %w", err)
        }
        return pool, nil
}

// Migrate applies embedded SQL migrations (idempotent, ordered by parsed
// version number, each inside its own transaction).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
        _, err := pool.Exec(ctx,
                `CREATE TABLE IF NOT EXISTS schema_migrations(
                        version int PRIMARY KEY,
                        applied_at timestamptz NOT NULL DEFAULT now())`)
        if err != nil {
                return err
        }
        entries, err := fs.ReadDir(migrationsFS, "migrations")
        if err != nil {
                return err
        }
        type mig struct {
                version int
                name    string
        }
        migs := make([]mig, 0, len(entries))
        for _, e := range entries {
                var version int
                if _, err := fmt.Sscanf(e.Name(), "%d", &version); err != nil {
                        continue
                }
                migs = append(migs, mig{version, e.Name()})
        }
        // sort by parsed version, not lexically ("10_x" must not run before "2_x")
        sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })
        for _, m := range migs {
                var exists bool
                if err := pool.QueryRow(ctx,
                        `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, m.version,
                ).Scan(&exists); err != nil {
                        return err
                }
                if exists {
                        continue
                }
                body, err := migrationsFS.ReadFile("migrations/" + m.name)
                if err != nil {
                        return err
                }
                // each migration runs in its own transaction: a mid-script failure must
                // not leave partially applied DDL
                tx, err := pool.Begin(ctx)
                if err != nil {
                        return err
                }
                if _, err := tx.Exec(ctx, string(body)); err != nil {
                        _ = tx.Rollback(ctx)
                        return fmt.Errorf("migration %s: %w", m.name, err)
                }
                if _, err := tx.Exec(ctx,
                        `INSERT INTO schema_migrations(version) VALUES($1)`, m.version); err != nil {
                        _ = tx.Rollback(ctx)
                        return err
                }
                if err := tx.Commit(ctx); err != nil {
                        return err
                }
        }
        return nil
}

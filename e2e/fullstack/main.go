// fullstack helper: boots an embedded PostgreSQL and holds until SIGINT/SIGTERM.
// Used by scripts/fullstack_e2e.sh to give the real cmd/api process a database.
//
//      go run ./e2e/fullstack -pgport 55433 -dir /tmp/ngfs/pg
package main

import (
        "flag"
        "fmt"
        "os"
        "os/signal"
        "syscall"

        embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
        pgport := flag.Uint64("pgport", 55433, "postgres port")
        dir := flag.String("dir", "/tmp/ngfs/pg", "postgres runtime dir")
        flag.Parse()

        pg := embeddedpostgres.NewDatabase(
                embeddedpostgres.DefaultConfig().
                        Port(uint32(*pgport)).
                        RuntimePath(*dir).
                        Username("ng").Password("ng").Database("nullgate"))
        if err := pg.Start(); err != nil {
                fmt.Fprintf(os.Stderr, "embedded postgres: %v\n", err)
                os.Exit(1)
        }
        fmt.Printf("PG_READY port=%d dir=%s\n", *pgport, *dir)

        sig := make(chan os.Signal, 1)
        signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
        <-sig
        fmt.Fprintln(os.Stderr, "stopping postgres…")
        _ = pg.Stop()
}

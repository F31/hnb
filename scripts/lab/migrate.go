// Lab entry point for the shared owning-plane migration engine, not a second migrator.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/F31/hnb/pkg/db"
)

func main() {
	owner := flag.String("owner", "", "platform, market or artifact")
	dir := flag.String("dir", "", "forward migration directory")
	file := flag.String("dsn-file", "", "private credential file")
	flag.Parse()
	info, err := os.Stat(*file)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		fmt.Fprintln(os.Stderr, "private DSN file required")
		os.Exit(1)
	}
	value, err := os.ReadFile(*file)
	if err != nil {
		os.Exit(1)
	}
	conn, err := sql.Open("postgres", strings.TrimSpace(string(value)))
	if err != nil {
		fmt.Fprintln(os.Stderr, "database configuration unavailable")
		os.Exit(1)
	}
	defer conn.Close()
	files, err := db.LoadMigrations(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migration input unavailable")
		os.Exit(1)
	}
	result, err := (db.Migrator{DB: conn, Owner: db.MigrationOwner(*owner)}).Apply(context.Background(), files)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"owner": *owner, "applied": len(result.Applied), "skipped": len(result.Skipped), "schemaAfter": result.SchemaAfter})
}

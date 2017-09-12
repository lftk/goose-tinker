package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/4396/goose-tinker/lib/goose"
)

// global options. available to any subcommands.
var flagPath = flag.String("path", "db", "folder containing db info")
var flagEnv = flag.String("env", "development", "which DB environment to use")
var flagPgSchema = flag.String("pgschema", "", "which postgres-schema to migrate (default = none)")
var strictMode = flag.Bool("strict", false, "strict mode (default = false)")

// helper to create a DBConf from the given flags
func dbConfFromFlags() (dbconf *goose.DBConf, err error) {
	return goose.NewDBConf(*flagPath, *flagEnv, *flagPgSchema)
}

func main() {
	flag.Parse()
	conf, err := dbConfFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	// collect all migrations
	min := int64(0)
	max := int64((1 << 63) - 1)
	migrations, e := goose.CollectMigrations(conf.MigrationsDir, min, max)
	if e != nil {
		log.Fatal(e)
	}

	db, e := goose.OpenDBFromDBConf(conf)
	if e != nil {
		log.Fatal("couldn't open DB:", e)
	}
	defer db.Close()

	// must ensure that the version table exists if we're running on a pristine DB
	if _, e := goose.EnsureDBVersion(conf, db); e != nil {
		log.Fatal(e)
	}

	fmt.Printf("goose: status for environment '%v'\n", conf.Env)

	goose.SortMigrations(migrations, true)
	for _, m := range migrations {
		runPendingMigration(conf, db, m.Version, m.Source)
	}
}

func runPendingMigration(conf *goose.DBConf, db *sql.DB, version int64, source string) {
	var row goose.MigrationRecord
	q := fmt.Sprintf("SELECT tstamp, is_applied FROM goose_db_version WHERE version_id=%d ORDER BY tstamp DESC LIMIT 1", version)
	e := db.QueryRow(q).Scan(&row.TStamp, &row.IsApplied)

	if e != nil && e != sql.ErrNoRows {
		return
	}

	if row.IsApplied {
		return
	}

	var err error
	switch filepath.Ext(source) {
	case ".go":
		err = goose.RunGoMigration(conf, source, version, true)
	case ".sql":
		err = goose.RunSQLMigration(conf, db, source, version, true)
	}

	if err != nil {
		if *strictMode {
			log.Fatalf("FAIL %v, quitting migration", err)
		}
		fmt.Println("FAIL ", filepath.Base(source))
		return
	}

	fmt.Println("OK   ", filepath.Base(source))
	return
}

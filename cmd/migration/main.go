package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const (
	migrationsDir = "db/migrations"
	dbURL         = "postgres://root:password@localhost:10000/db_master_data?sslmode=disable"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: go run main.go [create|up|down|force] [args...]")
	}

	cmd := os.Args[1]

	switch cmd {
	case "create":
		if len(os.Args) < 3 {
			log.Fatal("usage: go run main.go create <name>")
		}
		name := os.Args[2]
		createMigration(name)

	case "up":
		m, err := migrate.New("file://"+migrationsDir, dbURL)
		if err != nil {
			log.Fatal(err)
		}
		err = m.Up()
		if err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatal(err)
		}
		fmt.Println("✅ migration up applied")

	case "down":
		m, err := migrate.New("file://"+migrationsDir, dbURL)
		if err != nil {
			log.Fatal(err)
		}
		err = m.Steps(-1)
		if err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatal(err)
		}
		fmt.Println("✅ rolled back 1 step")

	case "force":
		if len(os.Args) < 3 {
			log.Fatal("usage: go run main.go force <version>")
		}
		version := os.Args[2]
		m, err := migrate.New("file://"+migrationsDir, dbURL)
		if err != nil {
			log.Fatal(err)
		}
		ver, err := strconv.Atoi(version)
		if err != nil {
			log.Fatal(err)
		}
		err = m.Force(ver)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("✅ forced version", version)

	default:
		log.Fatalf("unknown command: %s", cmd)
	}
}

// createMigration generates up/down migration files with timestamp prefix
func createMigration(name string) {
	timestamp := time.Now().Unix()
	upFile := fmt.Sprintf("%s/%d_%s.up.sql", migrationsDir, timestamp, name)
	downFile := fmt.Sprintf("%s/%d_%s.down.sql", migrationsDir, timestamp, name)

	// bikin file kosong
	for _, f := range []string{upFile, downFile} {
		cmd := exec.Command("bash", "-c", fmt.Sprintf("touch %s", f))
		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("created", f)
	}
}

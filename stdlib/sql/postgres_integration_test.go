//go:build integration
// +build integration

package sql

import (
	"context"
	"database/sql"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/influxdata/flux/runtime"
	_ "github.com/lib/pq"
)

type DbContainer struct {
	id   string
	port int
}

func (container *DbContainer) DSN() string {
	return fmt.Sprintf("postgresql://postgres@localhost:%d/postgres?sslmode=disable", container.port)
}

func (container *DbContainer) Connect() (*sql.DB, error) {
	return sql.Open("postgres", container.DSN())
}

func spawnDb() DbContainer {
	containerPort := 5432
	dbRunCmd := exec.Command(
		"docker",
		"run",
		"--rm",
		"--publish-all",
		"--detach",
		"-e", "POSTGRES_HOST_AUTH_METHOD=trust",
		"postgres",
		"postgres", "-c", "log_statement=all",
	)
	dbRunOutput, err := dbRunCmd.Output()
	if err != nil {
		panic(err)
	}
	containerID := strings.TrimSpace(string(dbRunOutput))
	dbPortCmd := exec.Command("docker", "port", containerID, fmt.Sprint(containerPort))
	dbPortOutput, err := dbPortCmd.Output()
	if err != nil {
		panic(err)
	}
	r := regexp.MustCompile(`0.0.0.0:(\d+)`)
	port, err := strconv.ParseInt(
		r.FindStringSubmatch(string(dbPortOutput))[1],
		10,
		32,
	)
	if err != nil {
		panic(err)
	}
	dbContainer := DbContainer{containerID, int(port)}
	db, err := dbContainer.Connect()
	defer db.Close()
	if err != nil {
		panic(err)
	}

	// Since we will sleep 100ms scaled by the current attempt count, a max of
	// 10 will mean the deadline should be roughly 5ish seconds in the worst
	//case.
	maxAttempts := 10
	if err := waitForDb(db, maxAttempts); err != nil {
		cleanupContainer(dbContainer.id)
		panic(err)
	}

	return dbContainer
}

func waitForDb(db *sql.DB, maxAttempts int) error {
	attempts := 0
	for {
		if err := db.Ping(); err != nil {
			attempts += 1
			if attempts > maxAttempts {
				return err
			}
			time.Sleep(time.Duration(attempts*100) * time.Millisecond)
		} else {
			return nil
		}
	}
}

func cleanupContainer(containerID string) {
	removeCmd := exec.Command("docker", "rm", "-f", containerID)
	err := removeCmd.Run()
	if err != nil {
		panic(err)
	}
}

// POC to make sure the container is cleaned up post-test, that it's ready to
// accept connections when the test runs, and so on.
func TestPGConnect(t *testing.T) {
	dbContainer := spawnDb()
	defer cleanupContainer(dbContainer.id)
	db, _ := dbContainer.Connect()
	defer db.Close()
	var (
		got      int
		expected int = 3
	)
	err := db.QueryRow("select 1 + 2").Scan(&got)
	if err != nil {
		panic(err)
	}
	if got != expected {
		panic(fmt.Sprintf("expected %d, got %d", expected, got))
	}
}

func TestPG_FromTo(t *testing.T) {
	dbContainer := spawnDb()
	defer cleanupContainer(dbContainer.id)
	db, _ := dbContainer.Connect()
	defer db.Close()
	_, err := db.Exec(`
	CREATE TABLE pets (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100),
		age SMALLINT
	);
	INSERT INTO pets (name, age)
	VALUES
		('Stanley', 15),
		('Lucy', 14)
	;
	`)
	if err != nil {
		panic(err)
	}
	// FIXME: figure out how to use an Option to feed the DSN into the flux.
	src := fmt.Sprintf(`
		import "sql"
		dsn = "%s"
		sql.from(
			driverName: "postgres",
			dataSourceName: dsn,
			query: "SELECT id, name, age FROM pets")
			|> map(fn: (r) => ({ r with age: r.age * 2 }))
			|> sql.to(
					driverName: "postgres",
					dataSourceName: dsn,
					table: "pets",
					batchSize: 10)
	`,
		dbContainer.DSN(),
	)

	_, _, err = runtime.Eval(context.Background(), src)
	if err != nil {
		panic(err)
	}
}

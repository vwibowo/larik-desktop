package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	mysql "github.com/go-sql-driver/mysql"
)

// These checks use disposable databases with sample_items(id, label) containing
// (1, 'one') and (2, 'two'). They are skipped during a normal go test run.
func TestLiveMySQLAndMariaDBReadOnly(t *testing.T) {
	for _, target := range []struct{ name, env string }{
		{"mysql", "LARIK_TEST_MYSQL_DSN"},
		{"mariadb", "LARIK_TEST_MARIADB_DSN"},
	} {
		t.Run(target.name, func(t *testing.T) {
			dsn := os.Getenv(target.env)
			if dsn == "" {
				t.Skipf("set %s to run the live %s check", target.env, target.name)
			}
			parsed, err := mysql.ParseDSN(dsn)
			if err != nil || parsed.DBName == "" {
				t.Fatalf("%s needs a valid DSN naming a database: %v", target.env, err)
			}
			m, err := New(map[string]ConnectionConfig{"live": {Driver: "mysql", DSNEnv: target.env}}, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			checkLiveRemote(t, m, parsed.DBName, "mysql")
		})
	}
}

func TestLiveSQLServerReadOnly(t *testing.T) {
	host := os.Getenv("LARIK_TEST_SQLSERVER_HOST")
	if host == "" {
		t.Skip("set LARIK_TEST_SQLSERVER_HOST and the other SQL Server test settings")
	}
	port, err := strconv.Atoi(os.Getenv("LARIK_TEST_SQLSERVER_PORT"))
	if err != nil {
		t.Fatal("LARIK_TEST_SQLSERVER_PORT must be a TCP port")
	}
	databaseName := os.Getenv("LARIK_TEST_SQLSERVER_DATABASE")
	user := os.Getenv("LARIK_TEST_SQLSERVER_USER")
	if databaseName == "" || user == "" || os.Getenv("LARIK_TEST_SQLSERVER_PASSWORD") == "" {
		t.Fatal("SQL Server test database, user, and password are required")
	}
	trust := false
	if value := os.Getenv("LARIK_TEST_SQLSERVER_TRUST_CERT"); value != "" {
		trust, err = strconv.ParseBool(value)
		if err != nil {
			t.Fatal("LARIK_TEST_SQLSERVER_TRUST_CERT must be true or false")
		}
	}
	p := ConnectionConfig{Driver: "sqlserver", Host: host, Port: port, Database: databaseName, User: user,
		PasswordEnv: "LARIK_TEST_SQLSERVER_PASSWORD", TrustServerCertificate: trust}
	m, err := New(map[string]ConnectionConfig{"live": p}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	checkLiveRemote(t, m, "dbo", "sqlserver")
}

func checkLiveRemote(t *testing.T, m *Manager, schema, driver string) {
	t.Helper()
	ctx := context.Background()
	tables, err := m.Schema(ctx, "live", false)
	if err != nil {
		t.Fatal(err)
	}
	var sample *Table
	for i := range tables {
		if tables[i].Schema == schema && tables[i].Name == "sample_items" {
			sample = &tables[i]
			break
		}
	}
	if sample == nil {
		t.Fatalf("%s.sample_items absent from schema", schema)
	}
	if len(sample.Columns) != 2 || sample.Columns[0].Name != "id" || !sample.Columns[0].PrimaryKey || sample.Columns[1].Name != "label" {
		t.Fatalf("unexpected sample_items columns: %+v", sample.Columns)
	}
	for offset, want := range []string{"one", "two"} {
		result, err := m.Rows(ctx, "live", schema, "sample_items", offset, 1, false)
		if err != nil || len(result.Rows) != 1 || fmt.Sprint(result.Rows[0][1]) != want {
			t.Fatalf("page %d = %+v, %v", offset, result, err)
		}
		if offset == 0 && (!result.Truncated || result.NextOffset != 1) {
			t.Fatalf("first page did not expose the next offset: %+v", result)
		}
	}
	statement := "SELECT label FROM " + quoteFor(driver, schema) + "." + quoteFor(driver, "sample_items") + " WHERE id = 1"
	result, err := m.Query(ctx, "live", statement, 10, false)
	if err != nil || len(result.Rows) != 1 || fmt.Sprint(result.Rows[0][0]) != "one" {
		t.Fatalf("read-only query = %+v, %v", result, err)
	}
	if _, err := m.Query(ctx, "live", "UPDATE "+quoteFor(driver, schema)+"."+quoteFor(driver, "sample_items")+" SET label = 'changed'", 10, false); err == nil {
		t.Fatal("write query was accepted")
	}
	check, err := m.Query(ctx, "live", statement, 10, false)
	if err != nil || len(check.Rows) != 1 || fmt.Sprint(check.Rows[0][0]) != "one" {
		t.Fatalf("data changed after rejected write: %+v, %v", check, err)
	}
}

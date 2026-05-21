package migrator

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"hermes.io/migrator/internal/config"
	"hermes.io/migrator/internal/converter"
	"hermes.io/migrator/internal/database"
	"hermes.io/migrator/internal/mysql"
	"hermes.io/migrator/internal/postgresql"
)

// Migrator orchestrates a migration from source to destination.
type Migrator struct {
	cfg    *config.Config
	source database.Database
	dest   database.Database
}

// New creates a Migrator from the provided configuration.
func New(cfg *config.Config) *Migrator {
	return &Migrator{cfg: cfg}
}

// Run executes the full migration.
func (m *Migrator) Run(ctx context.Context) error {
	// Build source
	m.source = buildDB(
		sourceParams(m.cfg),
	)
	// Build dest
	m.dest = buildDB(
		destParams(m.cfg),
	)

	slog.Info("Connecting to source database",
		"type", m.cfg.SourceType,
		"host", m.cfg.SourceHost,
		"database", m.cfg.SourceDatabase)
	if err := m.source.Connect(ctx); err != nil {
		return fmt.Errorf("connect to source: %w", err)
	}
	defer m.source.Close() //nolint:errcheck

	slog.Info("Connecting to destination database",
		"type", m.cfg.DestType,
		"host", m.cfg.DestHost,
		"database", m.cfg.DestDatabase)
	if err := m.dest.Connect(ctx); err != nil {
		return fmt.Errorf("connect to destination: %w", err)
	}
	defer m.dest.Close() //nolint:errcheck

	// Determine tables to migrate
	tables, err := m.tablesToMigrate(ctx)
	if err != nil {
		return err
	}
	slog.Info("Tables to migrate", "count", len(tables), "tables", tables)

	for _, table := range tables {
		if err := m.migrateTable(ctx, table); err != nil {
			return fmt.Errorf("migrate table %s: %w", table, err)
		}
	}

	slog.Info("Migration completed successfully")
	return nil
}

func (m *Migrator) tablesToMigrate(ctx context.Context) ([]string, error) {
	if len(m.cfg.Tables) > 0 {
		return m.cfg.Tables, nil
	}
	tables, err := m.source.ListTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("list source tables: %w", err)
	}
	return tables, nil
}

func (m *Migrator) migrateTable(ctx context.Context, table string) error {
	slog.Info("Migrating table", "table", table)

	if m.cfg.MigrateSchema {
		if err := m.migrateSchema(ctx, table); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}

	if m.cfg.MigrateData {
		if err := m.migrateData(ctx, table); err != nil {
			return fmt.Errorf("data: %w", err)
		}
	}

	return nil
}

func (m *Migrator) migrateSchema(ctx context.Context, table string) error {
	slog.Info("Migrating schema", "table", table)

	ddl, err := m.source.GetCreateTable(ctx, table)
	if err != nil {
		return fmt.Errorf("get create table DDL for %s: %w", table, err)
	}

	// Translate DDL if source and dest are different DB types
	if m.source.DBType() != m.dest.DBType() {
		ddl = translateDDL(ddl, m.source.DBType(), m.dest.DBType())
	}

	if err := m.dest.CreateTable(ctx, ddl); err != nil {
		return fmt.Errorf("create table %s in destination: %w", table, err)
	}

	slog.Info("Schema migrated", "table", table)
	return nil
}

func (m *Migrator) migrateData(ctx context.Context, table string) error {
	slog.Info("Migrating data", "table", table)

	// Truncate destination table first
	if err := m.dest.TruncateTable(ctx, table); err != nil {
		slog.Warn("Failed to truncate destination table, continuing",
			"table", table, "error", err)
	}

	batchSize := int(m.cfg.BatchSize)
	if batchSize <= 0 {
		batchSize = 1000
	}

	totalRows := 0
	for offset := 0; ; offset += batchSize {
		rows, err := m.source.QueryRows(ctx, table, offset, batchSize)
		if err != nil {
			return fmt.Errorf("query rows from %s at offset %d: %w", table, offset, err)
		}
		if len(rows) == 0 {
			break
		}

		if err := m.dest.InsertRows(ctx, table, rows); err != nil {
			return fmt.Errorf("insert rows into %s: %w", table, err)
		}

		totalRows += len(rows)
		slog.Info("Progress", "table", table, "rows_inserted", totalRows)

		if len(rows) < batchSize {
			break
		}
	}

	slog.Info("Data migrated", "table", table, "total_rows", totalRows)
	return nil
}

// dbParams groups connection parameters for a single database.
type dbParams struct {
	dbType   string
	host     string
	port     int32
	user     string
	password string
	dbName   string
	sslMode  string
}

func sourceParams(cfg *config.Config) dbParams {
	return dbParams{
		dbType:   cfg.SourceType,
		host:     cfg.SourceHost,
		port:     cfg.SourcePort,
		user:     cfg.SourceUsername,
		password: cfg.SourcePassword,
		dbName:   cfg.SourceDatabase,
		sslMode:  cfg.SourceSSLMode,
	}
}

func destParams(cfg *config.Config) dbParams {
	return dbParams{
		dbType:   cfg.DestType,
		host:     cfg.DestHost,
		port:     cfg.DestPort,
		user:     cfg.DestUsername,
		password: cfg.DestPassword,
		dbName:   cfg.DestDatabase,
		sslMode:  cfg.DestSSLMode,
	}
}

func buildDB(p dbParams) database.Database {
	switch strings.ToLower(p.dbType) {
	case "mysql":
		return mysql.New(p.host, p.port, p.user, p.password, p.dbName, p.sslMode)
	case "postgresql", "postgres":
		return postgresql.New(p.host, p.port, p.user, p.password, p.dbName, p.sslMode)
	default:
		// Return a PostgreSQL instance as default; the Connect will fail with a clear error.
		return postgresql.New(p.host, p.port, p.user, p.password, p.dbName, p.sslMode)
	}
}

// translateDDL performs a best-effort type translation of a CREATE TABLE DDL statement.
// This is intentionally simple — it handles common cases but may not cover all edge cases.
func translateDDL(ddl, srcType, dstType string) string {
	switch {
	case srcType == "mysql" && (dstType == "postgresql" || dstType == "postgres"):
		return mysqlDDLToPostgres(ddl)
	case (srcType == "postgresql" || srcType == "postgres") && dstType == "mysql":
		return postgresDDLToMySQL(ddl)
	default:
		return ddl
	}
}

var mysqlTypePattern = regexp.MustCompile(`(?i)\b(tinyint\(1\)|tinyint|smallint|mediumint|bigint|int(?:eger)?|float|double precision|double|decimal|numeric|char|varchar|tinytext|mediumtext|longtext|text|binary|varbinary|tinyblob|mediumblob|longblob|blob|datetime|timestamp|date|time|year|json|enum|set)(\([^)]*\))?`)

func mysqlDDLToPostgres(ddl string) string {
	// Remove MySQL-specific clauses
	ddl = strings.ReplaceAll(ddl, "AUTO_INCREMENT", "")
	ddl = strings.ReplaceAll(ddl, "auto_increment", "")

	// Remove ENGINE=, CHARSET=, COLLATE= table options
	enginePattern := regexp.MustCompile(`(?i)\s*(ENGINE|DEFAULT CHARSET|CHARSET|COLLATE|ROW_FORMAT|AUTO_INCREMENT)\s*=\s*\S+`)
	ddl = enginePattern.ReplaceAllString(ddl, "")

	// Replace backtick quoting with double-quote quoting
	ddl = strings.ReplaceAll(ddl, "`", `"`)

	// Translate types
	ddl = mysqlTypePattern.ReplaceAllStringFunc(ddl, func(match string) string {
		return converter.MySQLToPostgreSQL(match)
	})

	// Fix CREATE TABLE to use IF NOT EXISTS
	ddl = regexp.MustCompile(`(?i)CREATE TABLE`).ReplaceAllString(ddl, "CREATE TABLE IF NOT EXISTS")

	return ddl
}

var pgTypePattern = regexp.MustCompile(`(?i)\b(boolean|bool|smallint|int2|integer|int4|int|bigint|int8|real|float4|double precision|float8|numeric|decimal|character varying|varchar|character|char|text|bytea|timestamp without time zone|timestamp with time zone|timestamp|time without time zone|time with time zone|time|date|jsonb|json|uuid)(\([^)]*\))?`)

func postgresDDLToMySQL(ddl string) string {
	// Replace double-quote quoting with backtick quoting
	ddl = strings.ReplaceAll(ddl, `"`, "`")

	// Translate types
	ddl = pgTypePattern.ReplaceAllStringFunc(ddl, func(match string) string {
		return converter.PostgreSQLToMySQL(match)
	})

	// Remove IF NOT EXISTS from CREATE TABLE
	ddl = regexp.MustCompile(`(?i)CREATE TABLE IF NOT EXISTS`).ReplaceAllString(ddl, "CREATE TABLE")

	return ddl
}

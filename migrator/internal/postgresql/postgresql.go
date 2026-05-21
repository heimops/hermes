package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"

	"hermes.io/migrator/internal/database"
)

// PostgreSQL implements database.Database for PostgreSQL.
type PostgreSQL struct {
	host     string
	port     int32
	user     string
	password string
	dbName   string
	sslMode  string
	db       *sql.DB
}

// New creates a new PostgreSQL database handle.
func New(host string, port int32, user, password, dbName, sslMode string) *PostgreSQL {
	return &PostgreSQL{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		dbName:   dbName,
		sslMode:  sslMode,
	}
}

func (p *PostgreSQL) Connect(ctx context.Context) error {
	sslMode := p.sslMode
	if sslMode == "" {
		sslMode = "disable"
	}
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.host, p.port, p.user, p.password, p.dbName, sslMode)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("postgresql: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("postgresql: ping: %w", err)
	}
	p.db = db
	return nil
}

func (p *PostgreSQL) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

func (p *PostgreSQL) DBType() string { return "postgresql" }

func (p *PostgreSQL) ListTables(ctx context.Context) ([]string, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`)
	if err != nil {
		return nil, fmt.Errorf("postgresql: list tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func (p *PostgreSQL) GetTableInfo(ctx context.Context, table string) (*database.TableInfo, error) {
	// Get primary key columns.
	pkRows, err := p.db.QueryContext(ctx,
		`SELECT kcu.column_name
		 FROM information_schema.table_constraints tc
		 JOIN information_schema.key_column_usage kcu
		   ON tc.constraint_name = kcu.constraint_name
		  AND tc.table_schema = kcu.table_schema
		 WHERE tc.constraint_type = 'PRIMARY KEY'
		   AND tc.table_schema = 'public'
		   AND tc.table_name = $1`, table)
	if err != nil {
		return nil, fmt.Errorf("postgresql: get pk info for %s: %w", table, err)
	}
	pkCols := map[string]bool{}
	for pkRows.Next() {
		var col string
		if err := pkRows.Scan(&col); err != nil {
			pkRows.Close()
			return nil, err
		}
		pkCols[col] = true
	}
	pkRows.Close()
	if err := pkRows.Err(); err != nil {
		return nil, err
	}

	// Get column info.
	rows, err := p.db.QueryContext(ctx,
		`SELECT column_name, data_type, is_nullable, column_default
		 FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1
		 ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, fmt.Errorf("postgresql: get columns for %s: %w", table, err)
	}
	defer rows.Close()

	info := &database.TableInfo{Name: table}
	for rows.Next() {
		var col database.ColumnInfo
		var nullable string
		var def sql.NullString
		if err := rows.Scan(&col.Name, &col.DataType, &nullable, &def); err != nil {
			return nil, err
		}
		col.IsNullable = nullable == "YES"
		if def.Valid {
			col.Default = &def.String
		}
		col.IsPrimary = pkCols[col.Name]
		info.Columns = append(info.Columns, col)
	}
	return info, rows.Err()
}

// GetCreateTable builds a CREATE TABLE statement from column metadata.
// PostgreSQL has no SHOW CREATE TABLE equivalent.
func (p *PostgreSQL) GetCreateTable(ctx context.Context, table string) (string, error) {
	info, err := p.GetTableInfo(ctx, table)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS \"%s\" (\n", table))

	var pkCols []string
	colDefs := make([]string, 0, len(info.Columns))
	for _, col := range info.Columns {
		def := fmt.Sprintf("  \"%s\" %s", col.Name, col.DataType)
		if !col.IsNullable {
			def += " NOT NULL"
		}
		if col.Default != nil {
			def += fmt.Sprintf(" DEFAULT %s", *col.Default)
		}
		colDefs = append(colDefs, def)
		if col.IsPrimary {
			pkCols = append(pkCols, fmt.Sprintf("\"%s\"", col.Name))
		}
	}

	sb.WriteString(strings.Join(colDefs, ",\n"))

	if len(pkCols) > 0 {
		sb.WriteString(fmt.Sprintf(",\n  PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
	}
	sb.WriteString("\n)")

	return sb.String(), nil
}

func (p *PostgreSQL) QueryRows(ctx context.Context, table string, offset, limit int) ([]map[string]interface{}, error) {
	query := fmt.Sprintf(`SELECT * FROM "%s" LIMIT %d OFFSET %d`, table, limit, offset)
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query rows from %s: %w", table, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			row[col] = values[i]
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (p *PostgreSQL) CreateTable(ctx context.Context, ddl string) error {
	_, err := p.db.ExecContext(ctx, ddl)
	if err != nil {
		return fmt.Errorf("postgresql: create table: %w", err)
	}
	return nil
}

func (p *PostgreSQL) InsertRows(ctx context.Context, table string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgresql: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Derive column order from the first row.
	cols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		cols = append(cols, k)
	}

	colList := make([]string, len(cols))
	for i, c := range cols {
		colList[i] = fmt.Sprintf(`"%s"`, c)
	}

	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	query := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s)`,
		table,
		strings.Join(colList, ", "),
		strings.Join(placeholders, ", "))

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("postgresql: prepare insert for %s: %w", table, err)
	}
	defer stmt.Close()

	for _, row := range rows {
		vals := make([]interface{}, len(cols))
		for i, c := range cols {
			vals[i] = row[c]
		}
		if _, err := stmt.ExecContext(ctx, vals...); err != nil {
			return fmt.Errorf("postgresql: insert into %s: %w", table, err)
		}
	}

	return tx.Commit()
}

func (p *PostgreSQL) TruncateTable(ctx context.Context, table string) error {
	_, err := p.db.ExecContext(ctx, fmt.Sprintf(`TRUNCATE TABLE "%s"`, table))
	if err != nil {
		return fmt.Errorf("postgresql: truncate %s: %w", table, err)
	}
	return nil
}

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"hermes.io/migrator/internal/database"
)

// MySQL implements database.Database for MySQL/MariaDB.
type MySQL struct {
	host     string
	port     int32
	user     string
	password string
	dbName   string
	sslMode  string
	db       *sql.DB
}

// New creates a new MySQL database handle.
func New(host string, port int32, user, password, dbName, sslMode string) *MySQL {
	return &MySQL{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		dbName:   dbName,
		sslMode:  sslMode,
	}
}

func (m *MySQL) Connect(ctx context.Context) error {
	tls := "false"
	if m.sslMode != "" && m.sslMode != "disable" {
		tls = m.sslMode
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true&tls=%s",
		m.user, m.password, m.host, m.port, m.dbName, tls)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("mysql: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("mysql: ping: %w", err)
	}
	m.db = db
	return nil
}

func (m *MySQL) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

func (m *MySQL) DBType() string { return "mysql" }

func (m *MySQL) ListTables(ctx context.Context) ([]string, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT TABLE_NAME FROM information_schema.TABLES
		 WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
		 ORDER BY TABLE_NAME`, m.dbName)
	if err != nil {
		return nil, fmt.Errorf("mysql: list tables: %w", err)
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

func (m *MySQL) GetTableInfo(ctx context.Context, table string) (*database.TableInfo, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, COLUMN_KEY, EXTRA
		 FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		 ORDER BY ORDINAL_POSITION`, m.dbName, table)
	if err != nil {
		return nil, fmt.Errorf("mysql: get table info: %w", err)
	}
	defer rows.Close()

	info := &database.TableInfo{Name: table}
	for rows.Next() {
		var col database.ColumnInfo
		var nullable, colKey, extra string
		var def sql.NullString
		if err := rows.Scan(&col.Name, &col.DataType, &nullable, &def, &colKey, &extra); err != nil {
			return nil, err
		}
		col.IsNullable = nullable == "YES"
		if def.Valid {
			col.Default = &def.String
		}
		col.IsPrimary = colKey == "PRI"
		col.Extra = extra
		info.Columns = append(info.Columns, col)
	}
	return info, rows.Err()
}

func (m *MySQL) GetCreateTable(ctx context.Context, table string) (string, error) {
	row := m.db.QueryRowContext(ctx, fmt.Sprintf("SHOW CREATE TABLE `%s`", table))
	var tableName, ddl string
	if err := row.Scan(&tableName, &ddl); err != nil {
		return "", fmt.Errorf("mysql: show create table %s: %w", table, err)
	}
	return ddl, nil
}

func (m *MySQL) QueryRows(ctx context.Context, table string, offset, limit int) ([]map[string]interface{}, error) {
	query := fmt.Sprintf("SELECT * FROM `%s` LIMIT %d OFFSET %d", table, limit, offset)
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("mysql: query rows from %s: %w", table, err)
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

func (m *MySQL) CreateTable(ctx context.Context, ddl string) error {
	_, err := m.db.ExecContext(ctx, ddl)
	if err != nil {
		return fmt.Errorf("mysql: create table: %w", err)
	}
	return nil
}

func (m *MySQL) InsertRows(ctx context.Context, table string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	// Derive column order from the first row.
	cols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		cols = append(cols, k)
	}

	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	colList := make([]string, len(cols))
	for i, c := range cols {
		colList[i] = fmt.Sprintf("`%s`", c)
	}

	query := fmt.Sprintf("INSERT INTO `%s` (%s) VALUES (%s)",
		table,
		strings.Join(colList, ", "),
		strings.Join(placeholders, ", "))

	for _, row := range rows {
		vals := make([]interface{}, len(cols))
		for i, c := range cols {
			vals[i] = row[c]
		}
		if _, err := m.db.ExecContext(ctx, query, vals...); err != nil {
			return fmt.Errorf("mysql: insert into %s: %w", table, err)
		}
	}
	return nil
}

func (m *MySQL) TruncateTable(ctx context.Context, table string) error {
	_, err := m.db.ExecContext(ctx, fmt.Sprintf("TRUNCATE TABLE `%s`", table))
	if err != nil {
		return fmt.Errorf("mysql: truncate %s: %w", table, err)
	}
	return nil
}

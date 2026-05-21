package database

import "context"

// ColumnInfo describes a single column in a table.
type ColumnInfo struct {
	Name       string
	DataType   string
	IsNullable bool
	Default    *string
	IsPrimary  bool
	Extra      string
}

// TableInfo describes a table and its columns.
type TableInfo struct {
	Name    string
	Columns []ColumnInfo
}

// Database is the interface that all database implementations must satisfy.
type Database interface {
	// Connect establishes a connection to the database.
	Connect(ctx context.Context) error

	// Close releases the database connection.
	Close() error

	// ListTables returns all base table names in the database.
	ListTables(ctx context.Context) ([]string, error)

	// GetTableInfo returns column metadata for the given table.
	GetTableInfo(ctx context.Context, table string) (*TableInfo, error)

	// GetCreateTable returns a DDL statement that can recreate the table.
	GetCreateTable(ctx context.Context, table string) (string, error)

	// QueryRows returns up to limit rows starting at the given offset.
	QueryRows(ctx context.Context, table string, offset, limit int) ([]map[string]interface{}, error)

	// CreateTable executes a DDL statement to create a table.
	CreateTable(ctx context.Context, ddl string) error

	// InsertRows inserts rows into the given table.
	InsertRows(ctx context.Context, table string, rows []map[string]interface{}) error

	// TruncateTable removes all rows from the given table.
	TruncateTable(ctx context.Context, table string) error

	// DBType returns a string identifier for the database type ("mysql" or "postgresql").
	DBType() string
}

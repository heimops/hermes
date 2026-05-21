package converter

import (
	"regexp"
	"strings"
)

var (
	rePrecision = regexp.MustCompile(`\((\d+(?:,\s*\d+)?)\)`)
)

// MySQLToPostgreSQL converts a MySQL column type string to its PostgreSQL equivalent.
func MySQLToPostgreSQL(mysqlType string) string {
	lower := strings.ToLower(strings.TrimSpace(mysqlType))

	// tinyint(1) → boolean
	if lower == "tinyint(1)" {
		return "boolean"
	}

	base := baseType(lower)

	switch base {
	case "tinyint":
		return "smallint"
	case "smallint":
		return "smallint"
	case "mediumint":
		return "integer"
	case "int", "integer":
		return "integer"
	case "bigint":
		return "bigint"
	case "float":
		return "real"
	case "double", "double precision":
		return "double precision"
	case "decimal", "numeric":
		if m := rePrecision.FindString(lower); m != "" {
			return "numeric" + m
		}
		return "numeric"
	case "char":
		if m := rePrecision.FindString(lower); m != "" {
			return "char" + m
		}
		return "char"
	case "varchar":
		if m := rePrecision.FindString(lower); m != "" {
			return "varchar" + m
		}
		return "varchar(255)"
	case "tinytext", "text", "mediumtext", "longtext":
		return "text"
	case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob":
		return "bytea"
	case "date":
		return "date"
	case "time":
		return "time"
	case "datetime", "timestamp":
		return "timestamp"
	case "year":
		return "integer"
	case "json":
		return "jsonb"
	case "enum", "set":
		return "varchar(255)"
	}

	// Fallback: return as-is
	return mysqlType
}

// PostgreSQLToMySQL converts a PostgreSQL column type string to its MySQL equivalent.
func PostgreSQLToMySQL(pgType string) string {
	lower := strings.ToLower(strings.TrimSpace(pgType))
	base := baseType(lower)

	switch base {
	case "boolean", "bool":
		return "tinyint(1)"
	case "smallint", "int2":
		return "smallint"
	case "integer", "int", "int4":
		return "int"
	case "bigint", "int8":
		return "bigint"
	case "real", "float4":
		return "float"
	case "double precision", "float8":
		return "double"
	case "numeric", "decimal":
		if m := rePrecision.FindString(lower); m != "" {
			return "decimal" + m
		}
		return "decimal"
	case "char", "character":
		if m := rePrecision.FindString(lower); m != "" {
			return "char" + m
		}
		return "char"
	case "varchar", "character varying":
		if m := rePrecision.FindString(lower); m != "" {
			return "varchar" + m
		}
		return "varchar(255)"
	case "text":
		return "longtext"
	case "bytea":
		return "longblob"
	case "date":
		return "date"
	case "time", "time without time zone", "time with time zone":
		return "time"
	case "timestamp", "timestamp without time zone", "timestamp with time zone":
		return "datetime"
	case "json", "jsonb":
		return "json"
	case "uuid":
		return "varchar(36)"
	}

	// Fallback
	return pgType
}

// baseType extracts the base type name without precision/scale information.
func baseType(t string) string {
	idx := strings.Index(t, "(")
	if idx >= 0 {
		return strings.TrimSpace(t[:idx])
	}
	return strings.TrimSpace(t)
}

package mysql

import (
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/errors"
	"strings"
)

var _ driver.SchemaOperationsGrammar = (*SchemaGrammar)(nil)

func (r *SchemaGrammar) SchemaOperationPolicy(operation driver.SchemaOperation) driver.SchemaExecutionPolicy {
	return driver.SchemaExecutionPolicy{DisableForeignKeys: operation.Kind == "dropAllTables"}
}

func (r *SchemaGrammar) ForeignKeyStateQuery() string { return "select @@foreign_key_checks" }
func (r *SchemaGrammar) CompileSchemaOperation(operation driver.SchemaOperation) ([]driver.SchemaStatement, error) {
	var sql string
	switch operation.Kind {
	case "foreignKeys":
		sql = "SET FOREIGN_KEY_CHECKS=0"
		if operation.Enabled {
			sql = "SET FOREIGN_KEY_CHECKS=1"
		}
	case "createDatabase":
		sql = "create database " + r.wrap.Value(operation.Name)
		if operation.Charset != "" {
			sql += " default character set " + r.wrap.Value(operation.Charset)
		}
		if operation.Collation != "" {
			sql += " collate " + r.wrap.Value(operation.Collation)
		}
	case "dropDatabase":
		sql = "drop database if exists " + r.wrap.Value(operation.Name)
	case "dropAllTables":
		var names []string
		for _, table := range operation.Tables {
			name := table.Name
			if table.Schema != "" {
				name = table.Schema + "." + name
			}
			names = append(names, r.quoteQualifiedSchemaName(name))
		}
		if len(names) > 0 {
			sql = "drop table " + strings.Join(names, ", ")
		}
	case "dropAllViews":
		var names []string
		for _, view := range operation.Views {
			name := view.Name
			if view.Schema != "" {
				name = view.Schema + "." + name
			}
			names = append(names, r.quoteQualifiedSchemaName(name))
		}
		if len(names) > 0 {
			sql = "drop view " + strings.Join(names, ", ")
		}
	case "dropAllTypes":
		return nil, nil
	default:
		return nil, errors.SchemaFeatureNotSupported.Args(r.name, operation.Kind)
	}
	if sql == "" {
		return nil, nil
	}
	return []driver.SchemaStatement{{SQL: sql}}, nil
}

func (r *SchemaGrammar) quoteQualifiedSchemaName(name string) string {
	parts := strings.Split(name, ".")
	for i, part := range parts {
		parts[i] = r.wrap.Value(part)
	}
	return strings.Join(parts, ".")
}

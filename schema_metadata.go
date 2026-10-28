package mysql

import (
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/errors"
	"strings"
)

var _ driver.SchemaMetadataGrammar = (*SchemaGrammar)(nil)

func (r *SchemaGrammar) CurrentSchemas(database string) []string { return []string{database} }
func (r *SchemaGrammar) CompileSchemaMetadata(query driver.SchemaMetadataQuery) (driver.SchemaStatement, error) {
	switch query.Kind {
	case "hasTable":
		return driver.SchemaStatement{SQL: "select exists (select 1 from information_schema.tables where table_schema = coalesce(nullif(?, ''), schema()) and table_name = ? and table_type in ('BASE TABLE', 'SYSTEM VERSIONED')) as `exists`", Bindings: []any{query.Schema, query.Table}}, nil
	case "schemas":
		return driver.SchemaStatement{SQL: "select schema_name as name, schema_name = schema() as `default` from information_schema.schemata where schema_name not in ('information_schema', 'mysql', 'ndbinfo', 'performance_schema', 'sys') order by schema_name"}, nil
	case "columns":
		return driver.SchemaStatement{SQL: "select column_name as `name`, data_type as `type_name`, column_type as `type`, collation_name as `collation`, is_nullable as `nullable`, column_default as `default`, column_comment as `comment`, generation_expression as `expression`, extra as `extra` from information_schema.columns where table_schema = coalesce(nullif(?, ''), schema()) and table_name = ? order by ordinal_position asc", Bindings: []any{query.Schema, query.Table}}, nil
	case "indexes":
		return driver.SchemaStatement{SQL: "select index_name as `name`, group_concat(column_name order by seq_in_index) as `columns`, index_type as `type`, not non_unique as `unique` from information_schema.statistics where table_schema = coalesce(nullif(?, ''), schema()) and table_name = ? group by index_name, index_type, non_unique", Bindings: []any{query.Schema, query.Table}}, nil
	case "foreignKeys":
		return driver.SchemaStatement{SQL: "select kc.constraint_name as `name`, group_concat(kc.column_name order by kc.ordinal_position) as `columns`, kc.referenced_table_schema as `foreign_schema`, kc.referenced_table_name as `foreign_table`, group_concat(kc.referenced_column_name order by kc.ordinal_position) as `foreign_columns`, rc.update_rule as `on_update`, rc.delete_rule as `on_delete` from information_schema.key_column_usage kc join information_schema.referential_constraints rc on kc.constraint_schema = rc.constraint_schema and kc.constraint_name = rc.constraint_name where kc.table_schema = coalesce(nullif(?, ''), schema()) and kc.table_name = ? and kc.referenced_table_name is not null group by kc.constraint_name, kc.referenced_table_schema, kc.referenced_table_name, rc.update_rule, rc.delete_rule", Bindings: []any{query.Schema, query.Table}}, nil
	case "tables", "views":
		var prefix string
		column, fallback := "table_schema", "table_schema not in ('information_schema', 'mysql', 'ndbinfo', 'performance_schema', 'sys')"
		order := " order by table_schema, table_name"
		switch query.Kind {
		case "tables":
			prefix = "select table_name as `name`, table_schema as `schema`, (data_length + index_length) as `size`, table_comment as `comment`, engine as `engine`, table_collation as `collation` from information_schema.tables where table_type in ('BASE TABLE', 'SYSTEM VERSIONED') and "
		case "views":
			prefix = "select table_name as `name`, table_schema as `schema`, view_definition as `definition` from information_schema.views where "
		}
		filter, bindings := schemaPredicate(column, query.Schemas, fallback)
		return driver.SchemaStatement{SQL: prefix + filter + order, Bindings: bindings}, nil
	case "types":
		return driver.SchemaStatement{}, nil
	}
	return driver.SchemaStatement{}, errors.SchemaMetadataNotSupported.Args(query.Kind, r.name)
}
func schemaPredicate(column string, schemas []string, fallback string) (string, []any) {
	if len(schemas) == 0 || (len(schemas) == 1 && schemas[0] == "") {
		return fallback, nil
	}
	values := make([]any, len(schemas))
	for i, schema := range schemas {
		values[i] = schema
	}
	if len(schemas) == 1 {
		return column + " = ?", values
	}
	return column + " in (" + strings.TrimSuffix(strings.Repeat("?, ", len(schemas)), ", ") + ")", values
}

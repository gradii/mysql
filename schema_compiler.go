package mysql

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/errors"
	"github.com/spf13/cast"
)

var _ driver.SchemaCompiler = (*SchemaGrammar)(nil)

func describeColumn(column driver.ColumnDefinition) driver.ColumnDescription {
	if readable, ok := column.(driver.ColumnDescriber); ok {
		return readable.DescribeColumn()
	}
	return driver.ColumnDescription{}
}
func describeBlueprint(blueprint driver.Blueprint) driver.BlueprintDescription {
	if readable, ok := blueprint.(driver.BlueprintDescriber); ok {
		return readable.DescribeBlueprint()
	}
	return driver.BlueprintDescription{Table: blueprint.GetTableName(), Columns: blueprint.GetAddedColumns(), Commands: blueprint.GetCommands()}
}
func schemaExpression(value any) (string, error) {
	switch value := value.(type) {
	case nil:
		return "", nil
	case string:
		return value, nil
	case schema.Expression:
		return string(value), nil
	case interface{ SQL() string }:
		return value.SQL(), nil
	case interface{ GetValue() string }:
		return value.GetValue(), nil
	default:
		return "", errors.DatabaseUnsupportedType.Args(fmt.Sprintf("%T", value), "SQL string or expression")
	}
}
func (r *SchemaGrammar) schemaColumnType(column driver.ColumnDefinition) (string, error) {
	options := describeColumn(column)
	switch column.GetType() {
	case "raw":
		return options.RawDefinition, nil
	case "binary":
		if column.GetLength() == 0 {
			return "blob", nil
		}
		name := "varbinary"
		if options.Fixed {
			name = "binary"
		}
		return fmt.Sprintf("%s(%d)", name, column.GetLength()), nil
	case "year":
		return "year", nil
	case "ipAddress":
		return "varchar(45)", nil
	case "macAddress":
		return "varchar(17)", nil
	case "set":
		values := make([]string, len(column.GetAllowed()))
		for i, value := range column.GetAllowed() {
			values[i] = schemaDefaultValue(value)
		}
		return "set(" + strings.Join(values, ", ") + ")", nil
	case "vector":
		if options.Dimensions != nil {
			return fmt.Sprintf("vector(%d)", *options.Dimensions), nil
		}
		return "vector", nil
	case "computed":
		return "", errors.SchemaComputedRequiresType
	case "geometry", "geography":
		subtype := strings.ToLower(options.Subtype)
		if !slices.Contains([]string{"point", "linestring", "polygon", "geometrycollection", "multipoint", "multilinestring", "multipolygon"}, subtype) {
			subtype = "geometry"
		}
		if options.Srid != nil && *options.Srid != 0 {
			version, err := r.serverVersion()
			if err != nil {
				return "", err
			}
			if r.isMariaDB(version) {
				return fmt.Sprintf("%s ref_system_id=%d", subtype, *options.Srid), nil
			}
			return fmt.Sprintf("%s srid %d", subtype, *options.Srid), nil
		}
		return subtype, nil
	default:
		return schema.ColumnType(r, column), nil
	}
}
func (r *SchemaGrammar) schemaColumn(blueprint driver.Blueprint, column driver.ColumnDefinition) (string, error) {
	kind, err := r.schemaColumnType(column)
	if err != nil {
		return "", err
	}
	options := describeColumn(column)
	sql := r.wrap.Column(column.GetName()) + " " + kind + r.ModifyUnsigned(blueprint, column)
	if options.Charset != "" {
		sql += " character set " + r.wrap.Column(options.Charset)
	}
	if options.Collation != "" {
		sql += " collate " + schemaDefaultValue(options.Collation)
	}
	for _, generated := range []struct {
		value  any
		stored bool
	}{{options.VirtualAs, false}, {options.StoredAs, true}} {
		if generated.value == nil {
			continue
		}
		expression, err := schemaExpression(generated.value)
		if err != nil {
			return "", err
		}
		sql += " as (" + expression + ")"
		if generated.stored {
			sql += " stored"
		}
	}
	if options.VirtualAs != nil || options.StoredAs != nil {
		if options.Nullable != nil && !*options.Nullable {
			sql += " not null"
		}
	} else {
		sql += r.ModifyNullable(blueprint, column)
	}
	sql += r.ModifyDefault(blueprint, column) + r.ModifyOnUpdate(blueprint, column)
	if options.Invisible {
		sql += " invisible"
	}
	sql += r.ModifyIncrement(blueprint, column) + r.ModifyComment(blueprint, column) + r.ModifyAfter(blueprint, column) + r.ModifyFirst(blueprint, column)
	return sql, nil
}
func (r *SchemaGrammar) CompileSchemaCommand(blueprint driver.Blueprint, command *driver.Command) ([]driver.SchemaStatement, bool, error) {
	table := r.wrap.Table(blueprint.GetTableName())
	var sql string
	switch command.Name {
	case schema.CommandCreate:
		options := describeBlueprint(blueprint)
		if options.Charset == "" {
			options.Charset = r.charset
		}
		if options.Collation == "" {
			options.Collation = r.collation
		}
		columns := make([]string, 0, len(options.Columns))
		for _, column := range options.Columns {
			definition, err := r.schemaColumn(blueprint, column)
			if err != nil {
				return nil, true, err
			}
			columns = append(columns, definition)
		}
		for _, primary := range blueprint.GetCommands() {
			if primary.Name != schema.CommandPrimary {
				continue
			}
			algorithm := ""
			if primary.Algorithm != "" {
				algorithm = "using " + primary.Algorithm + " "
			}
			columns = append(columns, "primary key "+algorithm+"("+r.wrap.Columnize(primary.Columns)+")")
			primary.ShouldBeSkipped = true
		}
		sql = "create "
		if options.Temporary {
			sql += "temporary "
		}
		sql += "table " + table + " (" + strings.Join(columns, ", ") + ")"
		if options.Charset != "" {
			sql += " default character set " + r.wrap.Column(options.Charset)
		}
		if options.Collation != "" {
			sql += " collate " + schemaDefaultValue(options.Collation)
		}
		if options.Engine != "" {
			sql += " engine = " + options.Engine
		}
	case schema.CommandAdd:
		definition, err := r.schemaColumn(blueprint, command.Column)
		if err != nil {
			return nil, true, err
		}
		action := "add "
		if command.Column.IsChange() {
			action = "modify "
		}
		sql = "alter table " + table + " " + action + definition
		if describeColumn(command.Column).Instant {
			sql += ", algorithm=instant"
		}
	case "spatialIndex":
		sql = "alter table " + table + " add spatial " + r.wrap.Column(command.Index) + "(" + r.wrap.Columnize(command.Columns) + ")"
	case "dropSpatialIndex":
		sql = "alter table " + table + " drop index " + r.wrap.Column(command.Index)
	case "rawIndex":
		sql = "alter table " + table + " add index " + r.wrap.Column(command.Index) + "(" + command.Value + ")"
	default:
		return nil, false, nil
	}
	return []driver.SchemaStatement{{SQL: sql}}, true, nil
}
func (r *SchemaGrammar) CompileSchemaStartingValues(blueprint driver.Blueprint) ([]driver.SchemaStatement, error) {
	var statements []driver.SchemaStatement
	for _, column := range blueprint.GetAddedColumns() {
		value := describeColumn(column).StartingValue
		if value != nil && column.GetAutoIncrement() && *value != 0 {
			statements = append(statements, driver.SchemaStatement{SQL: "alter table " + r.wrap.Table(blueprint.GetTableName()) + " auto_increment = " + strconv.FormatUint(*value, 10)})
		}
	}
	return statements, nil
}

func schemaQuotedLiteral(value string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), "'", "''") + "'"
}
func schemaDefaultValue(value any) string {
	switch value := value.(type) {
	case schema.Expression:
		return string(value)
	case interface{ SQL() string }:
		return value.SQL()
	case interface{ GetValue() string }:
		return value.GetValue()
	case bool:
		if value {
			return "'1'"
		}
		return "'0'"
	default:
		return schemaQuotedLiteral(cast.ToString(value))
	}
}

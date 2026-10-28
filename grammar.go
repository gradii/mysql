package mysql

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	sq "github.com/Masterminds/squirrel"
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/database/query/core"
	"github.com/gradii/framework/database/query/grammars"
	"github.com/gradii/framework/database/schema"
	mysqlerrors "github.com/gradii/mysql/errors"
)

var _ driver.Grammar = &SchemaGrammar{}

type SchemaGrammar struct {
	attributeCommands []string
	database          string
	modifiers         []func(driver.Blueprint, driver.ColumnDefinition) string
	name              string
	prefix            string
	serials           []string
	version           string
	charset           string
	collation         string
	versionResolver   func() (string, error)
	wrap              *schema.Wrap
}

func NewSchemaGrammar(database, prefix, version, name string) *SchemaGrammar {
	grammar := &SchemaGrammar{
		attributeCommands: []string{schema.CommandComment},
		database:          database,
		name:              name,
		prefix:            prefix,
		serials:           []string{"bigInteger", "integer", "mediumInteger", "smallInteger", "tinyInteger"},
		version:           version,
		wrap:              schema.NewWrap(prefix),
	}
	grammar.wrap.SetValueWrapper(func(s string) string {
		return "`" + strings.ReplaceAll(s, "`", "``") + "`"
	})
	grammar.modifiers = []func(driver.Blueprint, driver.ColumnDefinition) string{
		// The sort should not be changed, it effects the SQL output
		grammar.ModifyUnsigned,
		grammar.ModifyNullable,
		grammar.ModifyDefault,
		grammar.ModifyOnUpdate,
		grammar.ModifyIncrement,
		grammar.ModifyComment,
		grammar.ModifyAfter,
		grammar.ModifyFirst,
	}

	return grammar
}

func (r *SchemaGrammar) CompileAdd(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("alter table %s add %s", r.wrap.Table(blueprint.GetTableName()), r.getColumn(blueprint, command.Column))
}

func (r *SchemaGrammar) CompileChange(blueprint driver.Blueprint, command *driver.Command) []string {
	return []string{
		fmt.Sprintf("alter table %s modify %s", r.wrap.Table(blueprint.GetTableName()), r.getColumn(blueprint, command.Column)),
	}
}

func (r *SchemaGrammar) CompileColumns(_, table string) (string, error) {
	table = r.prefix + table

	return fmt.Sprintf(
		"select column_name as `name`, data_type as `type_name`, column_type as `type`, "+
			"collation_name as `collation`, is_nullable as `nullable`, "+
			"column_default as `default`, column_comment as `comment`, "+
			"generation_expression as `expression`, extra as `extra` "+
			"from information_schema.columns where table_schema = %s and table_name = %s "+
			"order by ordinal_position asc", schemaQuotedLiteral(r.database), schemaQuotedLiteral(table)), nil
}

func (r *SchemaGrammar) CompileComment(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *SchemaGrammar) CompileCreate(blueprint driver.Blueprint) string {
	columns := r.getColumns(blueprint)
	primaryCommand := getCommandByName(blueprint.GetCommands(), "primary")
	if primaryCommand != nil {
		var algorithm string
		if primaryCommand.Algorithm != "" {
			algorithm = "using " + primaryCommand.Algorithm
		}
		columns = append(columns, fmt.Sprintf("primary key %s(%s)", algorithm, r.wrap.Columnize(primaryCommand.Columns)))

		primaryCommand.ShouldBeSkipped = true
	}

	return fmt.Sprintf("create table %s (%s)", r.wrap.Table(blueprint.GetTableName()), strings.Join(columns, ", "))
}

func (r *SchemaGrammar) CompileDefault(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *SchemaGrammar) CompileDisableForeignKeyConstraints() string {
	return "SET FOREIGN_KEY_CHECKS=0;"
}

func (r *SchemaGrammar) CompileDrop(blueprint driver.Blueprint) string {
	return fmt.Sprintf("drop table %s", r.wrap.Table(blueprint.GetTableName()))
}

func (r *SchemaGrammar) CompileDropAllDomains(_ []string) string {
	return ""
}

func (r *SchemaGrammar) CompileDropAllTables(_ string, tables []driver.Table) []string {
	var dropTables []string
	for _, table := range tables {
		dropTables = append(dropTables, table.Name)
	}

	return []string{
		r.CompileDisableForeignKeyConstraints(),
		fmt.Sprintf("drop table %s", r.wrap.Columnize(dropTables)),
		r.CompileEnableForeignKeyConstraints(),
	}
}

func (r *SchemaGrammar) CompileDropAllTypes(_ string, _ []driver.Type) []string {
	return nil
}

func (r *SchemaGrammar) CompileDropAllViews(_ string, views []driver.View) []string {
	var dropViews []string
	for _, view := range views {
		dropViews = append(dropViews, view.Name)
	}

	return []string{
		fmt.Sprintf("drop view %s", r.wrap.Columnize(dropViews)),
	}
}

func (r *SchemaGrammar) CompileDropColumn(blueprint driver.Blueprint, command *driver.Command) []string {
	columns := r.wrap.PrefixArray("drop", r.wrap.Columns(command.Columns))

	return []string{
		fmt.Sprintf("alter table %s %s", r.wrap.Table(blueprint.GetTableName()), strings.Join(columns, ", ")),
	}
}

func (r *SchemaGrammar) CompileDropForeign(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("alter table %s drop foreign key %s", r.wrap.Table(blueprint.GetTableName()), r.wrap.Column(command.Index))
}

func (r *SchemaGrammar) CompileDropFullText(blueprint driver.Blueprint, command *driver.Command) string {
	return r.CompileDropIndex(blueprint, command)
}

func (r *SchemaGrammar) CompileDropIfExists(blueprint driver.Blueprint) string {
	return fmt.Sprintf("drop table if exists %s", r.wrap.Table(blueprint.GetTableName()))
}

func (r *SchemaGrammar) CompileDropIndex(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("alter table %s drop index %s", r.wrap.Table(blueprint.GetTableName()), r.wrap.Column(command.Index))
}

func (r *SchemaGrammar) CompileDropPrimary(blueprint driver.Blueprint, _ *driver.Command) string {
	return fmt.Sprintf("alter table %s drop primary key", r.wrap.Table(blueprint.GetTableName()))
}

func (r *SchemaGrammar) CompileDropUnique(blueprint driver.Blueprint, command *driver.Command) string {
	return r.CompileDropIndex(blueprint, command)
}

func (r *SchemaGrammar) CompileEnableForeignKeyConstraints() string {
	return "SET FOREIGN_KEY_CHECKS=1;"
}

func (r *SchemaGrammar) CompileForeign(blueprint driver.Blueprint, command *driver.Command) string {
	sql := fmt.Sprintf("alter table %s add constraint %s foreign key (%s) references %s (%s)",
		r.wrap.Table(blueprint.GetTableName()),
		r.wrap.Column(command.Index),
		r.wrap.Columnize(command.Columns),
		r.wrap.Table(command.On),
		r.wrap.Columnize(command.References))
	if command.OnDelete != "" {
		sql += " on delete " + command.OnDelete
	}
	if command.OnUpdate != "" {
		sql += " on update " + command.OnUpdate
	}

	return sql
}

func (r *SchemaGrammar) CompileForeignKeys(_, table string) string {
	return fmt.Sprintf(
		`SELECT 
			kc.constraint_name AS name, 
			GROUP_CONCAT(kc.column_name ORDER BY kc.ordinal_position) AS columns, 
			kc.referenced_table_schema AS foreign_schema, 
			kc.referenced_table_name AS foreign_table, 
			GROUP_CONCAT(kc.referenced_column_name ORDER BY kc.ordinal_position) AS foreign_columns, 
			rc.update_rule AS on_update, 
			rc.delete_rule AS on_delete 
		FROM information_schema.key_column_usage kc 
		JOIN information_schema.referential_constraints rc 
			ON kc.constraint_schema = rc.constraint_schema 
			AND kc.constraint_name = rc.constraint_name 
		WHERE kc.table_schema = %s 
			AND kc.table_name = %s 
			AND kc.referenced_table_name IS NOT NULL 
		GROUP BY 
			kc.constraint_name, 
			kc.referenced_table_schema, 
			kc.referenced_table_name, 
			rc.update_rule, 
			rc.delete_rule`,
		schemaQuotedLiteral(r.database),
		schemaQuotedLiteral(table),
	)
}

func (r *SchemaGrammar) CompileFullText(blueprint driver.Blueprint, command *driver.Command) string {
	return r.compileKey(blueprint, command, "fulltext")
}

func (r *SchemaGrammar) CompileIndex(blueprint driver.Blueprint, command *driver.Command) string {
	var algorithm string
	if command.Algorithm != "" {
		algorithm = " using " + command.Algorithm
	}

	return fmt.Sprintf("alter table %s add %s %s%s(%s)",
		r.wrap.Table(blueprint.GetTableName()),
		"index",
		r.wrap.Column(command.Index),
		algorithm,
		r.wrap.Columnize(command.Columns),
	)
}

func (r *SchemaGrammar) CompileIndexes(_, table string) (string, error) {
	table = r.prefix + table

	return fmt.Sprintf(
		"select index_name as `name`, group_concat(column_name order by seq_in_index) as `columns`, "+
			"index_type as `type`, not non_unique as `unique` "+
			"from information_schema.statistics where table_schema = %s and table_name = %s "+
			"group by index_name, index_type, non_unique",
		schemaQuotedLiteral(r.database),
		schemaQuotedLiteral(table),
	), nil
}

func (r *SchemaGrammar) CompileJsonColumnsUpdate(values map[string]any) (map[string]any, error) {
	var (
		compiled = make(map[string]any)
		json     = App.GetJson()
	)
	for key, value := range values {
		if strings.Contains(key, "->") {
			segments := strings.SplitN(key, "->", 2)
			column, path := segments[0], strings.Trim(r.wrap.JsonPath(segments[1]), "'")

			val := reflect.ValueOf(value)
			if val.Kind() == reflect.Pointer {
				val = val.Elem()
			}

			if kind := val.Kind(); kind == reflect.Slice || kind == reflect.Array || kind == reflect.Map || kind == reflect.Struct || kind == reflect.Bool {
				binding, err := json.Marshal(value)
				if err != nil {
					return nil, err
				}
				value = legacyRaw("cast(? as json)", string(binding))

				// MariaDB does not support casting to JSON directly
				if r.name != Name {
					value = legacyRaw("json_extract(?, '$')", string(binding))
				}
			}

			expr, ok := compiled[column]
			if !ok {
				expr = legacyRaw(r.wrap.Column(column))
			}

			compiled[column] = legacyRaw("json_set(?,?,?)", expr, path, value)

			continue
		}

		compiled[key] = value
	}

	return compiled, nil
}

func (r *SchemaGrammar) CompileJsonContains(column string, value any, isNot bool) (string, []any, error) {
	field, path := r.wrap.JsonFieldAndPath(column)
	binding, err := App.GetJson().Marshal(value)
	if err != nil {
		return column, nil, err
	}

	return r.wrap.Not(fmt.Sprintf("json_contains(%s, ?%s)", field, path), isNot), []any{string(binding)}, nil
}

func (r *SchemaGrammar) CompileJsonContainsKey(column string, isNot bool) string {
	field, path := r.wrap.JsonFieldAndPath(column)

	return r.wrap.Not(fmt.Sprintf("ifnull(json_contains_path(%s, 'one'%s), 0)", field, path), isNot)
}

func (r *SchemaGrammar) CompileJsonLength(column string) string {
	field, path := r.wrap.JsonFieldAndPath(column)

	return fmt.Sprintf("json_length(%s%s)", field, path)
}

func (r *SchemaGrammar) CompileJsonSelector(column string) string {
	field, path := r.wrap.JsonFieldAndPath(column)

	return fmt.Sprintf("json_unquote(json_extract(%s%s))", field, path)
}

func (r *SchemaGrammar) CompileJsonValues(args ...any) []any {
	for i, arg := range args {
		val := reflect.ValueOf(arg)
		if val.Kind() == reflect.Pointer {
			if val.IsNil() {
				continue
			}
			val = val.Elem()
		}
		switch val.Kind() {
		case reflect.Bool:
			args[i] = fmt.Sprint(val.Interface())

		case reflect.Slice, reflect.Array:
			if length := val.Len(); length > 0 {
				values := make([]any, length)
				for j := 0; j < length; j++ {
					values[j] = val.Index(j).Interface()
				}
				args[i] = r.CompileJsonValues(values...)
			}
		default:

		}

	}
	return args
}

func (r *SchemaGrammar) CompileLockForUpdate(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	if conditions.LockForUpdate != nil && *conditions.LockForUpdate {
		builder = builder.Suffix("FOR UPDATE")
	}

	return builder
}

func (r *SchemaGrammar) CompilePlaceholderFormat() driver.PlaceholderFormat {
	return nil
}

func (r *SchemaGrammar) CompilePrimary(blueprint driver.Blueprint, command *driver.Command) string {
	var algorithm string
	if command.Algorithm != "" {
		algorithm = "using " + command.Algorithm
	}

	return fmt.Sprintf("alter table %s add primary key %s(%s)", r.wrap.Table(blueprint.GetTableName()), algorithm, r.wrap.Columnize(command.Columns))
}

func (r *SchemaGrammar) CompilePrune(_ string) string { return "" }

func (r *SchemaGrammar) CompileInRandomOrder(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	if conditions.InRandomOrder != nil && *conditions.InRandomOrder {
		conditions.OrderBy = []string{"RAND()"}
	}

	return builder
}

func (r *SchemaGrammar) CompileRename(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("rename table %s to %s", r.wrap.Table(blueprint.GetTableName()), r.wrap.Table(command.To))
}

func (r *SchemaGrammar) CompileRenameColumn(blueprint driver.Blueprint, command *driver.Command, columns []driver.Column) (string, error) {
	version, err := r.serverVersion()
	if err != nil {
		return "", err
	}
	if version != "" {
		v, parseErr := schemaVersion(version)
		if parseErr != nil {
			return "", parseErr
		}
		isMariaDB := r.isMariaDB(version)
		if (isMariaDB && v.LessThan(semver.New(10, 5, 2, "", ""))) || (!isMariaDB && v.LessThan(semver.New(8, 0, 3, "", ""))) {
			return r.compileLegacyRenameColumn(blueprint, command, columns)
		}
	}

	return fmt.Sprintf("alter table %s rename column %s to %s",
		r.wrap.Table(blueprint.GetTableName()),
		r.wrap.Column(command.From),
		r.wrap.Column(command.To),
	), nil
}

func (r *SchemaGrammar) CompileRenameIndex(blueprint driver.Blueprint, command *driver.Command, _ []driver.Index) []string {
	return []string{
		fmt.Sprintf("alter table %s rename index %s to %s", r.wrap.Table(blueprint.GetTableName()), r.wrap.Column(command.From), r.wrap.Column(command.To)),
	}
}

func (r *SchemaGrammar) CompileSharedLock(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	if conditions.SharedLock != nil && *conditions.SharedLock {
		builder = builder.Suffix("FOR SHARE")
	}

	return builder
}

func (r *SchemaGrammar) CompileTables(database string) string {
	return fmt.Sprintf("select table_name as `name`, (data_length + index_length) as `size`, "+
		"table_comment as `comment`, engine as `engine`, table_collation as `collation` "+
		"from information_schema.tables where table_schema = %s and table_type in ('BASE TABLE', 'SYSTEM VERSIONED') "+
		"order by table_name", schemaQuotedLiteral(database))
}

func (r *SchemaGrammar) CompileTableComment(blueprint driver.Blueprint, command *driver.Command) string {
	return "alter table " + r.wrap.Table(blueprint.GetTableName()) + " comment = " + schemaQuotedLiteral(command.Value)
}

func (r *SchemaGrammar) CompileTypes() string {
	return ""
}

func (r *SchemaGrammar) CompileUnique(blueprint driver.Blueprint, command *driver.Command) string {
	return r.compileKey(blueprint, command, "unique")
}

func (r *SchemaGrammar) CompileVersion() string {
	return "SELECT VERSION() AS value;"
}

func (r *SchemaGrammar) CompileViews(database string) string {
	return fmt.Sprintf("select table_name as `name`, view_definition as `definition` "+
		"from information_schema.views where table_schema = %s "+
		"order by table_name", schemaQuotedLiteral(database))
}

func (r *SchemaGrammar) GetAttributeCommands() []string {
	return r.attributeCommands
}

func (r *SchemaGrammar) ModifyAfter(_ driver.Blueprint, column driver.ColumnDefinition) string {
	if column.GetAfter() != "" {
		return fmt.Sprintf(" after %s", r.wrap.Column(column.GetAfter()))
	}

	return ""
}

func (r *SchemaGrammar) ModifyComment(_ driver.Blueprint, column driver.ColumnDefinition) string {
	if comment := column.GetComment(); comment != "" {
		// Escape special characters to prevent SQL injection
		comment = strings.ReplaceAll(comment, "'", "''")
		comment = strings.ReplaceAll(comment, "\\", "\\\\")

		return fmt.Sprintf(" comment '%s'", comment)
	}

	return ""
}

func (r *SchemaGrammar) ModifyDefault(_ driver.Blueprint, column driver.ColumnDefinition) string {
	if column.GetDefault() != nil {
		return fmt.Sprintf(" default %s", schemaDefaultValue(column.GetDefault()))
	}

	return ""
}

func (r *SchemaGrammar) ModifyNullable(_ driver.Blueprint, column driver.ColumnDefinition) string {
	if column.GetNullable() {
		return " null"
	} else {
		return " not null"
	}
}

func (r *SchemaGrammar) ModifyFirst(_ driver.Blueprint, column driver.ColumnDefinition) string {
	if column.IsFirst() {
		return " first"
	}

	return ""
}

func (r *SchemaGrammar) ModifyIncrement(blueprint driver.Blueprint, column driver.ColumnDefinition) string {
	if slices.Contains(r.serials, column.GetType()) && column.GetAutoIncrement() {
		if blueprint.HasCommand("primary") {
			return " auto_increment"
		}
		return " auto_increment primary key"
	}

	return ""
}

func (r *SchemaGrammar) ModifyOnUpdate(_ driver.Blueprint, column driver.ColumnDefinition) string {
	onUpdate := column.GetOnUpdate()
	if onUpdate != nil {
		switch value := onUpdate.(type) {
		case schema.Expression:
			return " on update " + string(value)
		case string:
			if onUpdate.(string) != "" {
				return " on update " + value
			}
		}
	}

	return ""
}

func (r *SchemaGrammar) ModifyUnsigned(_ driver.Blueprint, column driver.ColumnDefinition) string {
	if column.GetUnsigned() {
		return " unsigned"
	}

	return ""
}

func (r *SchemaGrammar) TypeBigInteger(_ driver.ColumnDefinition) string {
	return "bigint"
}

func (r *SchemaGrammar) TypeBoolean(_ driver.ColumnDefinition) string {
	return "tinyint(1)"
}

func (r *SchemaGrammar) TypeChar(column driver.ColumnDefinition) string {
	return fmt.Sprintf("char(%d)", column.GetLength())
}

func (r *SchemaGrammar) TypeDate(_ driver.ColumnDefinition) string {
	return "date"
}

func (r *SchemaGrammar) TypeDateTime(column driver.ColumnDefinition) string {
	current := "CURRENT_TIMESTAMP"
	precision := column.GetPrecision()
	if precision > 0 {
		current = fmt.Sprintf("CURRENT_TIMESTAMP(%d)", precision)
	}
	if column.GetUseCurrent() {
		column.Default(schema.Expression(current))
	}
	if column.GetUseCurrentOnUpdate() {
		column.OnUpdate(schema.Expression(current))
	}

	if precision > 0 {
		return fmt.Sprintf("datetime(%d)", precision)
	} else {
		return "datetime"
	}
}

func (r *SchemaGrammar) TypeDateTimeTz(column driver.ColumnDefinition) string {
	return r.TypeDateTime(column)
}

func (r *SchemaGrammar) TypeDecimal(column driver.ColumnDefinition) string {
	return fmt.Sprintf("decimal(%d, %d)", column.GetTotal(), column.GetPlaces())
}

func (r *SchemaGrammar) TypeDouble(_ driver.ColumnDefinition) string {
	return "double"
}

func (r *SchemaGrammar) TypeEnum(column driver.ColumnDefinition) string {
	allowed := column.GetAllowed()
	values := make([]string, len(allowed))
	for i, value := range allowed {
		values[i] = schemaDefaultValue(value)
	}
	return "enum(" + strings.Join(values, ", ") + ")"
}

func (r *SchemaGrammar) TypeFloat(column driver.ColumnDefinition) string {
	precision := column.GetPrecision()
	if precision > 0 {
		return fmt.Sprintf("float(%d)", precision)
	}

	return "float"
}

func (r *SchemaGrammar) TypeInteger(_ driver.ColumnDefinition) string {
	return "int"
}

func (r *SchemaGrammar) TypeJson(_ driver.ColumnDefinition) string {
	return "json"
}

func (r *SchemaGrammar) TypeJsonb(_ driver.ColumnDefinition) string {
	return "json"
}

func (r *SchemaGrammar) TypeLongText(_ driver.ColumnDefinition) string {
	return "longtext"
}

func (r *SchemaGrammar) TypeMediumInteger(_ driver.ColumnDefinition) string {
	return "mediumint"
}

func (r *SchemaGrammar) TypeMediumText(_ driver.ColumnDefinition) string {
	return "mediumtext"
}

func (r *SchemaGrammar) TypeSmallInteger(_ driver.ColumnDefinition) string {
	return "smallint"
}

func (r *SchemaGrammar) TypeString(column driver.ColumnDefinition) string {
	length := column.GetLength()
	if length > 0 {
		return fmt.Sprintf("varchar(%d)", length)
	}

	return "varchar(255)"
}

func (r *SchemaGrammar) TypeText(_ driver.ColumnDefinition) string {
	return "text"
}

func (r *SchemaGrammar) TypeTime(column driver.ColumnDefinition) string {
	if column.GetPrecision() > 0 {
		return fmt.Sprintf("time(%d)", column.GetPrecision())
	} else {
		return "time"
	}
}

func (r *SchemaGrammar) TypeTimeTz(column driver.ColumnDefinition) string {
	return r.TypeTime(column)
}

func (r *SchemaGrammar) TypeTimestamp(column driver.ColumnDefinition) string {
	current := "CURRENT_TIMESTAMP"
	precision := column.GetPrecision()
	if precision > 0 {
		current = fmt.Sprintf("CURRENT_TIMESTAMP(%d)", precision)
	}
	if column.GetUseCurrent() {
		column.Default(schema.Expression(current))
	}
	if column.GetUseCurrentOnUpdate() {
		column.OnUpdate(schema.Expression(current))
	}

	if precision > 0 {
		return fmt.Sprintf("timestamp(%d)", precision)
	} else {
		return "timestamp"
	}
}

func (r *SchemaGrammar) TypeTimestampTz(column driver.ColumnDefinition) string {
	return r.TypeTimestamp(column)
}

func (r *SchemaGrammar) TypeTinyInteger(_ driver.ColumnDefinition) string {
	return "tinyint"
}

func (r *SchemaGrammar) TypeTinyText(_ driver.ColumnDefinition) string {
	return "tinytext"
}

func (r *SchemaGrammar) TypeUuid(_ driver.ColumnDefinition) string {
	return "char(36)"
}

func (r *SchemaGrammar) addModifiers(sql string, blueprint driver.Blueprint, column driver.ColumnDefinition) string {
	for _, modifier := range r.modifiers {
		sql += modifier(blueprint, column)
	}

	return sql
}

func (r *SchemaGrammar) compileKey(blueprint driver.Blueprint, command *driver.Command, ttype string) string {
	var algorithm string
	if command.Algorithm != "" {
		algorithm = " using " + command.Algorithm
	}

	return fmt.Sprintf("alter table %s add %s %s%s(%s)",
		r.wrap.Table(blueprint.GetTableName()),
		ttype,
		r.wrap.Column(command.Index),
		algorithm,
		r.wrap.Columnize(command.Columns))
}

func (r *SchemaGrammar) compileLegacyRenameColumn(blueprint driver.Blueprint, command *driver.Command, columns []driver.Column) (string, error) {
	var column *driver.Column
	for i := range columns {
		if columns[i].Name == command.From {
			column = &columns[i]
			break
		}
	}
	if column == nil {
		return "", mysqlerrors.SchemaColumnNotFound.Args(command.From)
	}
	sql := column.Type
	if column.Collation != "" {
		sql += " collate " + r.wrap.Column(column.Collation)
	}
	if column.Generation != nil && column.Generation.Expression != nil {
		sql += " as (" + *column.Generation.Expression + ")"
		if strings.EqualFold(column.Generation.Type, "stored") {
			sql += " stored"
		} else {
			sql += " virtual"
		}
	}
	if column.Nullable {
		sql += " null"
	} else {
		sql += " not null"
	}
	if column.Generation == nil {
		if column.DefaultKnown {
			if column.DefaultValue != nil {
				value := fmt.Sprint(column.DefaultValue)
				version, err := r.serverVersion()
				if err != nil {
					return "", err
				}
				kind := strings.ToLower(column.TypeName)
				expression := strings.Contains(strings.ToLower(column.Extra), "default_generated") || ((kind == "timestamp" || kind == "datetime") && strings.HasPrefix(strings.ToLower(value), "current_timestamp"))
				numeric := slices.Contains([]string{"tinyint", "smallint", "mediumint", "int", "bigint", "decimal", "numeric", "float", "double", "bit", "year"}, kind)
				if expression || numeric || (r.isMariaDB(version) && value != "") {
					sql += " default " + value
				} else {
					sql += " default " + schemaDefaultValue(column.DefaultValue)
				}
			}
		} else if column.Default != "" {
			sql += " default " + column.Default
		}
	}
	if column.Autoincrement {
		sql += " auto_increment"
	}
	if update := regexp.MustCompile(`(?i)on update\s+(\w+(?:\([^)]*\))?)`).FindStringSubmatch(column.Extra); len(update) > 1 {
		sql += " on update " + update[1]
	}
	if strings.Contains(strings.ToLower(column.Extra), "invisible") {
		sql += " invisible"
	}
	definition := schema.NewColumnDefinition(column.Name, column.Type)
	definition.Comment(column.Comment)
	sql += r.ModifyComment(blueprint, definition)
	return fmt.Sprintf("alter table %s change %s %s %s", r.wrap.Table(blueprint.GetTableName()), r.wrap.Column(command.From), r.wrap.Column(command.To), sql), nil
}

func (r *SchemaGrammar) getColumns(blueprint driver.Blueprint) []string {
	var columns []string
	for _, column := range blueprint.GetAddedColumns() {
		columns = append(columns, r.getColumn(blueprint, column))
	}

	return columns
}

func (r *SchemaGrammar) getColumn(blueprint driver.Blueprint, column driver.ColumnDefinition) string {
	sql := fmt.Sprintf("%s %s", r.wrap.Column(column.GetName()), schema.ColumnType(r, column))

	return r.addModifiers(sql, blueprint, column)
}

func getCommandByName(commands []*driver.Command, name string) *driver.Command {
	commands = getCommandsByName(commands, name)
	if len(commands) == 0 {
		return nil
	}

	return commands[0]
}

func getCommandsByName(commands []*driver.Command, name string) []*driver.Command {
	var filteredCommands []*driver.Command
	for _, command := range commands {
		if command.Name == name {
			filteredCommands = append(filteredCommands, command)
		}
	}

	return filteredCommands
}

// Grammar is retained for existing drivers using the legacy constructor.
type Grammar = SchemaGrammar

func NewGrammar(database, prefix, version, name string) *SchemaGrammar {
	return NewSchemaGrammar(database, prefix, version, name)
}
func (r *SchemaGrammar) SetVersionResolver(resolve func() (string, error)) {
	r.versionResolver = resolve
}
func (r *SchemaGrammar) serverVersion() (string, error) {
	if r.versionResolver != nil {
		return r.versionResolver()
	}
	return r.version, nil
}
func (r *SchemaGrammar) isMariaDB(version string) bool {
	return strings.Contains(strings.ToLower(r.name), "maria") || strings.Contains(strings.ToLower(version), "mariadb")
}
func schemaVersion(version string) (*semver.Version, error) {
	if strings.HasPrefix(version, "5.5.5-") && strings.Contains(strings.ToLower(version), "mariadb") {
		version = strings.TrimPrefix(version, "5.5.5-")
	}
	// Vendor suffixes describe a release build, not a semantic prerelease.
	if number := regexp.MustCompile(`^\d+\.\d+\.\d+`).FindString(version); number != "" {
		version = number
	}
	return semver.NewVersion(version)
}

func legacyRaw(sql string, bindings ...any) any {
	return core.Raw(grammars.NewBaseGrammar("").SubstituteBindingsIntoRawSQL(sql, bindings))
}

// SetTableDefaults supplies connection defaults; explicit blueprint options win.
func (r *SchemaGrammar) SetTableDefaults(charset, collation string) {
	r.charset = charset
	r.collation = collation
}

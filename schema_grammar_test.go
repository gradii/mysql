package mysql

import (
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/database/schema"
	frameworkerrors "github.com/gradii/framework/errors"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSchemaGrammarAdvancedCreate(t *testing.T) {
	blueprint := schema.NewBlueprint(nil, "app_", "catalog.products")
	blueprint.Create()
	blueprint.Temporary()
	blueprint.InnoDb()
	blueprint.Charset("utf8mb4")
	blueprint.Collation("utf8mb4_unicode_ci")
	blueprint.ID().StartingValue(42)
	blueprint.Binary("digest", 16).Fixed()
	blueprint.Year("released")
	blueprint.IpAddress("ip")
	blueprint.MacAddress("mac")
	blueprint.Set("flags", []any{"a", "b'b"})
	blueprint.Integer("total").StoredAs("price * quantity")
	blueprint.String("label").VirtualAs("concat('a', id)").Nullable(false).Invisible().Charset("utf8mb4").Collation("utf8mb4_bin")
	blueprint.Geometry("location", "point", 4326)
	blueprint.Vector("embedding", 3)
	blueprint.RawColumn("opaque", "varbinary(8)")
	statements, err := blueprint.ToSql(NewSchemaGrammar("catalog", "app_", "8.0.34", "mysql"))
	require.NoError(t, err)
	require.Equal(t, []string{
		"create temporary table `catalog`.`app_products` (`id` bigint unsigned not null auto_increment primary key, `digest` binary(16) not null, `released` year not null, `ip` varchar(45) not null, `mac` varchar(17) not null, `flags` set('a', 'b''b') not null, `total` int as (price * quantity) stored, `label` varchar(255) character set `utf8mb4` collate 'utf8mb4_bin' as (concat('a', id)) not null invisible, `location` point srid 4326 not null, `embedding` vector(3) not null, `opaque` varbinary(8) not null) default character set `utf8mb4` collate 'utf8mb4_unicode_ci' engine = InnoDB",
		"alter table `catalog`.`app_products` auto_increment = 42",
	}, statements)
}
func TestSchemaGrammarAdvancedAlter(t *testing.T) {
	blueprint := schema.NewBlueprint(nil, "p_", "items")
	blueprint.String("label", 80).StoredAs("upper(code)").Change().After("id").Instant()
	blueprint.SpatialIndex("location")
	blueprint.RawIndex("(lower(`name`))", "lower_name")
	blueprint.DropSpatialIndex("location")
	statements, err := blueprint.ToSql(NewSchemaGrammar("db", "p_", "8.0.34", "mysql"))
	require.NoError(t, err)
	require.Equal(t, []string{
		"alter table `p_items` modify `label` varchar(80) as (upper(code)) stored after `id`, algorithm=instant",
		"alter table `p_items` add spatial `p_items_location_spatialindex`(`location`)",
		"alter table `p_items` add index `lower_name`((lower(`name`)))",
		"alter table `p_items` drop index `p_items_location_spatialindex`",
	}, statements)
}
func TestSchemaGrammarVersionedRename(t *testing.T) {
	for _, test := range []struct {
		name, version string
		legacy        bool
	}{
		{"mysql", "5.7.44", true}, {"mysql", "8.0.2", true}, {"mysql", "8.0.3", false},
		{"mariadb", "10.5.1-MariaDB", true}, {"mariadb", "10.5.2-MariaDB", false},
		{"mysql", "5.5.5-10.4.32-MariaDB", true}, {"mysql", "5.5.5-10.11.6-MariaDB", false},
	} {
		t.Run(test.name+test.version, func(t *testing.T) {
			grammar := NewSchemaGrammar("db", "p_", test.version, test.name)
			sql, err := grammar.CompileRenameColumn(schema.NewBlueprint(nil, "p_", "users"), &driver.Command{From: "old", To: "new"}, []driver.Column{{Name: "old", Type: "varchar(40)", TypeName: "varchar", Nullable: true, DefaultKnown: true, DefaultValue: ""}})
			require.NoError(t, err)
			if test.legacy {
				require.Contains(t, sql, " change `old` `new` varchar(40)")
			} else {
				require.Equal(t, "alter table `p_users` rename column `old` to `new`", sql)
			}
		})
	}
}
func TestSchemaGrammarLegacyRenameMetadata(t *testing.T) {
	grammar := NewSchemaGrammar("db", "p_", "5.7.44", "mysql")
	sql, err := grammar.CompileRenameColumn(schema.NewBlueprint(nil, "p_", "users"), &driver.Command{From: "old", To: "new"}, []driver.Column{{Name: "old", Type: "varchar(40)", TypeName: "varchar", Collation: "utf8mb4_bin", Nullable: true, DefaultKnown: true, DefaultValue: "", Comment: "user's value"}})
	require.NoError(t, err)
	require.Equal(t, "alter table `p_users` change `old` `new` varchar(40) collate `utf8mb4_bin` null default '' comment 'user''s value'", sql)
	expression := "price * quantity"
	sql, err = grammar.CompileRenameColumn(schema.NewBlueprint(nil, "p_", "users"), &driver.Command{From: "old", To: "new"}, []driver.Column{{Name: "old", Type: "int", TypeName: "int", Generation: &driver.ColumnGeneration{Type: "stored", Expression: &expression}, Extra: "STORED GENERATED", Nullable: true}})
	require.NoError(t, err)
	require.Contains(t, sql, "int as (price * quantity) stored null")
	sql, err = grammar.CompileRenameColumn(schema.NewBlueprint(nil, "p_", "users"), &driver.Command{From: "old", To: "new"}, []driver.Column{{Name: "old", Type: "timestamp(3)", TypeName: "timestamp", DefaultKnown: true, DefaultValue: "CURRENT_TIMESTAMP(3)", Extra: "DEFAULT_GENERATED on update CURRENT_TIMESTAMP(3)"}})
	require.NoError(t, err)
	require.Contains(t, sql, "default CURRENT_TIMESTAMP(3) on update CURRENT_TIMESTAMP(3)")
}
func TestSchemaGrammarVersionErrorsAndMariaDBSpatial(t *testing.T) {
	grammar := NewSchemaGrammar("db", "", "", "mysql")
	grammar.SetVersionResolver(func() (string, error) { return "", frameworkerrors.DatabaseConnectionIsNil })
	_, err := grammar.CompileRenameColumn(schema.NewBlueprint(nil, "", "users"), &driver.Command{From: "old", To: "new"}, nil)
	require.ErrorIs(t, err, frameworkerrors.DatabaseConnectionIsNil)
	blueprint := schema.NewBlueprint(nil, "", "places")
	blueprint.Create()
	blueprint.Geography("location", "point")
	_, err = blueprint.ToSql(grammar)
	require.ErrorIs(t, err, frameworkerrors.DatabaseConnectionIsNil)
	grammar.SetVersionResolver(func() (string, error) { return "10.11.6-MariaDB", nil })
	sql, err := blueprint.ToSql(grammar)
	require.NoError(t, err)
	require.Contains(t, sql[0], "point ref_system_id=4326")
}
func TestSchemaGrammarMetadataBindingsAndOperations(t *testing.T) {
	grammar := NewSchemaGrammar("db", "p_", "8.0.34", "mysql")
	for _, kind := range []string{"columns", "indexes", "foreignKeys", "hasTable"} {
		statement, err := grammar.CompileSchemaMetadata(driver.SchemaMetadataQuery{Kind: kind, Schema: "data'base", Table: "p_users"})
		require.NoError(t, err)
		require.Equal(t, []any{"data'base", "p_users"}, statement.Bindings)
		require.NotContains(t, statement.SQL, "data'base")
	}
	for _, kind := range []string{"tables", "views"} {
		statement, err := grammar.CompileSchemaMetadata(driver.SchemaMetadataQuery{Kind: kind, Schemas: []string{"one", "two"}})
		require.NoError(t, err)
		require.Contains(t, statement.SQL, "table_schema in (?, ?)")
		require.Equal(t, []any{"one", "two"}, statement.Bindings)
	}
	statements, err := grammar.CompileSchemaOperation(driver.SchemaOperation{Kind: "dropAllTables", Tables: []driver.Table{{Name: "p_users", Schema: "other"}}})
	require.NoError(t, err)
	require.Equal(t, []driver.SchemaStatement{{SQL: "drop table `other`.`p_users`"}}, statements)
	statements, err = grammar.CompileSchemaOperation(driver.SchemaOperation{Kind: "createDatabase", Name: "new`db", Charset: "utf8mb4", Collation: "utf8mb4_bin"})
	require.NoError(t, err)
	require.Equal(t, "create database `new``db` default character set `utf8mb4` collate `utf8mb4_bin`", statements[0].SQL)
	require.Equal(t, "select @@foreign_key_checks", grammar.ForeignKeyStateQuery())
}

func TestSchemaGrammarDefaultsAndLiteralEscaping(t *testing.T) {
	grammar := NewSchemaGrammar("db", "", "8.0.34", "mysql")
	grammar.SetTableDefaults("utf8mb4", "utf8mb4_unicode_ci")
	blueprint := schema.NewBlueprint(nil, "", "labels")
	blueprint.Create()
	blueprint.Charset("latin1")
	blueprint.Collation("latin1_bin")
	blueprint.String("label").Default("one\\two'three")
	blueprint.Enum("state", []any{"", "a'b"})
	sql, err := blueprint.ToSql(grammar)
	require.NoError(t, err)
	require.Contains(t, sql[0], "default 'one\\\\two''three'")
	require.Contains(t, sql[0], "enum('', 'a''b')")
	require.Contains(t, sql[0], "default character set `latin1` collate 'latin1_bin'")
	blueprint = schema.NewBlueprint(nil, "", "defaults")
	blueprint.Create()
	blueprint.ID()
	sql, err = blueprint.ToSql(grammar)
	require.NoError(t, err)
	require.Contains(t, sql[0], "default character set `utf8mb4` collate 'utf8mb4_unicode_ci'")
}

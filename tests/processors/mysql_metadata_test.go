package processors

import (
	"testing"

	contractsquery "github.com/gradii/framework/contracts/database/query"
	"github.com/gradii/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMySQLMetadataPreservesCombinedColumnAttributes(t *testing.T) {
	for _, processor := range []contractsquery.Processor{mysql.NewQueryProcessor(), mysql.NewMariaDBQueryProcessor()} {
		columns := processor.ProcessColumns([]contractsquery.Row{
			{"name": "id", "nullable": []byte("YES"), "extra": []byte("auto_increment INVISIBLE"), "default": nil},
			{"name": "total", "extra": "STORED GENERATED INVISIBLE", "expression": "price * quantity", "default": ""},
			{"name": "constant", "extra": "VIRTUAL GENERATED", "expression": "0", "default": "NULL"},
		})
		require.Len(t, columns, 3)
		assert.Equal(t, true, columns[0]["auto_increment"])
		assert.Equal(t, true, columns[0]["nullable"])
		assert.Nil(t, columns[0]["default"])
		assert.Equal(t, "", columns[1]["default"])
		assert.Equal(t, "NULL", columns[2]["default"])
		assert.Equal(t, contractsquery.Row{"type": "stored", "expression": "price * quantity"}, columns[1]["generation"])
		assert.Equal(t, contractsquery.Row{"type": "virtual", "expression": "0"}, columns[2]["generation"])
	}
}

func TestMySQLMetadataNormalizesIndexesAndForeignKeys(t *testing.T) {
	processor := mysql.NewQueryProcessor()
	indexes := processor.ProcessIndexes([]contractsquery.Row{
		{"name": []byte("PRIMARY"), "columns": "id,tenant_id", "type": "BTREE", "unique": "1"},
		{"name": "USERS_NAME", "columns": []string{"name"}, "type": "BTREE", "unique": []byte("0")},
	})
	assert.Equal(t, []contractsquery.Row{
		{"name": "primary", "columns": []string{"id", "tenant_id"}, "type": "btree", "unique": true, "primary": true},
		{"name": "users_name", "columns": []string{"name"}, "type": "btree", "unique": false, "primary": false},
	}, indexes)
	keys := processor.ProcessForeignKeys([]contractsquery.Row{{
		"name": "orders_user", "columns": "user_id,tenant_id", "foreign_schema": "app", "foreign_table": "users",
		"foreign_columns": "id,tenant_id", "on_update": "NO ACTION", "on_delete": "CASCADE",
	}})
	assert.Equal(t, []contractsquery.Row{{
		"name": "orders_user", "columns": []string{"user_id", "tenant_id"}, "foreign_schema": "app", "foreign_table": "users",
		"foreign_columns": []string{"id", "tenant_id"}, "on_update": "no action", "on_delete": "cascade",
	}}, keys)
}

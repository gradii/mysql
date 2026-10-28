package mysql

import (
	"testing"

	querybuilder "github.com/gradii/framework/database/query"
	"github.com/gradii/framework/database/query/core"
	"github.com/stretchr/testify/require"
)

// These cases mirror the original GrammarSuite JSON cases through QueryGrammar,
// checking executable SQL and binding order rather than compatibility expressions.
func TestQueryGrammarCompileJsonContains(t *testing.T) {
	t.Run("invalid value type", func(t *testing.T) {
		builder := querybuilder.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").
			WhereJsonContains("data->details", func() {}, core.BooleanAnd, false)
		builder.ToSQL()
		require.Error(t, builder.Error())
	})
	for _, test := range []struct {
		name, column string
		value        any
		not          bool
		sql          string
		bindings     []any
	}{
		{"single path with single value", "data->details", "value1", false, "select * from `goravel_users` where json_contains(`data`, ?, '$.\"details\"')", []any{`"value1"`}},
		{"single path with multiple values", "data->details", []string{"value1", "value2"}, false, "select * from `goravel_users` where json_contains(`data`, ?, '$.\"details\"')", []any{`["value1","value2"]`}},
		{"nested path with single value", "data->details->subdetails[0]", "value1", false, "select * from `goravel_users` where json_contains(`data`, ?, '$.\"details\".\"subdetails\"[0]')", []any{`"value1"`}},
		{"nested path with multiple values", "data->details[0]->subdetails", []string{"value1", "value2"}, false, "select * from `goravel_users` where json_contains(`data`, ?, '$.\"details\"[0].\"subdetails\"')", []any{`["value1","value2"]`}},
		{"with is not condition", "data->details", "value1", true, "select * from `goravel_users` where not json_contains(`data`, ?, '$.\"details\"')", []any{`"value1"`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := querybuilder.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").
				WhereJsonContains(test.column, test.value, core.BooleanAnd, test.not)
			require.Equal(t, test.sql, builder.ToSQL())
			require.NoError(t, builder.Error())
			require.Equal(t, test.bindings, builder.GetBindings())
		})
	}
}

func TestQueryGrammarCompileJsonContainKey(t *testing.T) {
	for _, test := range []struct {
		name, column string
		not          bool
		sql          string
	}{
		{"single path", "data->details", false, "select * from `goravel_users` where ifnull(json_contains_path(`data`, 'one', '$.\"details\"'), 0)"},
		{"single path with is not", "data->details", true, "select * from `goravel_users` where not ifnull(json_contains_path(`data`, 'one', '$.\"details\"'), 0)"},
		{"nested path", "data->details->subdetails", false, "select * from `goravel_users` where ifnull(json_contains_path(`data`, 'one', '$.\"details\".\"subdetails\"'), 0)"},
		{"nested path with array index", "data->details[0]->subdetails", false, "select * from `goravel_users` where ifnull(json_contains_path(`data`, 'one', '$.\"details\"[0].\"subdetails\"'), 0)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := querybuilder.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").
				WhereJsonContainsKey(test.column, core.BooleanAnd, test.not)
			require.Equal(t, test.sql, builder.ToSQL())
			require.NoError(t, builder.Error())
			require.Empty(t, builder.GetBindings())
		})
	}
}

func TestQueryGrammarCompileJsonLength(t *testing.T) {
	for _, test := range []struct {
		name, column, sql string
	}{
		{"single path", "data->details", "select * from `goravel_users` where json_length(`data`, '$.\"details\"') >= ?"},
		{"nested path", "data->details->subdetails", "select * from `goravel_users` where json_length(`data`, '$.\"details\".\"subdetails\"') >= ?"},
		{"nested path with array index", "data->details[0]->subdetails", "select * from `goravel_users` where json_length(`data`, '$.\"details\"[0].\"subdetails\"') >= ?"},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := querybuilder.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").
				WhereJsonLength(test.column, ">=", 2)
			require.Equal(t, test.sql, builder.ToSQL())
			require.NoError(t, builder.Error())
			require.Equal(t, []any{2}, builder.GetBindings())
		})
	}
}

func TestQueryGrammarCompileJsonColumnsUpdate(t *testing.T) {
	t.Run("invalid values", func(t *testing.T) {
		builder := querybuilder.NewBuilder(NewQueryGrammar("goravel_")).From("users", "")
		builder.ToUpdateSQL(map[string]any{"data->invalid": map[string]any{"value": func() {}}})
		require.Error(t, builder.Error())
	})
	for _, test := range []struct {
		name     string
		values   map[string]any
		sql      string
		bindings []any
	}{
		{"update single json column", map[string]any{"data->details": "details value"}, "update `goravel_users` set `data` = json_set(`data`, '$.\"details\"', ?) where `id` = ?", []any{"details value", 7}},
		{"update single json column(with nested path)", map[string]any{"data->details->subdetails[0]": "subdetails value"}, "update `goravel_users` set `data` = json_set(`data`, '$.\"details\".\"subdetails\"[0]', ?) where `id` = ?", []any{"subdetails value", 7}},
		{"update multiple json columns", map[string]any{"data->details": "details value", "data->info": "info value"}, "update `goravel_users` set `data` = json_set(`data`, '$.\"details\"', ?), `data` = json_set(`data`, '$.\"info\"', ?) where `id` = ?", []any{"details value", "info value", 7}},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := querybuilder.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").Where("id", 7)
			statement, bindings := builder.ToUpdateSQL(test.values)
			require.NoError(t, builder.Error())
			require.Equal(t, test.sql, statement)
			require.Equal(t, test.bindings, bindings)
		})
	}
}

func TestQueryGrammarLocks(t *testing.T) {
	for _, test := range []struct {
		name   string
		lock   func(*querybuilder.Builder) *querybuilder.Builder
		suffix string
	}{
		{"lock for update", func(b *querybuilder.Builder) *querybuilder.Builder { return b.LockForUpdate() }, "for update"},
		{"shared lock", func(b *querybuilder.Builder) *querybuilder.Builder { return b.SharedLock() }, "lock in share mode"},
		{"custom lock", func(b *querybuilder.Builder) *querybuilder.Builder { return b.Lock("for update skip locked") }, "for update skip locked"},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := test.lock(querybuilder.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").Where("id", 7))
			require.Equal(t, "select * from `goravel_users` where `id` = ? "+test.suffix, builder.ToSQL())
			require.NoError(t, builder.Error())
			require.Equal(t, []any{7}, builder.GetBindings())
		})
	}
}

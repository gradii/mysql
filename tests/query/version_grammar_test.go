package query

import (
	"sync"
	"testing"

	. "github.com/gradii/framework/database/query"
	"github.com/gradii/framework/database/query/core"
	"github.com/gradii/framework/errors"
	"github.com/gradii/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryGrammarDefersServerDiscoveryUntilCompilation(t *testing.T) {
	calls := 0
	grammar := mysql.NewQueryGrammar("app_", func() (string, error) {
		calls++
		return "10.11.8-MariaDB", nil
	})
	builder := NewBuilder(grammar).From("users", "").Select("data->language")
	assert.Zero(t, calls)
	assert.Equal(t, "select json_value(`data`, '$.\"language\"') from `app_users`", builder.ToSQL())
	require.NoError(t, builder.Error())
	assert.Equal(t, 1, calls)
}

func TestMySQLAndMariaDBJSONCompilationPreservesBindings(t *testing.T) {
	for _, test := range []struct {
		name, version, cast, selector string
	}{
		{"mysql", "8.0.32", "cast(? as json)", "json_unquote(json_extract(`data`, '$.\"language\"'))"},
		{"detected maria", "5.5.5-10.11.8-MariaDB", "json_query(?, '$')", "json_value(`data`, '$.\"language\"')"},
	} {
		t.Run(test.name, func(t *testing.T) {
			grammar := mysql.NewQueryGrammar("", func() (string, error) { return test.version, nil })
			selectBuilder := NewBuilder(grammar).From("users", "").Select("data->language")
			assert.Equal(t, "select "+test.selector+" from `users`", selectBuilder.ToSQL())
			builder := NewBuilder(grammar).From("users", "").Where("id", 7)
			statement, bindings := builder.ToUpdateSQL(core.UpdateValues{
				{Column: "data->enabled", Value: true},
				{Column: "data->tags", Value: []any{"en", 1}},
				{Column: "name", Value: "alice"},
			})
			require.NoError(t, builder.Error())
			assert.Equal(t, "update `users` set `data` = json_set(`data`, '$.\"enabled\"', true), `data` = json_set(`data`, '$.\"tags\"', "+test.cast+"), `name` = ? where `id` = ?", statement)
			assert.Equal(t, []any{"[\"en\",1]", "alice", 7}, bindings)
		})
	}

	grammar := mysql.NewMariaDBQueryGrammar("")
	assert.Equal(t, "json_query(?, '$')", grammar.CompileJsonValueCast("?"))
	assert.Equal(t, "json_value(`data`, '$.\"language\"')", grammar.WrapJsonSelector("data->language"))
}

func TestQueryGrammarVersionErrorsReachEveryCompilationEntry(t *testing.T) {
	for name, compile := range map[string]func(*Builder){
		"select":        func(b *Builder) { b.ToSQL() },
		"insert":        func(b *Builder) { b.ToInsertSQL([]map[string]any{{"name": "alice"}}) },
		"insert id":     func(b *Builder) { b.ToInsertGetIdSQL([]map[string]any{{"name": "alice"}}, "") },
		"insert ignore": func(b *Builder) { b.ToInsertOrIgnoreSQL([]map[string]any{{"name": "alice"}}) },
		"insert using":  func(b *Builder) { b.InsertUsing([]any{"name"}, "select name from archive").ToInsertUsingSQL() },
		"update":        func(b *Builder) { b.ToUpdateSQL(map[string]any{"data->a": []any{1}}) },
		"upsert": func(b *Builder) {
			b.ToUpsertSQL([]map[string]any{{"name": "alice"}}, []string{"name"}, []core.UpsertUpdate{{Column: "name"}})
		},
		"delete":   func(b *Builder) { b.ToDeleteSQL() },
		"truncate": func(b *Builder) { b.ToTruncateSQL() },
	} {
		t.Run(name, func(t *testing.T) {
			grammar := mysql.NewQueryGrammar("", func() (string, error) { return "", errors.DatabaseConfigNotFound })
			builder := NewBuilder(grammar).From("users", "")
			compile(builder)
			assert.ErrorIs(t, builder.Error(), errors.DatabaseConfigNotFound)
		})
	}
}

func TestMySQLLegacyGroupLimitVersionBoundary(t *testing.T) {
	for _, test := range []struct {
		version string
		legacy  bool
	}{
		{"5.7.44", true}, {"8.0.10", true}, {"8.0.11", false}, {"8.4.0", false}, {"5.5.5-10.11.8-MariaDB", false},
	} {
		t.Run(test.version, func(t *testing.T) {
			grammar := mysql.NewQueryGrammar("", func() (string, error) { return test.version, nil })
			builder := NewBuilder(grammar).From("posts", "").Where("published", 1).OrderBy("id", "desc").GroupLimit(2, "user_id").Offset(1)
			statement := builder.ToSQL()
			require.NoError(t, builder.Error())
			if test.legacy {
				assert.Equal(t, "select `laravel_table`.*, @laravel_row := if(@laravel_group = `user_id`, @laravel_row + 1, 1) as `laravel_row`, @laravel_group := `user_id` from (select @laravel_row := 0, @laravel_group := 0) as `laravel_vars`, (select * from `posts` where `published` = ? order by `user_id` asc, `id` desc) as `laravel_table` having `laravel_row` <= 3 and `laravel_row` > 1 order by `laravel_row`", statement)
			} else {
				assert.Contains(t, statement, "row_number() over (partition by `user_id` order by `id` desc)")
				assert.NotContains(t, statement, "@laravel_")
			}
			assert.Equal(t, []any{1}, builder.GetBindings())
		})
	}
}

func TestServerVersionControlsLateralJoins(t *testing.T) {
	for _, test := range []struct {
		version string
		allowed bool
	}{
		{"8.0.32", true}, {"10.11.8-MariaDB", false},
	} {
		t.Run(test.version, func(t *testing.T) {
			grammar := mysql.NewQueryGrammar("", func() (string, error) { return test.version, nil })
			builder := NewBuilder(grammar).From("users", "").JoinLateral("select 1", "counts", "left")
			statement := builder.ToSQL()
			if test.allowed {
				require.NoError(t, builder.Error())
				assert.Contains(t, statement, "left join lateral (select 1) as `counts` on true")
			} else {
				assert.ErrorIs(t, builder.Error(), errors.QueryLateralJoinNotSupported)
			}
		})
	}
}

func TestConcurrentGrammarCompilationsKeepConnectionCapabilities(t *testing.T) {
	grammars := []Grammar{
		mysql.NewQueryGrammar("mysql_", func() (string, error) { return "8.0.32", nil }),
		mysql.NewQueryGrammar("maria_", func() (string, error) { return "10.11.8-MariaDB", nil }),
	}
	want := []string{
		"select json_unquote(json_extract(`data`, '$.\"language\"')) from `mysql_users`",
		"select json_value(`data`, '$.\"language\"') from `maria_users`",
	}
	var workers sync.WaitGroup
	for n := 0; n < 20; n++ {
		for i, grammar := range grammars {
			workers.Add(1)
			go func() {
				defer workers.Done()
				builder := NewBuilder(grammar).From("users", "").Select("data->language")
				assert.Equal(t, want[i], builder.ToSQL())
				assert.NoError(t, builder.Error())
			}()
		}
	}
	workers.Wait()
}

func TestQueryGrammarRawSQLRetainsConnectionEscaping(t *testing.T) {
	for _, grammar := range []Grammar{mysql.NewQueryGrammar(""), mysql.NewMariaDBQueryGrammar("")} {
		calls := 0
		grammar.SetEscaper(func(value any, binary bool) string {
			calls++
			assert.Equal(t, "foo", value)
			assert.False(t, binary)
			return "'foo'"
		})
		assert.Equal(t, `select * from "users" where 'Hello\'World?' IS NOT NULL AND "email" = 'foo'`, grammar.SubstituteBindingsIntoRawSQL(`select * from "users" where 'Hello\'World?' IS NOT NULL AND "email" = ?`, []any{"foo"}))
		assert.Equal(t, 1, calls)
	}
}

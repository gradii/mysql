package mysql

import (
	"context"
	"database/sql/driver"
	"io"
	"sync/atomic"
	"testing"
	"time"

	native "github.com/go-sql-driver/mysql"
	"github.com/gradii/framework/contracts/database"
	contractsdriver "github.com/gradii/framework/contracts/database/driver"
	frameworkdriver "github.com/gradii/framework/database/driver"
	querybuilder "github.com/gradii/framework/database/query"
	"github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/errors"
	"github.com/gradii/mysql/contracts"
	mysqlerrors "github.com/gradii/mysql/errors"
	"github.com/stretchr/testify/require"
)

type testConnector struct{}

func (*testConnector) Connect(context.Context) (driver.Conn, error) { return nil, driver.ErrBadConn }
func (*testConnector) Driver() driver.Driver                        { return &native.MySQLDriver{} }

func TestConnectorConfiguration(t *testing.T) {
	endpoint := database.Config{
		Host: "2001:db8::1", Port: 3307, Username: "user", Password: "p:@/?#&ss",
		Database: "db/with?reserved% characters", Charset: "utf8mb4,utf8", Timezone: "Asia/Shanghai",
	}
	config, err := connectorConfig(endpoint)
	require.NoError(t, err)
	require.Equal(t, "[2001:db8::1]:3307", config.Addr)
	require.Equal(t, endpoint.Database, config.DBName)
	require.Equal(t, endpoint.Password, config.Passwd)
	require.Equal(t, "Asia/Shanghai", config.Loc.String())
	require.True(t, config.ParseTime)
	require.True(t, config.MultiStatements)
	require.Contains(t, config.FormatDSN(), "charset=utf8mb4,utf8")
	roundTrip, err := native.ParseDSN(config.FormatDSN())
	require.NoError(t, err)
	require.Equal(t, endpoint.Database, roundTrip.DBName)
	require.Equal(t, endpoint.Password, roundTrip.Passwd)
	connector, err := NewConnector(endpoint)
	require.NoError(t, err)
	require.NotNil(t, connector)
}

func TestConnectorExplicitDSNPreservesNativeOptions(t *testing.T) {
	dsn := "dsn_user:p%ss@tcp(localhost:3308)/dsn_db?charset=latin1&loc=Europe%2FParis&parseTime=false&multiStatements=false&tls=skip-verify&allowNativePasswords=false&timeout=3s&time_zone=%27%2B02%3A00%27"
	config, err := connectorConfig(database.Config{
		Dsn: dsn, Host: "ignored", Port: -1, Database: "ignored", Charset: "utf8mb4", Timezone: "invalid/location",
	})
	require.NoError(t, err)
	require.Equal(t, "dsn_user", config.User)
	require.Equal(t, "dsn_db", config.DBName)
	require.Equal(t, "localhost:3308", config.Addr)
	require.Equal(t, "Europe/Paris", config.Loc.String())
	require.False(t, config.ParseTime)
	require.False(t, config.MultiStatements)
	require.False(t, config.AllowNativePasswords)
	require.Equal(t, "skip-verify", config.TLSConfig)
	require.Equal(t, 3*time.Second, config.Timeout)
	require.Equal(t, "'+02:00'", config.Params["time_zone"])
	require.Contains(t, config.FormatDSN(), "charset=latin1")
}

func TestConnectorDefaultsAndValidation(t *testing.T) {
	config, err := connectorConfig(database.Config{Host: "localhost"})
	require.NoError(t, err)
	require.Equal(t, "localhost:3306", config.Addr)
	require.Same(t, time.UTC, config.Loc)
	for _, endpoint := range []database.Config{
		{}, {Host: "localhost", Port: -1}, {Host: "localhost", Port: 65536},
		{Host: "localhost", Timezone: "not/a/timezone"}, {Dsn: "malformed-dsn"},
	} {
		connector, err := NewConnector(endpoint)
		require.Error(t, err)
		require.Nil(t, connector)
	}
	_, err = NewConnector(database.Config{})
	require.ErrorIs(t, err, errors.DatabaseConfigNotFound)
	_, err = NewConnector(database.Config{Host: "localhost", Port: -1})
	require.ErrorIs(t, err, mysqlerrors.InvalidPort)
	custom := &testConnector{}
	connector, err := NewConnector(database.Config{Connector: custom, Dsn: "malformed-dsn"})
	require.NoError(t, err)
	require.Same(t, custom, connector)
}

func TestPoolPreservesCharsetTimezoneWithoutValidating(t *testing.T) {
	mysql := NewMysql(configValues{values: map[string]any{
		"database.connections.mysql.charset":       "utf8mb4",
		"database.connections.mysql.loc":           "invalid/location",
		"database.connections.mysql.prefix":        "tenant_",
		"database.connections.mysql.dsn":           "malformed-dsn",
		"database.connections.mysql.singular":      true,
		"database.connections.mysql.no_lower_case": true,
		"database.connections.mysql.read":          []contracts.Config{{Dsn: "read:pass@tcp(localhost:3306)/app"}},
	}}, nil, nil, "mysql")
	pool := mysql.Pool()
	require.Len(t, pool.Writers, 1)
	require.Len(t, pool.Readers, 1)
	for _, endpoint := range []database.Config{pool.Writers[0], pool.Readers[0]} {
		require.Equal(t, "utf8mb4", endpoint.Charset)
		require.Equal(t, "invalid/location", endpoint.Timezone)
		require.Equal(t, "tenant_", endpoint.Prefix)
		require.Nil(t, endpoint.Connector)
		require.False(t, endpoint.Singular)
		require.False(t, endpoint.NoLowerCase)
		require.Nil(t, endpoint.NameReplacer)
	}
	require.Equal(t, "malformed-dsn", pool.Writers[0].Dsn)
	_, err := mysql.Dialect().NewConnector(pool.Writers[0])
	require.Error(t, err)
}

func TestDialectComponentsAndLazyVersion(t *testing.T) {
	calls := 0
	resolver := func() (string, error) { calls++; return "8.0.36", nil }
	mysql := NewMysql(configValues{values: map[string]any{}}, nil, nil, "mysql")
	mysql.SetServerVersion(resolver)
	require.NotNil(t, mysql.Grammar())
	require.NotNil(t, mysql.Processor())
	dialect := mysql.Dialect()
	require.Equal(t, Name, dialect.Name())
	require.NotNil(t, dialect.NewQueryGrammar(contractsdriver.GrammarOptions{Prefix: "test_", ServerVersion: resolver}))
	require.NotNil(t, dialect.NewProcessor())
	require.Zero(t, calls)
	require.Equal(t, "MariaDB", NewDialect("mariadb").Name())
	require.NotNil(t, NewDialect("mariadb").NewQueryGrammar(contractsdriver.GrammarOptions{}))
	docker, err := mysql.Docker()
	require.Nil(t, docker)
	require.ErrorIs(t, err, mysqlerrors.DockerNotSupported)
}

// TestVersion preserves the original version-discovery regression without a
// Docker lifecycle: the framework owns the pool and caches its writer query.
func TestVersion(t *testing.T) {
	t.Parallel()
	testManagedServerVersion(t, "UTC")
}

// TestIssue706 preserves the slash-containing location regression. The native
// DSN must round-trip Asia/Shanghai before the managed resolver is exercised.
func TestIssue706(t *testing.T) {
	t.Parallel()
	testManagedServerVersion(t, "Asia/Shanghai")
}

func testManagedServerVersion(t *testing.T, timezone string) {
	t.Helper()
	name := t.Name()
	configuration := managedVersionConfig{configValues{values: map[string]any{
		"database.connections." + name + ".host":     "localhost",
		"database.connections." + name + ".database": "goravel",
		"database.connections." + name + ".username": "goravel",
		"database.connections." + name + ".password": "Framework!123",
		"database.connections." + name + ".charset":  "utf8mb4",
		"database.connections." + name + ".loc":      timezone,
	}}}
	mysql := NewMysql(configuration, nil, nil, name)
	pool := mysql.Pool()
	require.Len(t, pool.Writers, 1)
	endpoint := pool.Writers[0]
	require.Equal(t, timezone, endpoint.Timezone)
	nativeConfig, err := connectorConfig(endpoint)
	require.NoError(t, err)
	dsn := nativeConfig.FormatDSN()
	if timezone == "Asia/Shanghai" {
		require.Contains(t, dsn, "loc=Asia%2FShanghai")
	}
	roundTrip, err := native.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, timezone, roundTrip.Loc.String())
	require.Contains(t, dsn, "charset=utf8mb4")
	connector, err := NewConnector(endpoint)
	require.NoError(t, err)
	require.NotNil(t, connector)

	// Inject only the server response. Pool opening, discovery SQL, caching,
	// grammar injection, and dialect selection all use the production runtime.
	server := &versionTestConnector{t: t, version: "8.0.36"}
	pool.Writers[0].Connector = server
	managed, err := frameworkdriver.Open(configuration, nil, pool, name, nil, mysql.Dialect())
	require.NoError(t, err)
	require.NotNil(t, managed)
	t.Cleanup(func() { require.NoError(t, frameworkdriver.PurgeConnection(name)) })
	require.Zero(t, server.queries.Load())
	var resolverCalls atomic.Int64
	resolver := func() (string, error) {
		resolverCalls.Add(1)
		return managed.ServerVersion(context.Background())
	}
	mysql.SetServerVersion(resolver)
	grammar := mysql.Grammar()
	queryGrammar := mysql.Dialect().NewQueryGrammar(contractsdriver.GrammarOptions{ServerVersion: resolver})
	require.Zero(t, server.queries.Load())

	version, err := managed.ServerVersion(context.Background())
	require.NoError(t, err)
	require.Equal(t, "8.0.36", version)
	require.Contains(t, version, ".")
	for range 2 {
		beforeQuery := resolverCalls.Load()
		builder := querybuilder.NewBuilder(queryGrammar).From("users", "").Select("data->language")
		require.Equal(t, "select json_unquote(json_extract(`data`, '$.\"language\"')) from `users`", builder.ToSQL())
		require.NoError(t, builder.Error())
		require.Greater(t, resolverCalls.Load(), beforeQuery)
		beforeSchema := resolverCalls.Load()
		statement, err := grammar.CompileRenameColumn(schema.NewBlueprint(nil, "", "users"), &contractsdriver.Command{From: "before", To: "after"}, nil)
		require.NoError(t, err)
		require.Greater(t, resolverCalls.Load(), beforeSchema)
		require.Equal(t, "alter table `users` rename column `before` to `after`", statement)
	}
	require.Equal(t, int64(1), server.queries.Load())
}

type managedVersionConfig struct{ configValues }

func (c managedVersionConfig) GetDuration(_ string, fallback ...time.Duration) time.Duration {
	if len(fallback) > 0 {
		return fallback[0]
	}
	return 0
}

type versionTestConnector struct {
	t       *testing.T
	version string
	queries atomic.Int64
}

func (c *versionTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &versionTestConnection{server: c}, nil
}
func (*versionTestConnector) Driver() driver.Driver { return &native.MySQLDriver{} }

type versionTestConnection struct{ server *versionTestConnector }

func (*versionTestConnection) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*versionTestConnection) Close() error                        { return nil }
func (*versionTestConnection) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (*versionTestConnection) Ping(context.Context) error          { return nil }
func (c *versionTestConnection) QueryContext(_ context.Context, statement string, args []driver.NamedValue) (driver.Rows, error) {
	require.Equal(c.server.t, "select version()", statement)
	require.Empty(c.server.t, args)
	c.server.queries.Add(1)
	return &versionTestRows{version: c.server.version}, nil
}

type versionTestRows struct {
	version string
	done    bool
}

func (*versionTestRows) Columns() []string { return []string{"version()"} }
func (*versionTestRows) Close() error      { return nil }
func (r *versionTestRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	values[0], r.done = r.version, true
	return nil
}

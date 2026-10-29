package mysql

import (
	"context"
	"database/sql"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	native "github.com/go-sql-driver/mysql"
	"github.com/gradii/framework/contracts/database"
	"github.com/stretchr/testify/require"
)

// This test only uses an existing test server. Temporary tables belong to the
// leased connection and are removed on close; it does not manage server lifecycle.
func TestConnectorIntegrationCharsetAndTimezone(t *testing.T) {
	dsn := os.Getenv("GRADII_TEST_MYSQL_DSN")
	if dsn == "" {
		dsn = os.Getenv("GORAVEL_TEST_MYSQL_DSN")
	}
	if dsn == "" {
		t.Skip("set GRADII_TEST_MYSQL_DSN to use an existing test database")
	}
	base, err := native.ParseDSN(dsn)
	require.NoError(t, err)
	host, portText, err := net.SplitHostPort(base.Addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	for _, timezone := range []string{"UTC", "Asia/Shanghai"} {
		t.Run(timezone, func(t *testing.T) {
			connector, err := NewConnector(database.Config{
				Host: host, Port: port, Username: base.User, Password: base.Passwd,
				Database: base.DBName, Charset: "utf8mb4", Timezone: timezone,
			})
			require.NoError(t, err)
			pool := sql.OpenDB(connector)
			t.Cleanup(func() { require.NoError(t, pool.Close()) })
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			connection, err := pool.Conn(ctx)
			require.NoError(t, err)
			defer func() { require.NoError(t, connection.Close()) }()
			var charset, version string
			require.NoError(t, connection.QueryRowContext(ctx, "select @@character_set_connection").Scan(&charset))
			require.Equal(t, "utf8mb4", charset)
			require.NoError(t, connection.QueryRowContext(ctx, NewDialect().ServerVersionSQL()).Scan(&version))
			require.NotEmpty(t, version)
			_, err = connection.ExecContext(ctx, "create temporary table gradii_connector_times (occurred_at datetime(6), label varchar(100)) character set utf8mb4")
			require.NoError(t, err)
			location, err := time.LoadLocation(timezone)
			require.NoError(t, err)
			want := time.Date(2024, 2, 3, 4, 5, 6, 123456000, location)
			_, err = connection.ExecContext(ctx, "insert into gradii_connector_times (occurred_at, label) values (?, ?)", want, "你好🌏")
			require.NoError(t, err)
			var got time.Time
			var label string
			require.NoError(t, connection.QueryRowContext(ctx, "select occurred_at, label from gradii_connector_times").Scan(&got, &label))
			require.Equal(t, "你好🌏", label)
			require.Equal(t, timezone, got.Location().String())
			require.True(t, want.Equal(got))
		})
	}

	t.Run("explicit DSN session timezone", func(t *testing.T) {
		config := base.Clone()
		config.ParseTime = true
		config.Loc = time.UTC
		if config.Params == nil {
			config.Params = make(map[string]string)
		}
		config.Params["time_zone"] = "'+02:00'"
		require.NoError(t, config.Apply(native.Charset("latin1", "")))
		connector, err := NewConnector(database.Config{Dsn: config.FormatDSN(), Charset: "utf8mb4", Timezone: "Asia/Shanghai"})
		require.NoError(t, err)
		pool := sql.OpenDB(connector)
		t.Cleanup(func() { require.NoError(t, pool.Close()) })
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var charset, timezone string
		var parsed time.Time
		require.NoError(t, pool.QueryRowContext(ctx, "select @@character_set_connection, @@time_zone, cast('2024-02-03 04:05:06' as datetime)").Scan(&charset, &timezone, &parsed))
		require.Equal(t, "latin1", charset)
		require.Equal(t, "+02:00", timezone)
		require.Equal(t, "UTC", parsed.Location().String())
	})
}

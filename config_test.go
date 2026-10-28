package mysql

import (
	"strings"
	"testing"

	contractsconfig "github.com/gradii/framework/contracts/config"
	querybuilder "github.com/gradii/framework/database/query"
	"github.com/gradii/mysql/contracts"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type configValues struct {
	contractsconfig.Config
	values map[string]any
}

func (c configValues) Get(path string, fallback ...any) any {
	if value, ok := c.values[path]; ok {
		return value
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return nil
}

func (c configValues) GetString(path string, fallback ...string) string {
	if value, ok := c.values[path].(string); ok {
		return value
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}

func (c configValues) GetInt(path string, fallback ...int) int {
	if value, ok := c.values[path].(int); ok {
		return value
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return 0
}

func TestConfigEndpointInheritance(t *testing.T) {
	configuration := configValues{values: map[string]any{
		"database.connections.primary.host":     "writer.internal",
		"database.connections.primary.port":     3307,
		"database.connections.primary.database": "application",
		"database.connections.primary.username": "user",
		"database.connections.primary.password": "password",
		"database.connections.primary.charset":  "utf8mb4",
		"database.connections.primary.loc":      "Asia/Shanghai",
		"database.connections.primary.prefix":   "app_",
		"database.connections.primary.driver":   "mariadb",
		"database.connections.primary.read": []contracts.Config{
			{Host: "replica.internal"},
			{Dsn: "reader:pass@tcp(second:3306)/read_db", Charset: "latin1", Loc: "UTC"},
		},
	}}
	config := NewConfig(configuration, "primary")
	writers, readers := config.Writers(), config.Readers()
	require.Len(t, writers, 1)
	require.Len(t, readers, 2)
	require.Equal(t, "primary", config.Connection())
	require.Equal(t, configuration, config.Config())
	require.Equal(t, contracts.FullConfig{
		Config:     contracts.Config{Host: "writer.internal", Port: 3307, Database: "application", Username: "user", Password: "password", Charset: "utf8mb4", Loc: "Asia/Shanghai"},
		Connection: "primary", Driver: "MariaDB", Prefix: "app_",
	}, writers[0])
	expected := writers[0]
	expected.Host = "replica.internal"
	require.Equal(t, expected, readers[0])
	require.Equal(t, "reader:pass@tcp(second:3306)/read_db", readers[1].Dsn)
	require.Equal(t, "latin1", readers[1].Charset)
	require.Equal(t, "UTC", readers[1].Loc)
}

func TestConfigDSNAndConnectorInheritance(t *testing.T) {
	connector := &testConnector{}
	config := NewConfig(configValues{values: map[string]any{
		"database.connections.mysql.dsn":       "default:pass@tcp(default:3306)/default",
		"database.connections.mysql.connector": connector,
		"database.connections.mysql.write":     []contracts.Config{{}, {Dsn: "override:pass@tcp(override:3307)/override"}},
	}}, "mysql")
	writers := config.Writers()
	require.Len(t, writers, 2)
	require.Equal(t, "default:pass@tcp(default:3306)/default", writers[0].Dsn)
	require.Equal(t, "override:pass@tcp(override:3307)/override", writers[1].Dsn)
	require.Same(t, connector, writers[0].Connector)
	require.Same(t, connector, writers[1].Connector)
	require.Nil(t, config.Readers())
}

func TestConfigTimezoneFallback(t *testing.T) {
	for _, test := range []struct {
		name, connection, app, want string
	}{
		{"connection wins", "Asia/Shanghai", "Europe/Paris", "Asia/Shanghai"},
		{"application fallback", "", "Europe/Paris", "Europe/Paris"},
		{"UTC fallback", "", "", "UTC"},
	} {
		t.Run(test.name, func(t *testing.T) {
			configuration := configValues{values: map[string]any{
				"database.connections.mysql.loc": test.connection, "app.timezone": test.app,
			}}
			require.Equal(t, test.want, NewConfig(configuration, "mysql").Writers()[0].Loc)
		})
	}
}

func TestConfigExplicitEmptyPools(t *testing.T) {
	config := NewConfig(configValues{values: map[string]any{
		"database.connections.mysql.write": []contracts.Config{},
		"database.connections.mysql.read":  []contracts.Config{},
	}}, "mysql")
	require.Nil(t, config.Writers())
	require.Nil(t, config.Readers())
}

// ConfigTestSuite ports the original endpoint/default cases onto the native
// configuration contract. Table names are explicit; Go naming strategies are ignored.
type ConfigTestSuite struct {
	suite.Suite
	config *Config
	values map[string]any
}

func TestConfigTestSuite(t *testing.T) { suite.Run(t, new(ConfigTestSuite)) }

func (s *ConfigTestSuite) SetupTest() {
	s.values = make(map[string]any)
	for key, value := range map[string]any{
		"dsn": "dsn", "host": "localhost", "port": 3306,
		"database": "forge", "username": "root", "password": "123123",
		"prefix": "goravel_", "charset": "utf8mb4", "loc": "UTC",
	} {
		s.values["database.connections.mysql."+key] = value
	}
	s.config = NewConfig(configValues{values: s.values}, "mysql")
}

func (s *ConfigTestSuite) expectedConfig() contracts.FullConfig {
	endpoint := contracts.Config{Dsn: "dsn", Host: "localhost", Port: 3306, Database: "forge", Username: "root", Password: "123123"}
	endpoint.Charset = "utf8mb4"
	endpoint.Loc = "UTC"
	return contracts.FullConfig{Config: endpoint, Connection: "mysql", Driver: Name, Prefix: "goravel_"}
}

func (s *ConfigTestSuite) TestReads() {
	s.Run("success when configs is empty", func() {
		s.Nil(s.config.Readers())
	})
	s.Run("success when configs is not empty", func() {
		s.values["database.connections.mysql.read"] = []contracts.Config{contracts.Config{Dsn: "dsn", Host: "localhost", Port: 3306, Database: "forge", Username: "root", Password: "123123"}}
		s.Equal([]contracts.FullConfig{s.expectedConfig()}, s.config.Readers())
	})
}

func (s *ConfigTestSuite) TestWrites() {
	for _, test := range []struct {
		name      string
		endpoints any
	}{
		{"success when configs is empty", nil},
		{"success when configs is not empty", []contracts.Config{{Database: "forge"}}},
	} {
		s.Run(test.name, func() {
			s.values["database.connections.mysql.write"] = test.endpoints
			s.Equal([]contracts.FullConfig{s.expectedConfig()}, s.config.Writers())
		})
	}
}

func (s *ConfigTestSuite) TestFillDefault() {
	for _, test := range []struct {
		name        string
		endpoints   []contracts.Config
		appTimezone bool
	}{
		{name: "success when configs is empty", endpoints: []contracts.Config{}},
		{name: "success when configs have item but key is empty", endpoints: []contracts.Config{{}}},
		{name: "success when configs have item", endpoints: []contracts.Config{contracts.Config{Dsn: "dsn", Host: "localhost", Port: 3306, Database: "forge", Username: "root", Password: "123123"}}},
		{name: "success with app.timezone", endpoints: []contracts.Config{contracts.Config{Dsn: "dsn", Host: "localhost", Port: 3306, Database: "forge", Username: "root", Password: "123123"}}, appTimezone: true},
	} {
		s.Run(test.name, func() {
			// These were present in the original default cases. The native
			// configuration keeps endpoint options without inferring table names.
			s.values["database.connections.mysql.singular"] = false
			s.values["database.connections.mysql.no_lower_case"] = true
			s.values["database.connections.mysql.name_replacer"] = strings.NewReplacer("a", "b")
			if test.appTimezone {
				s.values["database.connections.mysql.loc"] = ""
				s.values["app.timezone"] = "UTC"
			}
			var expected []contracts.FullConfig
			if len(test.endpoints) > 0 {
				expected = []contracts.FullConfig{s.expectedConfig()}
			}
			s.Equal(expected, s.config.fillDefault(test.endpoints))
		})
	}
}

func (s *ConfigTestSuite) TestExplicitTableNames() {
	for _, singular := range []bool{false, true} {
		s.values["database.connections.mysql.singular"] = singular
		s.values["database.connections.mysql.no_lower_case"] = true
		s.values["database.connections.mysql.name_replacer"] = strings.NewReplacer("Data", "Unexpected")
		driver := NewMysql(configValues{values: s.values}, nil, nil, "mysql")
		endpoint := driver.Pool().Writers[0]
		s.False(endpoint.Singular)
		s.False(endpoint.NoLowerCase)
		s.Nil(endpoint.NameReplacer)
		query := querybuilder.NewBuilder(NewQueryGrammar(endpoint.Prefix)).From("Data", "")
		s.Equal("select * from `goravel_Data`", query.ToSQL())
	}
}

func (s *ConfigTestSuite) TestFillDefaultPreservesEndpointValues() {
	endpoint := contracts.Config{Dsn: "endpoint-dsn", Host: "endpoint-host", Port: 3307, Database: "endpoint-database", Username: "endpoint-user", Password: "endpoint-password", Charset: "latin1", Loc: "Europe/Paris"}
	expected := contracts.FullConfig{Config: endpoint, Connection: "mysql", Driver: Name, Prefix: "goravel_"}
	s.Equal([]contracts.FullConfig{expected}, s.config.fillDefault([]contracts.Config{endpoint}))
}

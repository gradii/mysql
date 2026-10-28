package contracts

import (
	"database/sql/driver"

	contractsconfig "github.com/gradii/framework/contracts/config"
)

type ConfigBuilder interface {
	Config() contractsconfig.Config
	Connection() string
	Readers() []FullConfig
	Writers() []FullConfig
}

// Replacer is retained for source compatibility. Model mappings must use explicit
// database names; the driver no longer applies a Go-symbol naming strategy.
// Deprecated: declare table and column names explicitly.
type Replacer interface {
	Replace(name string) string
}

// Config describes an endpoint in config/database.go. Empty fields inherit the
// named connection's settings. An explicit Connector takes precedence over Dsn.
type Config struct {
	Connector driver.Connector
	Dsn       string
	Host      string
	Port      int
	Database  string
	Username  string
	Password  string
	Charset   string
	Loc       string
}

// FullConfig contains the resolved endpoint and its connection identity.
type FullConfig struct {
	Config
	Connection string
	Driver     string
	Prefix     string
}

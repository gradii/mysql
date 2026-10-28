package mysql

import (
	"database/sql/driver"
	"net"
	"strconv"
	"strings"
	"time"

	native "github.com/go-sql-driver/mysql"
	"github.com/gradii/framework/contracts/database"
	"github.com/gradii/framework/errors"
	mysqlerrors "github.com/gradii/mysql/errors"
)

// NewConnector validates an endpoint without opening a connection. Explicit
// connectors and DSNs retain their native options, including TLS/session settings.
func NewConnector(endpoint database.Config) (driver.Connector, error) {
	if endpoint.Connector != nil {
		return endpoint.Connector, nil
	}
	config, err := connectorConfig(endpoint)
	if err != nil {
		return nil, err
	}
	return native.NewConnector(config)
}

func connectorConfig(endpoint database.Config) (*native.Config, error) {
	if endpoint.Dsn != "" {
		return native.ParseDSN(endpoint.Dsn)
	}
	if endpoint.Host == "" {
		return nil, errors.DatabaseConfigNotFound
	}
	port := endpoint.Port
	if port == 0 {
		port = 3306
	}
	if port < 1 || port > 65535 {
		return nil, mysqlerrors.InvalidPort.Args(port)
	}
	location := endpoint.Timezone
	if location == "" {
		location = "UTC"
	}
	loc, err := time.LoadLocation(location)
	if err != nil {
		return nil, err
	}

	config := native.NewConfig()
	config.Net = "tcp"
	config.Addr = net.JoinHostPort(strings.Trim(endpoint.Host, "[]"), strconv.Itoa(port))
	config.User, config.Passwd, config.DBName = endpoint.Username, endpoint.Password, endpoint.Database
	config.Loc, config.ParseTime, config.MultiStatements = loc, true, true
	if endpoint.Charset != "" {
		if err := config.Apply(native.Charset(endpoint.Charset, "")); err != nil {
			return nil, err
		}
		// Parse the driver's own encoding to preserve its charset fallback-list
		// semantics while keeping credential values as supplied by the application.
		parsed, err := native.ParseDSN(config.FormatDSN())
		if err != nil {
			return nil, err
		}
		parsed.User, parsed.Passwd, parsed.DBName = config.User, config.Passwd, config.DBName
		config = parsed
	}
	return config, nil
}

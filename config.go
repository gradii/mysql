package mysql

import (
	"database/sql/driver"

	"github.com/gradii/framework/contracts/config"
	"github.com/gradii/mysql/contracts"
)

type Config struct {
	config     config.Config
	connection string
}

func NewConfig(config config.Config, connection string) *Config {
	return &Config{config: config, connection: connection}
}

func (r *Config) Config() config.Config { return r.config }

func (r *Config) Connection() string { return r.connection }

func (r *Config) Readers() []contracts.FullConfig {
	configs, ok := r.config.Get("database.connections." + r.connection + ".read").([]contracts.Config)
	if !ok {
		return nil
	}
	return r.fillDefault(configs)
}

func (r *Config) Writers() []contracts.FullConfig {
	configs, ok := r.config.Get("database.connections." + r.connection + ".write").([]contracts.Config)
	if !ok {
		configs = []contracts.Config{{}}
	}
	return r.fillDefault(configs)
}

func (r *Config) fillDefault(configs []contracts.Config) []contracts.FullConfig {
	if len(configs) == 0 {
		return nil
	}

	path := "database.connections." + r.connection + "."
	fullConfigs := make([]contracts.FullConfig, 0, len(configs))
	for _, endpoint := range configs {
		if endpoint.Connector == nil {
			endpoint.Connector, _ = r.config.Get(path + "connector").(driver.Connector)
		}
		if endpoint.Dsn == "" {
			endpoint.Dsn = r.config.GetString(path + "dsn")
		}
		if endpoint.Host == "" {
			endpoint.Host = r.config.GetString(path + "host")
		}
		if endpoint.Port == 0 {
			endpoint.Port = r.config.GetInt(path + "port")
		}
		if endpoint.Username == "" {
			endpoint.Username = r.config.GetString(path + "username")
		}
		if endpoint.Password == "" {
			endpoint.Password = r.config.GetString(path + "password")
		}
		if endpoint.Database == "" {
			endpoint.Database = r.config.GetString(path + "database")
		}
		if endpoint.Charset == "" {
			endpoint.Charset = r.config.GetString(path + "charset")
		}
		if endpoint.Loc == "" {
			endpoint.Loc = r.config.GetString(path + "loc")
			if endpoint.Loc == "" {
				endpoint.Loc = r.config.GetString("app.timezone", "UTC")
			}
			if endpoint.Loc == "" {
				endpoint.Loc = "UTC"
			}
		}
		fullConfigs = append(fullConfigs, contracts.FullConfig{
			Config: endpoint, Connection: r.connection,
			Driver: dialectName(r.config.GetString(path + "driver")),
			Prefix: r.config.GetString(path + "prefix"),
		})
	}
	return fullConfigs
}

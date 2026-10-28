package mysql

import (
	"sync"

	"github.com/gradii/framework/contracts/config"
	"github.com/gradii/framework/contracts/database"
	contractsdriver "github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/contracts/log"
	"github.com/gradii/framework/contracts/process"
	"github.com/gradii/framework/contracts/testing/docker"
	"github.com/gradii/mysql/contracts"
	mysqlerrors "github.com/gradii/mysql/errors"
)

var _ contractsdriver.Driver = (*Mysql)(nil)
var _ contractsdriver.DriverWithDialect = (*Mysql)(nil)
var _ contractsdriver.DriverWithServerVersion = (*Mysql)(nil)

type Mysql struct {
	config        contracts.ConfigBuilder
	mu            sync.RWMutex
	serverVersion func() (string, error)
}

// NewMysql resolves a named connection. The log and process arguments are kept
// for source compatibility; pool ownership belongs to the framework.
func NewMysql(config config.Config, _ log.Log, _ process.Process, connection string) *Mysql {
	return &Mysql{config: NewConfig(config, connection)}
}

// Docker is the compatibility entry point required by the legacy Driver contract.
func (r *Mysql) Docker() (docker.DatabaseDriver, error) {
	return nil, mysqlerrors.DockerNotSupported
}

func (r *Mysql) Dialect() contractsdriver.Dialect {
	name := Name
	if writers := r.config.Writers(); len(writers) > 0 {
		name = writers[0].Driver
	}
	return NewDialect(name)
}

func (r *Mysql) Grammar() contractsdriver.Grammar {
	var endpoint contracts.FullConfig
	if writers := r.config.Writers(); len(writers) > 0 {
		endpoint = writers[0]
	}
	grammar := NewSchemaGrammar(endpoint.Database, endpoint.Prefix, "", dialectName(endpoint.Driver))
	grammar.SetTableDefaults(endpoint.Charset, r.config.Config().GetString("database.connections."+r.config.Connection()+".collation"))
	r.mu.RLock()
	resolver := r.serverVersion
	r.mu.RUnlock()
	grammar.SetVersionResolver(resolver)
	return grammar
}

// SetServerVersion receives the framework's cached managed-writer resolver.
func (r *Mysql) SetServerVersion(resolver func() (string, error)) {
	r.mu.Lock()
	r.serverVersion = resolver
	r.mu.Unlock()
}

// Pool only normalizes configuration. Connector validation is deferred to the
// error-returning dialect factory when the framework opens each endpoint.
func (r *Mysql) Pool() database.Pool {
	return database.Pool{
		Readers: r.fullConfigsToConfigs(r.config.Readers()),
		Writers: r.fullConfigsToConfigs(r.config.Writers()),
	}
}

func (r *Mysql) Processor() contractsdriver.Processor { return NewProcessor() }

func (r *Mysql) fullConfigsToConfigs(fullConfigs []contracts.FullConfig) []database.Config {
	configs := make([]database.Config, len(fullConfigs))
	for i, endpoint := range fullConfigs {
		configs[i] = database.Config{
			Connector: endpoint.Connector, Connection: endpoint.Connection,
			Dsn: endpoint.Dsn, Database: endpoint.Database, Driver: dialectName(endpoint.Driver),
			Host: endpoint.Host, Password: endpoint.Password, Port: endpoint.Port,
			Prefix: endpoint.Prefix, Username: endpoint.Username,
			Charset: endpoint.Charset, Timezone: endpoint.Loc,
		}
	}
	return configs
}

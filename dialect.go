package mysql

import (
	"database/sql/driver"
	"strings"

	"github.com/gradii/framework/contracts/database"
	contractsdriver "github.com/gradii/framework/contracts/database/driver"
	contractsquery "github.com/gradii/framework/contracts/database/query"
)

var _ contractsdriver.Dialect = (*Dialect)(nil)
var _ contractsdriver.DialectWithServerVersion = (*Dialect)(nil)
var _ contractsdriver.DialectWithEscaper = (*Dialect)(nil)

// Dialect supplies connection-specific MySQL or MariaDB runtime components.
type Dialect struct{ name string }

func NewDialect(name ...string) *Dialect {
	selected := Name
	if len(name) > 0 {
		selected = dialectName(name[0])
	}
	return &Dialect{name: selected}
}

func (d *Dialect) Name() string { return dialectName(d.name) }

func (d *Dialect) ServerVersionSQL() string { return "select version()" }

func (d *Dialect) NewConnector(endpoint database.Config) (driver.Connector, error) {
	return NewConnector(endpoint)
}

func (d *Dialect) NewQueryGrammar(options contractsdriver.GrammarOptions) contractsquery.Grammar {
	if d.Name() == "MariaDB" || strings.EqualFold(options.Driver, "mariadb") {
		return NewMariaDBQueryGrammar(options.Prefix, options.ServerVersion)
	}
	return NewQueryGrammar(options.Prefix, options.ServerVersion)
}

func (d *Dialect) NewProcessor() contractsquery.Processor {
	if d.Name() == "MariaDB" {
		return NewMariaDBQueryProcessor()
	}
	return NewQueryProcessor()
}

func dialectName(name string) string {
	if strings.EqualFold(name, "mariadb") {
		return "MariaDB"
	}
	return Name
}

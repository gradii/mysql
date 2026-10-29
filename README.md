# MySQL

MySQL and MariaDB runtime driver for `github.com/gradii/framework`. This fork of
[goravel/mysql](https://github.com/goravel/mysql) uses `database/sql` and
`github.com/go-sql-driver/mysql`, with no GORM dependency.

Register `&mysql.ServiceProvider{}` with the application and select the driver
through the connection's `via` callback:

```go
import (
    "github.com/gradii/framework/contracts/database/driver"
    mysqlfacades "github.com/gradii/mysql/facades"
)

"mysql": map[string]any{
    "driver": "mysql", // "mariadb" explicitly selects MariaDB behavior
    "host": "127.0.0.1",
    "port": 3306,
    "database": "application",
    "username": "application",
    "password": "",
    "charset": "utf8mb4",
    "loc": "UTC",
    "prefix": "",
    "via": func() (driver.Driver, error) {
        return mysqlfacades.Mysql("mysql")
    },
},
```

The framework owns pools, transactions, reader/writer routing and cached server
version discovery. The driver supplies connectors, query and schema grammars,
metadata processors and MySQL/MariaDB compatibility behavior.

Reader and writer entries use `[]contracts.Config`. Empty endpoint fields inherit
the named connection's values. An explicit `Connector` wins over a DSN; an explicit
DSN wins over individual connection fields and preserves native TLS, session and
other options. Generated connections enable `parseTime` and `multiStatements`.
Connector validation occurs when the framework opens the endpoint, not in `Pool()`.

`loc` falls back to `app.timezone`, then UTC. It controls Go time parsing and
representation, independently of MySQL's session `time_zone`; set session options
through the native DSN when needed. Connection `charset` is retained independently
of schema table/column charset and collation.

Model table and column mappings must be explicit. `prefix` remains supported;
`singular`, `name_replacer` and `no_lower_case` are removed. See
[NAMING_STRATEGY.md](NAMING_STRATEGY.md) for the mapping constraints. Docker helpers
and automatic installation tooling are outside this runtime module. The legacy
`Driver.Docker()` method returns an unsupported error.

## Development

This refactor requires the matching framework changes. The framework dependency
must be updated to a published revision containing the new driver contracts before
release. To work on sibling `mysql` and `goravel-framework` checkouts locally:

```sh
go work init . ../goravel-framework
go work edit -replace=github.com/gradii/framework@v1.20.0-alpha.0=../goravel-framework
go test ./...
go tool mockery
```

Workspace files are ignored. Release dependencies do not contain machine-specific
paths. Unit tests do not create Docker containers. Optional live connector tests
use `GRADII_TEST_MYSQL_DSN` (or `GORAVEL_TEST_MYSQL_DSN`) against an existing
disposable test database. For example, with credentials supplied by your test
environment:

```sh
go test -run TestConnectorIntegrationCharsetAndTimezone -v .
go test -run TestFrameworkIntegration -v .
```

The live connector test verifies UTF-8 text and datetime round trips in UTC and
Asia/Shanghai, plus independent native DSN charset and session timezone settings.
It creates connection-local temporary tables and closes its pools afterward.
The framework integration test uses a unique table prefix, removes its tables on
completion, and verifies DB/ORM CRUD, JSON predicates, explicit model mappings,
single-connection transactions, schema metadata and foreign-key session cleanup.

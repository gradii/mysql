# MySQL driver refactor plan

Planning baseline: framework `c53f685c` at `/Users/dog/Gradii/goravel-framework`, MySQL driver `fe4b4cf` at `/Users/dog/Gradii/mysql`. The runtime refactor is now implemented in the working trees of framework, mysql, postgres, sqlite, and sqlserver. The sections below retain the design rationale; verification and local/CI workspace setup are recorded in `goravel-framework/.cnb/README.md`.

The target is a `github.com/gradii/mysql` driver for `github.com/gradii/framework`, using `database/sql` and `github.com/go-sql-driver/mysql`. The MySQL package will own its concrete query grammar, schema grammar, processors, and connector construction. The framework will obtain these components from the configured driver and depend on their public interfaces.

Confirmed scope: **complete Schema Grammar, legacy MySQL/MariaDB compatibility, and charset/timezone configuration are mandatory migration requirements**. Complete means the union of the existing driver's schema implementation and the framework's newer schema branches. Docker helpers and installation tools are outside this refactor. Builds may be obfuscated: **database mapping must not depend on Go struct names, and automatic type-name-based table inference will not be restored**. Explicit mapping and the remaining reflection-sensitive paths are documented in [NAMING_STRATEGY.md](NAMING_STRATEGY.md).

**Architecture decision**

Use a dialect provider together with the standard Go connector. They have different responsibilities:

| Component | Owner | Responsibility |
| --- | --- | --- |
| Configured driver | MySQL package | Resolve named connection configuration, reader/writer endpoints, and dialect components. |
| Dialect | MySQL package | Construct MySQL/MariaDB grammars and processors; provide MySQL connection and SQL behavior. |
| Connector factory | MySQL package | Validate one resolved endpoint and return a `database/sql/driver.Connector`. |
| `driver.Connector` | Native Go SQL driver | Create physical connections through `Connect(context.Context)`. |
| Connection manager | Framework | Call `sql.OpenDB`, own pools, choose readers/writers, manage transactions, reconnects, telemetry, and shutdown. |
| Query builder and ORM | Framework | Build query state and model operations; compile through the injected grammar and process results through the injected processor. |
| Schema manager | Framework | Build migration descriptions, execute schema statements, cache metadata, and manage pinned sessions. |

This uses the useful part of a GORM-style dialect abstraction: a database package supplies database-specific behavior. It does not introduce a GORM dependency, GORM callbacks, or a dialect initializer that mutates ORM internals.

The existing `database.connections.<name>.via` callback remains the entry point for selecting a configured driver. A global registration mechanism is unnecessary for normal application execution. `Config.Connector`, which already has the standard Go connector type, remains an explicit per-endpoint override.

**Why changing the MySQL repository alone is insufficient**

The current framework already accepts external drivers, but several execution paths bypass the components they supply:

| Current location in the framework | Dependency to remove for MySQL |
| --- | --- |
| `database/driver/dialect.go` | Imports the native MySQL driver and implements `mysqlDSN` / `openMySQL`. |
| `database/query/grammars/factory.go` | Selects concrete MySQL/MariaDB query grammars by driver-name strings. |
| `database/query/processors/metadata.go` | Selects concrete processors by driver-name strings. |
| `database/db/db.go` | Constructs a query grammar internally when creating a connection. |
| `database/db/query.go` | Receives a query grammar but independently constructs a processor. |
| `database/gorm/query.go` | Receives the legacy driver grammar, then independently chooses the modern query grammar and processor. Connection refresh also reconstructs the processor by name. |
| `database/schema/schema.go` | Replaces the external driver's metadata processing path with a built-in processor for recognized databases. |
| `database/schema/ddl.go` | Contains MySQL type compilation, modifier ordering, generated-column handling, and command compilation. |
| `database/schema/metadata_sql.go` | Compiles MySQL catalog queries inside the framework. |
| `database/schema/operations.go` | Contains MySQL database creation and foreign-key session SQL. |
| `database/driver/runtime.go` | Selects MySQL server-version SQL and literal escaping behavior. |

There are also two different grammar/processor APIs: the external driver's older schema-oriented contracts and the framework's newer query grammar and processor contracts. They must be wired deliberately; changing only imports or moving `mysql.go` files would leave execution using the old internal selections.

**Target ownership and file moves**

Paths in the source column are relative to the framework unless marked as existing MySQL files.

| Source | Target in `/Users/dog/Gradii/mysql` | Treatment |
| --- | --- | --- |
| `database/driver/dialect.go`: MySQL DSN and connector construction | `connector.go` | Port to a validating connector factory; the framework retains pool ownership. |
| `database/query/grammars/mysql.go` | `query_grammar.go` | Move the current Laravel-compatible query implementation and its relevant tests. |
| `database/query/grammars/mariadb.go` | `mariadb_query_grammar.go` | Preserve MariaDB overrides alongside the MySQL grammar. |
| `database/query/processors/mysql.go`, `mariadb.go`, and MySQL-specific methods in `metadata.go` | `processor.go`, `mariadb_processor.go` | Consolidate into the modern processor API, retaining insert-ID and metadata behavior. |
| Existing MySQL `grammar.go` plus MySQL branches in framework schema DDL, metadata, and operations | `schema_grammar.go`, `schema_metadata.go`, `schema_operations.go` | Merge the implementations; the current framework parity behavior is the reference. |
| MySQL-specific server-version SQL and escaping | `dialect.go` and grammar helpers | Supply through dialect/runtime capabilities. |
| Existing MySQL `mysql.go` | `mysql.go` | Keep the configured-driver facade; remove GORM-based connection/version construction. |

Generic builder state, model reflection, row scanning, relations, soft deletion, migration orchestration, transaction management, shared compilation algorithms, and generic scalar conversion stay in the framework. Other database implementations can use the new contracts later; this refactor extracts the MySQL family first.

**Public contracts and dependency direction**

The proposed shape is a `Dialect` interface exposed by the configured driver. Its component factories are conceptually:

```go
type Dialect interface {
    Name() string
    NewConnector(endpoint database.Config) (sqldriver.Connector, error)
    NewQueryGrammar(options GrammarOptions) querycontracts.Grammar
    NewProcessor() querycontracts.Processor
}
```

The implemented contract is `contracts/database/driver.Dialect`. Schema grammars remain supplied through `Driver.Grammar()` and expose `SchemaCompiler`, metadata, operation, and execution-policy capabilities. `GrammarOptions` carries the prefix, driver name, and managed server-version resolver. Grammar construction does not open a separate SQL pool.

1. Extract the query grammar and processor interfaces into a dependency-neutral public contract package, such as `contracts/database/query`. Put their dependency-free query state and expression declarations there as necessary; use type aliases from the existing public locations during migration. Keep shared algorithms in implementation packages.
2. Add `Dialect` and a temporary `DriverWithDialect` capability in `contracts/database/driver`. This lets the MySQL fork adopt the new path before all other drivers are ported. Once migration is complete, make dialect provision part of the main driver contract and remove the temporary capability check.
3. Avoid a cycle from `contracts/database/driver` through `database/query/grammars` and `database/schema` back to the driver contracts. Contracts must not import the concrete compiler packages or the external MySQL module.
4. Define a schema compiler contract taking a public blueprint description and returning statements with bindings. Add a public snapshot of column/command options: generated expressions, charset/collation, invisible/instant flags, spatial options, defaults, nullable state, and other currently supported attributes. Preserve unset versus explicit false/zero. The external compiler must not need private fields of `schema.Blueprint` or `ColumnDefinition`.
5. Include typed metadata requests and schema operation compilation in that contract. The framework executes the returned statements; it continues to own metadata caching, connection pinning, and restoration of session state.
6. Store the resolved dialect/components with the connection runtime. Do not put a dialect interface in `database.Config` if it creates an import cycle; that struct already serves as the endpoint description.

The dependency direction is `application -> gradii/mysql -> framework contracts/shared bases`. Framework production packages must not import `gradii/mysql`. Cross-module verification belongs in the MySQL driver tests or the framework's separate integration module.

**Implementation sequence**

1. **Establish public extension points in the framework.**

   Introduce the contracts and connection runtime described above. Route DB, ORM, and Schema through injected factories while keeping temporary legacy adapters for unported drivers. Add an external-package test dialect with distinctive grammar output and processor behavior to prove the injected implementations are actually called. A missing component from a dialect-enabled driver must produce a declared error rather than silently selecting a built-in MySQL implementation.

   Export narrow shared helpers needed by the moved compiler. In particular, the current MySQL grammar accesses `BaseGrammar.wrap`, which is private; expose the necessary JSON field/path operation rather than the mutable wrapper itself. Keep `BaseGrammar.Attach` wired to the outer MySQL or MariaDB grammar so calls from shared compilation continue to reach its overrides. Extract reusable processor scalar helpers without keeping MySQL-specific behavior in the common base.

2. **Fork the MySQL module and replace GORM connection construction.**

   Change the runtime module to `github.com/gradii/mysql` and change framework/self imports, facade binding, mockery configuration, runtime documentation, and CI references consistently. Use the matching `github.com/gradii/framework` revision. Use an ignored local workspace for development; release metadata must not depend on absolute paths on this machine. Keep license and upstream attribution. Do not migrate automatic installation scripts or Docker helper functionality.

   Implement `NewConnector` using the native driver's configuration parser and `NewConnector`. Keep config normalization in `Pool()` free of network I/O. Because `Pool()` cannot return errors, connector validation must happen through the error-returning factory when the framework opens an endpoint.

   Preserve explicit DSN precedence, read/write inheritance, charset, location/timezone, `parseTime`, multi-statement behavior, and native DSN options. Transfer `Charset` and `Loc -> Timezone` explicitly when producing framework endpoint configs; the upstream conversion currently omits them. Preserve the location fallback `connection.loc -> app.timezone -> UTC`. A native driver's `loc` controls time parsing/representation; it is not a substitute for MySQL's session `time_zone`. Session parameters supplied through an explicit DSN must remain intact. Cover connection charset separately from schema/table/column charset and collation. Retain the database table prefix; remove or explicitly deprecate type/field-name-based `singular`, `name_replacer`, and `no_lower_case` settings rather than restoring their implicit Go-symbol mapping.

   Opening precedence becomes: explicit `Config.Connector`, otherwise the configured dialect's connector factory. Remove the MySQL fallback in `openDialect` once the new driver is wired. Update the early empty-database check in `BuildConnection`: a DSN-only endpoint or supplied connector must not be discarded merely because `Config.Database` is empty. Distinguish intentionally unconfigured endpoints from malformed configuration.

3. **Move the query grammars and processors, then update every construction path.**

   Port the framework implementations, retaining the existing Laravel parity fixes. Make DB and ORM use both components supplied by the driver. Update `database/db/NewQuery`, ORM construction, `refreshConnection`, `RebindGrammar`, cloning, subqueries, relation queries, transactions, and raw-SQL rendering. A connection switch must switch the grammar, processor, prefix, escaping, and server capabilities together.

   Create grammar instances with connection-specific state; do not share a mutable global grammar between connections. Retain `SetEscaper`, upsert alias settings, and server-version access. Keep insert-ID processing tied to the `sql.Result` from the same insert operation; do not obtain `LAST_INSERT_ID()` from a different pooled connection.

   Resolve MySQL versus MariaDB deliberately. Preserve explicit dialect selection, and use the existing connection-owned version mechanism for server detection and version-dependent behavior. Keep MariaDB's lateral-join restrictions and JSON differences, MySQL's legacy group-limit behavior, and version-specific schema rules. SQL-only compilation tests provide explicit server information and require no database connection. Version discovery must use the managed writer pool, be cached per connection, and surface discovery errors instead of caching an `UNKNOWN` string as a version.

4. **Move MySQL schema compilation and metadata processing completely.**

   Merge the upstream schema grammar with the current framework MySQL branches. Route both direct blueprint compilation and `facades.Schema()` through the driver compiler. Cover DDL, catalog queries, database creation/deletion, type/default/modifier rendering, and foreign-key state queries. Use statement bindings for metadata values and identifier quoting for identifiers.

   Complete Schema Grammar is a dedicated completion gate with this inventory:

   | Category | Required coverage |
   | --- | --- |
   | Databases and tables | Create/drop database; create/drop/drop-if-exists/rename table; drop-all tables/views; temporary tables; engine; table comments; charset and collation. |
   | Column lifecycle | Add/change/drop/rename; correct SQL when ordinary and advanced columns occur together. |
   | Base types | Signed/unsigned integer widths and auto-increment; bool; char/string/text variants; decimal/float/double; enum; JSON; date/time/datetime/timestamp and precision; UUID/ULID mappings. |
   | Extended types | Binary/blob/fixed/variable binary, year, IP/MAC, set, raw type definitions, supported spatial subtypes/SRID, and existing version-dependent vector support. Unsupported server capabilities must report their existing errors. |
   | All modifiers | Nullable/default/expression/current timestamp/on-update, unsigned/auto-increment, comment, first/after, charset/collation, stored/virtual generated values, invisible, starting value/from, and instant. Preserve unset/explicit false/explicit removal. |
   | Indexes | Primary/ordinary/unique/fulltext/spatial/raw-expression; composite columns, explicit/default names and algorithm; create/drop/rename. |
   | Foreign keys | Single/composite references, update/delete actions, add/drop, typed ID helpers, constraint state queries and enable/disable statements. |
   | Metadata | Schemas/current schema; tables/views; columns/full type/default/nullable/generation; indexes/foreign keys; prefix and qualified-name handling; metadata used by Has* and migration operations. |
   | Compatibility | MySQL/MariaDB product/version detection; legacy rename-column SQL; supported version-dependent type/modifier syntax; explicit errors or established no-op behavior for unsupported operations. |

   Audit and replace the remaining calls to the old `CompileCreate/Add/Change/Drop/DropIfExists/DropColumn/Rename/RenameColumn/RenameIndex`, index/foreign-key compilers, `CompileTableComment`, `GetAttributeCommands`, `Type*`, and reflective `Modify*` dispatch. Moving only `database/query/grammars/mysql.go` does not satisfy this step. Empty `CompileComment`/`CompileDefault` methods do not mean those features are absent: MySQL renders them inline.

   Preserve legacy rename-column behavior at the exact existing boundaries: MySQL `<8.0.3` and MariaDB `<10.5.2` use `CHANGE` with the old column definition; newer versions use `RENAME COLUMN`. Test preservation of type, collation, nullable/default, comment, auto-increment, and on-update attributes. Verify MySQL SRID and MariaDB's alternative syntax independently. Server-specific APIs such as vector types must state the actual versions verified.

   Before deleting any old implementation, complete the readable schema contract: current `driver.Blueprint` lacks table engine/charset/collation/temporary access, and several column options have setters without getters. Export a complete description/snapshot rather than requiring the external compiler to access concrete framework fields. Compilation must return SQL, bindings, and errors; command ordering and generic Blueprint orchestration stay in the framework.

   Keep the schema manager responsible for acquiring one writer session, running foreign-key changes and the callback on that session, and restoring the original state on success, failure, or panic. The MySQL package provides the SQL and required behavior; moving SQL ownership must not move session management into a separate pool.

   Consolidate the old typed schema processor and the newer query processor behind one canonical implementation. Adapt public framework metadata return types at the boundary without maintaining two competing MySQL conversion algorithms. Preserve generated-column metadata, nullable defaults, index/foreign-key normalization, and MySQL/MariaDB catalog differences.

5. **Remove the remaining GORM runtime paths and finish framework extraction.**

   Replace GORM-dependent runtime grammar methods and tests with the new query/schema interfaces. Docker helpers and installation tools are not being ported. Isolate or remove their legacy sources from the new runtime module's default build rather than retaining their GORM dependency. If the old `Driver` contract is retained temporarily, its Docker member is an unsupported compatibility entry point, not a requirement to implement a helper; prefer separating optional tooling capabilities from the runtime contract.

   Remove `gorm.io/driver/mysql`, `gorm.io/gorm`, and GORM-only transitive dependencies from the MySQL module after checking the whole dependency graph. Remove the framework's concrete MySQL/MariaDB grammar and processor implementations, their factory cases, and aliases that expose those concrete types. Keep all original framework test files and cases in the framework repository, using test-only adapters to the external drivers. Port upstream driver cases to the new APIs and add runtime regressions alongside them. Test-only database lifecycle cases live in framework/tests; do not retain the original GORM implementations or a legacy compatibility module.

   For MySQL runtime specifics still selected by name, supply small dialect capabilities for version SQL, literal escaping, and native error classification. Shared cancellation handling and transaction retry policy remain in the framework. Existing generic helpers shared with other drivers can stay, but the MySQL execution path must no longer bypass its dialect.

   Declare new errors through the framework error constructor in the agreed error list; do not add inline errors. Regenerate mocks with mockery instead of editing generated files.

6. **Integrate and release in dependency order.**

   Land framework extension contracts first, then port and test `gradii/mysql`, then switch application configuration and framework MySQL integration imports to the fork. Finally remove the transitional MySQL implementations from the framework after both DB and ORM use the external components.

   The current framework integration module still references `github.com/goravel/{mysql,postgres,sqlite,sqlserver}`. Import paths are type identity in Go: mixing upstream framework contracts with `github.com/gradii/framework` is not a compatibility solution. Verify MySQL through a focused integration harness in this driver repository first. The full multi-database integration module also needs matching ports/adapters for the remaining upstream drivers; do not claim its migration is complete based only on MySQL results.

**Verification and completion criteria**

| Area | Required evidence |
| --- | --- |
| Dependency boundaries | Framework production imports contain no `github.com/gradii/mysql`; the MySQL dependency graph contains no GORM or upstream framework packages. |
| Injection | A deliberately different test grammar and processor are used by DB, ORM, Schema, connection switches, and transactions; no name-based fallback hides a missing component. |
| Connector | DSN-only and explicit-connector endpoints work; invalid DSNs return errors; charset/location and read/write overrides retain existing behavior. |
| Query grammar | Port the MySQL/MariaDB portions of the existing Laravel parity suite, including JSON updates/binding order, empty inserts, upserts, full text, lateral joins, locks, and group limits. |
| Schema/processor | Compare compiled DDL and metadata fixtures before and after the move; verify generated columns, defaults, foreign keys, indexes, charset/collation, and version-sensitive behavior. |
| Real MySQL/MariaDB | Run DB, ORM, complete schema lifecycle, insert-ID, transaction, and read/write routing tests against existing test servers at explicitly recorded versions. No Docker or installer functionality is required. |
| Explicit mapping | Follow `NAMING_STRATEGY.md`: use stable table/column descriptions, reject missing required mapping, preserve prefix rules, align scanning/writing/migration generation, and isolate caches. Audit relation/event reflection and verify the actual obfuscated build; do not restore Go-type-name table fallback. |
| Connection state | Confirm prefix/escaping isolation between two connections, race-free version caching, and foreign-key restoration on the same leased session. |
| Removal | No concrete MySQL query/schema compiler or processor remains selected by the framework's production path; shared helpers contain shared behavior only. |

Run the existing framework database suite and the forked MySQL suite after each relevant phase. Add focused cross-module tests when components move; perform the broader framework suite after integration. Choose the supported MySQL/MariaDB version matrix from the existing application's requirements before removing any legacy SQL behavior.

The first implementation milestone is a GORM-free `gradii/mysql` runtime connector plus verified injection contracts. The refactor is complete only after the full MySQL/MariaDB Schema Grammar inventory, required legacy compatibility, and charset/timezone behavior have been verified alongside the external query grammar and processor, and the internal MySQL selections have been removed. Docker and installation tools are excluded.

package mysql

import (
	"strings"

	contractsquery "github.com/gradii/framework/contracts/database/query"
	"github.com/gradii/framework/database/query/processors"
)

var _ contractsquery.Processor = (*QueryProcessor)(nil)

func NewQueryProcessor() *QueryProcessor { return &QueryProcessor{} }

// QueryProcessor uses the ID recorded by the insert operation, matching MySqlProcessor in Laravel.
// Go's sql.Result supplies the connection-local ID that Laravel's MySqlConnection records.
type QueryProcessor struct{ processors.BaseProcessor }

// ProcessColumns ports MySqlProcessor::processColumns, including generated-column metadata.
func (p *QueryProcessor) ProcessColumns(results []contractsquery.Row, sql ...string) []contractsquery.Row {
	columns := make([]contractsquery.Row, 0, len(results))
	for _, result := range results {
		extra := strings.ToLower(processors.MetadataString(result["extra"]))
		var generation any
		if processors.MetadataString(result["expression"]) != "" {
			var kind any
			switch {
			case strings.Contains(extra, "stored generated"):
				kind = "stored"
			case strings.Contains(extra, "virtual generated"):
				kind = "virtual"
			}
			generation = contractsquery.Row{"type": kind, "expression": result["expression"]}
		}
		columns = append(columns, contractsquery.Row{
			"name": result["name"], "type_name": result["type_name"], "type": result["type"],
			"collation": result["collation"], "nullable": strings.EqualFold(processors.MetadataString(result["nullable"]), "YES"),
			"default": result["default"], "auto_increment": strings.Contains(extra, "auto_increment"),
			"comment": processors.NullableComment(result["comment"]), "generation": generation,
		})
	}
	return columns
}

func (p *QueryProcessor) ProcessIndexes(results []contractsquery.Row) []contractsquery.Row {
	indexes := make([]contractsquery.Row, 0, len(results))
	for _, result := range results {
		name := strings.ToLower(processors.MetadataString(result["name"]))
		indexes = append(indexes, contractsquery.Row{
			"name": name, "columns": processors.MetadataColumns(result["columns"]),
			"type":   strings.ToLower(processors.MetadataString(result["type"])),
			"unique": processors.MetadataBool(result["unique"]), "primary": name == "primary",
		})
	}
	return indexes
}

func (p *QueryProcessor) ProcessForeignKeys(results []contractsquery.Row) []contractsquery.Row {
	keys := make([]contractsquery.Row, 0, len(results))
	for _, result := range results {
		keys = append(keys, contractsquery.Row{
			"name": result["name"], "columns": processors.MetadataColumns(result["columns"]),
			"foreign_schema": result["foreign_schema"], "foreign_table": result["foreign_table"],
			"foreign_columns": processors.MetadataColumns(result["foreign_columns"]),
			"on_update":       strings.ToLower(processors.MetadataString(result["on_update"])),
			"on_delete":       strings.ToLower(processors.MetadataString(result["on_delete"])),
		})
	}
	return keys
}

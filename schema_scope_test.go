package mysql

import (
	"context"
	"testing"

	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/errors"
	"github.com/stretchr/testify/require"
)

type currentSchemaCatalog struct {
	driver.SchemaCatalog
	rows      []map[string]any
	err       error
	statement driver.SchemaStatement
}

func (c *currentSchemaCatalog) Select(statement driver.SchemaStatement) ([]map[string]any, error) {
	c.statement = statement
	return c.rows, c.err
}

func TestCurrentSchemaUsesWriterDatabase(t *testing.T) {
	g := NewSchemaGrammar("stale_database", "", "8.0.0", Name)
	catalog := &currentSchemaCatalog{rows: []map[string]any{{"name": "actual_database"}}}
	names, err := g.ResolveCurrentSchemas(driver.SchemaConnectionInfo{Database: "stale_database"}, catalog)
	require.NoError(t, err)
	require.Equal(t, []string{"actual_database"}, names)
	require.Equal(t, driver.SchemaStatement{SQL: "select database() as name"}, catalog.statement)
	for _, rows := range [][]map[string]any{nil, {{"name": nil}}, {{"name": ""}}, {{"name": "first"}, {"name": "second"}}} {
		_, err := g.ResolveCurrentSchemas(driver.SchemaConnectionInfo{}, &currentSchemaCatalog{rows: rows})
		require.ErrorIs(t, err, errors.SchemaCurrentNamespaceUnavailable)
	}
	_, err = g.ResolveCurrentSchemas(driver.SchemaConnectionInfo{}, nil)
	require.ErrorIs(t, err, errors.SchemaCurrentNamespaceUnavailable)
	_, err = g.ResolveCurrentSchemas(driver.SchemaConnectionInfo{}, &currentSchemaCatalog{err: context.Canceled})
	require.ErrorIs(t, err, context.Canceled)
}

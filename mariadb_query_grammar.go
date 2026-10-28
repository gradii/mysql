package mysql

import "github.com/gradii/framework/database/query/core"

// MariaDBQueryGrammar preserves MariaDB's JSON casts, window limits, and lateral-join restriction.
type MariaDBQueryGrammar struct{ *QueryGrammar }

func NewMariaDBQueryGrammar(prefix string, serverVersion ...func() (string, error)) *MariaDBQueryGrammar {
	g := &MariaDBQueryGrammar{QueryGrammar: NewQueryGrammar(prefix, serverVersion...)}
	g.mariaDB = true
	// Re-attach the outer dialect so shared compilation reaches MariaDB overrides.
	g.Attach(g)
	return g
}

func (g *MariaDBQueryGrammar) CompileJoinLateral(join core.Join, expression string) string { return "" }
func (g *MariaDBQueryGrammar) UseLegacyGroupLimit(b core.Query) bool                       { return false }

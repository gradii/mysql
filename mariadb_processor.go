package mysql

// MariaDBQueryProcessor inherits MySQL's insert-ID processing, as in Laravel.
type MariaDBQueryProcessor struct{ QueryProcessor }

func NewMariaDBQueryProcessor() *MariaDBQueryProcessor { return &MariaDBQueryProcessor{} }

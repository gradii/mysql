package mysql

import (
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/gradii/framework/errors"
	"github.com/stretchr/testify/require"
)

func TestDialectEscape(t *testing.T) {
	for _, test := range []struct {
		value  any
		binary bool
		want   string
	}{
		{nil, false, "null"}, {(*string)(nil), false, "null"},
		{sql.NullString{}, false, "null"}, {sql.NullString{String: "can't", Valid: true}, false, "'can''t'"},
		{true, false, "1"}, {false, false, "0"}, {int64(-4), false, "-4"}, {uint64(4), false, "4"},
		{1.25, false, "1.25"}, {"a\\b'c", false, "'a\\\\b''c'"},
		{[]byte{0, 255, 65}, true, "x'00ff41'"},
		{time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC), false, "'2024-02-03 04:05:06'"},
	} {
		literal, err := NewDialect().Escape(test.value, test.binary)
		require.NoError(t, err)
		require.Equal(t, test.want, literal)
	}
	for _, test := range []struct {
		value  any
		binary bool
		want   error
	}{
		{math.NaN(), false, errors.DatabaseEscapeNonFinite},
		{math.Inf(1), false, errors.DatabaseEscapeNonFinite},
		{"has\x00null", false, errors.DatabaseEscapeNullByte},
		{string([]byte{255}), false, errors.DatabaseEscapeInvalidUTF8},
		{1, true, errors.DatabaseUnsupportedType},
		{struct{}{}, false, errors.DatabaseUnsupportedType},
	} {
		_, err := NewDialect().Escape(test.value, test.binary)
		require.ErrorIs(t, err, test.want)
	}
}

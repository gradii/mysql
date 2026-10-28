package mysql

import (
	"testing"

	contractsdriver "github.com/gradii/framework/contracts/database/driver"
	databasedb "github.com/gradii/framework/database/db"
	"github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/foundation/json"
	mocksdriver "github.com/gradii/framework/mocks/database/driver"
	mocksfoundation "github.com/gradii/framework/mocks/foundation"
	"github.com/gradii/framework/support/convert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type GrammarSuite struct {
	suite.Suite
	grammar *Grammar
}

func TestGrammarSuite(t *testing.T) {
	suite.Run(t, &GrammarSuite{})
}

func (s *GrammarSuite) SetupTest() {
	s.grammar = NewGrammar("goravel", "goravel_", "8.0.3", Name)
}

func (s *GrammarSuite) TestCompileAdd() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockColumn := mocksdriver.NewColumnDefinition(s.T())

	mockBlueprint.EXPECT().GetTableName().Return("users").Once()
	mockColumn.EXPECT().GetName().Return("name").Once()
	mockColumn.EXPECT().GetType().Return("string").Twice()
	mockColumn.EXPECT().GetDefault().Return("goravel").Twice()
	mockColumn.EXPECT().GetNullable().Return(false).Once()
	mockColumn.EXPECT().GetLength().Return(1).Once()
	mockColumn.EXPECT().GetOnUpdate().Return(nil).Once()
	mockColumn.EXPECT().GetComment().Return("comment").Once()
	mockColumn.EXPECT().GetUnsigned().Return(false).Once()
	mockColumn.EXPECT().GetAfter().Return("id").Twice()
	mockColumn.EXPECT().IsFirst().Return(false).Once()

	sql := s.grammar.CompileAdd(mockBlueprint, &contractsdriver.Command{
		Column: mockColumn,
	})

	s.Equal("alter table `goravel_users` add `name` varchar(1) not null default 'goravel' comment 'comment' after `id`", sql)
}

func (s *GrammarSuite) TestCompileChange() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockColumn := mocksdriver.NewColumnDefinition(s.T())

	mockBlueprint.EXPECT().GetTableName().Return("users").Once()
	mockColumn.EXPECT().GetName().Return("name").Once()
	mockColumn.EXPECT().GetType().Return("string").Twice()
	mockColumn.EXPECT().GetDefault().Return("goravel").Twice()
	mockColumn.EXPECT().GetNullable().Return(false).Once()
	mockColumn.EXPECT().GetLength().Return(1).Once()
	mockColumn.EXPECT().GetOnUpdate().Return(nil).Once()
	mockColumn.EXPECT().GetComment().Return("comment").Once()
	mockColumn.EXPECT().GetUnsigned().Return(false).Once()
	mockColumn.EXPECT().GetAfter().Return("").Once()
	mockColumn.EXPECT().IsFirst().Return(true).Once()

	sql := s.grammar.CompileChange(mockBlueprint, &contractsdriver.Command{
		Column: mockColumn,
	})

	s.Equal([]string{"alter table `goravel_users` modify `name` varchar(1) not null default 'goravel' comment 'comment' first"}, sql)
}

func (s *GrammarSuite) TestCompileCreate() {
	mockColumn1 := mocksdriver.NewColumnDefinition(s.T())
	mockColumn2 := mocksdriver.NewColumnDefinition(s.T())
	mockBlueprint := mocksdriver.NewBlueprint(s.T())

	// postgres.go::CompileCreate
	primaryCommand := &contractsdriver.Command{
		Name:      "primary",
		Columns:   []string{"role_id", "user_id"},
		Algorithm: "btree",
	}
	mockBlueprint.EXPECT().GetCommands().Return([]*contractsdriver.Command{
		primaryCommand,
	}).Once()
	mockBlueprint.EXPECT().GetTableName().Return("users").Once()
	// utils.go::getColumns
	mockBlueprint.EXPECT().GetAddedColumns().Return([]contractsdriver.ColumnDefinition{
		mockColumn1, mockColumn2,
	}).Once()
	// utils.go::getColumns
	mockColumn1.EXPECT().GetName().Return("id").Once()
	// utils.go::getType
	mockColumn1.EXPECT().GetType().Return("integer").Once()
	// postgres.go::TypeInteger
	mockColumn1.EXPECT().GetAutoIncrement().Return(true).Once()
	// postgres.go::ModifyDefault
	mockColumn1.EXPECT().GetDefault().Return(nil).Once()
	// postgres.go::ModifyIncrement
	mockBlueprint.EXPECT().HasCommand("primary").Return(false).Once()
	mockColumn1.EXPECT().GetType().Return("integer").Once()
	// postgres.go::ModifyNullable
	mockColumn1.EXPECT().GetNullable().Return(false).Once()
	mockColumn1.EXPECT().GetOnUpdate().Return(nil).Once()
	mockColumn1.EXPECT().GetComment().Return("id").Once()
	mockColumn1.EXPECT().GetUnsigned().Return(true).Once()
	mockColumn1.EXPECT().GetAfter().Return("").Once()
	mockColumn1.EXPECT().IsFirst().Return(false).Once()

	// utils.go::getColumns
	mockColumn2.EXPECT().GetName().Return("name").Once()
	// utils.go::getType
	mockColumn2.EXPECT().GetType().Return("string").Once()
	// postgres.go::TypeString
	mockColumn2.EXPECT().GetLength().Return(100).Once()
	// postgres.go::ModifyDefault
	mockColumn2.EXPECT().GetDefault().Return(nil).Once()
	// postgres.go::ModifyIncrement
	mockColumn2.EXPECT().GetType().Return("string").Once()
	// postgres.go::ModifyNullable
	mockColumn2.EXPECT().GetNullable().Return(true).Once()
	mockColumn2.EXPECT().GetOnUpdate().Return(nil).Once()
	mockColumn2.EXPECT().GetComment().Return("name").Once()
	mockColumn2.EXPECT().GetUnsigned().Return(false).Once()
	mockColumn2.EXPECT().GetAfter().Return("").Once()
	mockColumn2.EXPECT().IsFirst().Return(false).Once()

	s.Equal("create table `goravel_users` (`id` int unsigned not null auto_increment primary key comment 'id', `name` varchar(100) null comment 'name', primary key using btree(`role_id`, `user_id`))",
		s.grammar.CompileCreate(mockBlueprint))
	s.True(primaryCommand.ShouldBeSkipped)
}

func (s *GrammarSuite) TestCompileDropAllTables() {
	s.Equal([]string{
		"SET FOREIGN_KEY_CHECKS=0;",
		"drop table `domain`, `email`",
		"SET FOREIGN_KEY_CHECKS=1;",
	}, s.grammar.CompileDropAllTables("goravel_", []contractsdriver.Table{
		{Name: "domain"},
		{Name: "email"},
	}))
}

func (s *GrammarSuite) TestCompileDropAllViews() {
	s.Equal([]string{
		"drop view `domain`, `email`",
	}, s.grammar.CompileDropAllViews("goravel_", []contractsdriver.View{
		{Name: "domain"},
		{Name: "email"},
	}))
}

func (s *GrammarSuite) TestCompileDropColumn() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockBlueprint.EXPECT().GetTableName().Return("users").Once()

	s.Equal([]string{
		"alter table `goravel_users` drop `id`, drop `name`",
	}, s.grammar.CompileDropColumn(mockBlueprint, &contractsdriver.Command{
		Columns: []string{"id", "name"},
	}))
}

func (s *GrammarSuite) TestCompileDropIfExists() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockBlueprint.EXPECT().GetTableName().Return("users").Once()

	s.Equal("drop table if exists `goravel_users`", s.grammar.CompileDropIfExists(mockBlueprint))
}

func (s *GrammarSuite) TestCompileForeign() {
	var mockBlueprint *mocksdriver.Blueprint

	beforeEach := func() {
		mockBlueprint = mocksdriver.NewBlueprint(s.T())
		mockBlueprint.EXPECT().GetTableName().Return("users").Once()
	}

	tests := []struct {
		name      string
		command   *contractsdriver.Command
		expectSql string
	}{
		{
			name: "with on delete and on update",
			command: &contractsdriver.Command{
				Index:      "fk_users_role_id",
				Columns:    []string{"role_id", "user_id"},
				On:         "roles",
				References: []string{"id", "user_id"},
				OnDelete:   "cascade",
				OnUpdate:   "restrict",
			},
			expectSql: "alter table `goravel_users` add constraint `fk_users_role_id` foreign key (`role_id`, `user_id`) references `goravel_roles` (`id`, `user_id`) on delete cascade on update restrict",
		},
		{
			name: "without on delete and on update",
			command: &contractsdriver.Command{
				Index:      "fk_users_role_id",
				Columns:    []string{"role_id", "user_id"},
				On:         "roles",
				References: []string{"id", "user_id"},
			},
			expectSql: "alter table `goravel_users` add constraint `fk_users_role_id` foreign key (`role_id`, `user_id`) references `goravel_roles` (`id`, `user_id`)",
		},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			beforeEach()

			sql := s.grammar.CompileForeign(mockBlueprint, test.command)
			s.Equal(test.expectSql, sql)
		})
	}
}

func (s *GrammarSuite) TestCompileIndex() {
	var mockBlueprint *mocksdriver.Blueprint

	beforeEach := func() {
		mockBlueprint = mocksdriver.NewBlueprint(s.T())
		mockBlueprint.EXPECT().GetTableName().Return("users").Once()
	}

	tests := []struct {
		name      string
		command   *contractsdriver.Command
		expectSql string
	}{
		{
			name: "with Algorithm",
			command: &contractsdriver.Command{
				Index:     "fk_users_role_id",
				Columns:   []string{"role_id", "user_id"},
				Algorithm: "btree",
			},
			expectSql: "alter table `goravel_users` add index `fk_users_role_id` using btree(`role_id`, `user_id`)",
		},
		{
			name: "without Algorithm",
			command: &contractsdriver.Command{
				Index:   "fk_users_role_id",
				Columns: []string{"role_id", "user_id"},
			},
			expectSql: "alter table `goravel_users` add index `fk_users_role_id`(`role_id`, `user_id`)",
		},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			beforeEach()

			sql := s.grammar.CompileIndex(mockBlueprint, test.command)
			s.Equal(test.expectSql, sql)
		})
	}
}

func (s *GrammarSuite) TestCompileJsonColumnsUpdate() {
	tests := []struct {
		name           string
		values         map[string]any
		expectedValues []map[string]any
		hasError       bool
	}{
		{
			name: "invalid values",
			values: map[string]any{"data->invalid": map[string]any{
				"value": func() {},
			}},
			hasError: true,
		},
		{
			name:   "update single json column",
			values: map[string]any{"data->details": "details value"},
			expectedValues: []map[string]any{
				{"data": databasedb.Raw("json_set(?,?,?)", databasedb.Raw("`data`"), `$."details"`, "details value")},
			},
		},
		{
			name:   "update single json column(with nested path)",
			values: map[string]any{"data->details->subdetails[0]": "subdetails value"},
			expectedValues: []map[string]any{
				{"data": databasedb.Raw("json_set(?,?,?)", databasedb.Raw("`data`"), `$."details"."subdetails"[0]`, "subdetails value")},
			},
		},
		{
			name:   "update multiple json columns",
			values: map[string]any{"data->details": "details value", "data->info": "info value"},
			expectedValues: []map[string]any{
				{
					"data": databasedb.Raw(
						"json_set(?,?,?)",
						databasedb.Raw(
							"json_set(?,?,?)",
							databasedb.Raw("`data`"),
							`$."details"`, "details value",
						),
						`$."info"`, "info value",
					),
				},
				{
					"data": databasedb.Raw(
						"json_set(?,?,?)",
						databasedb.Raw(
							"json_set(?,?,?)",
							databasedb.Raw("`data`"),
							`$."info"`, "info value",
						),
						`$."details"`, "details value",
					),
				},
			},
		},
	}

	mockApp := mocksfoundation.NewApplication(s.T())

	originApp := App
	App = mockApp
	s.T().Cleanup(func() {
		App = originApp
	})

	for _, tt := range tests {
		s.Run(tt.name, func() {
			mockApp.EXPECT().GetJson().Return(json.New()).Once()
			actualValues, err := s.grammar.CompileJsonColumnsUpdate(tt.values)
			if tt.hasError {
				s.Error(err)
			} else {
				s.Subset(tt.expectedValues, []any{actualValues})
				s.NoError(err)
			}
		})
	}
}

func (s *GrammarSuite) TestCompileJsonContains() {
	tests := []struct {
		name          string
		column        string
		value         any
		isNot         bool
		expectedSql   string
		expectedValue []any
		hasError      bool
	}{
		{
			name:     "invalid value type",
			column:   "data->details",
			value:    func() {},
			hasError: true,
		},
		{
			name:          "single path with single value",
			column:        "data->details",
			value:         "value1",
			expectedSql:   "json_contains(`data`, ?, '$.\"details\"')",
			expectedValue: []any{`"value1"`},
		},
		{
			name:          "single path with multiple values",
			column:        "data->details",
			value:         []string{"value1", "value2"},
			expectedSql:   "json_contains(`data`, ?, '$.\"details\"')",
			expectedValue: []any{`["value1","value2"]`},
		},
		{
			name:          "nested path with single value",
			column:        "data->details->subdetails[0]",
			value:         "value1",
			expectedSql:   "json_contains(`data`, ?, '$.\"details\".\"subdetails\"[0]')",
			expectedValue: []any{`"value1"`},
		},
		{
			name:          "nested path with multiple values",
			column:        "data->details[0]->subdetails",
			value:         []string{"value1", "value2"},
			expectedSql:   "json_contains(`data`, ?, '$.\"details\"[0].\"subdetails\"')",
			expectedValue: []any{`["value1","value2"]`},
		},
		{
			name:          "with is not condition",
			column:        "data->details",
			value:         "value1",
			isNot:         true,
			expectedSql:   "not json_contains(`data`, ?, '$.\"details\"')",
			expectedValue: []any{`"value1"`},
		},
	}

	mockApp := mocksfoundation.NewApplication(s.T())

	originApp := App
	App = mockApp
	s.T().Cleanup(func() {
		App = originApp
	})

	for _, tt := range tests {
		s.Run(tt.name, func() {
			mockApp.EXPECT().GetJson().Return(json.New()).Once()
			actualSql, actualValue, err := s.grammar.CompileJsonContains(tt.column, tt.value, tt.isNot)
			if tt.hasError {
				s.Error(err)
			} else {
				s.Equal(tt.expectedSql, actualSql)
				s.Equal(tt.expectedValue, actualValue)
				s.NoError(err)
			}
		})
	}
}

func (s *GrammarSuite) TestCompileJsonContainKey() {
	tests := []struct {
		name        string
		column      string
		isNot       bool
		expectedSql string
	}{
		{
			name:        "single path",
			column:      "data->details",
			expectedSql: "ifnull(json_contains_path(`data`, 'one', '$.\"details\"'), 0)",
		},
		{
			name:        "single path with is not",
			column:      "data->details",
			isNot:       true,
			expectedSql: "not ifnull(json_contains_path(`data`, 'one', '$.\"details\"'), 0)",
		},
		{
			name:        "nested path",
			column:      "data->details->subdetails",
			expectedSql: "ifnull(json_contains_path(`data`, 'one', '$.\"details\".\"subdetails\"'), 0)",
		},
		{
			name:        "nested path with array index",
			column:      "data->details[0]->subdetails",
			expectedSql: "ifnull(json_contains_path(`data`, 'one', '$.\"details\"[0].\"subdetails\"'), 0)",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.expectedSql, s.grammar.CompileJsonContainsKey(tt.column, tt.isNot))
		})
	}
}

func (s *GrammarSuite) TestCompileJsonLength() {
	tests := []struct {
		name        string
		column      string
		expectedSql string
	}{
		{
			name:        "single path",
			column:      "data->details",
			expectedSql: "json_length(`data`, '$.\"details\"')",
		},
		{
			name:        "nested path",
			column:      "data->details->subdetails",
			expectedSql: "json_length(`data`, '$.\"details\".\"subdetails\"')",
		},
		{
			name:        "nested path with array index",
			column:      "data->details[0]->subdetails",
			expectedSql: "json_length(`data`, '$.\"details\"[0].\"subdetails\"')",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.expectedSql, s.grammar.CompileJsonLength(tt.column))
		})
	}
}

func (s *GrammarSuite) TestCompileJsonValues() {
	tests := []struct {
		name     string
		args     []any
		expected []any
	}{
		{
			name:     "number values",
			args:     []any{1},
			expected: []any{1},
		},
		{
			name:     "number values",
			args:     []any{[]int{1, 2, 3}},
			expected: []any{[]any{1, 2, 3}},
		},
		{
			name:     "string values",
			args:     []any{"value1", "value2", "value3"},
			expected: []any{"value1", "value2", "value3"},
		},
		{
			name:     "boolean values",
			args:     []any{true, false},
			expected: []any{"true", "false"},
		},
		{
			name:     "pointer values",
			args:     []any{convert.Pointer(true)},
			expected: []any{"true"},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.expected, s.grammar.CompileJsonValues(tt.args...))
		})
	}
}

func (s *GrammarSuite) TestCompilePrimary() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockBlueprint.EXPECT().GetTableName().Return("users").Once()

	s.Equal("alter table `goravel_users` add primary key (`role_id`, `user_id`)", s.grammar.CompilePrimary(mockBlueprint, &contractsdriver.Command{
		Columns: []string{"role_id", "user_id"},
	}))
}

func (s *GrammarSuite) TestCompileKey() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockBlueprint.EXPECT().GetTableName().Return("users").Twice()

	s.Equal("alter table `goravel_users` add unique `index`(`id`, `name`)", s.grammar.compileKey(mockBlueprint, &contractsdriver.Command{
		Algorithm: "",
		Columns:   []string{"id", "name"},
		Index:     "index",
	}, "unique"))

	s.Equal("alter table `goravel_users` add unique `index` using btree(`id`, `name`)", s.grammar.compileKey(mockBlueprint, &contractsdriver.Command{
		Algorithm: "btree",
		Columns:   []string{"id", "name"},
		Index:     "index",
	}, "unique"))
}

func (s *GrammarSuite) TestCompileRenameColumn() {
	var (
		mockBlueprint = mocksdriver.NewBlueprint(s.T())
		mockColumn    = mocksdriver.NewColumnDefinition(s.T())
	)

	// Test case: MySQL version is greater than or equal to 8.0.3
	mockBlueprint.EXPECT().GetTableName().Return("users").Once()
	s.grammar.version = "8.0.3"
	sql, err := s.grammar.CompileRenameColumn(mockBlueprint, &contractsdriver.Command{
		Column: mockColumn,
		From:   "before",
		To:     "after",
	}, nil)

	s.NoError(err)
	s.Equal("alter table `goravel_users` rename column `before` to `after`", sql)

	// Test case: MySQL version is less than 8.0.3
	mockBlueprint.EXPECT().GetTableName().Return("users").Once()
	s.grammar.version = "5.7.2"
	sql, err = s.grammar.CompileRenameColumn(mockBlueprint, &contractsdriver.Command{
		Column: mockColumn,
		From:   "before",
		To:     "after",
	}, []contractsdriver.Column{
		{
			Collation: "utf8mb4_unicode_ci",
			Comment:   "test comment",
			Default:   "'goravel'",
			Name:      "before",
			Nullable:  true,
			Type:      "varchar",
		},
	})

	s.NoError(err)
	s.Equal("alter table `goravel_users` change `before` `after` varchar collate `utf8mb4_unicode_ci` null default 'goravel' comment 'test comment'", sql)
}

func (s *GrammarSuite) TestGetColumns() {
	mockColumn1 := mocksdriver.NewColumnDefinition(s.T())
	mockColumn2 := mocksdriver.NewColumnDefinition(s.T())
	mockBlueprint := mocksdriver.NewBlueprint(s.T())

	mockBlueprint.EXPECT().GetAddedColumns().Return([]contractsdriver.ColumnDefinition{
		mockColumn1, mockColumn2,
	}).Once()
	mockBlueprint.EXPECT().HasCommand("primary").Return(false).Once()

	mockColumn1.EXPECT().GetName().Return("id").Once()
	mockColumn1.EXPECT().GetType().Return("integer").Twice()
	mockColumn1.EXPECT().GetDefault().Return(nil).Once()
	mockColumn1.EXPECT().GetNullable().Return(false).Once()
	mockColumn1.EXPECT().GetOnUpdate().Return(nil).Once()
	mockColumn1.EXPECT().GetAutoIncrement().Return(true).Once()
	mockColumn1.EXPECT().GetComment().Return("id").Once()
	mockColumn1.EXPECT().GetUnsigned().Return(true).Once()
	mockColumn1.EXPECT().GetAfter().Return("").Once()
	mockColumn1.EXPECT().IsFirst().Return(false).Once()

	mockColumn2.EXPECT().GetName().Return("name").Once()
	mockColumn2.EXPECT().GetType().Return("string").Twice()
	mockColumn2.EXPECT().GetDefault().Return("goravel").Twice()
	mockColumn2.EXPECT().GetNullable().Return(true).Once()
	mockColumn2.EXPECT().GetOnUpdate().Return(nil).Once()
	mockColumn2.EXPECT().GetLength().Return(10).Once()
	mockColumn2.EXPECT().GetComment().Return("name").Once()
	mockColumn2.EXPECT().GetUnsigned().Return(false).Once()
	mockColumn2.EXPECT().GetAfter().Return("").Once()
	mockColumn2.EXPECT().IsFirst().Return(false).Once()

	s.Equal([]string{"`id` int unsigned not null auto_increment primary key comment 'id'", "`name` varchar(10) null default 'goravel' comment 'name'"}, s.grammar.getColumns(mockBlueprint))
}

func (s *GrammarSuite) TestModifyDefault() {
	var (
		mockBlueprint *mocksdriver.Blueprint
		mockColumn    *mocksdriver.ColumnDefinition
	)

	tests := []struct {
		name      string
		setup     func()
		expectSql string
	}{
		{
			name: "without change and default is nil",
			setup: func() {
				mockColumn.EXPECT().GetDefault().Return(nil).Once()
			},
		},
		{
			name: "without change and default is not nil",
			setup: func() {
				mockColumn.EXPECT().GetDefault().Return("goravel").Twice()
			},
			expectSql: " default 'goravel'",
		},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			mockBlueprint = mocksdriver.NewBlueprint(s.T())
			mockColumn = mocksdriver.NewColumnDefinition(s.T())

			test.setup()

			sql := s.grammar.ModifyDefault(mockBlueprint, mockColumn)

			s.Equal(test.expectSql, sql)
		})
	}
}

func (s *GrammarSuite) TestModifyNullable() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetNullable().Return(true).Once()

	s.Equal(" null", s.grammar.ModifyNullable(mockBlueprint, mockColumn))

	mockColumn.EXPECT().GetNullable().Return(false).Once()

	s.Equal(" not null", s.grammar.ModifyNullable(mockBlueprint, mockColumn))
}

func (s *GrammarSuite) TestModifyIncrement() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())

	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockBlueprint.EXPECT().HasCommand("primary").Return(false).Once()
	mockColumn.EXPECT().GetType().Return("bigInteger").Once()
	mockColumn.EXPECT().GetAutoIncrement().Return(true).Once()

	s.Equal(" auto_increment primary key", s.grammar.ModifyIncrement(mockBlueprint, mockColumn))
}

func (s *GrammarSuite) TestModifyOnUpdate() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetOnUpdate().Return(schema.Expression("CURRENT_TIMESTAMP")).Once()

	s.Equal(" on update CURRENT_TIMESTAMP", s.grammar.ModifyOnUpdate(mockBlueprint, mockColumn))

	mockColumn.EXPECT().GetOnUpdate().Return("CURRENT_TIMESTAMP").Once()
	s.Equal(" on update CURRENT_TIMESTAMP", s.grammar.ModifyOnUpdate(mockBlueprint, mockColumn))

	mockColumn.EXPECT().GetOnUpdate().Return("").Once()
	s.Empty(s.grammar.ModifyOnUpdate(mockBlueprint, mockColumn))

	mockColumn.EXPECT().GetOnUpdate().Return(nil).Once()
	s.Empty(s.grammar.ModifyOnUpdate(mockBlueprint, mockColumn))
}

func (s *GrammarSuite) TestTableComment() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockBlueprint.EXPECT().GetTableName().Return("users").Once()

	s.Equal("alter table `goravel_users` comment = 'It''s a table comment'", s.grammar.CompileTableComment(mockBlueprint, &contractsdriver.Command{
		Value: "It's a table comment",
	}))
}

func (s *GrammarSuite) TestTypeBoolean() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())

	s.Equal("tinyint(1)", s.grammar.TypeBoolean(mockColumn))
}

func (s *GrammarSuite) TestTypeDateTime() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetPrecision().Return(3).Once()
	mockColumn.EXPECT().GetUseCurrent().Return(true).Once()
	mockColumn.EXPECT().Default(schema.Expression("CURRENT_TIMESTAMP(3)")).Return(mockColumn).Once()
	mockColumn.EXPECT().GetUseCurrentOnUpdate().Return(true).Once()
	mockColumn.EXPECT().OnUpdate(schema.Expression("CURRENT_TIMESTAMP(3)")).Return(mockColumn).Once()
	s.Equal("datetime(3)", s.grammar.TypeDateTime(mockColumn))

	mockColumn = mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetPrecision().Return(0).Once()
	mockColumn.EXPECT().GetUseCurrent().Return(false).Once()
	mockColumn.EXPECT().GetUseCurrentOnUpdate().Return(false).Once()
	s.Equal("datetime", s.grammar.TypeDateTime(mockColumn))
}

func (s *GrammarSuite) TestTypeDecimal() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetTotal().Return(4).Once()
	mockColumn.EXPECT().GetPlaces().Return(2).Once()

	s.Equal("decimal(4, 2)", s.grammar.TypeDecimal(mockColumn))
}

func (s *GrammarSuite) TestTypeEnum() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetAllowed().Return([]any{"a", "b"}).Once()

	s.Equal(`enum('a', 'b')`, s.grammar.TypeEnum(mockColumn))
}

func (s *GrammarSuite) TestTypeFloat() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetPrecision().Return(0).Once()

	s.Equal("float", s.grammar.TypeFloat(mockColumn))

	mockColumn.EXPECT().GetPrecision().Return(2).Once()

	s.Equal("float(2)", s.grammar.TypeFloat(mockColumn))
}

func (s *GrammarSuite) TestTypeString() {
	mockColumn1 := mocksdriver.NewColumnDefinition(s.T())
	mockColumn1.EXPECT().GetLength().Return(100).Once()

	s.Equal("varchar(100)", s.grammar.TypeString(mockColumn1))

	mockColumn2 := mocksdriver.NewColumnDefinition(s.T())
	mockColumn2.EXPECT().GetLength().Return(0).Once()

	s.Equal("varchar(255)", s.grammar.TypeString(mockColumn2))
}

func (s *GrammarSuite) TestTypeTimestamp() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetPrecision().Return(3).Once()
	mockColumn.EXPECT().GetUseCurrent().Return(true).Once()
	mockColumn.EXPECT().Default(schema.Expression("CURRENT_TIMESTAMP(3)")).Return(mockColumn).Once()
	mockColumn.EXPECT().GetUseCurrentOnUpdate().Return(true).Once()
	mockColumn.EXPECT().OnUpdate(schema.Expression("CURRENT_TIMESTAMP(3)")).Return(mockColumn).Once()
	s.Equal("timestamp(3)", s.grammar.TypeTimestamp(mockColumn))

	mockColumn = mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetPrecision().Return(0).Once()
	mockColumn.EXPECT().GetUseCurrent().Return(false).Once()
	mockColumn.EXPECT().GetUseCurrentOnUpdate().Return(false).Once()
	s.Equal("timestamp", s.grammar.TypeTimestamp(mockColumn))
}

func (s *GrammarSuite) TestTypeUuid() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())

	s.Equal("char(36)", s.grammar.TypeUuid(mockColumn))
}

func TestGetCommandByName(t *testing.T) {
	commands := []*contractsdriver.Command{
		{Name: "create"},
		{Name: "update"},
		{Name: "delete"},
	}

	// Test case: Command exists
	result := getCommandByName(commands, "update")
	assert.NotNil(t, result)
	assert.Equal(t, "update", result.Name)

	// Test case: Command does not exist
	result = getCommandByName(commands, "drop")
	assert.Nil(t, result)
}

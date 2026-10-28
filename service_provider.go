package mysql

import (
	"github.com/gradii/framework/contracts/binding"
	"github.com/gradii/framework/contracts/foundation"
	"github.com/gradii/framework/errors"
	mysqlerrors "github.com/gradii/mysql/errors"
)

const (
	Binding = "gradii.mysql"
	Name    = "MySQL"
)

var App foundation.Application

type ServiceProvider struct{}

func (r *ServiceProvider) Relationship() binding.Relationship {
	return binding.Relationship{
		Bindings:     []string{Binding},
		Dependencies: []string{binding.Config},
		ProvideFor:   []string{binding.DB, binding.Orm, binding.Schema},
	}
}

func (r *ServiceProvider) Register(app foundation.Application) {
	App = app
	app.BindWith(Binding, func(app foundation.Application, parameters map[string]any) (any, error) {
		config := app.MakeConfig()
		if config == nil {
			return nil, errors.ConfigFacadeNotSet.SetModule(Name)
		}
		connection, ok := parameters["connection"].(string)
		if !ok || connection == "" {
			return nil, mysqlerrors.ConnectionNotSet
		}
		return NewMysql(config, nil, nil, connection), nil
	})
}

func (r *ServiceProvider) Boot(foundation.Application) {}

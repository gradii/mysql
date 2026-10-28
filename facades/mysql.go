package facades

import (
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/mysql"
	mysqlerrors "github.com/gradii/mysql/errors"
)

func Mysql(connection string) (driver.Driver, error) {
	if mysql.App == nil {
		return nil, mysqlerrors.ServiceProviderNotRegistered
	}
	instance, err := mysql.App.MakeWith(mysql.Binding, map[string]any{"connection": connection})
	if err != nil {
		return nil, err
	}
	return instance.(*mysql.Mysql), nil
}

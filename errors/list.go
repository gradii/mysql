package errors

import frameworkerrors "github.com/gradii/framework/errors"

var (
	DockerNotSupported           = frameworkerrors.New("Docker helpers are not supported by the MySQL runtime driver")
	ServiceProviderNotRegistered = frameworkerrors.New("please register the MySQL service provider")
	ConnectionNotSet             = frameworkerrors.New("MySQL connection name is required")
	InvalidPort                  = frameworkerrors.New("invalid MySQL port: %d")
	SchemaColumnNotFound         = frameworkerrors.New("Column %s does not exist")
)

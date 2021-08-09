package main

import "github.com/kelseyhightower/envconfig"

type brokerConfig struct {
	BrokerUsername            string `envconfig:"broker_username" required:"true"`
	BrokerPassword            string `envconfig:"broker_password" required:"true"`
	StorageGridTenantUsername string `envconfig:"storagegrid_tenant_username" required:"true"`
	StorageGridTenantPassword string `envconfig:"storagegrid_tenant_password" required:"true"`
	StorageGridAdminURL       string `envconfig:"storagegrid_admin_url" required:"true"`
	StorageGridSkipSSLCheck   bool   `envconfig:"storagegrid_skip_ssl_check" default:"false"`
	StorageGridAccountID      string `envconfig:"storagegrid_account_id" required:"true"`
	S3Endpoint                string `envconfig:"s3_endpoint" required:"true"`
	S3Region                  string `envconfig:"s3_region" default:"us-east-1"`
	S3ForcePathStyle          bool   `envconfig:"s3_path_style" default:"true"`
	LogLevel                  string `envconfig:"log_level" default:"INFO"`
	Port                      string `envconfig:"port" default:"3000"`
	DocsURL                   string `envconfig:"docsurl" default:""`
}

func brokerConfigLoad() (brokerConfig, error) {
	var config brokerConfig
	err := envconfig.Process("", &config)
	if err != nil {
		return brokerConfig{}, err
	}

	return config, nil
}

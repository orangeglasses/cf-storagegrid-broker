package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"
)

func main() {
	var logLevels = map[string]lager.LogLevel{
		"DEBUG": lager.DEBUG,
		"INFO":  lager.INFO,
		"ERROR": lager.ERROR,
		"FATAL": lager.FATAL,
	}

	config, err := brokerConfigLoad()
	if err != nil {
		panic(err)
	}

	brokerCredentials := brokerapi.BrokerCredentials{
		Username: config.BrokerUsername,
		Password: config.BrokerPassword,
	}

	services, err := CatalogLoad("./catalog.json")
	if err != nil {
		panic(err)
	}

	for i, _ := range services {
		services[i].Metadata.DocumentationUrl = config.DocsURL
	}

	logger := lager.NewLogger("cf-storagegrid-broker")
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, logLevels[config.LogLevel]))

	sgClient, err := NewStorageGridClient(config.StorageGridAdminURL, config.StorageGridSkipSSLCheck, config.StorageGridAccountID, config.StorageGridTenantUsername, config.StorageGridTenantPassword)
	if err != nil {
		log.Fatal(err)
	}

	s3Client, err := NewS3Client(sgClient, config.S3Region, config.S3Endpoint, config.S3ForcePathStyle, config.StorageGridSkipSSLCheck)
	if err != nil {
		log.Fatal(err)
	}

	serviceBroker := &broker{
		services: services,
		env:      config,
		sgClient: sgClient,
		s3client: s3Client,
	}

	brokerHandler := brokerapi.New(serviceBroker, logger, brokerCredentials)
	fmt.Println("Starting service")
	http.Handle("/", brokerHandler)
	http.ListenAndServe(":"+config.Port, nil)
}

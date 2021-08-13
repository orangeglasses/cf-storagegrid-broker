package main

import (
	"encoding/json"
	"io/ioutil"

	"github.com/pivotal-cf/brokerapi"
)

func CatalogLoad(catalogFilePath string) ([]brokerapi.Service, error) {
	var services []brokerapi.Service

	inBuf, err := ioutil.ReadFile(catalogFilePath)
	if err != nil {
		return []brokerapi.Service{}, err
	}

	err = json.Unmarshal(inBuf, &services)
	if err != nil {
		return []brokerapi.Service{}, err
	}

	services[0].Metadata.DocumentationUrl = ""
	return services, nil
}

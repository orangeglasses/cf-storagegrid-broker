package main

import (
	"encoding/json"
	"fmt"
)

func getBucketsFromGroup(group sgGroup) ([]string, error) {
	var (
		pol         interface{}
		bucketNames []string
	)

	err := json.Unmarshal(group.Policies, &pol)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse policy for group %s", group.DisplayName)
	}

	st := pol.(map[string]interface{})["s3"].(map[string]interface{})["Statement"].([]interface{})[0] //ugly but it works :)
	for _, res := range st.(map[string]interface{})["Resource"].([]interface{}) {
		var name string
		fmt.Sscanf(res.(string), "urn:sgws:s3:::%s", &name)
		bucketNames = append(bucketNames, name)
	}

	return bucketNames, nil
}

package main

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func getFriendlyNameFromBucketName(bucketName string) string {
	return bucketName[0 : len(bucketName)-33]
}

func generatNewFullName(friendlyName string) string {
	if friendlyName == "" {
		friendlyName = "bucket"
	}

	return fmt.Sprintf("%s-%s", friendlyName, strings.ReplaceAll(uuid.New().String(), "-", ""))
}

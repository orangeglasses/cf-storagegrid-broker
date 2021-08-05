package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
)

//"urn:sgws:s3:::{{.BucketName}}"

func GenerateS3Policy(instanceID string, buckets []Bucket) (string, error) {
	t := template.Must(template.ParseFiles("group_policy.json.tmpl"))

	var (
		BucketRsrcs  []string
		ObjectsRsrcs []string
	)

	for _, bucket := range buckets {
		BucketRsrcs = append(BucketRsrcs, fmt.Sprintf("urn:sgws:s3:::%s", bucket.name))
		ObjectsRsrcs = append(ObjectsRsrcs, fmt.Sprintf("urn:sgws:s3:::%s/*", bucket.name))
	}

	brBytes, err := json.Marshal(BucketRsrcs)
	if err != nil {
		return "", fmt.Errorf("Error generating policy: %s", err)
	}

	orBytes, err := json.Marshal(ObjectsRsrcs)
	if err != nil {
		return "", fmt.Errorf("Error generating policy: %s", err)
	}

	data := struct {
		InstanceID      string
		BucketResources string
		ObjectResources string
	}{
		InstanceID:      instanceID,
		BucketResources: string(brBytes),
		ObjectResources: string(orBytes),
	}

	var b bytes.Buffer
	t.Execute(&b, data)

	return b.String(), nil
}

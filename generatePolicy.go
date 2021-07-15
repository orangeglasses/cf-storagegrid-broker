package main

import (
	"bytes"
	"text/template"
)

func GenerateS3Policy(BucketName string) (string, error) {
	t := template.Must(template.ParseFiles("group_policy.json.tmpl"))

	data := struct {
		BucketName string
	}{
		BucketName: BucketName,
	}

	var b bytes.Buffer
	t.Execute(&b, data)

	return b.String(), nil

}

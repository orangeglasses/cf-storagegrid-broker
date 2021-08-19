package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pivotal-cf/brokerapi/domain/apiresponses"
)

type ProvisionParamsBucket struct {
	Name       string `json:"name"`
	Region     string `json:"region"`
	Versioning bool   `json:"versioning"`
}

type ProvisionParameters struct {
	Buckets []ProvisionParamsBucket `json:"buckets"`
}

func (b *broker) getBucketsFromGroup(group sgGroup) (map[string]Bucket, error) {
	var (
		pol interface{}
	)

	buckets := make(map[string]Bucket)

	err := json.Unmarshal(group.Policies, &pol)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse policy for group %s", group.DisplayName)
	}

	if pol == nil {
		return buckets, nil
	}

	st := pol.(map[string]interface{})["s3"].(map[string]interface{})["Statement"].([]interface{})[0] //ugly but it works :)
	for _, res := range st.(map[string]interface{})["Resource"].([]interface{}) {
		var name string
		fmt.Sscanf(res.(string), "urn:sgws:s3:::%s", &name)

		region, err := b.s3client.GetBucketRegion(name)
		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				if awsErr.Code() == s3.ErrCodeNoSuchBucket { //do not error out when a bucket is not found. However, keep it on the list. The delete action will ignore the error too and after that the bucket will be deleted from the policy
					log.Printf("Bucket in policy but not found in S3: %s", name)
				} else {
					return nil, fmt.Errorf("Unable to determine region for bucket %s. %s", name, err)
				}
			} else {
				return nil, fmt.Errorf("Unable to determine region for bucket %s. %s", name, err)
			}
		}

		versioning, _ := b.s3client.GetBucketVersioning(name)

		buckets[getFriendlyNameFromBucketName(name)] = Bucket{
			name:       name,
			region:     region,
			versioning: versioning,
		}
	}

	return buckets, nil
}

func (b *broker) getRequestedBucketsFromParams(rawParams json.RawMessage) (map[string]Bucket, error) {
	returnBuckets := make(map[string]Bucket)

	var params ProvisionParameters
	err := json.Unmarshal(rawParams, &params)
	if err != nil {
		return nil, apiresponses.ErrRawParamsInvalid
	}

	for _, reqBucket := range params.Buckets {
		var friendlyPart string
		if len(reqBucket.Name) > 27 {
			friendlyPart = reqBucket.Name[0:27]
		} else {
			friendlyPart = reqBucket.Name
		}

		region := b.s3client.Region
		if reqBucket.Region != "" {
			region = reqBucket.Region
		}

		bucket := Bucket{
			name:       friendlyPart,
			region:     region,
			versioning: reqBucket.Versioning,
		}
		returnBuckets[bucket.name] = bucket
	}

	return returnBuckets, nil
}

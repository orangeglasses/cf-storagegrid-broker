package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
	"github.com/pivotal-cf/brokerapi"
	"github.com/pivotal-cf/brokerapi/domain"
	"github.com/pivotal-cf/brokerapi/domain/apiresponses"
)

type broker struct {
	services []brokerapi.Service
	env      brokerConfig
	sgClient *storageGridClient
	s3client *s3client
}

type CredBucket struct {
	URI    string `json:"uri"`
	Name   string `json:"name"`
	Bucket string `json:"bucket"`
	Region string `json:"region"`
}

type Credentials struct {
	InsecureSkipVerify bool         `json:"insecure_skip_verify"`
	AccessKeyID        string       `json:"access_key_id"`
	SecretAccessKey    string       `json:"secret_access_key"`
	Buckets            []CredBucket `json:"bucket"`
	Endpoint           string       `json:"endpoint"`
	PathStyleAccess    bool         `json:"pathStyleAccess"`
}

type ProvisionParamsBucket struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}

type ProvisionParameters struct {
	Buckets []ProvisionParamsBucket `json:"buckets"`
}

type Bucket struct {
	name   string
	region string
}

func (b *broker) Services(context context.Context) ([]brokerapi.Service, error) {
	return b.services, nil
}

func (b *broker) Provision(context context.Context, instanceID string, details domain.ProvisionDetails, asyncAllowed bool) (domain.ProvisionedServiceSpec, error) {
	var createBuckets []Bucket

	groupName := strings.ReplaceAll(instanceID, "-", "")

	if details.RawParameters != nil && len(details.RawParameters) > 0 {
		var params ProvisionParameters
		err := json.Unmarshal(details.RawParameters, &params)
		if err != nil {
			return domain.ProvisionedServiceSpec{}, apiresponses.ErrRawParamsInvalid
		}

		for _, reqBucket := range params.Buckets {
			var friendlyPart string
			if len(reqBucket.Name) > 27 {
				friendlyPart = reqBucket.Name[0:27]
			} else {
				friendlyPart = reqBucket.Name
			}

			bucket := Bucket{
				name:   fmt.Sprintf("%s-%s", friendlyPart, strings.ReplaceAll(uuid.New().String(), "-", "")),
				region: reqBucket.Region,
			}
			createBuckets = append(createBuckets, bucket)
		}

	} else {
		bucket := Bucket{
			name:   strings.ReplaceAll(uuid.New().String(), "-", ""),
			region: b.s3client.Region,
		}
		createBuckets = append(createBuckets, bucket)
	}

	policy, err := GenerateS3Policy(groupName, createBuckets)
	if err != nil {
		return domain.ProvisionedServiceSpec{}, fmt.Errorf("Generating policy failed: %s", err)
	}

	//1. Create a group with appropriate policy first
	log.Printf("Creating group with name: %s", groupName)
	grp, err := b.sgClient.CreateGroup(groupName, policy)
	if err != nil {
		if ae, ok := err.(apiError); ok {
			if ae.statusCode == http.StatusConflict {
				return domain.ProvisionedServiceSpec{}, apiresponses.ErrInstanceAlreadyExists
			}
		}
		return domain.ProvisionedServiceSpec{}, fmt.Errorf("Group Creation Failed: %s", err)
	}

	//2. Create a buckets
	for _, bucket := range createBuckets {
		log.Printf("Creating bucket with name: %s", bucket.name)
		_, err = b.s3client.CreateBucket(bucket.name, bucket.region)
		if err != nil {
			b.sgClient.DeleteGroup(grp.ID)
			return domain.ProvisionedServiceSpec{}, fmt.Errorf("Creating bucket failed with error: %s", err)
		}
	}

	spec := domain.ProvisionedServiceSpec{
		IsAsync:       false,
		AlreadyExists: false,
		DashboardURL:  "", //TODO set Dashboard URL??
		OperationData: "",
	}

	return spec, nil
}

func (b *broker) GetInstance(ctx context.Context, instanceID string) (domain.GetInstanceDetailsSpec, error) {
	return domain.GetInstanceDetailsSpec{}, fmt.Errorf("Instances are not retrievable")
}

func (b *broker) Deprovision(context context.Context, instanceID string, details brokerapi.DeprovisionDetails, asyncAllowed bool) (domain.DeprovisionServiceSpec, error) {
	instance := strings.ReplaceAll(instanceID, "-", "")
	log.Printf("Deleting instance with name: %s", instance)

	//1. Get Group
	grp, err := b.sgClient.GetGroupByName(instance)
	if err != nil {
		return domain.DeprovisionServiceSpec{}, nil
	}

	//2. get buckets names from group olicy
	bucketNames, err := getBucketsFromGroup(grp)
	fmt.Println(bucketNames)
	if err != nil {
		return domain.DeprovisionServiceSpec{}, fmt.Errorf("Error getting buckets for group %s: %s", grp.DisplayName, err)
	}

	//3. Delete buckets
	for _, bucketName := range bucketNames {
		log.Printf("Deleting bucket %s\n", bucketName)
		if _, err := b.s3client.DeleteBucket(bucketName); err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				if awsErr.Code() != s3.ErrCodeNoSuchBucket {
					return domain.DeprovisionServiceSpec{}, err
				}
			}
		}
	}

	//4. Delete group
	if err := b.sgClient.DeleteGroup(grp.ID); err != nil {
		return domain.DeprovisionServiceSpec{}, err
	}

	return domain.DeprovisionServiceSpec{}, nil
}

func (b *broker) Bind(context context.Context, instanceID, bindingID string, details domain.BindDetails, asyncAllowed bool) (domain.Binding, error) {
	instance := strings.ReplaceAll(instanceID, "-", "")
	userName := strings.ReplaceAll(bindingID, "-", "")

	log.Printf("Creating binding %s for instance %s", userName, instance)

	//1a retrieve group
	group, err := b.sgClient.GetGroupByName(instance)
	if err != nil {
		return domain.Binding{}, fmt.Errorf("Error retrieving group: %s", err)
	}

	//2 retrieve bucket names and create buckets array
	bucketNames, err := getBucketsFromGroup(group)
	if err != nil {
		return domain.Binding{}, fmt.Errorf("Unable to retrieve buckets for instance %s", instance)
	}
	fmt.Println(bucketNames)

	//3 Create storage grid user in group
	userFullName := fmt.Sprintf("Binding to app GUID: %s", details.AppGUID)
	if details.AppGUID == "" {
		userFullName = fmt.Sprintf("Service Key in space GUID: %s", details.BindResource.SpaceGuid)
	}

	user, err := b.sgClient.CreateUser(userName, userFullName, []string{group.ID})
	if err != nil {
		if ae, ok := err.(apiError); ok {
			if ae.statusCode != 409 {
				return domain.Binding{}, err
			} else {
				user, _ = b.sgClient.GetUserByName(userName)
			}
		} else {
			return domain.Binding{}, err
		}
	}

	//4 generate permanent creds for user
	creds, err := b.sgClient.CreateS3CredsForUser(user.ID)

	//5. return bind info
	var buckets []CredBucket
	for _, name := range bucketNames {
		r, err := b.s3client.GetBucketRegion(name)
		if err != nil {
			return domain.Binding{}, fmt.Errorf("Unable to determine region for bucket %s", name)
		}

		friendlyName := name[0 : len(name)-33]
		b := CredBucket{
			URI:    fmt.Sprintf("s3://%s:%s@%s/%s", url.QueryEscape(creds.AccessKey), url.QueryEscape(creds.SecretAccessKey), b.s3client.Endpoint, name),
			Name:   friendlyName,
			Bucket: name,
			Region: r,
		}
		buckets = append(buckets, b)
	}

	binding := domain.Binding{
		Credentials: Credentials{
			InsecureSkipVerify: b.env.StorageGridSkipSSLCheck,
			AccessKeyID:        creds.AccessKey,
			SecretAccessKey:    creds.SecretAccessKey,
			Buckets:            buckets,
			Endpoint:           b.s3client.Endpoint,
			PathStyleAccess:    b.env.S3ForcePathStyle,
		},
	}

	return binding, nil
}

func (b *broker) GetBinding(ctx context.Context, instanceID, bindingID string) (domain.GetBindingSpec, error) {
	return domain.GetBindingSpec{}, fmt.Errorf("Bindings are not retrievable")
}

func (b *broker) Unbind(context context.Context, instanceID, bindingID string, details domain.UnbindDetails, asyncAllowed bool) (domain.UnbindSpec, error) {
	userName := strings.ReplaceAll(bindingID, "-", "")

	log.Printf("Deleting binding %s", userName)

	//1. delete user
	user, err := b.sgClient.GetUserByName(userName)
	if err != nil {
		return domain.UnbindSpec{}, err
	}

	err = b.sgClient.DeleteUser(user.ID)

	return domain.UnbindSpec{}, err
}

func (b *broker) Update(context context.Context, instanceID string, details brokerapi.UpdateDetails, asyncAllowed bool) (domain.UpdateServiceSpec, error) {
	return domain.UpdateServiceSpec{}, nil
}

func (b *broker) LastOperation(context context.Context, instanceID string, details domain.PollDetails) (brokerapi.LastOperation, error) {
	return brokerapi.LastOperation{}, nil
}

func (b *broker) LastBindingOperation(ctx context.Context, instanceID, bindingID string, details domain.PollDetails) (domain.LastOperation, error) {
	return domain.LastOperation{}, nil
}

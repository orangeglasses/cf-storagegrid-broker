package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
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

type Credentials struct {
	URI                string `json:"uri"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	AccessKeyID        string `json:"access_key_id"`
	SecretAccessKey    string `json:"secret_access_key"`
	Region             string `json:"region"`
	Bucket             string `json:"bucket"`
	Endpoint           string `json:"endpoint"`
}

type ProvisionParameters struct {
	Region string `json:"region"`
}

func (b *broker) Services(context context.Context) ([]brokerapi.Service, error) {
	return b.services, nil
}

func (b *broker) Provision(context context.Context, instanceID string, details domain.ProvisionDetails, asyncAllowed bool) (domain.ProvisionedServiceSpec, error) {
	//we won't support setting region from parameter for now.
	/*var params ProvisionParameters
	err := json.Unmarshal(details.RawParameters, &params)
	if err != nil {
		return domain.ProvisionedServiceSpec{}, err
	}*/

	bucketName := strings.ReplaceAll(instanceID, "-", "")
	region := b.s3client.Region

	policy, err := GenerateS3Policy(bucketName)
	if err != nil {
		return domain.ProvisionedServiceSpec{}, fmt.Errorf("Generating policy failed: %s", err)
	}

	//1. Create a group with appropriate policy first
	log.Printf("Creating group with name: %s", bucketName)
	grp, err := b.sgClient.CreateGroup(bucketName, policy)
	if err != nil {
		if ae, ok := err.(apiError); ok {
			if ae.statusCode == http.StatusConflict {
				return domain.ProvisionedServiceSpec{}, apiresponses.ErrInstanceAlreadyExists
			}
		}
		return domain.ProvisionedServiceSpec{}, fmt.Errorf("Group Creation Failed: %s", err)
	}

	//2. Create a bucket
	log.Printf("Creating bucket with name: %s", bucketName)
	_, err = b.s3client.CreateBucket(bucketName, region)
	if err != nil {
		b.sgClient.DeleteGroup(grp.ID)
		return domain.ProvisionedServiceSpec{}, fmt.Errorf("Creating bucket failed with error: %s", err)
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
	bucketName := strings.ReplaceAll(instanceID, "-", "")
	log.Printf("Deleting instance with name: %s", bucketName)

	//1. Delete group
	grp, err := b.sgClient.GetGroupByName(bucketName)
	if err != nil {
		return domain.DeprovisionServiceSpec{}, nil
	}

	if err := b.sgClient.DeleteGroup(grp.ID); err != nil {
		return domain.DeprovisionServiceSpec{}, err
	}

	//2. Delete bucket
	if _, err := b.s3client.DeleteBucket(bucketName); err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == s3.ErrCodeNoSuchBucket {
				return domain.DeprovisionServiceSpec{}, nil
			}
		}

		return domain.DeprovisionServiceSpec{}, err
	}

	return domain.DeprovisionServiceSpec{}, nil
}

func (b *broker) Bind(context context.Context, instanceID, bindingID string, details domain.BindDetails, asyncAllowed bool) (domain.Binding, error) {
	bucketName := strings.ReplaceAll(instanceID, "-", "")
	userName := strings.ReplaceAll(bindingID, "-", "")

	log.Printf("Creating binding %s for bucket %s", userName, bucketName)

	//1a retrieve groupd id
	group, err := b.sgClient.GetGroupByName(bucketName)
	if err != nil {
		return domain.Binding{}, fmt.Errorf("Error retrieving groupID: %s", err)
	}

	//1b Create storage grid user
	user, err := b.sgClient.CreateUser(userName, fmt.Sprintf("Binding to app GUID: %s", details.AppGUID), []string{group.ID})
	if err != nil {
		return domain.Binding{}, nil
	}

	//2. generate permanent creds for user
	creds, err := b.sgClient.CreateS3CredsForUser(user.ID)

	//3. return bind info
	binding := domain.Binding{
		Credentials: Credentials{
			URI:                fmt.Sprintf("s3://%s:%s@%s/%s", url.QueryEscape(creds.AccessKey), url.QueryEscape(creds.SecretAccessKey), b.s3client.Endpoint, instanceID),
			InsecureSkipVerify: b.env.StorageGridSkipSSLCheck,
			AccessKeyID:        creds.AccessKey,
			SecretAccessKey:    creds.SecretAccessKey,
			Region:             b.s3client.Region,
			Bucket:             instanceID,
			Endpoint:           b.s3client.Endpoint,
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

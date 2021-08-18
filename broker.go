package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

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
	URI        string `json:"uri"`
	Name       string `json:"name"`
	Bucket     string `json:"bucket"`
	Region     string `json:"region"`
	Versioning bool   `json:"versioning"`
}

type Credentials struct {
	InsecureSkipVerify bool         `json:"insecure_skip_verify"`
	AccessKeyID        string       `json:"access_key_id"`
	SecretAccessKey    string       `json:"secret_access_key"`
	Buckets            []CredBucket `json:"buckets"`
	Endpoint           string       `json:"endpoint"`
	PathStyleAccess    bool         `json:"pathStyleAccess"`
}

type Bucket struct {
	name       string
	region     string
	versioning bool
}

func (b *broker) Services(context context.Context) ([]brokerapi.Service, error) {
	return b.services, nil
}

func (b *broker) Provision(context context.Context, instanceID string, details domain.ProvisionDetails, asyncAllowed bool) (domain.ProvisionedServiceSpec, error) {
	createBuckets := make(map[string]Bucket)
	var err error

	groupName := strings.ReplaceAll(instanceID, "-", "")

	if details.RawParameters != nil && len(details.RawParameters) > 0 {
		createBuckets, err = b.getRequestedBucketsFromParams(details.RawParameters)
		if err != nil {
			return domain.ProvisionedServiceSpec{}, err
		}
		for key, bckt := range createBuckets {
			bckt.name = generatNewFullName(key)
			createBuckets[key] = bckt
		}
	} else {
		bucket := Bucket{
			name:   generatNewFullName(""),
			region: b.s3client.Region,
		}
		createBuckets[bucket.name] = bucket
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

	//2. Create buckets
	var createdBuckets []string
	for _, bucket := range createBuckets {
		log.Printf("Creating bucket with name: %s", bucket.name)
		_, err = b.s3client.CreateBucket(bucket.name, bucket.region)
		if err != nil {
			b.sgClient.DeleteGroup(grp.ID)

			for _, delBucket := range createdBuckets {
				b.s3client.DeleteBucket(delBucket)
			}
			return domain.ProvisionedServiceSpec{}, fmt.Errorf("Creating bucket failed with error: %s", err)
		}

		if bucket.versioning {
			err = b.s3client.EnableBucketVersioning(bucket.name)
			if err != nil {
				log.Printf("Enabling versioning on %s failed: %s", bucket.name, err)
			}
		}

		createdBuckets = append(createdBuckets, bucket.name)
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
		if sge, ok := err.(apiError); ok {
			if sge.statusCode == 404 {
				return domain.DeprovisionServiceSpec{}, brokerapi.ErrInstanceDoesNotExist
			} else {
				return domain.DeprovisionServiceSpec{}, fmt.Errorf("Error getting group from storageGrid: %s", err)
			}
		} else {
			return domain.DeprovisionServiceSpec{}, fmt.Errorf("Error getting group from storageGrid: %s", err)
		}
	}

	//2. get buckets names from group policy
	buckets, err := b.getBucketsFromGroup(grp)
	if err != nil {
		return domain.DeprovisionServiceSpec{}, fmt.Errorf("Error getting buckets for group %s: %s", grp.DisplayName, err)
	}

	//3. Delete buckets
	deletedBuckets, errs := b.deleteBuckets(buckets)

	if len(errs) > 0 {
		if len(deletedBuckets) > 0 {
			// if there were any buckets deleted we need to update the policy
			for friendlyName := range deletedBuckets {
				delete(buckets, friendlyName)
			}

			if len(buckets) == 0 {
				//If all buckets were deleted anyways despite the error we'll delete the group and exit successfully
				log.Printf("Deleting group %s\n", grp.DisplayName)
				if err := b.sgClient.DeleteGroup(grp.ID); err != nil {
					return domain.DeprovisionServiceSpec{}, err
				}
				return domain.DeprovisionServiceSpec{}, nil
			}

			//if not all buckets were deleted we'll have to generate a new policy containing the remaining buckets
			policy, err := GenerateS3Policy(instance, buckets)
			if err != nil {
				return domain.DeprovisionServiceSpec{}, err
			}

			_, err = b.sgClient.UpdateGroupPolicy(grp, policy)
			if err != nil {
				log.Printf("Error updating group policy: %s\n", err)
			}
		}

		var errorString string
		for _, e := range errs {
			errorString = fmt.Sprintf("%s -- %s", errorString, e)
		}

		return domain.DeprovisionServiceSpec{}, fmt.Errorf("Errors while deleting service instance: %s", errorString)
	}

	//4. Delete group
	log.Printf("Deleting group %s\n", grp.DisplayName)
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
	buckets, err := b.getBucketsFromGroup(group)
	if err != nil {
		return domain.Binding{}, fmt.Errorf("Unable to retrieve buckets for instance %s", instance)
	}

	//3 Create storage grid user in group
	userFullName := fmt.Sprintf("Binding to app GUID: %s", details.AppGUID)
	if details.AppGUID == "" {
		userFullName = fmt.Sprintf("Service Key")
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
	credBuckets := []CredBucket{}
	for friendlyName, bckt := range buckets {
		cb := CredBucket{
			URI:        fmt.Sprintf("s3://%s:%s@%s/%s", url.QueryEscape(creds.AccessKey), url.QueryEscape(creds.SecretAccessKey), b.s3client.Endpoint, bckt.name),
			Name:       friendlyName,
			Bucket:     bckt.name,
			Region:     bckt.region,
			Versioning: bckt.versioning,
		}
		credBuckets = append(credBuckets, cb)
	}

	binding := domain.Binding{
		Credentials: Credentials{
			InsecureSkipVerify: b.env.StorageGridSkipSSLCheck,
			AccessKeyID:        creds.AccessKey,
			SecretAccessKey:    creds.SecretAccessKey,
			Buckets:            credBuckets,
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

func (b *broker) Update(context context.Context, instanceID string, details domain.UpdateDetails, asyncAllowed bool) (domain.UpdateServiceSpec, error) {
	// get current buckets
	instance := strings.ReplaceAll(instanceID, "-", "")
	group, err := b.sgClient.GetGroupByName(instance)
	if err != nil {
		return domain.UpdateServiceSpec{}, fmt.Errorf("Error retrieving group: %s", err)
	}

	currentBuckets, err := b.getBucketsFromGroup(group)
	if err != nil {
		return domain.UpdateServiceSpec{}, fmt.Errorf("Unable to retrieve buckets for instance %s", instance)
	}

	// get reqbuckets
	requestedBuckets := make(map[string]Bucket)
	if details.RawParameters != nil && len(details.RawParameters) > 0 {
		requestedBuckets, err = b.getRequestedBucketsFromParams(details.RawParameters)
	} else {
		return domain.UpdateServiceSpec{}, apiresponses.ErrRawParamsInvalid
	}

	// figure out which ones to delete. range over current, if you can't find it in req then on delete list
	deleteList := make(map[string]Bucket)
	for key, bckt := range currentBuckets {
		if _, ok := requestedBuckets[key]; !ok {
			deleteList[key] = bckt
		}
	}

	// figure out which ones to create. range over req, if you can't find it in current then on create list
	createList := make(map[string]Bucket)
	for friendlyName, bckt := range requestedBuckets {
		if _, ok := currentBuckets[friendlyName]; !ok {
			//if it's not in current bucket list we'll generate a new name and add it to the create list
			bckt.name = generatNewFullName(friendlyName)
			createList[friendlyName] = bckt
		}
	}

	//delete buckets and remove deleted buckets from currentlist
	deletedBuckets, delErr := b.deleteBuckets(deleteList)
	for friendlyName := range deletedBuckets {
		delete(currentBuckets, friendlyName)
	}

	//create buckets and add created buckets to current list
	var createErr []error
	for friendlyName, bucket := range createList {
		log.Printf("Creating bucket with name: %s", bucket.name)
		_, err = b.s3client.CreateBucket(bucket.name, bucket.region)
		if err != nil {
			createErr = append(createErr, err)
		} else {
			currentBuckets[friendlyName] = bucket
		}
	}

	policy, err := GenerateS3Policy(instance, currentBuckets)
	if err != nil {
		return domain.UpdateServiceSpec{}, fmt.Errorf("Generating policy failed: %s", err)
	}
	_, err = b.s3client.SgClient.UpdateGroupPolicy(group, policy)
	if err != nil {
		return domain.UpdateServiceSpec{}, err
	}

	var errString string
	if len(createErr) > 0 || len(delErr) > 0 {
		for _, e := range createErr {
			errString = fmt.Sprintf("%s-%s", errString, e.Error())
		}

		for _, e := range delErr {
			errString = fmt.Sprintf("%s-%s", errString, e.Error())
		}

		return domain.UpdateServiceSpec{}, fmt.Errorf("Errors occured while updating service: %s", errString)

	}

	spec := domain.UpdateServiceSpec{
		IsAsync:       false,
		DashboardURL:  "",
		OperationData: "",
	}
	return spec, nil
}

func (b *broker) LastOperation(context context.Context, instanceID string, details domain.PollDetails) (brokerapi.LastOperation, error) {
	return brokerapi.LastOperation{}, nil
}

func (b *broker) LastBindingOperation(ctx context.Context, instanceID, bindingID string, details domain.PollDetails) (domain.LastOperation, error) {
	return domain.LastOperation{}, nil
}

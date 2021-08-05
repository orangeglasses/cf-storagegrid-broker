package main

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type s3client struct {
	SgClient       *storageGridClient
	Client         *s3.S3
	Creds          *credentials.Credentials
	CredExpire     time.Time
	CredMutex      sync.Mutex
	Endpoint       string
	ForcePathStyle bool
	SkipSSL        bool
	Region         string
}

func NewS3Client(sgClient *storageGridClient, region, endpoint string, pathStyle, skipssl bool) (*s3client, error) {
	return &s3client{
		SgClient:       sgClient,
		Client:         nil,
		Creds:          nil,
		CredExpire:     time.Now(),
		CredMutex:      sync.Mutex{},
		Endpoint:       endpoint,
		ForcePathStyle: pathStyle,
		SkipSSL:        skipssl,
		Region:         region,
	}, nil
}

func (c *s3client) login() error {
	c.CredMutex.Lock()
	if time.Now().Before(c.CredExpire) {
		c.CredMutex.Unlock()
		return nil
	}
	defer c.CredMutex.Unlock()

	expireTime := time.Now().Add(15 * time.Minute)
	creds, err := c.SgClient.CreateS3CredsForCurrentUser(expireTime)
	c.Creds = credentials.NewStaticCredentials(creds.AccessKey, creds.SecretAccessKey, "")
	if err != nil {
		return err
	}
	c.CredExpire = expireTime

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: c.SkipSSL},
	}

	httpClient := http.Client{
		Transport: tr,
		Timeout:   5 * time.Second,
	}

	sess, err := session.NewSession(&aws.Config{
		HTTPClient:       &httpClient,
		Credentials:      c.Creds,
		Endpoint:         aws.String(c.Endpoint),
		Region:           aws.String(c.Region),
		S3ForcePathStyle: aws.Bool(c.ForcePathStyle),
	},
	)

	if err != nil {
		return err
	}

	c.Client = s3.New(sess)
	return nil
}

func (c *s3client) CreateBucket(bucketName, region string) (*s3.CreateBucketOutput, error) {
	err := c.login()
	if err != nil {
		return nil, err
	}

	useRegion := region
	if region == "" {
		useRegion = c.Region
	}

	cbOutput, err := c.Client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(useRegion),
		},
	})

	if err != nil {
		return nil, err
	}

	return cbOutput, nil
}

func (c *s3client) DeleteBucket(bucketName string) (*s3.DeleteBucketOutput, error) {
	err := c.login()
	if err != nil {
		return nil, err
	}

	dbOutput, err := c.Client.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})

	if err != nil {
		return nil, err
	}

	return dbOutput, nil
}

func (c *s3client) GetBucketRegion(bucketName string) (string, error) {
	err := c.login()
	if err != nil {
		return "", err
	}

	res, err := c.Client.GetBucketLocation(&s3.GetBucketLocationInput{Bucket: aws.String(bucketName)})
	if err != nil {
		return "", err
	}

	if *res == (s3.GetBucketLocationOutput{}) {
		return "us-east-1", nil
	}

	return *res.LocationConstraint, nil
}

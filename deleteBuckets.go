package main

import (
	"log"
	"sync"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
)

type bucketDeleteStatus struct {
	friendlyName string
	name         string
	region       string
	deleted      bool
	err          error
}

func (b *broker) deleteBuckets(buckets map[string]Bucket) (map[string]Bucket, []error) {
	var (
		delWG sync.WaitGroup
	)

	deletedBuckets := make(map[string]Bucket)
	statusChan := make(chan bucketDeleteStatus)

	delWG.Add(len(buckets))

	go func() {
		delWG.Wait()
		close(statusChan)
	}()

	//Delete buckets concurrently. Sequentially is too slow for more than about 6 buckets.
	for friendlyName, bucket := range buckets {

		go func(friendlyName string, bucket Bucket) {
			defer delWG.Done()

			log.Printf("Deleting bucket %s\n", bucket.name)

			status := bucketDeleteStatus{
				friendlyName: friendlyName,
				region:       bucket.region,
				name:         bucket.name,
				deleted:      true,
				err:          nil,
			}
			if _, err := b.s3client.DeleteBucket(bucket.name); err != nil {
				if awsErr, ok := err.(awserr.Error); ok {
					if awsErr.Code() == s3.ErrCodeNoSuchBucket {
						statusChan <- status
						return
					}
				}

				status.deleted = false
				status.err = err
				statusChan <- status
				return
			}

			statusChan <- status
			return
		}(friendlyName, bucket)
	}

	var errors []error
	for status := range statusChan {
		if status.err != nil {
			errors = append(errors, status.err)
		}

		if status.deleted {
			deletedBuckets[status.friendlyName] = Bucket{
				name:   status.name,
				region: status.region,
			}
		}
	}

	if len(errors) > 0 {
		return deletedBuckets, errors
	}

	return deletedBuckets, nil
}

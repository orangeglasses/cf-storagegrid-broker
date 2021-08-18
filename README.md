# cf-storagegrid-broker
This is an Open Servicebroker API compatible broker for NetApp StorageGrid. For now only S3 is supported (no support for swift in this boker).

# deploying
This broker is designed to run on cloudfoundry. Just "cf push" it and the run "cf create-service-broker". Some environment variables need to be set. see manifest-example.yml. 

# usage
Once the broker is deployed and registered and service access is enabled you'll be able to create buckets on-demand.

## create a single bucket
Just run: ```cf create-service s3-bucket standard mybucket```. Obviously "mybucket" can be replaced with any name you like.

## create multiple buckets
you can also create multiple buckets at once. This could be necessary if, for example, your app requires more than one bucket but only can use one creentials pair.

Before you create the service you'll need to create a json file which looks similar to this:
```
{
    "buckets": [
        { 
            "name": "bucket1",
            "region": "lab"
        },
        { 
            "name": "bucket2",
            "region": "us-east-1"
        },
        { 
            "name": "anotherbucket"
            "versioning": true
        }
    ]
}
  ```

- The bucket name is just the friendly name. The broker will add a unique ID to it before it creates the bucket. 
- The region parameter is optional. If you don't use the region parameter the region specified in the "S3_REGION" environment variable will be used.
- The versioning parameter is optional as well. The default is "false" which means versioning is disabled. If you need versioning enabled on your bucket set "versioning" to true.


## add/delete buckets to/from existing service
It is possible to add or delete buckets to/from an existing service instance. Pleae note that deletion is only possible if the bucket is empty. If you originially deployed the buckets using the json as explained above you can simply update you json file to represent the state of the new state of the service. Meaning that if you delete buckets from the json they will also be deleted from the service. If you add buckets to the json they'll of course be created. 

After updating your json file run: ```cf update-service mybucket -c <json file>```

**please note: you'll have to re-create any service-keys and re-bind any apps after updating the service!**

## using the buckets
To get access to the buckets you either bind the service to an app like so: ``cf bind-service myapp mybucket```. Or you can create a service-key if you want to access to bucket from outside cloud foundry: ```cf create-service-key mybucket mykey```

## deleting a service instance
**Only empty buckets can be deleted!**

To delete a service instance: ```cf delete-service mybucket```

To delete a single bucket from a multi-bucket service instance please see "add/delete buckets to/from existing service" above.

If you tried deleting a non-empty bucket an error message will be shown. To regain access to the bucket just create a new service-key or bind the service to an app an delete any data inside the bucket. Then try deleting again.



# cf-storagegrid-broker
This is an Open Servicebroker API compatible broker for NetApp StorageGrid. For now only S3 is supported (no support for swift in this boker).

# deploying
This borker is designed to run on cloudfoundry. Just "cf push" it and the run "cf create-service-broker". Some environment variables need to be set. see manifest-example.yml.
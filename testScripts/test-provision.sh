curl http://broker:broker@localhost:3000/v2/service_instances/c41cab85-d688-4dc5-bc5e-5264262207ab?accepts_incomplete=true -d '{
  "service_id": "dd0f1727-8d9d-4cf3-8c9d-631ce5d9e789",
  "plan_id": "a31fec23-a86b-4d3a-87d2-f44b620b9c04",
  "context": {
    "platform": "cloudfoundry"
  },
  "parameters": {
    "buckets": [
    { "name": "bucket1",
    "region": "lab"
    },
    { "name": "bucket2",
    "region": "us-east-1"
    },
    { "name": "bucket3",
    "versioning": true
    }
  ]},
  "organization_guid": "c0eda3a0-a224-4985-9e50-6c6b9a4a9115",
  "space_guid": "21284559-5dfb-4e72-98fc-16cc92b2012e"
}' -X PUT -H "X-Broker-API-Version: 2.16" -H "Content-Type: application/json"
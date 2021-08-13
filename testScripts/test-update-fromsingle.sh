curl http://broker:broker@localhost:3000/v2/service_instances/90349a10-7309-4ddc-999b-ef7d851c55c0?accepts_incomplete=true -d '{
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
    { "name": "bucket4"
    }
  ]},
  "previous_values": {
      "plan_id": "a31fec23-a86b-4d3a-87d2-f44b620b9c04"
  }
}' -X PATCH -H "X-Broker-API-Version: 2.16" -H "Content-Type: application/json"
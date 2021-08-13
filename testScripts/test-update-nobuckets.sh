curl http://broker:broker@localhost:3000/v2/service_instances/c41cab85-d688-4dc5-bc5e-5264262207ab?accepts_incomplete=true -d '{
  "service_id": "dd0f1727-8d9d-4cf3-8c9d-631ce5d9e789",
  "plan_id": "a31fec23-a86b-4d3a-87d2-f44b620b9c04",
  "context": {
    "platform": "cloudfoundry"
  },
  "parameters": {
    "buckets": [    
  ]},
  "previous_values": {
      "plan_id": "a31fec23-a86b-4d3a-87d2-f44b620b9c04"
  }
}' -X PATCH -H "X-Broker-API-Version: 2.16" -H "Content-Type: application/json"
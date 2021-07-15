curl http://broker:broker@localhost:3000/v2/service_instances/c41cab85-d688-4dc5-bc5e-5264262207ab/service_bindings/c63606aa-54d4-4037-93e8-56da7000ba5e?accepts_incomplete=true -d '{
  "context": {
    "platform": "cloudfoundry"  
  },
  "service_id": "dd0f1727-8d9d-4cf3-8c9d-631ce5d9e789",
  "plan_id": "a31fec23-a86b-4d3a-87d2-f44b620b9c04",
  "bind_resource": {
    "app_guid": "a31fec23-a86b-4d3a-87d2-f44b620b9c04"
  },
  "parameters": {
    "parameter1-name-here": 1,
    "parameter2-name-here": "parameter2-value-here"
  }
}' -X PUT -H "X-Broker-API-Version: 2.16" -H "Content-Type: application/json"
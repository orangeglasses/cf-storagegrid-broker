package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type adminAPI struct {
	s *storageGridClient
	b *broker
}

func (a adminAPI) FindGroupForBucketHandler(w http.ResponseWriter, r *http.Request) {
	bucketName := r.URL.Query().Get("bucket")
	grpName, err := a.FindGroupForBucket(bucketName, "")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(grpName)
}

func (a adminAPI) FindGroupForBucket(bucketName, lastGroupURN string) (string, error) {
	groupsResp, err := a.s.DoApiRequest("GET", fmt.Sprintf("org/groups?page?limit=100&marker=%v&includeMarker=false", lastGroupURN), nil, http.StatusOK)
	if err != nil {
		return "", err
	}

	if len(groupsResp.Data) == 0 {
		return "", fmt.Errorf("Service ID not found")
	}

	var grps []sgGroup
	err = json.Unmarshal(groupsResp.Data, &grps)

	var lastGrpUrn string
	for _, grp := range grps {
		bckts, err := a.b.getBucketsFromGroup(grp)
		if err != nil {
			continue
		}

		for _, bckt := range bckts {
			if bckt.name == bucketName {
				return grp.DisplayName, nil
			}
		}

		lastGrpUrn = grp.GroupURN
	}

	return a.FindGroupForBucket(bucketName, lastGrpUrn) //if still not found repeat request for the next page using the last groupURN as a marker. recursive! yeah!
}

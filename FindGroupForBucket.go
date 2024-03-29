package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
)

type adminAPI struct {
	s        *storageGridClient
	b        *broker
	username string
	password string
}

func (a adminAPI) FindGroupForBucketHandler(w http.ResponseWriter, r *http.Request) {
	username, password, ok := r.BasicAuth()
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "Please provide the broker username and password")
		return
	}

	usernameHash := sha256.Sum256([]byte(username))
	passwordHash := sha256.Sum256([]byte(password))
	expectedUsernameHash := sha256.Sum256([]byte(a.username))
	expectedPasswordHash := sha256.Sum256([]byte(a.password))

	usernameMatch := (subtle.ConstantTimeCompare(usernameHash[:], expectedUsernameHash[:]) == 1)
	passwordMatch := (subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1)

	if !usernameMatch || !passwordMatch {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	bucketName := r.URL.Query().Get("bucket")
	if bucketName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	grpName, err := a.FindGroupForBucket(bucketName, "")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(grpName)
}

func (a adminAPI) FindGroupForBucket(bucketName, lastGroupURN string) (string, error) {
	reqURL := "org/groups?type=local&limit=100"
	if lastGroupURN != "" {
		reqURL = fmt.Sprintf("org/groups?type=local&limit=100&marker=%v&includeMarker=false", lastGroupURN)
	}

	groupsResp, err := a.s.DoApiRequest("GET", reqURL, nil, http.StatusOK)
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

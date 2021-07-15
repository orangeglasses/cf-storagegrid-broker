package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type storageGridClient struct {
	httpClient      *http.Client
	URL             url.URL
	accountID       string
	username        string
	password        string
	token           string
	tokenExpireTime time.Time
	tokenMutex      sync.Mutex
}

type apiResponse struct {
	ResponseTime string          `json:"responseTime"`
	Status       string          `json:"status"`
	ApiVersion   string          `json:"apiVersion"`
	Deprecated   bool            `json:"deprecated"`
	Data         json.RawMessage `json:"data"`
}

type apiError struct {
	err        string
	body       string
	statusCode int
}

type sgUser struct {
	ID         string   `json:"id"`
	AccountId  string   `json:"accountId"`
	FullName   string   `json:"fullName"`
	UniqueName string   `json:"uniqueName"`
	UserURN    string   `json:"userURN"`
	Federated  bool     `json:"federated"`
	MemberOf   []string `json:"memberOf"`
	Disable    bool     `json:"disable"`
}

type sgGroup struct {
	ID          string          `json:"id,omitempty"`
	AccountId   string          `json:"accountId,omitempty"`
	DisplayName string          `json:"displayName"`
	UniqueName  string          `json:"uniqueName"`
	GroupURN    string          `json:"groupURN,omitempty"`
	Federated   bool            `json:"federated,omitempty"`
	Policies    json.RawMessage `json:"policies,omitempty"`
}

type sgS3Cred struct {
	ID              string `json:"id"`
	AccountId       string `json:"accountId"`
	DisplayName     string `json:"displayName"`
	UserURN         string `json:"userURN"`
	UserUUID        string `json:"userUUID"`
	Expires         string `json:"expires"`
	AccessKey       string `json:"accessKey"`
	SecretAccessKey string `json:"secretAccessKey"`
}

func (e apiError) Error() string {
	return fmt.Sprintf("%s. return code: %v. return body: %s", e.err, e.statusCode, e.body)
}

// Creates a new storageGrid client
func NewStorageGridClient(urlString string, skipssl bool, accountID, username, password string) (*storageGridClient, error) {
	urlParsed, err := url.Parse(urlString)
	if err != nil {
		return nil, fmt.Errorf("Error parsing storageGrid URL: %v", err.Error())
	}

	if urlParsed.Path == "" {
		urlParsed.Path = "/api/v3"
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipssl},
	}

	httpClient := http.Client{Transport: tr}

	sgClient := storageGridClient{
		httpClient:      &httpClient,
		URL:             *urlParsed,
		accountID:       accountID,
		username:        username,
		password:        password,
		token:           "",
		tokenExpireTime: time.Time{},
		tokenMutex:      sync.Mutex{},
	}

	return &sgClient, nil
}

// Logs into storageGrid when token is expired, saves the token and stores the expiration time
func (s *storageGridClient) login() error {
	//Set a lock to make this concurrency safe
	s.tokenMutex.Lock()
	defer s.tokenMutex.Unlock()

	//If token is not expired just exit succesfully
	if !s.tokenExpireTime.Before(time.Now()) {
		return nil
	}

	authInfo := struct {
		AccountID string `json:"accountId"`
		Username  string `json:"username"`
		Password  string `json:"password"`
		Cookie    bool   `json:"cookie"`
		CsrfToken bool   `json:"csrfToken"`
	}{
		AccountID: s.accountID,
		Username:  s.username,
		Password:  s.password,
		Cookie:    false,
		CsrfToken: false,
	}

	var authResp apiResponse

	reqBody, err := json.Marshal(authInfo)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Post(fmt.Sprintf("%s/authorize", s.URL.String()), "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("Error logging in to storageGrid: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bdy, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("Login failed: %s", bdy)
	}

	err = json.NewDecoder(resp.Body).Decode(&authResp)
	if err != nil {
		return fmt.Errorf("Error decoding response: %s", err)
	}

	err = json.Unmarshal(authResp.Data, &s.token)
	if err != nil {
		return fmt.Errorf("Error parsing token")
	}

	expTime := resp.Header.Get("Expires")
	s.tokenExpireTime, err = time.Parse("Mon, 2 Jan 2006 15:04:05 GMT", expTime)
	if err != nil {
		return fmt.Errorf("Error parsing token expiration time: %s", err)
	}
	return nil
}

//Does API request against storageGrid API.
func (s *storageGridClient) DoApiRequest(method, path string, body []byte, checkForCode int) (apiResponse, error) {
	var apiResp apiResponse

	req, err := http.NewRequest(method, fmt.Sprintf("%s/%s", s.URL.String(), path), bytes.NewReader(body))
	if err != nil {
		return apiResp, fmt.Errorf("Error creating request: %s", err)
	}

	if err := s.login(); err != nil {
		return apiResp, fmt.Errorf("Error logging in: %s", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", s.token))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return apiResp, fmt.Errorf("Error doing http request: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != checkForCode {
		bdy, _ := ioutil.ReadAll(resp.Body)
		return apiResp, apiError{
			err:        fmt.Sprintf("HTTP request returned error"),
			body:       string(bdy),
			statusCode: resp.StatusCode,
		}
	}

	err = json.NewDecoder(resp.Body).Decode(&apiResp)
	if err != nil {
		if err.Error() == "EOF" {
			return apiResp, nil
		} else {
			return apiResp, fmt.Errorf("Error decoding response: %s", err)
		}
	}

	return apiResp, nil
}

// Creates S3 creds for a user
func (s *storageGridClient) CreateS3CredsForUser(userid string) (sgS3Cred, error) {
	credsInfo := struct{}{}
	reqBody, _ := json.Marshal(credsInfo)

	createResp, err := s.DoApiRequest("POST", fmt.Sprintf("org/users/%s/s3-access-keys", userid), reqBody, http.StatusCreated)
	if err != nil {
		return sgS3Cred{}, err
	}

	var s3cred sgS3Cred
	err = json.Unmarshal(createResp.Data, &s3cred)
	if err != nil {
		return sgS3Cred{}, err
	}

	return s3cred, nil
}

func (s *storageGridClient) CreateS3CredsForCurrentUser(expires time.Time) (sgS3Cred, error) {
	credsInfo := struct {
		Expires string `json:"expires"`
	}{
		Expires: expires.Format("2006-01-02T15:04:05.000Z"),
	}
	reqBody, _ := json.Marshal(credsInfo)

	createResp, err := s.DoApiRequest("POST", fmt.Sprintf("org/users/current-user/s3-access-keys"), reqBody, http.StatusCreated)
	if err != nil {
		return sgS3Cred{}, err
	}

	var s3cred sgS3Cred
	err = json.Unmarshal(createResp.Data, &s3cred)
	if err != nil {
		return sgS3Cred{}, err
	}

	return s3cred, nil
}

func (s *storageGridClient) DeleteS3CredsForCurrentUser(id string) error {
	_, err := s.DoApiRequest("DELETE", fmt.Sprintf("org/users/current-user/s3-access-keys/%s", id), nil, http.StatusNoContent)
	if err != nil {
		return err
	}

	return nil
}

func (s *storageGridClient) GetUserByName(userName string) (sgUser, error) {
	userResp, err := s.DoApiRequest("GET", fmt.Sprintf("org/users/user/%s", userName), nil, http.StatusOK)
	if err != nil {
		return sgUser{}, err
	}

	var user sgUser

	err = json.Unmarshal(userResp.Data, &user)
	if err != nil {
		return sgUser{}, nil
	}

	return user, nil
}

func (s *storageGridClient) GetGroupByName(groupName string) (sgGroup, error) {
	groupResp, err := s.DoApiRequest("GET", fmt.Sprintf("org/groups/group/%s", groupName), nil, http.StatusOK)
	if err != nil {
		return sgGroup{}, err
	}

	var grp sgGroup
	err = json.Unmarshal(groupResp.Data, &grp)
	if err != nil {
		return sgGroup{}, fmt.Errorf("Error unmarshalling grp %s", err)
	}

	return grp, nil
}

func (s *storageGridClient) CreateUser(username, fullName string, groupIDs []string) (sgUser, error) {
	userInfo := struct {
		FullName   string   `json:"fullName"`
		MemberOf   []string `json:"memberOf"`
		Disable    bool     `json:"disable"`
		UniqueName string   `json:"uniqueName"`
	}{
		FullName:   fullName,
		MemberOf:   groupIDs,
		Disable:    false,
		UniqueName: fmt.Sprintf("user/%s", username),
	}
	reqBody, _ := json.Marshal(userInfo)

	UserCreateResp, err := s.DoApiRequest("POST", "/org/users", reqBody, http.StatusCreated)
	if err != nil {
		return sgUser{}, err
	}

	var user sgUser
	err = json.Unmarshal(UserCreateResp.Data, &user)
	if err != nil {
		return sgUser{}, err
	}

	return user, nil
}

func (s *storageGridClient) DeleteUser(userID string) error {
	_, err := s.DoApiRequest("DELETE", fmt.Sprintf("org/users/%s", userID), nil, http.StatusNoContent)
	if err != nil {
		return err
	}

	return nil
}

func (s *storageGridClient) CreateGroup(groupName, policy string) (sgGroup, error) {
	grp := sgGroup{
		DisplayName: groupName,
		UniqueName:  fmt.Sprintf("group/%s", groupName),
		Policies:    json.RawMessage(policy),
	}
	reqBody, err := json.Marshal(grp)
	if err != nil {
		return sgGroup{}, fmt.Errorf("Marshalling group object to json failed: %s", err)
	}

	result, err := s.DoApiRequest("POST", fmt.Sprintf("org/groups"), reqBody, http.StatusCreated)
	if err != nil {
		return sgGroup{}, err
	}

	err = json.Unmarshal(result.Data, &grp)
	if err != nil {
		return sgGroup{}, err
	}

	return grp, nil
}

func (s *storageGridClient) DeleteGroup(groupID string) error {
	_, err := s.DoApiRequest("DELETE", fmt.Sprintf("org/groups/%s", groupID), nil, http.StatusNoContent)
	if err != nil {
		return err
	}

	return nil
}

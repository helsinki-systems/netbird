package idp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/netbirdio/netbird/management/server/telemetry"

	log "github.com/sirupsen/logrus"
)

// IDP interface for Elma
type ElmaIDP struct {
	httpClient   ManagerHTTPClient
	elmaDomain   string
	clientID     string
	clientSecret string
	orgId        string
	jwtToken     string
	jwtExpire    int64
	appMetrics   telemetry.AppMetrics
}

// Configuration for Elma
type ElmaClientConfig struct {
	ElmaDomain   string
	OrgID        string
	ClientID     string
	ClientSecret string
}

type elmaTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64 `json:"expires_in"`
}

type elmaUserResponse struct {
	PublicID     string `json:"publicID"`
	Cn           string `json:"cn"`
	Mail         string `json:"mail"`
}

// Ensures we have a valid JWT and returns that
func (m *ElmaIDP) jwt() (string, error) {
	// Return cached token
	if m.jwtExpire != 0 && m.jwtExpire <= time.Now().Unix()-int64(5) && m.jwtToken != "" {
		return m.jwtToken, nil
	}

	p, err := json.Marshal(&map[string]string{
		"grant_type":    "client_credentials",
		"scope":         "view_user",
		"client_id":     m.clientID,
		"client_secret": m.clientSecret,
	})
	if err != nil {
		return "", err
	}
	payload := strings.NewReader(string(p))

	req, err := http.NewRequest("POST", "https://elma.id/oidc/token", payload)
	if err != nil {
		return "", err
	}

	req.Header.Add("content-type", "application/json")

	res, err := m.httpClient.Do(req)
	if err != nil {
		if m.appMetrics != nil {
			m.appMetrics.IDPMetrics().CountRequestError()
		}
		return "", err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", fmt.Errorf("unable to get token, statusCode %d", res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	var bodyJson elmaTokenResponse
	if err := json.Unmarshal(body, &bodyJson); err != nil {
		return "", err
	}

	m.jwtExpire = bodyJson.ExpiresIn + time.Now().Unix()
	m.jwtToken = bodyJson.AccessToken

	return m.jwtToken, nil
}

func NewElmaIDP(config ElmaClientConfig, appMetrics telemetry.AppMetrics) (*ElmaIDP, error) {
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	httpTransport.MaxIdleConns = 5

	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: httpTransport,
	}

	return &ElmaIDP{httpClient: httpClient, appMetrics: appMetrics, clientID: config.ClientID, clientSecret: config.ClientSecret, elmaDomain: config.ElmaDomain, orgId: config.OrgID, jwtToken: "", jwtExpire: 0}, nil
}

// GetUserDataByID is a Elma implementation of the IDP interface GetUserDataByID method
func (m *ElmaIDP) GetUserDataByID(ctx context.Context, userId string, appMetadata AppMetadata) (*UserData, error) {
	req, err := http.NewRequest("GET", "https://"+m.elmaDomain+"/elma/api/organization/"+m.orgId+"/user/"+userId, nil)
	if err != nil {
		return nil, err
	}
	jwt, err := m.jwt()
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+jwt)
	log.WithContext(ctx).Debug("requesting user info for idp manager")

	res, err := m.httpClient.Do(req)
	if err != nil {
		if m.appMetrics != nil {
			m.appMetrics.IDPMetrics().CountRequestError()
		}
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		if m.appMetrics != nil {
			m.appMetrics.IDPMetrics().CountRequestError()
		}
		return nil, fmt.Errorf("unable to ask for user info, statusCode %d", res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var bodyJson elmaUserResponse
	if err := json.Unmarshal(body, &bodyJson); err != nil {
		return nil, err
	}

	return &UserData{
		ID:    bodyJson.PublicID,
		Name:  bodyJson.Cn,
		Email: bodyJson.Mail,
	}, nil
}

// GetAccount is a Elma implementation of the IDP interface GetAccount method
func (m *ElmaIDP) GetAccount(ctx context.Context, accountId string) ([]*UserData, error) {
	p, err := json.Marshal(&map[string]map[string]int{
		"pagination":    map[string]int{
			"page": 0,
			"limit": 9001,
		},
	})
	if err != nil {
		return nil, err
	}
	payload := strings.NewReader(string(p))

	req, err := http.NewRequest("GET", "https://"+m.elmaDomain+"/elma/api/organization/"+m.orgId+"/user", payload)
	if err != nil {
		return nil, err
	}
	jwt, err := m.jwt()
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+jwt)
	log.WithContext(ctx).Debug("requesting account info for idp manager")

	res, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("unable to ask for account info, statusCode %d", res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var bodyJson []elmaUserResponse
	if err := json.Unmarshal(body, &bodyJson); err != nil {
		return nil, err
	}

	users := make([]*UserData, 0)
	for _, user := range bodyJson {
		users = append(users, &UserData{
			ID:    user.PublicID,
			Name:  user.Cn,
			Email: user.Mail,
			AppMetadata: AppMetadata{
				WTAccountID: accountId,
			},
		})
	}

	return users, nil
}

// UpdateUserAppMetadata is a mock implementation of the IDP interface UpdateUserAppMetadata method
func (m *ElmaIDP) UpdateUserAppMetadata(ctx context.Context, userId string, appMetadata AppMetadata) error {
	return nil
}

// GetAllAccounts is a mock implementation of the IDP interface GetAllAccounts method
func (m *ElmaIDP) GetAllAccounts(ctx context.Context) (map[string][]*UserData, error) {
	return nil, fmt.Errorf("method GetAllAccounts not implemented")
}

// CreateUser is a mock implementation of the IDP interface CreateUser method
func (m *ElmaIDP) CreateUser(ctx context.Context, email, name, accountID, invitedByEmail string) (*UserData, error) {
	return nil, fmt.Errorf("method CreateUser not implemented")
}

// GetUserByEmail is a mock implementation of the IDP interface GetUserByEmail method
func (m *ElmaIDP) GetUserByEmail(ctx context.Context, email string) ([]*UserData, error) {
	return nil, fmt.Errorf("method GetUserByEmail not implemented")
}

// InviteUserByID is a mock implementation of the IDP interface InviteUserByID method
func (m *ElmaIDP) InviteUserByID(ctx context.Context, userID string) error {
	return fmt.Errorf("method InviteUserByID not implemented")
}

// DeleteUser is a mock implementation of the IDP interface DeleteUser method
func (m *ElmaIDP) DeleteUser(ctx context.Context, userID string) error {
	return fmt.Errorf("method DeleteUser not implemented")
}

package vatusa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	ErrMissingEnv    = errors.New("missing environment variables")
	ErrInvalidStatus = errors.New("invalid status code returned")
)

type Client struct {
	url    string
	apiKey string
	hc     *http.Client
}

func NewClient(url, apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		url:    url,
		apiKey: apiKey,
		hc:     httpClient,
	}
}

func (c *Client) FetchRoster(ctx context.Context) ([]Controller, error) {
	req, err := c.newVatusaRequest(ctx, http.MethodGet, "/facility/ZAU/roster/both")
	if err != nil {
		return nil, err
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStatus, resp.Status)
	}

	var vatusaData Roster

	err = json.NewDecoder(resp.Body).Decode(&vatusaData)
	if err != nil {
		return nil, err
	}

	return vatusaData.Data, nil
}

func (c *Client) FetchACE(ctx context.Context) ([]int, error) {
	req, err := c.newVatusaRequest(ctx, http.MethodGet, "/user/roles/ZHQ/ACE")
	if err != nil {
		return nil, err
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStatus, resp.Status)
	}

	var aceData ACERoster

	err = json.NewDecoder(resp.Body).Decode(&aceData)
	if err != nil {
		return nil, err
	}

	cids := make([]int, 0, len(aceData.Data))
	for _, member := range aceData.Data {
		cids = append(cids, member.CID)
	}

	return cids, nil
}

func (c *Client) FetchFacilityInfo(ctx context.Context) (*FacilityInfo, error) {
	req, err := c.newVatusaRequest(ctx, http.MethodGet, "/facility/ZAU")
	if err != nil {
		return nil, err
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStatus, resp.Status)
	}

	var facilityData Facility

	err = json.NewDecoder(resp.Body).Decode(&facilityData)
	if err != nil {
		return nil, err
	}

	return &facilityData.Data.Facility.Info, nil
}

func (c *Client) newVatusaRequest(ctx context.Context, method, path string) (*http.Request, error) {
	if c.apiKey == "" || c.url == "" {
		return nil, ErrMissingEnv
	}

	req, err := http.NewRequestWithContext(
		ctx,
		method,
		fmt.Sprintf("%s%s?apikey=%s&t=%d", c.url, path, c.apiKey, time.Now().UnixMilli()),
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "zau-roster-sync/1.0")

	return req, nil
}

func (ct *CertSyncDate) UnmarshalJSON(byteSlice []byte) error {
	// The JSON data byte array is a quoted string, e.g., "2025-12-08 03:23:09"
	// We unmarshal it into a regular string variable.
	if string(byteSlice) == "null" {
		return nil
	}

	stringTime := string(bytes.Trim(byteSlice, "\""))

	timeTime, err := time.Parse("2006-01-02 15:04:05", stringTime)
	if err != nil {
		return err
	}

	*ct = CertSyncDate(timeTime)

	return nil
}

package vatusa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNewClientDefaultsToTimeout(t *testing.T) {
	client := NewClient("https://vatusa.example.com", "key", nil)

	if client.hc.Timeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", client.hc.Timeout)
	}
}

func TestFetchRosterBuildsURLAndDecodes(t *testing.T) {
	var gotPath, gotURLPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RawQuery
		gotURLPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"testing":false,"data":[{"cid":1234567,"fname":"John","lname":"Doe","email":"john@example.com","facility":"ZAU","rating":3,"membership":"home"}]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret", nil)

	controllers, err := client.FetchRoster(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotURLPath != "/facility/ZAU/roster/both" {
		t.Errorf("expected path /facility/ZAU/roster/both, got %q", gotURLPath)
	}

	if !strings.Contains(gotPath, "apikey=secret") {
		t.Errorf("expected apikey=secret in query, got %q", gotPath)
	}

	if !strings.Contains(gotPath, "&t=") {
		t.Errorf("expected timestamp param in query, got %q", gotPath)
	}

	if len(controllers) != 1 || controllers[0].CID != 1234567 {
		t.Fatalf("unexpected controllers: %+v", controllers)
	}
}

func TestFetchACEBuildsURLAndDecodes(t *testing.T) {
	var gotPath, gotURLPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RawQuery
		gotURLPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"cid":878508}]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret", nil)

	cids, err := client.FetchACE(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotURLPath != "/user/roles/ZHQ/ACE" {
		t.Errorf("expected path /user/roles/ZHQ/ACE, got %q", gotURLPath)
	}

	if !strings.Contains(gotPath, "apikey=secret") {
		t.Errorf("expected apikey=secret in query, got %q", gotPath)
	}

	if !strings.Contains(gotPath, "&t=") {
		t.Errorf("expected timestamp param in query, got %q", gotPath)
	}

	if !slices.Equal(cids, []int{878508}) {
		t.Errorf("expected cids [878508], got %v", cids)
	}
}

func TestFetchFacilityInfoBuildsURLAndDecodes(t *testing.T) {
	var gotPath, gotURLPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RawQuery
		gotURLPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"testing":false,"data":{"facility":{"info":{"id":"ZAU","atm":1000001,"datm":1000002,"ta":1000003,"ec":1000004,"fe":1000005,"wm":1000006},"roles":[]}}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret", nil)

	info, err := client.FetchFacilityInfo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotURLPath != "/facility/ZAU" {
		t.Errorf("expected path /facility/ZAU, got %q", gotURLPath)
	}

	if !strings.Contains(gotPath, "apikey=secret") {
		t.Errorf("expected apikey=secret in query, got %q", gotPath)
	}

	if !strings.Contains(gotPath, "&t=") {
		t.Errorf("expected timestamp param in query, got %q", gotPath)
	}

	want := FacilityInfo{ATM: 1000001, DATM: 1000002, TA: 1000003, EC: 1000004, FE: 1000005, WM: 1000006}
	if *info != want {
		t.Errorf("expected facility info %+v, got %+v", want, *info)
	}
}

func TestFetchFacilityInfoReturnsErrorOnNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret", nil)

	_, err := client.FetchFacilityInfo(context.Background())
	if err == nil {
		t.Fatal("expected error on non-200 response")
	}
}

func TestFetchFacilityInfoReturnsErrorWhenMissingConfig(t *testing.T) {
	client := NewClient("", "", nil)

	_, err := client.FetchFacilityInfo(context.Background())
	if err == nil {
		t.Fatal("expected error when URL or API key is missing")
	}
}

func TestFetchFacilityInfoReturnsErrorOnInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`not json`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret", nil)

	_, err := client.FetchFacilityInfo(context.Background())
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestFetchACEReturnsErrorOnNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret", nil)

	_, err := client.FetchACE(context.Background())
	if err == nil {
		t.Fatal("expected error on non-200 response")
	}
}

func TestFetchACEReturnsErrorWhenMissingConfig(t *testing.T) {
	client := NewClient("", "", nil)

	_, err := client.FetchACE(context.Background())
	if err == nil {
		t.Fatal("expected error when URL or API key is missing")
	}
}

func TestFetchRosterReturnsErrorOnNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret", nil)

	_, err := client.FetchRoster(context.Background())
	if err == nil {
		t.Fatal("expected error on non-200 response")
	}
}

func TestFetchRosterReturnsErrorWhenMissingConfig(t *testing.T) {
	client := NewClient("", "", nil)

	_, err := client.FetchRoster(context.Background())
	if err == nil {
		t.Fatal("expected error when URL or API key is missing")
	}
}

func TestCertSyncDateUnmarshal(t *testing.T) {
	var date CertSyncDate

	err := date.UnmarshalJSON([]byte(`"2025-12-08 03:23:09"`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := time.Time(date).String(); got != "2025-12-08 03:23:09 +0000 UTC" {
		t.Errorf("unexpected date: %s", got)
	}

	var nullDate CertSyncDate

	err = nullDate.UnmarshalJSON([]byte(`null`))
	if err != nil {
		t.Fatalf("unexpected error on null: %v", err)
	}
}

func TestCertSyncDateUnmarshalInvalidFormat(t *testing.T) {
	var date CertSyncDate

	err := date.UnmarshalJSON([]byte(`"not a timestamp"`))
	if err == nil {
		t.Fatal("expected error on invalid date format")
	}
}

func TestFetchRosterReturnsErrorOnInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`not json`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret", nil)

	_, err := client.FetchRoster(context.Background())
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

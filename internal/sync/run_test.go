package sync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/vzauartcc/roster-sync/internal/vatusa"
	"github.com/vzauartcc/roster-sync/internal/zau"
)

const (
	existingUserVatusaJSON = `{"testing":false,"data":[{"cid":1000001,"fname":"Jane","lname":"Smith","email":"jane@example.com","facility":"ZAU","rating":3,"membership":"home","flag_broadcastOptedIn":true,"roles":[{"id":1,"cid":1000001,"facility":"ZAU","role":"ATM","created_at":"2026-01-01T00:00:00Z"}],"facility_join":"2026-01-01T00:00:00Z"}]}`

	existingUserZauJSON = `{"home":[{"cid":1000001,"fname":"Jane","lname":"Smith","email":"jane@example.com","rating":3,"homeFacility":"ZAU","broadcast":true,"member":true,"vis":false,"prefName":false,"certCodes":[],"roleCodes":["atm"]}],"visiting":[],"removed":[]}`

	emptyZauJSON = `{"home":[],"visiting":[],"removed":[]}`

	atManagerRolesJSON = `[{"name":"Air Traffic Manager","code":"ATM","order":1,"class":"staff"}]`

	aceRolesJSON = `[{"name":"Air Traffic Manager","code":"ATM","order":1,"class":"staff"},{"name":"Assistant to the Chief Excutive Officer","code":"ACE","order":2,"class":"staff"}]`

	aceRosterJSON = `{"data":[{"cid":1000001}]}`

	aceRosterPath = "/user/roles/ZHQ/ACE"
	facilityPath  = "/facility/ZAU"

	facilityJSON = `{"testing":false,"data":{"facility":{"info":{"atm":1000001,"datm":0,"ta":0,"ec":1000001,"fe":0,"wm":0},"roles":[]}}}`
)

type request struct {
	method string
	path   string
	body   string
}

func newFakeVatusa(t *testing.T, rosterJSON string, status int) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		if status != http.StatusOK {
			writer.WriteHeader(status)

			return
		}

		writer.Header().Set("Content-Type", "application/json")

		switch req.URL.Path {
		case aceRosterPath:
			_, _ = writer.Write([]byte(`{"data":[]}`))
		case facilityPath:
			_, _ = writer.Write([]byte(facilityJSON))
		default:
			_, _ = writer.Write([]byte(rosterJSON))
		}
	}))
}

func newFakeZau(
	t *testing.T,
	userJSON string,
	userStatus int,
	rolesJSON string,
	rolesStatus int,
) (*httptest.Server, *[]request) {
	t.Helper()

	requests := &[]request{}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		*requests = append(*requests, request{method: req.Method, path: req.URL.Path, body: string(body)})

		writer.Header().Set("Content-Type", "application/json")

		switch req.URL.Path {
		case "/user":
			if userStatus != http.StatusOK {
				writer.WriteHeader(userStatus)

				return
			}

			_, _ = writer.Write([]byte(userJSON))
		case "/controller/role":
			if rolesStatus != http.StatusOK {
				writer.WriteHeader(rolesStatus)

				return
			}

			_, _ = writer.Write([]byte(rolesJSON))
		default:
			_, _ = writer.Write([]byte(`{}`))
		}
	}))

	return server, requests
}

func findRequest(requests *[]request, method, path string) *request {
	for i := range *requests {
		if (*requests)[i].method == method && (*requests)[i].path == path {
			return &(*requests)[i]
		}
	}

	return nil
}

func newClients(t *testing.T, vatusaURL, zauURL string) (*vatusa.Client, *zau.Client) {
	t.Helper()

	return vatusa.NewClient(vatusaURL, "key", nil), zau.NewClient(zauURL, "key", nil)
}

func TestRunCreatesNewUserWithRoles(t *testing.T) {
	vatusaSrv := newFakeVatusa(t, existingUserVatusaJSON, http.StatusOK)
	defer vatusaSrv.Close()

	zauSrv, requests := newFakeZau(t, `{"home":[{"cid":9999999,"fname":"Other","lname":"User","email":"o@example.com","rating":3,"homeFacility":"ZAU","broadcast":true,"member":true,"vis":false,"prefName":false,"certCodes":[],"roleCodes":[]}],"visiting":[],"removed":[]}`, http.StatusOK, atManagerRolesJSON, http.StatusOK)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	result, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Added != 1 {
		t.Errorf("expected 1 added, got %d", result.Added)
	}

	post := findRequest(requests, http.MethodPost, "/controller/1000001")
	if post == nil {
		t.Fatalf("expected POST /controller/1000001, requests: %+v", *requests)
	}

	var payload zau.PostControllerPayload

	err = json.Unmarshal([]byte(post.body), &payload)
	if err != nil {
		t.Fatalf("invalid post body %q: %v", post.body, err)
	}

	if payload.CID != 1000001 || !payload.IsMember {
		t.Errorf("unexpected payload: %+v", payload)
	}

	if !slices.Equal(payload.RoleCodes, []string{roleATM}) {
		t.Errorf("expected roles [atm], got %v", payload.RoleCodes)
	}
}

func TestRunUpdatesRolesWithExistingRolesIncluded(t *testing.T) {
	vatusaJSON := `{"testing":false,"data":[{"cid":1000001,"fname":"Jane","lname":"Smith","email":"jane@example.com","facility":"ZAU","rating":3,"membership":"home","flag_broadcastOptedIn":true,"roles":[{"id":1,"cid":1000001,"facility":"ZAU","role":"ATM","created_at":"2026-01-01T00:00:00Z"},{"id":2,"cid":1000001,"facility":"ZAU","role":"EC","created_at":"2026-01-01T00:00:00Z"}],"facility_join":"2026-01-01T00:00:00Z"}]}`
	zauRoles := `[{"name":"Air Traffic Manager","code":"ATM","order":1,"class":"staff"},{"name":"Events Coordinator","code":"EC","order":2,"class":"staff"}]`

	vatusaSrv := newFakeVatusa(t, vatusaJSON, http.StatusOK)
	defer vatusaSrv.Close()

	zauSrv, requests := newFakeZau(t, existingUserZauJSON, http.StatusOK, zauRoles, http.StatusOK)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	result, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.UpdatedRoles != 1 {
		t.Errorf("expected 1 role update, got %d", result.UpdatedRoles)
	}

	patch := findRequest(requests, http.MethodPut, "/controller/1000001/roles")
	if patch == nil {
		t.Fatalf("expected PUT /controller/1000001/roles, requests: %+v", *requests)
	}

	var payload zau.RolesControllerPayload

	err = json.Unmarshal([]byte(patch.body), &payload)
	if err != nil {
		t.Fatalf("invalid patch body %q: %v", patch.body, err)
	}

	if !slices.Equal(payload.Roles, []string{roleATM, roleEC}) {
		t.Errorf("expected roles [atm ec], got %v", payload.Roles)
	}
}

func TestRunAssignsTeamRoleToFacilityNonHolder(t *testing.T) {
	vatusaJSON := `{"testing":false,"data":[{"cid":1000002,"fname":"Alex","lname":"Jones","email":"alex@example.com","facility":"ZAU","rating":3,"membership":"home","flag_broadcastOptedIn":true,"roles":[{"id":1,"cid":1000002,"facility":"ZAU","role":"EC","created_at":"2026-01-01T00:00:00Z"}],"facility_join":"2026-01-01T00:00:00Z"}]}`
	zauRoles := `[{"name":"Events Coordinator","code":"EC","order":1,"class":"staff"},{"name":"Events Team","code":"ET","order":2,"class":"staff"}]`

	vatusaSrv := newFakeVatusa(t, vatusaJSON, http.StatusOK)
	defer vatusaSrv.Close()

	zauSrv, requests := newFakeZau(t, existingUserZauJSON, http.StatusOK, zauRoles, http.StatusOK)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	result, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Added != 1 {
		t.Errorf("expected 1 added, got %d", result.Added)
	}

	post := findRequest(requests, http.MethodPost, "/controller/1000002")
	if post == nil {
		t.Fatalf("expected POST /controller/1000002, requests: %+v", *requests)
	}

	var payload zau.PostControllerPayload

	err = json.Unmarshal([]byte(post.body), &payload)
	if err != nil {
		t.Fatalf("invalid post body %q: %v", post.body, err)
	}

	if !slices.Equal(payload.RoleCodes, []string{roleET}) {
		t.Errorf("expected roles [et], got %v", payload.RoleCodes)
	}
}

func TestRunDegradesWhenFacilityInfoFetchFails(t *testing.T) {
	vatusaJSON := `{"testing":false,"data":[{"cid":1000002,"fname":"Alex","lname":"Jones","email":"alex@example.com","facility":"ZAU","rating":3,"membership":"home","flag_broadcastOptedIn":true,"roles":[{"id":1,"cid":1000002,"facility":"ZAU","role":"EC","created_at":"2026-01-01T00:00:00Z"}],"facility_join":"2026-01-01T00:00:00Z"}]}`
	zauRoles := `[{"name":"Events Coordinator","code":"EC","order":1,"class":"staff"},{"name":"Events Team","code":"ET","order":2,"class":"staff"}]`

	vatusaSrv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		writer.Header().Set("Content-Type", "application/json")

		switch req.URL.Path {
		case facilityPath:
			writer.WriteHeader(http.StatusInternalServerError)
		case aceRosterPath:
			_, _ = writer.Write([]byte(`{"data":[]}`))
		default:
			_, _ = writer.Write([]byte(vatusaJSON))
		}
	}))
	defer vatusaSrv.Close()

	zauSrv, requests := newFakeZau(t, existingUserZauJSON, http.StatusOK, zauRoles, http.StatusOK)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	result, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if err != nil {
		t.Fatalf("expected degrade, got error: %v", err)
	}

	if result.Added != 1 {
		t.Errorf("expected 1 added, got %d", result.Added)
	}

	post := findRequest(requests, http.MethodPost, "/controller/1000002")
	if post == nil {
		t.Fatalf("expected POST /controller/1000002, requests: %+v", *requests)
	}

	var payload zau.PostControllerPayload

	err = json.Unmarshal([]byte(post.body), &payload)
	if err != nil {
		t.Fatalf("invalid post body %q: %v", post.body, err)
	}

	if !slices.Equal(payload.RoleCodes, []string{roleET}) {
		t.Errorf("expected roles [et], got %v", payload.RoleCodes)
	}
}

func TestRunGrantsACERoleToExistingUser(t *testing.T) {
	vatusaJSON := `{"testing":false,"data":[{"cid":1000001,"fname":"Jane","lname":"Smith","email":"jane@example.com","facility":"ZAU","rating":3,"membership":"home","flag_broadcastOptedIn":true,"roles":[{"id":1,"cid":1000001,"facility":"ZAU","role":"ATM","created_at":"2026-01-01T00:00:00Z"}],"facility_join":"2026-01-01T00:00:00Z"}]}`
	zauJSON := `{"home":[{"cid":1000001,"fname":"Jane","lname":"Smith","email":"jane@example.com","rating":3,"homeFacility":"ZAU","broadcast":true,"member":true,"vis":false,"prefName":false,"certCodes":[],"roleCodes":["atm"]}],"visiting":[],"removed":[]}`

	vatusaSrv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		writer.Header().Set("Content-Type", "application/json")

		switch req.URL.Path {
		case aceRosterPath:
			_, _ = writer.Write([]byte(aceRosterJSON))
		case facilityPath:
			_, _ = writer.Write([]byte(facilityJSON))
		default:
			_, _ = writer.Write([]byte(vatusaJSON))
		}
	}))
	defer vatusaSrv.Close()

	zauSrv, requests := newFakeZau(t, zauJSON, http.StatusOK, aceRolesJSON, http.StatusOK)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	result, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ACEGrants != 1 {
		t.Errorf("expected 1 ACE grant, got %d", result.ACEGrants)
	}

	put := findRequest(requests, http.MethodPut, "/controller/1000001/roles")
	if put == nil {
		t.Fatalf("expected PUT /controller/1000001/roles, requests: %+v", *requests)
	}

	var payload zau.RolesControllerPayload

	err = json.Unmarshal([]byte(put.body), &payload)
	if err != nil {
		t.Fatalf("invalid put body %q: %v", put.body, err)
	}

	if !slices.Equal(payload.Roles, []string{roleATM, aceRole}) {
		t.Errorf("expected roles [atm ace], got %v", payload.Roles)
	}
}

func TestRunDoesNotInsertACEMemberWhoIsNotInZau(t *testing.T) {
	vatusaJSON := `{"testing":false,"data":[{"cid":1000001,"fname":"Jane","lname":"Smith","email":"jane@example.com","facility":"ZAU","rating":3,"membership":"home","roles":[{"id":1,"cid":1000001,"facility":"ZAU","role":"ATM","created_at":"2026-01-01T00:00:00Z"}],"facility_join":"2026-01-01T00:00:00Z"}]}`
	aceRoster := `{"data":[{"cid":9999999}]}`
	zauJSON := `{"home":[{"cid":1000001,"fname":"Jane","lname":"Smith","email":"jane@example.com","rating":3,"homeFacility":"ZAU","broadcast":true,"member":true,"vis":false,"prefName":false,"certCodes":[],"roleCodes":["atm"]}],"visiting":[],"removed":[]}`

	zauSrv, requests := newFakeZau(t, zauJSON, http.StatusOK, aceRolesJSON, http.StatusOK)
	defer zauSrv.Close()

	vatusaSrv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		writer.Header().Set("Content-Type", "application/json")

		switch req.URL.Path {
		case aceRosterPath:
			_, _ = writer.Write([]byte(aceRoster))
		case facilityPath:
			_, _ = writer.Write([]byte(facilityJSON))
		default:
			_, _ = writer.Write([]byte(vatusaJSON))
		}
	}))
	defer vatusaSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	result, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ACEGrants != 0 {
		t.Errorf("expected 0 ACE grants for non-existent user, got %d", result.ACEGrants)
	}

	for _, req := range *requests {
		if req.method == http.MethodPost {
			t.Errorf("expected no user creation, got %+v", req)
		}
	}
}

func TestRunAppliesNoWritesWhenNothingChanged(t *testing.T) {
	vatusaSrv := newFakeVatusa(t, existingUserVatusaJSON, http.StatusOK)
	defer vatusaSrv.Close()

	zauSrv, requests := newFakeZau(t, existingUserZauJSON, http.StatusOK, atManagerRolesJSON, http.StatusOK)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	result, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != (Result{}) {
		t.Errorf("expected empty result, got %+v", result)
	}

	for _, req := range *requests {
		if req.method != http.MethodGet {
			t.Errorf("expected only GET requests, got %+v", req)
		}
	}
}

func TestRunAbortsOnVatusaFetchError(t *testing.T) {
	vatusaSrv := newFakeVatusa(t, `{}`, http.StatusInternalServerError)
	defer vatusaSrv.Close()

	zauSrv, _ := newFakeZau(t, emptyZauJSON, http.StatusOK, `[]`, http.StatusOK)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	_, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if err == nil {
		t.Fatal("expected error when VATUSA fetch fails")
	}
}

func TestRunAbortsOnZauFetchError(t *testing.T) {
	vatusaSrv := newFakeVatusa(t, `{}`, http.StatusOK)
	defer vatusaSrv.Close()

	zauSrv, _ := newFakeZau(t, `{}`, http.StatusInternalServerError, `[]`, http.StatusOK)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	_, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if err == nil {
		t.Fatal("expected error when ZAU fetch fails")
	}
}

func TestRunDegradesWhenRolesFetchFails(t *testing.T) {
	vatusaSrv := newFakeVatusa(t, existingUserVatusaJSON, http.StatusOK)
	defer vatusaSrv.Close()

	zauSrv, requests := newFakeZau(t, existingUserZauJSON, http.StatusOK, `[]`, http.StatusInternalServerError)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	result, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if err != nil {
		t.Fatalf("expected degrade, got error: %v", err)
	}

	if result != (Result{}) {
		t.Errorf("expected empty result, got %+v", result)
	}

	if put := findRequest(requests, http.MethodPut, "/controller/1000001"); put != nil {
		t.Errorf("expected no role PUT when roles fetch fails, got %+v", *put)
	}
}

func TestRunAbortsOnEmptyVatusaRoster(t *testing.T) {
	vatusaSrv := newFakeVatusa(t, `{"testing":false,"data":[]}`, http.StatusOK)
	defer vatusaSrv.Close()

	zauSrv, requests := newFakeZau(t, existingUserZauJSON, http.StatusOK, `[]`, http.StatusOK)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	_, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if !errors.Is(err, ErrEmptyVatusaRoster) {
		t.Fatalf("expected ErrEmptyVatusaRoster, got %v", err)
	}

	for _, req := range *requests {
		if req.method != http.MethodGet {
			t.Errorf("expected no write requests, got %+v", req)
		}
	}
}

func TestRunAbortsOnEmptyZauRoster(t *testing.T) {
	vatusaSrv := newFakeVatusa(t, existingUserVatusaJSON, http.StatusOK)
	defer vatusaSrv.Close()

	zauSrv, _ := newFakeZau(t, emptyZauJSON, http.StatusOK, `[]`, http.StatusOK)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	_, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if !errors.Is(err, ErrEmptyZauRoster) {
		t.Fatalf("expected ErrEmptyZauRoster, got %v", err)
	}
}

func TestRunProceedsWhenOnlyHomeEmpty(t *testing.T) {
	vatusaJSON := `{"testing":false,"data":[{"cid":1000001,"fname":"Jane","lname":"Smith","email":"jane@example.com","facility":"ZAU","rating":3,"membership":"visit","flag_broadcastOptedIn":true,"roles":[{"id":1,"cid":1000001,"facility":"ZAU","role":"ATM","created_at":"2026-01-01T00:00:00Z"}],"facility_join":"2026-01-01T00:00:00Z"}]}`
	zauJSON := `{"home":[],"visiting":[{"cid":1000001,"fname":"Jane","lname":"Smith","email":"jane@example.com","rating":3,"homeFacility":"ZAU","broadcast":true,"member":true,"vis":true,"prefName":false,"certCodes":[],"roleCodes":["atm"]}],"removed":[]}`

	vatusaSrv := newFakeVatusa(t, vatusaJSON, http.StatusOK)
	defer vatusaSrv.Close()

	zauSrv, _ := newFakeZau(t, zauJSON, http.StatusOK, atManagerRolesJSON, http.StatusOK)
	defer zauSrv.Close()

	vatusaClient, zauClient := newClients(t, vatusaSrv.URL, zauSrv.URL)

	result, err := Run(context.Background(), vatusaClient, zauClient, fixedNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != (Result{}) {
		t.Errorf("expected empty result, got %+v", result)
	}
}

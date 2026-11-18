package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApplyHandlesAllOperationKinds(t *testing.T) {
	zauSrv, requests := newFakeZau(t, emptyZauJSON, http.StatusOK, `[]`, http.StatusOK)
	defer zauSrv.Close()

	_, zauClient := newClients(t, zauSrv.URL, zauSrv.URL)

	ops := []Operation{
		{Kind: OpUpdateCore, CID: 1000001, VUser: vController(1000001, "home", nil, "ZAU")},
		{Kind: OpUpdateMembership, CID: 1000002, VUser: vController(1000002, "home", nil, "ZAU")},
		{Kind: OpUpdateVisit, CID: 1000003, VUser: vController(1000003, "visit", nil, "ZAU")},
		{Kind: OpUpdateRating, CID: 1000004, VUser: vController(1000004, "home", nil, "ZAU")},
		{Kind: OpUpdateRoles, CID: 1000005, Roles: []string{roleATM}},
		{Kind: OpRemoveMember, CID: 1000006},
		{Kind: OpRemoveCerts, CID: 1000007},
	}

	result := apply(context.Background(), zauClient, ops, fixedNow)

	want := Result{
		UpdatedCore:   1,
		MadeMember:    1,
		MadeVisitor:   1,
		UpdatedRating: 1,
		UpdatedRoles:  1,
		RemovedMember: 1,
		CertsRemoved:  1,
	}

	if result != want {
		t.Errorf("expected result %+v, got %+v", want, result)
	}

	wantRequests := []request{
		{method: http.MethodPatch, path: "/controller/1000001"},
		{method: http.MethodPatch, path: "/controller/1000002/member"},
		{method: http.MethodPatch, path: "/controller/1000003/visit"},
		{method: http.MethodPatch, path: "/controller/1000004/rating"},
		{method: http.MethodPut, path: "/controller/1000005/roles"},
		{method: http.MethodPatch, path: "/controller/1000006/member"},
		{method: http.MethodPatch, path: "/controller/1000007/remove-cert"},
	}

	if len(*requests) != len(wantRequests) {
		t.Fatalf("expected %d write requests, got %d: %+v", len(wantRequests), len(*requests), *requests)
	}

	for i, wantReq := range wantRequests {
		gotReq := (*requests)[i]

		if gotReq.method != wantReq.method || gotReq.path != wantReq.path {
			t.Errorf("request %d: expected %s %s, got %s %s",
				i, wantReq.method, wantReq.path, gotReq.method, gotReq.path)
		}
	}
}

func TestApplyDoesNotCountFailedWrites(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, zauClient := newClients(t, server.URL, server.URL)

	ops := []Operation{
		{Kind: OpUpdateCore, CID: 1000001, VUser: vController(1000001, "home", nil, "ZAU")},
		{Kind: OpUpdateRating, CID: 1000002, VUser: vController(1000002, "home", nil, "ZAU")},
	}

	result := apply(context.Background(), zauClient, ops, fixedNow)

	if result != (Result{}) {
		t.Errorf("expected empty result on failed writes, got %+v", result)
	}
}

func TestApplyTracksOpCreate(t *testing.T) {
	zauSrv, requests := newFakeZau(t, emptyZauJSON, http.StatusOK, `[]`, http.StatusOK)
	defer zauSrv.Close()

	_, zauClient := newClients(t, zauSrv.URL, zauSrv.URL)

	ops := []Operation{
		{Kind: OpCreate, CID: 1000001, VUser: vController(1000001, "home", nil, "ZAU")},
	}

	result := apply(context.Background(), zauClient, ops, fixedNow)

	if result.Added != 1 {
		t.Errorf("expected 1 added, got %d", result.Added)
	}

	if got := findRequest(requests, http.MethodPost, "/controller/1000001"); got == nil {
		t.Errorf("expected POST /controller/1000001, requests: %+v", *requests)
	}
}

func TestApplyLogsUnknownOperationKind(t *testing.T) {
	zauSrv, requests := newFakeZau(t, emptyZauJSON, http.StatusOK, `[]`, http.StatusOK)
	defer zauSrv.Close()

	_, zauClient := newClients(t, zauSrv.URL, zauSrv.URL)

	ops := []Operation{{Kind: OperationKind(99), CID: 1000001}}

	result := apply(context.Background(), zauClient, ops, fixedNow)

	if result != (Result{}) {
		t.Errorf("expected empty result for unknown op, got %+v", result)
	}

	if len(*requests) != 0 {
		t.Errorf("expected no requests for unknown op, got %+v", *requests)
	}
}

func TestOperationKindString(t *testing.T) {
	tests := map[OperationKind]string{
		OpCreate:           "create",
		OpUpdateCore:       "update-core",
		OpUpdateMembership: "update-membership",
		OpUpdateVisit:      "update-visit",
		OpUpdateRating:     "update-rating",
		OpUpdateRoles:      "update-roles",
		OpRemoveMember:     "remove-member",
		OpRemoveCerts:      "remove-certs",
		OperationKind(99):  "unknown",
	}

	for kind, want := range tests {
		if got := kind.String(); got != want {
			t.Errorf("OperationKind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}

func TestAddNewRolesDeduplicates(t *testing.T) {
	got := addNewRoles([]string{roleATM, vatusaRoleATM}, []string{roleEC, vatusaRoleATM})

	want := []string{roleATM, roleEC}

	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

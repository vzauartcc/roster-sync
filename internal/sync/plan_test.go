package sync

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/vzauartcc/roster-sync/internal/vatusa"
	"github.com/vzauartcc/roster-sync/internal/zau"
)

var fixedNow = time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

func vController(cid int, membership string, roles []string, facility string) vatusa.Controller {
	controller := vatusa.Controller{
		CID:              cid,
		FName:            "F",
		LName:            "L",
		Email:            "f@example.com",
		Facility:         facility,
		Rating:           3,
		Membership:       membership,
		BroadcastOptedIn: true,
		FacilityJoinDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	for i, role := range roles {
		controller.Roles = append(controller.Roles, vatusa.Role{
			ID:        i + 1,
			CID:       cid,
			Facility:  "ZAU",
			Role:      role,
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		})
	}

	return controller
}

func zUser(cid int, member, visitor bool, roleCodes []string) zau.User {
	return zau.User{
		CID:                  cid,
		FName:                "F",
		LName:                "L",
		Email:                "f@example.com",
		Rating:               3,
		HomeFacility:         "ZAU",
		FlagBroadcastOptedIn: true,
		IsMember:             member,
		IsVisitor:            visitor,
		UseNamePrivacy:       false,
		RoleCodes:            roleCodes,
	}
}

func assertOp(t *testing.T, ops []Operation, kind OperationKind) {
	t.Helper()

	for _, operation := range ops {
		if operation.Kind == kind {
			return
		}
	}

	t.Errorf("expected op %s in ops %+v", kind, ops)
}

func assertNoOp(t *testing.T, ops []Operation, kind OperationKind) {
	t.Helper()

	for _, operation := range ops {
		if operation.Kind == kind {
			t.Errorf("expected no %s op, got %+v", kind, ops)
		}
	}
}

func TestPlanCreatesNewUserWithFilteredRoles(t *testing.T) {
	vUser := vController(1000001, "home", []string{"ATM", "C3"}, "ZAU")
	vUser.Roles = append(vUser.Roles,
		vatusa.Role{Facility: "BOS", Role: "ATM"},
		vatusa.Role{Facility: "ZHQ", Role: "ZZZ"},
	)

	ops := Plan(zau.Roster{}, []vatusa.Controller{vUser}, []string{"atm", "c3"}, vatusa.FacilityInfo{ATM: 1000001}, fixedNow)

	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d: %+v", len(ops), ops)
	}

	operation := ops[0]
	if operation.Kind != OpCreate {
		t.Fatalf("expected OpCreate, got %s", operation.Kind)
	}

	want := []string{"atm", "c3"}
	if !slices.Equal(operation.Roles, want) {
		t.Errorf("expected roles %v, got %v", want, operation.Roles)
	}
}

func TestPlanCreatesVisitorWithRoles(t *testing.T) {
	vUser := vController(1000002, "visit", []string{"EC"}, "ZAU")

	ops := Plan(zau.Roster{}, []vatusa.Controller{vUser}, []string{"atm", "ec"}, vatusa.FacilityInfo{EC: 1000002}, fixedNow)

	if len(ops) != 1 || ops[0].Kind != OpCreate {
		t.Fatalf("expected 1 create op, got %+v", ops)
	}

	if !slices.Equal(ops[0].Roles, []string{"ec"}) {
		t.Errorf("expected visitor to receive roles, got %v", ops[0].Roles)
	}
}

func planNewUserRoles(t *testing.T, role string, available []string, staff vatusa.FacilityInfo) []string {
	t.Helper()

	vUser := vController(1000001, "home", []string{role}, "ZAU")

	ops := Plan(zau.Roster{}, []vatusa.Controller{vUser}, available, staff, fixedNow)
	if len(ops) != 1 || ops[0].Kind != OpCreate {
		t.Fatalf("expected 1 create op, got %+v", ops)
	}

	return ops[0].Roles
}

var allRoleCodes = []string{"atm", "datm", "ta", "ec", "fe", "wm", "et", "ft", "wt"}

func TestPlanAssignsNamedRoleToFacilityStaffHolder(t *testing.T) {
	cases := []struct {
		name  string
		role  string
		want  string
		staff vatusa.FacilityInfo
	}{
		{"ATM", "ATM", "atm", vatusa.FacilityInfo{ATM: 1000001}},
		{"DATM", "DATM", "datm", vatusa.FacilityInfo{DATM: 1000001}},
		{"TA", "TA", "ta", vatusa.FacilityInfo{TA: 1000001}},
		{"EC", "EC", "ec", vatusa.FacilityInfo{EC: 1000001}},
		{"FE", "FE", "fe", vatusa.FacilityInfo{FE: 1000001}},
		{"WM", "WM", "wm", vatusa.FacilityInfo{WM: 1000001}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := planNewUserRoles(t, tc.role, allRoleCodes, tc.staff)

			want := []string{tc.want}
			if !slices.Equal(got, want) {
				t.Errorf("expected roles %v, got %v", want, got)
			}
		})
	}
}

func TestPlanAssignsTeamRoleToNonHolder(t *testing.T) {
	cases := []struct {
		role string
		team string
	}{
		{"EC", "et"},
		{"FE", "ft"},
		{"WM", "wt"},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			got := planNewUserRoles(t, tc.role, allRoleCodes, vatusa.FacilityInfo{})

			want := []string{tc.team}
			if !slices.Equal(got, want) {
				t.Errorf("expected roles %v, got %v", want, got)
			}
		})
	}
}

func TestPlanSkipsSeniorRoleForNonHolder(t *testing.T) {
	for _, role := range []string{"ATM", "DATM", "TA"} {
		t.Run(role, func(t *testing.T) {
			got := planNewUserRoles(t, role, allRoleCodes, vatusa.FacilityInfo{})

			if len(got) != 0 {
				t.Errorf("expected no roles, got %v", got)
			}
		})
	}
}

func TestPlanSkipsTeamRoleWhenNotAvailable(t *testing.T) {
	cases := []struct {
		role      string
		available string
	}{
		{"EC", "ec"},
		{"FE", "fe"},
		{"WM", "wm"},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			got := planNewUserRoles(t, tc.role, []string{tc.available}, vatusa.FacilityInfo{})

			if len(got) != 0 {
				t.Errorf("expected no roles, got %v", got)
			}
		})
	}
}

func TestPlanAddsTeamRoleToExistingNonHolder(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"ia"})}}
	vUser := vController(1000001, "home", []string{"EC"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"ec", "et"}, vatusa.FacilityInfo{}, fixedNow)

	if len(ops) != 1 || ops[0].Kind != OpUpdateRoles {
		t.Fatalf("expected 1 role update op, got %+v", ops)
	}

	want := []string{"ia", "et"}
	if !slices.Equal(ops[0].Roles, want) {
		t.Errorf("expected roles %v, got %v", want, ops[0].Roles)
	}
}

func TestPlanRemovesTeamRoleWhenControllerIsHolder(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"et"})}}
	vUser := vController(1000001, "home", []string{"EC"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"ec", "et"}, vatusa.FacilityInfo{EC: 1000001}, fixedNow)

	if len(ops) != 1 || ops[0].Kind != OpUpdateRoles {
		t.Fatalf("expected 1 role update op, got %+v", ops)
	}

	want := []string{"ec"}
	if !slices.Equal(ops[0].Roles, want) {
		t.Errorf("expected roles %v, got %v", want, ops[0].Roles)
	}
}

func TestPlanRemovesTeamRoleWhenHolderAlreadyHasBoth(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"ec", "et"})}}
	vUser := vController(1000001, "home", []string{"EC"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"ec", "et"}, vatusa.FacilityInfo{EC: 1000001}, fixedNow)

	if len(ops) != 1 || ops[0].Kind != OpUpdateRoles {
		t.Fatalf("expected 1 role update op, got %+v", ops)
	}

	want := []string{"ec"}
	if !slices.Equal(ops[0].Roles, want) {
		t.Errorf("expected roles %v, got %v", want, ops[0].Roles)
	}
}

func TestPlanKeepsTeamRoleWhenExplicitlyAssigned(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"ec"})}}
	vUser := vController(1000001, "home", []string{"EC", "ET"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"ec", "et"}, vatusa.FacilityInfo{EC: 1000001}, fixedNow)

	if len(ops) != 1 || ops[0].Kind != OpUpdateRoles {
		t.Fatalf("expected 1 role update op, got %+v", ops)
	}

	want := []string{"ec", "et"}
	if !slices.Equal(ops[0].Roles, want) {
		t.Errorf("expected roles %v, got %v", want, ops[0].Roles)
	}
}

func TestPlanNoOpsWhenNothingChanged(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"atm"})}}
	vUser := vController(1000001, "home", []string{"ATM"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"atm", "ec"}, vatusa.FacilityInfo{ATM: 1000001, EC: 1000001}, fixedNow)

	if len(ops) != 0 {
		t.Errorf("expected no ops, got %+v", ops)
	}
}

func TestPlanEmitsCoreUpdateOnChange(t *testing.T) {
	z := zUser(1000001, true, false, []string{"atm"})
	z.FName = "Old"
	roster := zau.Roster{Home: []zau.User{z}}
	vUser := vController(1000001, "home", []string{"ATM"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"atm"}, vatusa.FacilityInfo{ATM: 1000001}, fixedNow)

	assertOp(t, ops, OpUpdateCore)
}

func TestPlanEmitsRatingUpdateOnChange(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"atm"})}}
	vUser := vController(1000001, "home", []string{"ATM"}, "ZAU")
	vUser.Rating = 5

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"atm"}, vatusa.FacilityInfo{ATM: 1000001}, fixedNow)

	assertOp(t, ops, OpUpdateRating)
}

func TestPlanEmitsMembershipWhenNotMember(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, false, false, []string{"atm"})}}
	vUser := vController(1000001, "home", []string{"ATM"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"atm"}, vatusa.FacilityInfo{ATM: 1000001}, fixedNow)

	assertOp(t, ops, OpUpdateMembership)
}

func TestPlanEmitsVisitUpdateWhenStatusChanges(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, true, nil)}}
	vUser := vController(1000001, "home", nil, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"atm"}, vatusa.FacilityInfo{ATM: 1000001}, fixedNow)

	assertOp(t, ops, OpUpdateVisit)
}

func TestPlanEmitsVisitUpdateWhenVisitorHomeFacilityChanges(t *testing.T) {
	z := zUser(1000001, true, true, nil)
	z.HomeFacility = "BOS"
	roster := zau.Roster{Home: []zau.User{z}}
	vUser := vController(1000001, "visit", nil, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"atm"}, vatusa.FacilityInfo{ATM: 1000001}, fixedNow)

	assertOp(t, ops, OpUpdateVisit)
}

func TestPlanEmitsRoleUpdateWithUnion(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"atm"})}}
	vUser := vController(1000001, "home", []string{"ATM", "EC"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"atm", "ec"}, vatusa.FacilityInfo{ATM: 1000001, EC: 1000001}, fixedNow)

	if len(ops) != 1 || ops[0].Kind != OpUpdateRoles {
		t.Fatalf("expected 1 role update op, got %+v", ops)
	}

	want := []string{"atm", "ec"}
	if !slices.Equal(ops[0].Roles, want) {
		t.Errorf("expected roles %v, got %v", want, ops[0].Roles)
	}
}

func TestPlanNoRoleUpdateWhenNoNewRoles(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"atm"})}}
	vUser := vController(1000001, "home", []string{"ATM"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"atm"}, vatusa.FacilityInfo{ATM: 1000001}, fixedNow)

	assertNoOp(t, ops, OpUpdateRoles)
}

func TestPlanNoRoleUpdateWhenZauUserHasExtraRole(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"atm", "ia"})}}
	vUser := vController(1000001, "home", []string{"ATM"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"atm", "ec"}, vatusa.FacilityInfo{ATM: 1000001, EC: 1000001}, fixedNow)

	if len(ops) != 0 {
		t.Errorf("expected no ops, got %+v", ops)
	}
}

func TestPlanNoRoleUpdateWhenCurrentRoleCodesUppercase(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"ATM"})}}
	vUser := vController(1000001, "home", []string{"ATM"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"atm"}, vatusa.FacilityInfo{ATM: 1000001}, fixedNow)

	if len(ops) != 0 {
		t.Errorf("expected no ops, got %+v", ops)
	}
}

func TestPlanRoleUpdateMergesLowercaseWhenCurrentUppercase(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"ATM"})}}
	vUser := vController(1000001, "home", []string{"ATM", "EC"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"atm", "ec"}, vatusa.FacilityInfo{ATM: 1000001, EC: 1000001}, fixedNow)

	if len(ops) != 1 || ops[0].Kind != OpUpdateRoles {
		t.Fatalf("expected 1 role update op, got %+v", ops)
	}

	want := []string{"atm", "ec"}
	if !slices.Equal(ops[0].Roles, want) {
		t.Errorf("expected roles %v, got %v", want, ops[0].Roles)
	}
}

func TestPlanEmitsRemoveMemberForMissingCID(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"atm"})}}

	ops := Plan(roster, nil, []string{"atm"}, vatusa.FacilityInfo{}, fixedNow)

	assertOp(t, ops, OpRemoveMember)
}

func TestPlanEmitsRemoveCertsAfterSixMonths(t *testing.T) {
	removalDate := fixedNow.AddDate(0, -7, 0)
	z := zUser(1000001, false, false, nil)
	z.RemovalDate = &removalDate
	z.CertCodes = []string{"C1"}
	roster := zau.Roster{Removed: []zau.User{z}}

	ops := Plan(roster, nil, []string{"atm"}, vatusa.FacilityInfo{}, fixedNow)

	assertOp(t, ops, OpRemoveCerts)
}

func TestPlanNoRemoveCertsWithinSixMonths(t *testing.T) {
	removalDate := fixedNow.AddDate(0, -1, 0)
	z := zUser(1000001, false, false, nil)
	z.RemovalDate = &removalDate
	z.CertCodes = []string{"C1"}
	roster := zau.Roster{Removed: []zau.User{z}}

	ops := Plan(roster, nil, []string{"atm"}, vatusa.FacilityInfo{}, fixedNow)

	assertNoOp(t, ops, OpRemoveCerts)
}

func TestPlanNoRemoveCertsWithoutCertCodes(t *testing.T) {
	removalDate := fixedNow.AddDate(0, -7, 0)
	z := zUser(1000001, false, false, nil)
	z.RemovalDate = &removalDate
	roster := zau.Roster{Removed: []zau.User{z}}

	ops := Plan(roster, nil, []string{"atm"}, vatusa.FacilityInfo{}, fixedNow)

	assertNoOp(t, ops, OpRemoveCerts)
}

func TestPlanHandlesEmptyHomeRoster(t *testing.T) {
	z := zUser(1000002, true, true, []string{"ec"})
	z.HomeFacility = "BOS"
	roster := zau.Roster{Home: nil, Visiting: []zau.User{z}}
	vUser := vController(1000002, "visit", []string{"EC"}, "ZAU")

	ops := Plan(roster, []vatusa.Controller{vUser}, []string{"ec"}, vatusa.FacilityInfo{EC: 1000002}, fixedNow)

	if len(ops) != 1 || ops[0].Kind != OpUpdateVisit {
		t.Fatalf("expected 1 visit update op, got %+v", ops)
	}
}

func TestPlanIsDeterministic(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"atm"})}}
	vUser := vController(1000001, "home", []string{"ATM", "EC"}, "ZAU")

	staff := vatusa.FacilityInfo{ATM: 1000001, EC: 1000001}

	first := Plan(roster, []vatusa.Controller{vUser}, []string{"atm", "ec"}, staff, fixedNow)
	second := Plan(roster, []vatusa.Controller{vUser}, []string{"atm", "ec"}, staff, fixedNow)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("expected deterministic plans:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func TestPlanACEGrantsRoleToExistingUser(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"atm"})}}

	ops := PlanACE(roster, []int{1000001}, []string{"atm", "ace"})

	if len(ops) != 1 || ops[0].Kind != OpAddACE {
		t.Fatalf("expected 1 add-ace op, got %+v", ops)
	}

	if ops[0].CID != 1000001 {
		t.Errorf("expected cid 1000001, got %d", ops[0].CID)
	}

	want := []string{"atm", "ace"}
	if !slices.Equal(ops[0].Roles, want) {
		t.Errorf("expected roles %v, got %v", want, ops[0].Roles)
	}
}

func TestPlanACESkipsNonExistentUser(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, nil)}}

	ops := PlanACE(roster, []int{9999999}, []string{"ace"})

	if len(ops) != 0 {
		t.Errorf("expected no ops for non-existent user, got %+v", ops)
	}
}

func TestPlanACESkipsUserWhoAlreadyHasACE(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"ace"})}}

	ops := PlanACE(roster, []int{1000001}, []string{"ace"})

	if len(ops) != 0 {
		t.Errorf("expected no ops when user already has ace, got %+v", ops)
	}
}

func TestPlanACESkipsWhenRoleNotAvailable(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, nil)}}

	ops := PlanACE(roster, []int{1000001}, []string{"atm"})

	if len(ops) != 0 {
		t.Errorf("expected no ops when ace role not available, got %+v", ops)
	}
}

func TestPlanACEMergesUppercaseExistingRoles(t *testing.T) {
	roster := zau.Roster{Home: []zau.User{zUser(1000001, true, false, []string{"ATM"})}}

	ops := PlanACE(roster, []int{1000001}, []string{"atm", "ace"})

	if len(ops) != 1 || ops[0].Kind != OpAddACE {
		t.Fatalf("expected 1 add-ace op, got %+v", ops)
	}

	want := []string{"atm", "ace"}
	if !slices.Equal(ops[0].Roles, want) {
		t.Errorf("expected roles %v, got %v", want, ops[0].Roles)
	}
}

func TestPlanACEVisitingUserGetsRole(t *testing.T) {
	roster := zau.Roster{Visiting: []zau.User{zUser(1000001, false, true, nil)}}

	ops := PlanACE(roster, []int{1000001}, []string{"ace"})

	if len(ops) != 1 || ops[0].Kind != OpAddACE {
		t.Fatalf("expected 1 add-ace op for visiting user, got %+v", ops)
	}
}

func TestPlanDoesNotMutateInputRoster(t *testing.T) {
	home := []zau.User{zUser(1000001, true, false, []string{"atm"})}
	roster := zau.Roster{Home: home}
	vUser := vController(1000001, "home", []string{"ATM"}, "ZAU")

	_ = Plan(roster, []vatusa.Controller{vUser}, []string{"atm"}, vatusa.FacilityInfo{ATM: 1000001}, fixedNow)

	if len(home) != 1 {
		t.Fatalf("input home slice modified, len=%d", len(home))
	}

	if len(home[0].RoleCodes) != 1 || home[0].RoleCodes[0] != "atm" {
		t.Fatalf("input home user modified: %+v", home[0])
	}
}

package sync

import (
	"strings"
	"time"

	"github.com/vzauartcc/roster-sync/internal/vatusa"
	"github.com/vzauartcc/roster-sync/internal/zau"
)

// Plan computes the operations needed to bring the ZAU roster in line with
// the VATUSA roster. It is pure: no I/O, no logging, and it does not mutate
// its inputs.
func Plan(
	zauRoster zau.Roster,
	vatusaControllers []vatusa.Controller,
	availableRoles []string,
	staff vatusa.FacilityInfo,
	now time.Time,
) []Operation {
	allZauControllers := make([]zau.User, 0,
		len(zauRoster.Home)+len(zauRoster.Visiting)+len(zauRoster.Removed))
	allZauControllers = append(allZauControllers, zauRoster.Home...)
	allZauControllers = append(allZauControllers, zauRoster.Visiting...)
	allZauControllers = append(allZauControllers, zauRoster.Removed...)

	zauByCID := make(map[int]zau.User, len(allZauControllers))
	for _, controller := range allZauControllers {
		if _, exists := zauByCID[controller.CID]; !exists {
			zauByCID[controller.CID] = controller
		}
	}

	availableRoleSet := make(map[string]bool, len(availableRoles))
	for _, role := range availableRoles {
		availableRoleSet[strings.ToLower(role)] = true
	}

	ops := make([]Operation, 0, len(vatusaControllers))

	sixMonthsAgo := now.AddDate(0, -6, -1)

	for _, controller := range zauRoster.Removed {
		if controller.RemovalDate == nil {
			continue
		}

		isOlderThanSixMonths := controller.RemovalDate.Before(sixMonthsAgo)

		hasCertCodes := len(controller.CertCodes) > 0

		if isOlderThanSixMonths && hasCertCodes {
			ops = append(ops, Operation{
				Kind: OpRemoveCerts,
				CID:  controller.CID,
			})
		}
	}

	vatusaCIDs := make(map[int]bool, len(vatusaControllers))
	for _, controller := range vatusaControllers {
		vatusaCIDs[controller.CID] = true
	}

	for _, controller := range allZauControllers {
		if !controller.IsMember {
			continue
		}

		if _, exists := vatusaCIDs[controller.CID]; !exists {
			ops = append(ops, Operation{
				Kind: OpRemoveMember,
				CID:  controller.CID,
			})
		}
	}

	for _, vUser := range vatusaControllers {
		zUser, exists := zauByCID[vUser.CID]

		desiredRoles := filterRoles(vUser.Roles, availableRoleSet, staff, vUser.CID)

		if !exists {
			ops = append(ops, Operation{
				Kind:  OpCreate,
				CID:   vUser.CID,
				VUser: vUser,
				Roles: desiredRoles,
			})

			continue
		}

		if checkCoreInfo(vUser, zUser) {
			ops = append(ops, Operation{
				Kind:  OpUpdateCore,
				CID:   vUser.CID,
				VUser: vUser,
			})
		}

		if checkVisitingStatus(vUser, zUser, vUser.Membership != homeMembership) {
			ops = append(ops, Operation{
				Kind:  OpUpdateVisit,
				CID:   vUser.CID,
				VUser: vUser,
			})
		}

		if !zUser.IsMember {
			ops = append(ops, Operation{
				Kind:  OpUpdateMembership,
				CID:   vUser.CID,
				VUser: vUser,
			})
		}

		if zUser.Rating != vUser.Rating {
			ops = append(ops, Operation{
				Kind:  OpUpdateRating,
				CID:   vUser.CID,
				VUser: vUser,
			})
		}

		roles := addNewRoles(zUser.RoleCodes, desiredRoles)
		if !sameRoleSet(roles, zUser.RoleCodes) {
			ops = append(ops, Operation{
				Kind:  OpUpdateRoles,
				CID:   vUser.CID,
				ZUser: zUser,
				Roles: roles,
			})
		}
	}

	return ops
}

// filterRoles returns the lowercase roles assignable on the ZAU roster:
// roles for ZAU or ZHQ that exist in the roster's available roles.
func filterRoles(
	roles []vatusa.Role,
	available map[string]bool,
	staff vatusa.FacilityInfo,
	cid int,
) []string {
	assignable := make([]string, 0, len(roles))

	for _, userRole := range roles {
		if userRole.Facility != "ZAU" && userRole.Facility != "ZHQ" {
			continue
		}

		lcRole := strings.ToLower(userRole.Role)

		if !available[lcRole] {
			continue
		}

		switch lcRole {
		case "atm":
			if staff.ATM == cid {
				assignable = append(assignable, lcRole)
			}
		case "datm":
			if staff.DATM == cid {
				assignable = append(assignable, lcRole)
			}
		case "ta":
			if staff.TA == cid {
				assignable = append(assignable, lcRole)
			}
		case "ec":
			assignable = appendStaffOrTeam(assignable, lcRole, "et", staff.EC == cid, available)
		case "fe":
			assignable = appendStaffOrTeam(assignable, lcRole, "ft", staff.FE == cid, available)
		case "wm":
			assignable = appendStaffOrTeam(assignable, lcRole, "wt", staff.WM == cid, available)
		default:
			assignable = append(assignable, lcRole)
		}
	}

	return assignable
}

// appendStaffOrTeam appends the named role when the CID is the facility
// staff holder, otherwise the team role when it is available.
func appendStaffOrTeam(
	assignable []string,
	role, team string,
	isHolder bool,
	available map[string]bool,
) []string {
	if isHolder {
		return append(assignable, role)
	}

	if available[team] {
		return append(assignable, team)
	}

	return assignable
}

// staffTeamPairs maps each named staff role to the team role used when the
// controller is not the named holder.
var staffTeamPairs = [][2]string{
	{"ec", "et"},
	{"fe", "ft"},
	{"wm", "wt"},
}

// addNewRoles merges the desired roles into the current set. Existing roles
// are preserved, except that the team role is dropped when the controller is
// the named staff holder (e.g. et is removed when ec is assigned). Role codes
// are normalized to lowercase on both sides so the membership check is
// case-insensitive against the VATUSA-side roles, which are already lowercased
// by filterRoles.
func addNewRoles(current []string, newRoles []string) []string {
	desired := make(map[string]bool, len(newRoles))
	for _, role := range newRoles {
		desired[strings.ToLower(role)] = true
	}

	excluded := make(map[string]bool)

	for _, pair := range staffTeamPairs {
		if desired[pair[0]] && !desired[pair[1]] {
			excluded[pair[1]] = true
		}
	}

	seen := make(map[string]bool, len(current)+len(newRoles))

	merged := make([]string, 0, len(current)+len(newRoles))

	for _, role := range current {
		lcRole := strings.ToLower(role)

		if seen[lcRole] || excluded[lcRole] {
			continue
		}

		seen[lcRole] = true
		merged = append(merged, lcRole)
	}

	for _, role := range newRoles {
		lcRole := strings.ToLower(role)

		if seen[lcRole] {
			continue
		}

		seen[lcRole] = true
		merged = append(merged, lcRole)
	}

	return merged
}

// sameRoleSet reports whether two role slices contain the same codes,
// ignoring order, case, and duplicates.
func sameRoleSet(current []string, desired []string) bool {
	set := make(map[string]bool, len(current))

	for _, role := range current {
		set[strings.ToLower(role)] = true
	}

	for _, role := range desired {
		lcRole := strings.ToLower(role)

		if !set[lcRole] {
			return false
		}

		delete(set, lcRole)
	}

	return len(set) == 0
}

func checkCoreInfo(vUser vatusa.Controller, zUser zau.User) bool {
	return vUser.FName != zUser.FName || vUser.LName != zUser.LName || vUser.Email != zUser.Email ||
		vUser.BroadcastOptedIn != zUser.FlagBroadcastOptedIn ||
		vUser.NamePrivacyEnabled != zUser.UseNamePrivacy
}

func checkVisitingStatus(vUser vatusa.Controller, zUser zau.User, visiting bool) bool {
	return zUser.IsVisitor != visiting || (visiting && zUser.HomeFacility != vUser.Facility)
}

// PlanACE computes the operations needed to grant the ACE role to ZAU users
// who hold it in VATUSA. It never creates users: a user must already exist in
// the ZAU roster to receive the role. The ACE role is merged into the user's
// existing roles without removing any.
func PlanACE(
	zauRoster zau.Roster,
	aceCIDs []int,
	availableRoles []string,
) []Operation {
	allZauControllers := make([]zau.User, 0,
		len(zauRoster.Home)+len(zauRoster.Visiting)+len(zauRoster.Removed))
	allZauControllers = append(allZauControllers, zauRoster.Home...)
	allZauControllers = append(allZauControllers, zauRoster.Visiting...)
	allZauControllers = append(allZauControllers, zauRoster.Removed...)

	zauByCID := make(map[int]zau.User, len(allZauControllers))
	for _, controller := range allZauControllers {
		if _, exists := zauByCID[controller.CID]; !exists {
			zauByCID[controller.CID] = controller
		}
	}

	if !hasRole(availableRoles, aceRole) {
		return nil
	}

	ops := make([]Operation, 0)

	for _, cid := range aceCIDs {
		zUser, exists := zauByCID[cid]
		if !exists {
			continue
		}

		if hasRole(zUser.RoleCodes, aceRole) {
			continue
		}

		ops = append(ops, Operation{
			Kind:  OpAddACE,
			CID:   cid,
			ZUser: zUser,
			Roles: addNewRoles(zUser.RoleCodes, []string{aceRole}),
		})
	}

	return ops
}

// aceRole is the ZAU role code granted to VATUSA ACE members.
const aceRole = "ace"

// hasRole reports whether role appears in roles as a case-insensitive match.
func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if strings.EqualFold(r, role) {
			return true
		}
	}

	return false
}

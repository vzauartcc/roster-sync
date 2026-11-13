package sync

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/vzauartcc/roster-sync/internal/vatusa"
	"github.com/vzauartcc/roster-sync/internal/zau"
)

// homeMembership is the VATUSA membership value for home controllers.
const homeMembership = "home"

var (
	ErrEmptyVatusaRoster = errors.New("empty VATUSA roster")
	ErrEmptyZauRoster    = errors.New("empty ZAU roster")
)

// Run performs a single roster sync: fetch both rosters, compute the plan,
// and apply it. Fetch failures for the rosters are fatal; a role-fetch
// failure degrades gracefully (the sync proceeds without role updates).
// A fully-empty roster from either source aborts the sync as a safety guard
// against mass changes.
func Run(
	ctx context.Context,
	vatusaClient *vatusa.Client,
	zauClient *zau.Client,
	now time.Time,
) (Result, error) {
	vatusaControllers, err := vatusaClient.FetchRoster(ctx)
	if err != nil {
		return Result{}, err
	}

	zauRoster, err := zauClient.FetchRoster(ctx)
	if err != nil {
		return Result{}, err
	}

	if len(vatusaControllers) == 0 {
		return Result{}, ErrEmptyVatusaRoster
	}

	if len(zauRoster.Home) == 0 && len(zauRoster.Visiting) == 0 && len(zauRoster.Removed) == 0 {
		return Result{}, ErrEmptyZauRoster
	}

	staff, err := vatusaClient.FetchFacilityInfo(ctx)
	if err != nil {
		slog.Warn("failed to fetch staff roster, continuing without staff sync", "error", err)

		staff = &vatusa.FacilityInfo{
			ATM:  0,
			DATM: 0,
			TA:   0,
			EC:   0,
			FE:   0,
			WM:   0,
		}
	}

	availableRoles, err := zauClient.FetchRoles(ctx)
	if err != nil {
		slog.Warn("failed to fetch ZAU roles, continuing without role sync", "error", err)

		availableRoles = []string{}
	}

	ops := Plan(zauRoster, vatusaControllers, availableRoles, *staff, now)

	aceCIDs, err := vatusaClient.FetchACE(ctx)
	if err != nil {
		slog.Warn("failed to fetch VATUSA ACE roster, continuing without ACE sync", "error", err)
	} else {
		ops = append(ops, PlanACE(zauRoster, aceCIDs, availableRoles)...)
	}

	result := apply(ctx, zauClient, ops, now)

	return result, nil
}

func apply(ctx context.Context, zauClient *zau.Client, ops []Operation, now time.Time) Result {
	var result Result

	for _, operation := range ops {
		var err error

		switch operation.Kind {
		case OpCreate:
			vUser := operation.VUser

			err = zauClient.CreateUser(
				ctx,
				vUser.CID,
				vUser.FName,
				vUser.LName,
				vUser.Email,
				vUser.BroadcastOptedIn,
				vUser.NamePrivacyEnabled,
				vUser.Rating,
				vUser.Facility,
				true,
				vUser.Membership != homeMembership,
				vUser.FacilityJoinDate,
				operation.Roles,
			)
			if err == nil {
				result.Added++
			}
		case OpUpdateCore:
			vUser := operation.VUser

			err = zauClient.SetCoreDetails(
				ctx,
				vUser.CID,
				vUser.FName,
				vUser.LName,
				vUser.Email,
				vUser.BroadcastOptedIn,
				vUser.NamePrivacyEnabled,
			)
			if err == nil {
				result.UpdatedCore++
			}
		case OpUpdateMembership:
			vUser := operation.VUser

			err = zauClient.SetHomeController(ctx, vUser.CID, true, vUser.FacilityJoinDate)
			if err == nil {
				result.MadeMember++
			}
		case OpUpdateVisit:
			vUser := operation.VUser

			err = zauClient.SetVisitingController(
				ctx,
				vUser.CID,
				vUser.Membership != homeMembership,
				vUser.Facility,
			)
			if err == nil {
				result.MadeVisitor++
			}
		case OpUpdateRating:
			vUser := operation.VUser

			err = zauClient.SetRating(ctx, vUser.CID, vUser.Rating)
			if err == nil {
				result.UpdatedRating++
			}
		case OpUpdateRoles:
			err = zauClient.SetRoles(ctx, operation.CID, operation.Roles)
			if err == nil {
				result.UpdatedRoles++
			}
		case OpRemoveMember:
			err = zauClient.SetHomeController(ctx, operation.CID, false, now)
			if err == nil {
				result.RemovedMember++
			}
		case OpRemoveCerts:
			err = zauClient.RemoveCerts(ctx, operation.CID)
			if err == nil {
				result.CertsRemoved++
			}
		case OpAddACE:
			err = zauClient.SetRoles(ctx, operation.CID, operation.Roles)
			if err == nil {
				result.ACEGrants++
			}
		}

		if err != nil {
			slog.Error(
				"failed to apply operation",
				"op",
				operation.Kind,
				"cid",
				operation.CID,
				"error",
				err,
			)
		}
	}

	return result
}

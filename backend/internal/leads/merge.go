package leads

import (
	"time"

	"leadflow/internal/models"
)

// Merge combines two records of the same person into the one we keep. "keep" is
// the older, original lead; "drop" is the later duplicate that is being folded in.
//
// Rules:
//   - Anything keep already has wins; gaps are filled from drop.
//   - Consent from either record counts, but an opt-out from either record wins.
//     Honouring a "stop contacting me" is the safe side of the DPDP Act.
//   - The earliest first response and earliest consent time are kept.
//   - The furthest-along status wins (see statusRank).
//
// Identity fields (id, code, phone, source, created_at) are never changed.
func Merge(keep, drop models.Lead) models.Lead {
	out := keep

	if out.ProjectID == nil {
		out.ProjectID = drop.ProjectID
	}
	if out.Wants == nil {
		out.Wants = drop.Wants
	}
	if out.BudgetMinLakh == nil {
		out.BudgetMinLakh = drop.BudgetMinLakh
	}
	if out.BudgetMaxLakh == nil {
		out.BudgetMaxLakh = drop.BudgetMaxLakh
	}
	if out.AssignedTo == nil {
		out.AssignedTo = drop.AssignedTo
	}

	out.FirstResponseAt = earliest(keep.FirstResponseAt, drop.FirstResponseAt)

	out.Consent = keep.Consent || drop.Consent
	out.ConsentAt = earliest(keep.ConsentAt, drop.ConsentAt)
	out.OptedOut = keep.OptedOut || drop.OptedOut
	out.OptedOutAt = earliest(keep.OptedOutAt, drop.OptedOutAt)

	if statusRank(drop.Status) > statusRank(keep.Status) {
		out.Status = drop.Status
	}
	return out
}

// statusRank orders lead stages. A live booking outranks a past visit, because
// "visit booked" is the state a salesperson needs to act on.
func statusRank(status string) int {
	switch status {
	case models.LeadLost:
		return 0
	case models.LeadNew:
		return 1
	case models.LeadEngaged:
		return 2
	case models.LeadVisited:
		return 3
	case models.LeadVisitBooked:
		return 4
	}
	return 0
}

func earliest(a, b *time.Time) *time.Time {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case b.Before(*a):
		return b
	}
	return a
}

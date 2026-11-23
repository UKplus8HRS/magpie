package usage

// A subscription's calls at its plan's equivalent price (provider.PlanCost)
// beside the official one. A plan's quota is a week's, so its calls are
// summed by week as the summary counts them: the first quota's worth of a
// week's API cost is at the plan's k, the rest past it at the official
// price. magpie reads no per-call sign that a call was billed from credits
// or pay as you go past a plan (Record.Credits is a plan's own credits,
// not overage), so the week's running sum is what tells overage apart. A
// week starts on Monday at 00:00 in the summary's zone; a period that
// starts mid-week counts the calls of that week before it first
// (seedPlanWeeks), so a "Today" past the quota is past it.

import (
	"time"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// planShare is a call's cost at its provider's plan-equivalent price,
// set only when the provider has a plan.
type planShare struct {
	cost float64
	api  float64
	on   bool
}

// addPlan adds r to t and, when p is set, counts its plan-equivalent cost
// alongside its official API cost.
func (t *Totals) addPlan(r Record, pr *catalog.Price, p planShare) {
	t.add(r, pr)
	if p.on {
		t.PlanCost += p.cost
		t.PlanAPI += p.api
		t.Planned++
	}
}

// planWeeks is the API cost each plan's calls ran up in each week so far,
// by provider and the week's Monday.
type planWeeks map[string]float64

// weekStart is the Monday 00:00 of t's week, in t's zone.
func weekStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d-(int(t.Weekday())+6)%7, 0, 0, 0, 0, t.Location())
}

func planWeekKey(prov string, t time.Time) string {
	return prov + "|" + weekStart(t).Format(time.DateOnly)
}

// planPrices is the plan of each provider that has one, by its id.
func planPrices() map[string]*provider.PlanPrice {
	out := map[string]*provider.PlanPrice{}
	for _, p := range provider.All() {
		if p.Plan != nil && p.Plan.Factor() != 1 {
			out[p.ID] = p.Plan
		}
	}
	return out
}

// share is r's plan share, at r's place in its week, and counts r into
// the week: a call of no plan, no price or no tokens has none. loc is the
// zone the weeks are read in.
func (w planWeeks) share(plans map[string]*provider.PlanPrice, r Record, pr *catalog.Price, loc *time.Location) planShare {
	plan := plans[r.Provider]
	if plan == nil || pr == nil || r.Input+r.Output == 0 {
		return planShare{}
	}
	api := r.CostAt(*pr)
	k := planWeekKey(r.Provider, r.Time.In(loc))
	used := w[k]
	w[k] = used + api
	return planShare{cost: provider.PlanCost(plan, api, used), api: api, on: true}
}

// seedPlanWeeks is the API cost each plan ran up in from's week before
// from: what a period that starts mid-week counts its quota on from.
func (u *summaryRun) seedPlanWeeks() planWeeks {
	w := planWeeks{}
	if len(u.plans) == 0 || u.from.IsZero() {
		return w
	}
	loc := u.now.Location()
	from := u.from.In(loc)
	start := weekStart(from)
	if !start.Before(from) {
		return w
	}
	Visit(start, func(r Record) {
		if r.Time.Before(start) || !r.Time.Before(from) {
			return
		}
		r.renameBy(u.renamed)
		if r.IsRejected() || u.cleared.hides(&r) || u.plans[r.Provider] == nil {
			return
		}
		w.share(u.plans, r, u.priceOf(r), loc)
	})
	return w
}

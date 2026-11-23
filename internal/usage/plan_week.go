package usage

// Calls priced on a plan consume a share of one weekly allowance per
// provider. Each model has its own API USD value for the full allowance.
// Record.Credits gives plan credit units, so it cannot identify overage.

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

// planWeeks is the fraction used, by provider and the week's Monday.
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
		if p.Plan != nil {
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
	model := r.Served
	if model == "" {
		model = r.Model
	}
	if _, quota := plan.Weekly(model); quota <= 0 {
		return planShare{}
	}
	api := r.CostAt(*pr)
	k := planWeekKey(r.Provider, r.Time.In(loc))
	used := w[k]
	cost, fraction := provider.PlanCost(plan, model, api, used)
	w[k] = min(1, used+fraction)
	return planShare{cost: cost, api: api, on: true}
}

// seedPlanWeeks counts the fraction used before a summary's midweek start.
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

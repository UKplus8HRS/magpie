package provider

import (
	"maps"
	"math"

	"github.com/yetone/magpie/internal/catalog"
)

// WeeksPerMonth turns a plan's price or quota per month into one per week.
const WeeksPerMonth = 4.33

// PlanPrice holds what a provider's subscription costs and the API USD
// value of its full allowance when spent entirely on a given model.
type PlanPrice struct {
	Price    float64            `json:"price"`
	PricePer string             `json:"pricePer,omitempty"` // month (default) or week
	Quota    float64            `json:"quota,omitempty"`    // fallback when no model override matches
	QuotaPer string             `json:"quotaPer,omitempty"` // week (default) or month, for all quotas
	Models   map[string]float64 `json:"models,omitempty"`
}

const (
	PlanWeek  = "week"
	PlanMonth = "month"
)

func perWeek(v float64, per, def string) float64 {
	if per == "" {
		per = def
	}
	if per == PlanMonth {
		return v / WeeksPerMonth
	}
	return v
}

// Weekly returns the plan's weekly price and the model's weekly quota in
// API USD. A zero quota means that the call is outside this estimate.
func (p *PlanPrice) Weekly(model string) (price, quota float64) {
	if p == nil {
		return 0, 0
	}
	q := p.Quota
	if v, ok := p.Models[catalog.SameModel(model)]; ok {
		q = v
	}
	return perWeek(p.Price, p.PricePer, PlanMonth), perWeek(q, p.QuotaPer, PlanWeek)
}

// Factor is the price per API dollar for a model's in-quota calls.
func (p *PlanPrice) Factor(model string) float64 {
	if p == nil || CheckPlanPrice(*p) != "" {
		return 1
	}
	price, quota := p.Weekly(model)
	if quota <= 0 {
		return 1
	}
	return price / quota
}

// PlanCost returns the cost and the fraction of the provider's shared
// weekly allowance used by a call. used is the fraction used before it.
// Calls with no quota consume none. A call crossing 100% is split at the cap.
func PlanCost(p *PlanPrice, model string, apiCost, used float64) (cost, fraction float64) {
	price, quota := p.Weekly(model)
	if quota <= 0 || apiCost <= 0 {
		return apiCost, 0
	}
	within := min(apiCost, max(0, 1-used)*quota)
	return (within/quota)*price + (apiCost - within), min(1, apiCost/quota)
}

// Empty reports the zero-value payload used to remove a saved plan.
func (p PlanPrice) Empty() bool {
	return p.Price == 0 && p.PricePer == "" && p.Quota == 0 && p.QuotaPer == "" && len(p.Models) == 0
}

// CheckPlanPrice validates a plan as submitted before normalization.
func CheckPlanPrice(p PlanPrice) string {
	switch {
	case math.IsNaN(p.Price) || math.IsInf(p.Price, 0) || p.Price < 0:
		return "a plan's price is 0 or more"
	case math.IsNaN(p.Quota) || math.IsInf(p.Quota, 0) || p.Quota < 0:
		return "a plan's default quota is 0 or more in API USD"
	case !planPer(p.PricePer) || !planPer(p.QuotaPer):
		return "a plan's price and quota are per week or per month"
	case p.Quota == 0 && len(p.Models) == 0:
		return "a plan needs a default quota or a model quota"
	}
	seen := make(map[string]bool, len(p.Models))
	for model, q := range p.Models {
		id := catalog.SameModel(model)
		if id == "" || seen[id] {
			return "plan model quotas need distinct model IDs"
		}
		seen[id] = true
		if math.IsNaN(q) || math.IsInf(q, 0) || perWeek(q, p.QuotaPer, PlanWeek) <= 0 {
			return "a plan's model quota is more than 0 in API USD"
		}
	}
	if p.Quota > 0 && perWeek(p.Quota, p.QuotaPer, PlanWeek) == 0 {
		return "a plan's default quota is too small"
	}
	return ""
}

func planPer(s string) bool { return s == "" || s == PlanWeek || s == PlanMonth }

func (p PlanPrice) clone() *PlanPrice {
	p.Models = maps.Clone(p.Models)
	return &p
}

// normalPlan stores model quotas under the catalog's canonical spelling.
func normalPlan(p *PlanPrice) *PlanPrice {
	if p == nil || CheckPlanPrice(*p) != "" {
		return nil
	}
	c := *p
	if len(p.Models) != 0 {
		c.Models = make(map[string]float64, len(p.Models))
		for model, q := range p.Models {
			c.Models[catalog.SameModel(model)] = q
		}
	}
	return &c
}

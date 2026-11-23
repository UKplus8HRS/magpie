package provider

// A plan's equivalent price: what a subscription's calls really cost the
// user, beside what the official API would charge for them. A coding plan
// (Claude Max, a ChatGPT plan, Gemini, Kimi, GLM…) costs a fixed price for
// a quota the user measured in API dollars ("the Gemini plan's weekly quota
// is worth $60 at API prices"), so each API dollar of it costs
//
//	k = plan price per week / full weekly quota in API USD
//
// and a call's plan-equivalent cost is its API cost times k. A month is
// WeeksPerMonth weeks. One k per provider: every model of it shares it.
// Within each week the first quota's worth of API cost is at k; anything
// above it is overage, billed from credits or pay as you go, so it is at
// the official price (k = 1).

import (
	"math"
)

// WeeksPerMonth turns a plan's price or quota per month into one per week.
const WeeksPerMonth = 4.33

// PlanPrice is what a subscription costs the user, for the plan-equivalent
// price Usage shows beside the official one. Price is in USD per PricePer;
// Quota is the plan's full quota per QuotaPer, measured in USD at the
// official API price (what the user entered, not what they used).
type PlanPrice struct {
	Price float64 `json:"price"`
	// PricePer is PlanMonth (the default, "") or PlanWeek
	PricePer string  `json:"pricePer,omitempty"`
	Quota    float64 `json:"quota"`
	// QuotaPer is PlanWeek (the default, "") or PlanMonth
	QuotaPer string `json:"quotaPer,omitempty"`
}

// The periods a plan's price and quota are entered per.
const (
	PlanWeek  = "week"
	PlanMonth = "month"
)

// perWeek is v, entered per per, as an amount per week; def is the period
// "" stands for.
func perWeek(v float64, per, def string) float64 {
	if per == "" {
		per = def
	}
	if per == PlanMonth {
		return v / WeeksPerMonth
	}
	return v
}

// Weekly is the plan's price and full quota per week, in USD.
func (p *PlanPrice) Weekly() (price, quota float64) {
	if p == nil {
		return 0, 0
	}
	return perWeek(p.Price, p.PricePer, PlanMonth), perWeek(p.Quota, p.QuotaPer, PlanWeek)
}

// Factor is k: what one API dollar of the plan's quota costs the user. 1
// for no plan, or one that doesn't check (CheckPlanPrice): such a
// provider's calls stay at the official price.
func (p *PlanPrice) Factor() float64 {
	if p == nil || CheckPlanPrice(*p) != "" {
		return 1
	}
	price, quota := p.Weekly()
	return price / quota
}

// PlanCost is what a call of apiCost (USD at the official price) costs at
// the plan's equivalent price, used being the API cost of the plan's calls
// before it in the same week: the part within the week's quota at k, the
// part past it at the official price. No plan is apiCost.
func PlanCost(p *PlanPrice, apiCost, used float64) float64 {
	k := p.Factor()
	if k == 1 || apiCost <= 0 {
		return apiCost
	}
	_, quota := p.Weekly()
	within := math.Max(0, math.Min(apiCost, quota-used))
	return within*k + (apiCost - within)
}

// CheckPlanPrice says what is wrong with a plan's price, "" when nothing:
// a price of 0 or more, a quota above 0, each per a week or a month.
func CheckPlanPrice(p PlanPrice) string {
	switch {
	case math.IsNaN(p.Price) || math.IsInf(p.Price, 0) || p.Price < 0:
		return "a plan's price is 0 or more"
	case math.IsNaN(p.Quota) || math.IsInf(p.Quota, 0) || p.Quota <= 0:
		return "a plan's quota is more than 0 USD at API prices"
	case !planPer(p.PricePer) || !planPer(p.QuotaPer):
		return "a plan's price and quota are per week or per month"
	}
	return ""
}

func planPer(s string) bool { return s == "" || s == PlanWeek || s == PlanMonth }

// normalPlan is p as providers.json keeps it: nil for none or one that
// doesn't check, so a hand-edited file can't price calls at nonsense.
func normalPlan(p *PlanPrice) *PlanPrice {
	if p == nil || CheckPlanPrice(*p) != "" {
		return nil
	}
	c := *p
	return &c
}

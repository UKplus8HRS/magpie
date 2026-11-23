package provider

import (
	"encoding/json"
	"testing"
)

func TestPlanFactor(t *testing.T) {
	cases := []struct {
		name string
		plan *PlanPrice
		want float64
	}{
		{"no plan", nil, 1},
		// $20 a month for a quota worth $60 a week at API prices
		{"monthly price, weekly quota", &PlanPrice{Price: 20, PricePer: PlanMonth, Quota: 60, QuotaPer: PlanWeek}, 20 / 4.33 / 60},
		{"the defaults are a monthly price and a weekly quota", &PlanPrice{Price: 20, Quota: 60}, 20 / 4.33 / 60},
		{"weekly price, weekly quota", &PlanPrice{Price: 5, PricePer: PlanWeek, Quota: 50}, 0.1},
		{"monthly price, monthly quota", &PlanPrice{Price: 200, PricePer: PlanMonth, Quota: 2000, QuotaPer: PlanMonth}, 0.1},
		{"weekly price, monthly quota", &PlanPrice{Price: 10, PricePer: PlanWeek, Quota: 433, QuotaPer: PlanMonth}, 0.1},
		{"a free plan", &PlanPrice{Price: 0, Quota: 60}, 0},
		{"no quota is no plan", &PlanPrice{Price: 20}, 1},
		{"an unknown period is no plan", &PlanPrice{Price: 20, PricePer: "day", Quota: 60}, 1},
		{"a negative price is no plan", &PlanPrice{Price: -1, Quota: 60}, 1},
	}
	for _, c := range cases {
		if got := c.plan.Factor(); !near(got, c.want) {
			t.Errorf("%s: k = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPlanCostSplitsAtTheWeeklyQuota(t *testing.T) {
	// $5 a week for $50 of API usage a week: k = 0.1
	p := &PlanPrice{Price: 5, PricePer: PlanWeek, Quota: 50}
	cases := []struct {
		name      string
		api, used float64
		want      float64
	}{
		{"well within the quota", 10, 0, 1},
		{"up to the quota exactly", 10, 40, 1},
		{"across the quota: $4 within, $6 over", 10, 46, 0.4 + 6},
		{"past the quota: all at the official price", 10, 50, 10},
		{"long past it", 10, 500, 10},
		{"nothing", 0, 10, 0},
	}
	for _, c := range cases {
		if got := PlanCost(p, c.api, c.used); !near(got, c.want) {
			t.Errorf("%s: PlanCost(%v, used %v) = %v, want %v", c.name, c.api, c.used, got, c.want)
		}
	}
	if got := PlanCost(nil, 10, 1000); got != 10 {
		t.Errorf("no plan: PlanCost = %v, want the API cost 10", got)
	}
}

func TestPlanCostMonthlyQuotaCapsEachWeek(t *testing.T) {
	// a quota entered per month caps each week at a 4.33rd of it
	p := &PlanPrice{Price: 4.33, PricePer: PlanMonth, Quota: 433, QuotaPer: PlanMonth} // $1 for $100 a week
	if got := PlanCost(p, 20, 90); !near(got, 10*0.01+10) {
		t.Errorf("PlanCost = %v, want $10 at k=0.01 and $10 over", got)
	}
}

func TestPlanPriceKeptInProvidersJSON(t *testing.T) {
	in := Provider{ID: "gemini", Plan: &PlanPrice{Price: 20, PricePer: PlanMonth, Quota: 60, QuotaPer: PlanWeek}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Provider
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Plan == nil || *out.Plan != *in.Plan {
		t.Fatalf("plan read back as %+v from %s", out.Plan, b)
	}
	// a hand-edited plan that doesn't check is dropped, not priced at
	if got := normalize(Provider{ID: "x", Plan: &PlanPrice{Price: 20}}).Plan; got != nil {
		t.Errorf("a plan with no quota normalized to %+v, want none", got)
	}
	if got := normalize(in).Plan; got == nil || *got != *in.Plan {
		t.Errorf("a good plan normalized to %+v", got)
	}
}

func TestAccountKeepsItsPlan(t *testing.T) {
	plan := &PlanPrice{Price: 200, Quota: 1500}
	a := accountSettings(Provider{ID: "claude"}, Provider{ID: "claude", Plan: plan})
	if a.Plan == nil || *a.Plan != *plan {
		t.Fatalf("a signed-in account's plan = %+v, want %+v", a.Plan, plan)
	}
}

package usage

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// planHome is a HOME with sol at $2 a million tokens in, a provider "plan"
// on a plan of $5 a week for $10 of API usage a week (k = 0.5), and one
// "payg" with none.
func planHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{"anthropic":{"id":"anthropic","models":{"sol":{"id":"sol","cost":{"input":2,"output":8}},"opus":{"id":"opus","cost":{"input":2,"output":8}},"fable":{"id":"fable","cost":{"input":2,"output":8}}}}}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	for _, p := range []provider.Provider{
		{ID: "plan", Name: "Plan", Key: "sk-plan", Chat: "https://plan.example/v1", Plan: &provider.PlanPrice{Price: 5, PricePer: provider.PlanWeek, Quota: 10, QuotaPer: provider.PlanWeek}},
		{ID: "payg", Name: "PAYG", Key: "sk-payg", Chat: "https://payg.example/v1"},
	} {
		if err := provider.Save(p); err != nil {
			t.Fatal(err)
		}
	}
}

// a $4 call (2M tokens in at $2) to prov at at
func planCall(prov string, at time.Time) Record {
	return Record{Time: at, Agent: "codex", Provider: prov, Model: "sol", Input: 2e6, Status: 200}
}

func nearUSD(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// A plan's calls count at k up to the week's quota and at the official
// price past it; a call to a provider of no plan stays at the official
// price, and a new week starts the quota again.
func TestSummaryPlanCostSplitsAtTheWeeklyQuota(t *testing.T) {
	planHome(t)
	loc := time.Local
	mon := time.Date(2026, 11, 16, 9, 0, 0, 0, loc) // a Monday
	recs := []Record{
		planCall("plan", mon),                  // used $0: $4 at 0.5 = $2
		planCall("plan", mon.Add(time.Hour)),   // used $4: $2
		planCall("payg", mon.Add(2*time.Hour)), // no plan: $4, not planned
		planCall("plan", mon.Add(3*time.Hour)), // used $8: $2 within at 0.5 + $2 over = $3
		planCall("plan", mon.Add(4*time.Hour)), // used $12: all over, $4
		planCall("plan", mon.AddDate(0, 0, 7)), // the next Monday: $2 again
	}
	now := mon.AddDate(0, 0, 8)
	s := summarize(All, now, recs)
	if !nearUSD(s.Cost, 24) {
		t.Fatalf("official cost $%v, want $24", s.Cost)
	}
	if s.Planned != 5 || !nearUSD(s.PlanAPI, 20) || !nearUSD(s.PlanCost, 2+2+3+4+2) {
		t.Errorf("plan: %d calls, API $%v, plan $%v; want 5, $20, $13", s.Planned, s.PlanAPI, s.PlanCost)
	}
	for _, g := range s.Models {
		switch g.Provider {
		case "payg":
			if g.Planned != 0 || g.PlanCost != 0 {
				t.Errorf("a provider with no plan has a plan cost: %+v", g.Totals)
			}
		case "plan":
			if !nearUSD(g.PlanCost, 13) {
				t.Errorf("the plan's model: plan $%v, want $13", g.PlanCost)
			}
		}
	}
}

// A period that starts mid-week counts its quota on from the calls of
// that week before it: a Wednesday past the quota since Monday is at the
// official price.
func TestSummaryPlanCostCountsTheWeekBeforeThePeriod(t *testing.T) {
	planHome(t)
	loc := time.Local
	mon := time.Date(2026, 11, 16, 9, 0, 0, 0, loc)
	wed := mon.AddDate(0, 0, 2)
	for _, r := range []Record{planCall("plan", mon), planCall("plan", mon.Add(time.Hour))} {
		Append(r)
	}
	today := planCall("plan", wed.Add(time.Hour)) // $8 used before it: $1 + $2 over
	s := summarize(Today, wed.Add(2*time.Hour), []Record{today})
	if s.Planned != 1 || !nearUSD(s.PlanCost, 3) {
		t.Errorf("Wednesday's call: %d planned at $%v, want 1 at $3 (Monday's $8 of the $10 quota counted)", s.Planned, s.PlanCost)
	}
}

func TestSummaryPlanModelsShareWeeklyQuota(t *testing.T) {
	planHome(t)
	if err := provider.Save(provider.Provider{ID: "plan", Name: "Plan", Key: "sk-plan", Chat: "https://plan.example/v1", Plan: &provider.PlanPrice{
		Price: 5, PricePer: provider.PlanWeek, Models: map[string]float64{"opus": 8, "fable": 6},
	}}); err != nil {
		t.Fatal(err)
	}
	mon := time.Date(2026, 11, 16, 9, 0, 0, 0, time.Local)
	wed := mon.AddDate(0, 0, 2)
	opus := planCall("plan", mon)
	opus.Model = "opus" // $4 consumes half the $8 Opus allowance
	Append(opus)
	unconfigured := planCall("plan", wed)
	fable := planCall("plan", wed.Add(time.Hour))
	fable.Model = "fable" // $3 of its $4 fit: half the $5 plan price + $1 overage
	s := summarize(Today, wed.Add(2*time.Hour), []Record{unconfigured, fable})
	if s.Planned != 1 || !nearUSD(s.Cost, 8) || !nearUSD(s.PlanAPI, 4) || !nearUSD(s.PlanCost, 3.5) {
		t.Fatalf("Wednesday after half the allowance was used by Opus: %d planned, API $%v ($%v on the plan), plan $%v", s.Planned, s.Cost, s.PlanAPI, s.PlanCost)
	}
}

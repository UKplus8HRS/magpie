package provider

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/plugin"
)

func TestPluginAsideWindowNamesItsModelsForDisplay(t *testing.T) {
	const hydrogen = "gemini-4-hydrogen"
	models := []string{hydrogen, hydrogen + "-xhigh", "gemini-3.8-flash"}
	q := quotaOfPlugin(SubscriptionQuota{Provider: "agy-cli"}, plugin.Usage{Windows: []plugin.UsageWindow{
		{Name: "Gemini · 7 days", Used: fptr(4), Aside: true, Models: models},
		{Name: "Gemini · 5 hours", Aside: true, Models: models, Display: "Limits not available"},
	}})
	if any, ok := routedAllowance("agy-cli", "", q, time.Now()); ok || len(any) != 0 {
		t.Fatalf("display-only windows weighed in routing: %+v", any)
	}
	data, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	var card SubscriptionQuota
	if err := json.Unmarshal(data, &card); err != nil {
		t.Fatal(err)
	}
	if len(card.Windows) != 2 || !slices.Equal(card.Windows[0].DisplayModels, models) || len(card.Windows[1].DisplayModels) != 0 {
		t.Fatalf("display model scopes: %s", data)
	}
}

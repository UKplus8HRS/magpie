package gui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

func TestProviderSaveModelQuotas(t *testing.T) {
	sandboxHome(t)
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	post := func(fields string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		body := `{"id":"plan","name":"Plan","chat":"http://127.0.0.1:1/v1"` + fields + `}`
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/provider/save", strings.NewReader(body)))
		return w
	}
	if w := post(`,"new":true,"plan":{"price":100,"quotaPer":"month","models":{"Anthropic/Claude Opus 5.5":4640}}`); w.Code != 200 {
		t.Fatalf("save model-only plan: %d %s", w.Code, w.Body)
	}
	checkPlan := func() {
		t.Helper()
		p, err := provider.Find("plan")
		if err != nil || p.Plan == nil || p.Plan.Quota != 0 || p.Plan.Models["claude-opus-5-5"] != 4640 {
			t.Fatalf("saved model quotas: %v, %v", p, err)
		}
	}
	checkPlan()
	if w := post(`,"plan":{"price":100,"models":{"Claude Opus 5.5":1,"claude-opus-5-5":1}}`); w.Code < 400 {
		t.Fatalf("save accepted duplicate model IDs: %d %s", w.Code, w.Body)
	}
	if w := post(""); w.Code != 200 {
		t.Fatalf("save omitting plan: %d %s", w.Code, w.Body)
	}
	checkPlan()
	if w := post(`,"plan":{}`); w.Code != 200 {
		t.Fatalf("clear plan: %d %s", w.Code, w.Body)
	}
	if p, err := provider.Find("plan"); err != nil || p.Plan != nil {
		t.Fatalf("plan remains after clear: %v, %v", p, err)
	}
}

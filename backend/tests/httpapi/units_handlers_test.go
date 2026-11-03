package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestCatalogueListsAvailableUnitsWithFacts(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	code, sold := "Z-"+randDigits(4), "Z-"+randDigits(4)
	if _, err := testPool.Exec(ctx, `
		INSERT INTO units (code, project_id, bhk, layout, tower, floor, area_sqft, facing, indicative_price_lakh, available)
		VALUES ($1, 'project-1', 3, '3bhk', 'Z', 9, 1640, 'East', 108, true),
		       ($2, 'project-1', 2, '2bhk', 'Z', 4, 1120, 'North', 72, false)`, code, sold); err != nil {
		t.Fatal(err)
	}

	list := call(t, http.MethodGet, "/api/units", nil, nil)
	wantStatus(t, list, http.StatusOK)
	if !strings.Contains(list.Body.String(), `"code":"`+code+`"`) || strings.Contains(list.Body.String(), sold) {
		t.Errorf("catalogue must list available flats only: %s", list.Body.String())
	}

	one := call(t, http.MethodGet, "/api/units/"+code, nil, nil)
	wantStatus(t, one, http.StatusOK)
	for _, want := range []string{`"layout":"3bhk"`, `"reraNumber":"TEST-RERA"`, `"latitude"`, `"facts"`} {
		if !strings.Contains(one.Body.String(), want) {
			t.Errorf("unit page missing %s: %s", want, one.Body.String())
		}
	}
	wantStatus(t, call(t, http.MethodGet, "/api/units/"+sold, nil, nil), http.StatusNotFound)
	wantStatus(t, call(t, http.MethodGet, "/api/units/not-a-code", nil, nil), http.StatusNotFound)
}

func randDigits(n int) string {
	const digits = "0123456789"
	h := randHex(n)
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = digits[int(h[i])%10]
	}
	return string(out)
}

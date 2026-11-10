package sheets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recorded struct {
	method string
	path   string
	query  string
	values [][]string
}

func newTestClient(t *testing.T, columnA [][]string, status int) (*Client, *[]recorded) {
	t.Helper()
	var calls []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recorded{method: r.Method, path: r.URL.EscapedPath(), query: r.URL.RawQuery}
		if r.Body != nil {
			var body struct {
				Values [][]string `json:"values"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.values = body.Values
		}
		calls = append(calls, rec)
		if status != http.StatusOK {
			http.Error(w, `{"error":{"message":"denied"}}`, status)
			return
		}
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{"values": columnA})
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return &Client{http: srv.Client(), baseURL: srv.URL, sheetID: "SHEET", sheetName: "Sheet1"}, &calls
}

func TestUpsertUpdatesExistingRowCaseInsensitive(t *testing.T) {
	c, calls := newTestClient(t, [][]string{{"file_name"}, {"other.mp4"}, {" EP03.mp4 "}}, http.StatusOK)

	err := c.Upsert(context.Background(), Row{FileName: "ep03.mp4", Title: "Judul", Description: "Desk", Hashtag: "#a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(*calls))
	}
	put := (*calls)[1]
	if put.method != http.MethodPut || !strings.HasSuffix(put.path, "/SHEET/values/Sheet1%21A3:D3") || put.query != "valueInputOption=RAW" {
		t.Fatalf("unexpected update call: %+v", put)
	}
	if got := put.values[0]; strings.Join(got, "|") != "ep03.mp4|Judul|Desk|#a" {
		t.Fatalf("unexpected values: %v", got)
	}
}

func TestUpsertAppendsWhenMissing(t *testing.T) {
	c, calls := newTestClient(t, [][]string{{"file_name"}, {"other.mp4"}}, http.StatusOK)

	if err := c.Upsert(context.Background(), Row{FileName: "new.mp4", Title: "=1+1"}); err != nil {
		t.Fatal(err)
	}
	app := (*calls)[1]
	if app.method != http.MethodPost || !strings.HasSuffix(app.path, "/values/Sheet1%21A:D:append") {
		t.Fatalf("unexpected append call: %+v", app)
	}
	if !strings.Contains(app.query, "valueInputOption=RAW") || !strings.Contains(app.query, "insertDataOption=INSERT_ROWS") {
		t.Fatalf("unexpected append query: %s", app.query)
	}
	if app.values[0][1] != "=1+1" {
		t.Fatalf("title should be sent verbatim, got %q", app.values[0][1])
	}
}

func TestUpsertDoesNotMatchHeaderRow(t *testing.T) {
	c, calls := newTestClient(t, [][]string{{"file_name"}}, http.StatusOK)

	if err := c.Upsert(context.Background(), Row{FileName: "file_name"}); err != nil {
		t.Fatal(err)
	}
	if (*calls)[1].method != http.MethodPost {
		t.Fatalf("header row must not be overwritten: %+v", (*calls)[1])
	}
}

func TestUpsertReturnsGoogleError(t *testing.T) {
	c, _ := newTestClient(t, nil, http.StatusForbidden)

	err := c.Upsert(context.Background(), Row{FileName: "x.mp4"})
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected 403 error with body, got %v", err)
	}
}

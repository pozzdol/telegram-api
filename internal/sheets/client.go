package sheets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
)

const (
	defaultBaseURL = "https://sheets.googleapis.com/v4/spreadsheets"
	scope          = "https://www.googleapis.com/auth/spreadsheets"
)

// Row matches the sheet columns: file_name | judul | deskripsi | hashtag.
type Row struct {
	FileName    string
	Title       string
	Description string
	Hashtag     string
}

type Client struct {
	http      *http.Client
	baseURL   string
	sheetID   string
	sheetName string
}

func New(ctx context.Context, credentialsFile, sheetID, sheetName string) (*Client, error) {
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read service account file: %w", err)
	}
	cfg, err := google.JWTConfigFromJSON(data, scope)
	if err != nil {
		return nil, fmt.Errorf("parse service account file: %w", err)
	}
	return &Client{
		http:      cfg.Client(ctx),
		baseURL:   defaultBaseURL,
		sheetID:   sheetID,
		sheetName: sheetName,
	}, nil
}

// Upsert overwrites the row whose file_name matches (case-insensitive, like the n8n lookup),
// otherwise appends a new row.
func (c *Client) Upsert(ctx context.Context, r Row) error {
	rowNum, err := c.findRow(ctx, r.FileName)
	if err != nil {
		return err
	}

	body := map[string]any{"values": [][]string{{r.FileName, r.Title, r.Description, r.Hashtag}}}
	if rowNum > 0 {
		rng := fmt.Sprintf("%s!A%d:D%d", c.sheetName, rowNum, rowNum)
		return c.do(ctx, http.MethodPut, c.valuesURL(rng, "")+"?valueInputOption=RAW", body, nil)
	}
	return c.do(ctx, http.MethodPost, c.valuesURL(c.sheetName+"!A:D", ":append")+"?valueInputOption=RAW&insertDataOption=INSERT_ROWS", body, nil)
}

// findRow returns the 1-based row number of fileName in column A, or 0 if absent. Row 1 is the header.
func (c *Client) findRow(ctx context.Context, fileName string) (int, error) {
	var res struct {
		Values [][]string `json:"values"`
	}
	if err := c.do(ctx, http.MethodGet, c.valuesURL(c.sheetName+"!A:A", ""), nil, &res); err != nil {
		return 0, err
	}
	want := strings.ToLower(strings.TrimSpace(fileName))
	for i, row := range res.Values {
		if i == 0 || len(row) == 0 {
			continue
		}
		if strings.ToLower(strings.TrimSpace(row[0])) == want {
			return i + 1, nil
		}
	}
	return 0, nil
}

func (c *Client) valuesURL(rng, suffix string) string {
	return fmt.Sprintf("%s/%s/values/%s%s", c.baseURL, url.PathEscape(c.sheetID), url.PathEscape(rng), suffix)
}

func (c *Client) do(ctx context.Context, method, u string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("google sheets request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("google sheets %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

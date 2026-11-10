package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateVideoMetadata(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	cases := []struct {
		name, file, title, desc, tag string
		wantErr                      string
	}{
		{"ok", "ep03.mp4", "Judul", "Deskripsi", "#a", ""},
		{"uppercase extension", "EP03.MOV", "Judul", "", "", ""},
		{"not a video", "notes.pdf", "Judul", "", "", "must be a video"},
		{"missing title", "a.mp4", "", "", "", "judul is required"},
		{"title at limit", "a.mp4", long(100), "", "", ""},
		{"title too long", "a.mp4", long(101), "", "", "at most 100"},
		{"title multibyte at limit", "a.mp4", strings.Repeat("é", 100), "", "", ""},
		{"angle brackets", "a.mp4", "<b>x</b>", "", "", "< or >"},
		{"caption at limit", "a.mp4", "T", long(2200 - 1 - 2 - 2 - 1), "#", ""},
		{"caption too long", "a.mp4", "T", long(2200), "", "at most 2200"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateVideoMetadata(tc.file, tc.title, tc.desc, tc.tag)
			if tc.wantErr == "" && got != "" {
				t.Fatalf("expected valid, got %q", got)
			}
			if tc.wantErr != "" && !strings.Contains(got, tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, got)
			}
		})
	}
}

func TestUploadVideoWithoutSheetReturns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/upload-video", NewFileHandler(nil, nil).UploadVideo)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/upload-video", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

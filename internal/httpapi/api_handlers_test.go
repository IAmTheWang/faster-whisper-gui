package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"faster-whisper-gui/internal/fsbrowse"
	"faster-whisper-gui/internal/settings"
	"faster-whisper-gui/internal/transcribe"
)

// apiTestMux registers s's routes on a fresh mux, the way appserver does, so
// the tests also cover routing (method+pattern, catch-all), not just handlers.
func apiTestMux(s *Server) *http.ServeMux {
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	return mux
}

func apiTestGet(h http.Handler, target string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, target, nil))
	return rr
}

func apiTestDecode(t *testing.T, rr *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), v); err != nil {
		t.Fatalf("response is not the expected JSON (%v): %s", err, rr.Body.String())
	}
}

// apiTestErrorBody asserts rr is a JSON error with the given status and
// returns its message.
func apiTestErrorBody(t *testing.T, rr *httptest.ResponseRecorder, wantStatus int) string {
	t.Helper()
	if rr.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body %s", rr.Code, wantStatus, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var e errorResponse
	apiTestDecode(t, rr, &e)
	if e.Error == "" {
		t.Errorf("error body has an empty message: %s", rr.Body.String())
	}
	return e.Error
}

func TestHandleDrives(t *testing.T) {
	s, _ := newCreateJobServer(t)
	rr := apiTestGet(apiTestMux(s), "/api/drives")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var drives []fsbrowse.Drive
	apiTestDecode(t, rr, &drives)
	if len(drives) == 0 {
		t.Fatal("no drives returned")
	}
	re := regexp.MustCompile(`^[A-Z]:\\$`)
	for _, d := range drives {
		if !re.MatchString(d.Name) {
			t.Errorf("drive name %q, want a root like C:\\", d.Name)
		}
	}
}

// browseFixture builds a directory holding one of everything the listing
// filters care about.
func browseFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"a.mp4":     "video",
		"b.mp3":     "audio!",
		"tool.exe":  "exe",
		"notes.txt": "text",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func browseURL(path, kind string) string {
	q := url.Values{"path": {path}}
	if kind != "" {
		q.Set("kind", kind)
	}
	return "/api/browse?" + q.Encode()
}

// entryTypes maps entry name -> type for easy comparison.
func entryTypes(l fsbrowse.Listing) map[string]string {
	m := map[string]string{}
	for _, e := range l.Entries {
		m[e.Name] = e.Type
	}
	return m
}

func TestHandleBrowse_Kinds(t *testing.T) {
	s, _ := newCreateJobServer(t)
	mux := apiTestMux(s)
	dir := browseFixture(t)

	tests := []struct {
		name string
		kind string
		want map[string]string
	}{
		{"default is media", "", map[string]string{"sub": "dir", "a.mp4": "video", "b.mp3": "audio"}},
		{"explicit media", "media", map[string]string{"sub": "dir", "a.mp4": "video", "b.mp3": "audio"}},
		{"exe", "exe", map[string]string{"sub": "dir", "tool.exe": "file"}},
		{"dir lists directories only", "dir", map[string]string{"sub": "dir"}},
		{"unknown kind falls back to media", "bogus", map[string]string{"sub": "dir", "a.mp4": "video", "b.mp3": "audio"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := apiTestGet(mux, browseURL(dir, tt.kind))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
			}
			var l fsbrowse.Listing
			apiTestDecode(t, rr, &l)
			if l.Path != dir {
				t.Errorf("listing path = %q, want %q", l.Path, dir)
			}
			if l.Parent != filepath.Dir(dir) {
				t.Errorf("listing parent = %q, want %q", l.Parent, filepath.Dir(dir))
			}
			got := entryTypes(l)
			if len(got) != len(tt.want) {
				t.Fatalf("entries = %v, want %v", got, tt.want)
			}
			for name, typ := range tt.want {
				if got[name] != typ {
					t.Errorf("entry %q has type %q, want %q (all: %v)", name, got[name], typ, got)
				}
			}
			// Directories sort before files.
			if len(l.Entries) > 0 && l.Entries[0].Type != "dir" {
				t.Errorf("first entry is %+v, want the directory first", l.Entries[0])
			}
		})
	}
}

func TestHandleBrowse_EmptyDirectorySerializesAsEmptyArray(t *testing.T) {
	s, _ := newCreateJobServer(t)
	rr := apiTestGet(apiTestMux(s), browseURL(t.TempDir(), ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	// The frontend reads entries.length, so null would break it.
	if !strings.Contains(rr.Body.String(), `"entries":[]`) {
		t.Errorf("body = %s, want \"entries\":[]", rr.Body.String())
	}
}

func TestHandleBrowse_NormalizesPath(t *testing.T) {
	s, _ := newCreateJobServer(t)
	mux := apiTestMux(s)
	dir := browseFixture(t)

	messy := strings.ReplaceAll(dir, `\`, "/") + "/sub/.."
	rr := apiTestGet(mux, browseURL(messy, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var l fsbrowse.Listing
	apiTestDecode(t, rr, &l)
	if l.Path != dir {
		t.Errorf("listing path = %q, want the cleaned %q", l.Path, dir)
	}
}

func TestHandleBrowse_BadPaths(t *testing.T) {
	s, _ := newCreateJobServer(t)
	mux := apiTestMux(s)
	dir := browseFixture(t)

	tests := []struct {
		name   string
		target string
		msg    string // substring expected in the error message ("" = any)
	}{
		{"no path parameter", "/api/browse", "missing path"},
		{"empty path parameter", "/api/browse?path=", "missing path"},
		{"relative path", browseURL(`sub\dir`, ""), "not absolute"},
		{"drive-relative path", browseURL(`C:foo`, ""), "not absolute"},
		{"nonexistent directory", browseURL(filepath.Join(dir, "missing"), ""), ""},
		{"a file, not a directory", browseURL(filepath.Join(dir, "a.mp4"), ""), "not a directory"},
		{"UNC path", browseURL(`\\server\share\dir`, ""), "UNC"},
		{"UNC path with forward slashes", browseURL(`//server/share/dir`, ""), "UNC"},
		{"long-path namespace", browseURL(`\\?\C:\Windows`, ""), "UNC"},
		{"device namespace", browseURL(`\\.\pipe\x`, ""), "UNC"},
		{"reserved device name", browseURL(`C:\con`, ""), "reserved device name"},
		{"reserved device name with extension", browseURL(`C:\temp\NUL.txt`, ""), "reserved device name"},
		{"alternate data stream", browseURL(dir+`:stream`, ""), "colon"},
		{"bad path with kind=exe", browseURL(`\\server\share`, "exe"), "UNC"},
		{"bad path with kind=dir", browseURL(`\\server\share`, "dir"), "UNC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := apiTestGet(mux, tt.target)
			msg := apiTestErrorBody(t, rr, http.StatusBadRequest)
			if tt.msg != "" && !strings.Contains(msg, tt.msg) {
				t.Errorf("error = %q, want it to mention %q", msg, tt.msg)
			}
		})
	}
}

func TestHandleModels_Empty(t *testing.T) {
	s := newTestServer(t) // fresh config: empty models directory
	rr := apiTestGet(apiTestMux(s), "/api/models")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	// Must be [] not null: the dropdown code iterates the array.
	if got := strings.TrimSpace(rr.Body.String()); got != "[]" {
		t.Errorf("body = %q, want []", got)
	}
}

func TestHandleModels_ListsOnlyModelFiles(t *testing.T) {
	s, _ := newCreateJobServer(t) // already holds ggml-tiny.bin (1 byte)
	modelsDir := s.Config.EffectiveModelsDir()
	for name, content := range map[string]string{
		"ggml-small.bin": "12345",
		"GGML-Base.BIN":  "xy",
		"readme.txt":     "not a model",
		"ggml-notes.txt": "not a model either",
		"model.bin":      "missing the ggml- prefix",
	} {
		if err := os.WriteFile(filepath.Join(modelsDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(modelsDir, "ggml-dir.bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	rr := apiTestGet(apiTestMux(s), "/api/models")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	var models []transcribe.Model
	apiTestDecode(t, rr, &models)

	var ids []string
	byID := map[string]transcribe.Model{}
	for _, m := range models {
		ids = append(ids, m.ID)
		byID[m.ID] = m
	}
	sort.Strings(ids)
	if got := strings.Join(ids, ","); got != "base,small,tiny" {
		t.Fatalf("model ids = %s, want base,small,tiny", got)
	}
	small := byID["small"]
	if small.Filename != "ggml-small.bin" || small.SizeBytes != 5 || small.Path != filepath.Join(modelsDir, "ggml-small.bin") {
		t.Errorf("small = %+v", small)
	}
	if byID["base"].Filename != "GGML-Base.BIN" {
		t.Errorf("base filename = %q, want the on-disk spelling preserved", byID["base"].Filename)
	}
}

func TestHandleModels_FollowsSettingsOverride(t *testing.T) {
	s, _ := newCreateJobServer(t)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "ggml-large.bin"), []byte("l"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Config.Settings.Update(func(p *settings.Paths) error { p.ModelsDir = other; return nil }); err != nil {
		t.Fatal(err)
	}

	rr := apiTestGet(apiTestMux(s), "/api/models")
	var models []transcribe.Model
	apiTestDecode(t, rr, &models)
	if len(models) != 1 || models[0].ID != "large" {
		t.Fatalf("models = %+v, want only the model from the overridden directory", models)
	}
}

func TestHandleModels_UnreadableDirIs500(t *testing.T) {
	s, _ := newCreateJobServer(t)
	gone := filepath.Join(t.TempDir(), "deleted")
	if _, err := s.Config.Settings.Update(func(p *settings.Paths) error { p.ModelsDir = gone; return nil }); err != nil {
		t.Fatal(err)
	}
	apiTestErrorBody(t, apiTestGet(apiTestMux(s), "/api/models"), http.StatusInternalServerError)
}

func TestHandleLanguages(t *testing.T) {
	s, _ := newCreateJobServer(t)
	rr := apiTestGet(apiTestMux(s), "/api/languages")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	var langs []Language
	apiTestDecode(t, rr, &langs)
	if len(langs) == 0 || langs[0].Code != "auto" {
		t.Fatalf("languages = %+v, want auto-detect listed first", langs)
	}

	seen := map[string]bool{}
	for _, l := range langs {
		if l.Code == "" || l.Label == "" {
			t.Errorf("language %+v has an empty code or label", l)
		}
		if seen[l.Code] {
			t.Errorf("language code %q listed twice", l.Code)
		}
		seen[l.Code] = true
		// Whatever the dropdown offers, job creation has to accept.
		if err := validateJobOptions(createJobRequest{Language: l.Code}); err != nil {
			t.Errorf("listed language %q is rejected by validateJobOptions: %v", l.Code, err)
		}
	}
	for _, code := range []string{"auto", "en", "zh", "ja"} {
		if !seen[code] {
			t.Errorf("language %q missing from the list", code)
		}
	}
}

func TestRegisterRoutes_CatchAllIsJSON404(t *testing.T) {
	s, _ := newCreateJobServer(t)
	mux := apiTestMux(s)

	tests := []struct {
		name, method, path string
	}{
		{"typo", "GET", "/api/typo"},
		{"nested unknown", "GET", "/api/jobs/abc/nope"},
		{"unsupported method on a real path", "DELETE", "/api/jobs"},
		{"unsupported method on settings", "PATCH", "/api/settings"},
		{"trailing slash variant", "GET", "/api/languages/"},
		{"POST to a GET-only route", "POST", "/api/languages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(tt.method, tt.path, nil))
			msg := apiTestErrorBody(t, rr, http.StatusNotFound)
			if !strings.Contains(msg, tt.method+" "+tt.path) {
				t.Errorf("error = %q, want it to name %q", msg, tt.method+" "+tt.path)
			}
		})
	}

	t.Run("real routes still win over the catch-all", func(t *testing.T) {
		for _, path := range []string{"/api/languages", "/api/models", "/api/drives", "/api/jobs"} {
			if rr := apiTestGet(mux, path); rr.Code != http.StatusOK {
				t.Errorf("GET %s = %d, want 200: %s", path, rr.Code, rr.Body.String())
			}
		}
	})
}

package fsbrowse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsVideoFile(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"mp4", `E:\v\a.mp4`, true},
		{"uppercase extension", `E:\v\a.MP4`, true},
		{"mkv", `E:\v\a.mkv`, true},
		{"srt is not a video", `E:\v\a.srt`, false},
		{"txt is not a video", `E:\v\a.txt`, false},
		{"no extension", `E:\v\a`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsVideoFile(tt.path); got != tt.want {
				t.Errorf("IsVideoFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()

	mustWrite(t, filepath.Join(dir, "b_video.mp4"), "video")
	mustWrite(t, filepath.Join(dir, "a_video.mkv"), "video")
	mustWrite(t, filepath.Join(dir, "notes.txt"), "not a video")
	mustWrite(t, filepath.Join(dir, "readme.srt"), "not a video")
	if err := os.Mkdir(filepath.Join(dir, "z_subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "a_subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	listing, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if listing.Path != dir {
		t.Errorf("Path = %q, want %q", listing.Path, dir)
	}
	wantParent := filepath.Dir(dir)
	if listing.Parent != wantParent {
		t.Errorf("Parent = %q, want %q", listing.Parent, wantParent)
	}

	if len(listing.Entries) != 4 {
		t.Fatalf("Entries = %+v, want 4 entries (2 dirs + 2 videos, no .txt/.srt)", listing.Entries)
	}

	// Directories first (alphabetical), then videos (alphabetical).
	wantOrder := []string{"a_subdir", "z_subdir", "a_video.mkv", "b_video.mp4"}
	for i, name := range wantOrder {
		if listing.Entries[i].Name != name {
			t.Errorf("Entries[%d].Name = %q, want %q", i, listing.Entries[i].Name, name)
		}
	}
	if listing.Entries[0].Type != "dir" || listing.Entries[2].Type != "video" {
		t.Errorf("unexpected entry types: %+v", listing.Entries)
	}
}

func TestListRootHasNoParent(t *testing.T) {
	// filepath.Dir on a root path returns the root itself; List should
	// surface that as "no parent" rather than an infinite-loop-inviting
	// self-referential parent.
	root := filepath.VolumeName(t.TempDir()) + `\`
	listing, err := List(root)
	if err != nil {
		t.Fatalf("List(%q): %v", root, err)
	}
	if listing.Parent != "" {
		t.Errorf("Parent = %q, want empty for a root path", listing.Parent)
	}
}

func TestValidateAbsDir(t *testing.T) {
	dir := t.TempDir()

	if _, err := ValidateAbsDir("relative\\path"); err == nil {
		t.Error("expected error for relative path")
	}
	if _, err := ValidateAbsDir(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Error("expected error for nonexistent directory")
	}
	file := filepath.Join(dir, "a.txt")
	mustWrite(t, file, "x")
	if _, err := ValidateAbsDir(file); err == nil {
		t.Error("expected error when path is a file, not a directory")
	}

	forwardSlash := filepathToForwardSlash(dir)
	got, err := ValidateAbsDir(forwardSlash)
	if err != nil {
		t.Fatalf("ValidateAbsDir(%q): %v", forwardSlash, err)
	}
	if got != filepath.Clean(dir) {
		t.Errorf("got %q, want %q", got, filepath.Clean(dir))
	}
}

func TestValidateVideoFile(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	mustWrite(t, video, "video")
	notVideo := filepath.Join(dir, "notes.txt")
	mustWrite(t, notVideo, "text")

	if _, err := ValidateVideoFile("relative\\clip.mp4"); err == nil {
		t.Error("expected error for relative path")
	}
	if _, err := ValidateVideoFile(filepath.Join(dir, "missing.mp4")); err == nil {
		t.Error("expected error for nonexistent file")
	}
	if _, err := ValidateVideoFile(dir); err == nil {
		t.Error("expected error when path is a directory")
	}
	if _, err := ValidateVideoFile(notVideo); err == nil {
		t.Error("expected error for a non-video extension")
	}

	got, err := ValidateVideoFile(video)
	if err != nil {
		t.Fatalf("ValidateVideoFile(%q): %v", video, err)
	}
	if got != filepath.Clean(video) {
		t.Errorf("got %q, want %q", got, filepath.Clean(video))
	}
}

func TestEnsureWritable(t *testing.T) {
	dir := t.TempDir()

	notYetExisting := filepath.Join(dir, "new.srt")
	if err := EnsureWritable(notYetExisting); err != nil {
		t.Errorf("EnsureWritable(new file) = %v, want nil", err)
	}
	if _, err := os.Stat(notYetExisting); err != nil {
		t.Errorf("expected probe to create the file: %v", err)
	}

	existing := filepath.Join(dir, "existing.srt")
	mustWrite(t, existing, "previous subtitle content")
	if err := EnsureWritable(existing); err != nil {
		t.Errorf("EnsureWritable(existing writable file) = %v, want nil", err)
	}
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "previous subtitle content" {
		t.Errorf("EnsureWritable must not truncate existing content, got %q", data)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func filepathToForwardSlash(p string) string {
	out := make([]rune, 0, len(p))
	for _, r := range p {
		if r == '\\' {
			r = '/'
		}
		out = append(out, r)
	}
	return string(out)
}

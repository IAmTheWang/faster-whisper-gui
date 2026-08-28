package fsbrowse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsMediaFile(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"mp4", `E:\v\a.mp4`, true},
		{"uppercase extension", `E:\v\a.MP4`, true},
		{"mkv", `E:\v\a.mkv`, true},
		{"m4a is an audio file", `E:\v\a.m4a`, true},
		{"mp3 is an audio file", `E:\v\a.mp3`, true},
		{"uppercase audio extension", `E:\v\a.M4A`, true},
		{"srt is not media", `E:\v\a.srt`, false},
		{"txt is not media", `E:\v\a.txt`, false},
		{"no extension", `E:\v\a`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsMediaFile(tt.path); got != tt.want {
				t.Errorf("IsMediaFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMediaKind(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"mp4 is video", `E:\v\a.mp4`, "video"},
		{"m4a is audio", `E:\v\a.m4a`, "audio"},
		{"txt is neither", `E:\v\a.txt`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MediaKind(tt.path); got != tt.want {
				t.Errorf("MediaKind(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsExeFile(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"exe", `C:\tools\ffmpeg.exe`, true},
		{"uppercase extension", `C:\tools\FFMPEG.EXE`, true},
		{"dll is not an exe", `C:\tools\whisper.dll`, false},
		{"no extension", `C:\tools\ffmpeg`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsExeFile(tt.path); got != tt.want {
				t.Errorf("IsExeFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestListFilteredDirsOnly(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "video.mp4"), "video")
	mustWrite(t, filepath.Join(dir, "tool.exe"), "exe")
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	listing, err := ListFiltered(dir, func(string) string { return "" })
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Type != "dir" {
		t.Fatalf("Entries = %+v, want exactly the one subdirectory", listing.Entries)
	}
}

func TestListFilteredExe(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "video.mp4"), "video")
	mustWrite(t, filepath.Join(dir, "ffmpeg.exe"), "exe")

	listing, err := ListFiltered(dir, func(name string) string {
		if IsExeFile(name) {
			return "file"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Name != "ffmpeg.exe" || listing.Entries[0].Type != "file" {
		t.Fatalf("Entries = %+v, want exactly ffmpeg.exe typed \"file\"", listing.Entries)
	}
}

func TestValidateExeFile(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "ffmpeg.exe")
	mustWrite(t, exe, "exe")
	notExe := filepath.Join(dir, "notes.txt")
	mustWrite(t, notExe, "text")

	if _, err := ValidateExeFile("relative\\ffmpeg.exe"); err == nil {
		t.Error("expected error for relative path")
	}
	if _, err := ValidateExeFile(filepath.Join(dir, "missing.exe")); err == nil {
		t.Error("expected error for nonexistent file")
	}
	if _, err := ValidateExeFile(dir); err == nil {
		t.Error("expected error when path is a directory")
	}
	if _, err := ValidateExeFile(notExe); err == nil {
		t.Error("expected error for a non-.exe extension")
	}

	got, err := ValidateExeFile(exe)
	if err != nil {
		t.Fatalf("ValidateExeFile(%q): %v", exe, err)
	}
	if got != filepath.Clean(exe) {
		t.Errorf("got %q, want %q", got, filepath.Clean(exe))
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()

	mustWrite(t, filepath.Join(dir, "b_video.mp4"), "video")
	mustWrite(t, filepath.Join(dir, "a_video.mkv"), "video")
	mustWrite(t, filepath.Join(dir, "c_audio.m4a"), "audio")
	mustWrite(t, filepath.Join(dir, "notes.txt"), "not media")
	mustWrite(t, filepath.Join(dir, "readme.srt"), "not media")
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

	if len(listing.Entries) != 5 {
		t.Fatalf("Entries = %+v, want 5 entries (2 dirs + 2 videos + 1 audio, no .txt/.srt)", listing.Entries)
	}

	// Directories first (alphabetical), then media files (alphabetical).
	wantOrder := []string{"a_subdir", "z_subdir", "a_video.mkv", "b_video.mp4", "c_audio.m4a"}
	for i, name := range wantOrder {
		if listing.Entries[i].Name != name {
			t.Errorf("Entries[%d].Name = %q, want %q", i, listing.Entries[i].Name, name)
		}
	}
	if listing.Entries[0].Type != "dir" || listing.Entries[2].Type != "video" || listing.Entries[4].Type != "audio" {
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

func TestValidateMediaFile(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	mustWrite(t, video, "video")
	audio := filepath.Join(dir, "clip.m4a")
	mustWrite(t, audio, "audio")
	notMedia := filepath.Join(dir, "notes.txt")
	mustWrite(t, notMedia, "text")

	if _, err := ValidateMediaFile("relative\\clip.mp4"); err == nil {
		t.Error("expected error for relative path")
	}
	if _, err := ValidateMediaFile(filepath.Join(dir, "missing.mp4")); err == nil {
		t.Error("expected error for nonexistent file")
	}
	if _, err := ValidateMediaFile(dir); err == nil {
		t.Error("expected error when path is a directory")
	}
	if _, err := ValidateMediaFile(notMedia); err == nil {
		t.Error("expected error for a non-media extension")
	}

	got, err := ValidateMediaFile(video)
	if err != nil {
		t.Fatalf("ValidateMediaFile(%q): %v", video, err)
	}
	if got != filepath.Clean(video) {
		t.Errorf("got %q, want %q", got, filepath.Clean(video))
	}

	gotAudio, err := ValidateMediaFile(audio)
	if err != nil {
		t.Fatalf("ValidateMediaFile(%q): %v", audio, err)
	}
	if gotAudio != filepath.Clean(audio) {
		t.Errorf("got %q, want %q", gotAudio, filepath.Clean(audio))
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

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.srt")
	mustWrite(t, src, "1\n00:00:00,000 --> 00:00:01,000\nhello\n")

	dst := filepath.Join(dir, "dest", "copy.srt")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read copy: %v", err)
	}
	want, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("copy content = %q, want %q", got, want)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source should be untouched: %v", err)
	}

	// Copying again over an existing destination should overwrite, not append.
	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile (overwrite): %v", err)
	}
	got, err = os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("copy content after overwrite = %q, want %q", got, want)
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

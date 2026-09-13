// Package fsbrowse exposes the local filesystem to the frontend: listing
// drives and directories, filtering to media (video/audio) files, and
// validating/normalizing paths that arrive from the client (which is the
// only way this tool can get an absolute path to a media file, since
// browsers won't hand one over — see the project plan for why).
package fsbrowse

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Drive is one logical drive available for browsing.
type Drive struct {
	Name  string `json:"name"`  // e.g. "C:\\"
	Label string `json:"label"` // e.g. "Local Disk"
}

// mediaExtensions maps each recognized video/audio file extension to its
// kind ("video" or "audio"), for both directory-listing filtering and
// job-submission validation.
var mediaExtensions = map[string]string{
	// video
	".mp4": "video", ".mkv": "video", ".mov": "video", ".avi": "video", ".wmv": "video",
	".flv": "video", ".webm": "video", ".m4v": "video", ".ts": "video", ".mpg": "video",
	".mpeg": "video",
	// audio
	".m4a": "audio", ".mp3": "audio", ".wav": "audio", ".aac": "audio", ".flac": "audio",
	".ogg": "audio", ".wma": "audio", ".opus": "audio",
}

// MediaKind reports the kind of media path's extension looks like
// ("video" or "audio"), or "" if it isn't a recognized media extension.
func MediaKind(path string) string {
	return mediaExtensions[strings.ToLower(filepath.Ext(path))]
}

// IsMediaFile reports whether path's extension looks like a video or audio
// container.
func IsMediaFile(path string) bool {
	return MediaKind(path) != ""
}

// IsExeFile reports whether path has a .exe extension — used when browsing
// for ffmpeg.exe/whisper-cli.exe overrides (see the Settings panel).
func IsExeFile(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".exe"
}

// ListDrives enumerates the available drive letters.
func ListDrives() ([]Drive, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("list drives: %w", err)
	}

	var drives []Drive
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		rootPtr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		driveType := windows.GetDriveType(rootPtr)
		if driveType == windows.DRIVE_NO_ROOT_DIR || driveType == windows.DRIVE_UNKNOWN {
			continue // present in the bitmask but not actually accessible
		}
		drives = append(drives, Drive{Name: root, Label: driveTypeLabel(driveType)})
	}
	return drives, nil
}

func driveTypeLabel(t uint32) string {
	switch t {
	case windows.DRIVE_REMOVABLE:
		return "Removable Disk"
	case windows.DRIVE_FIXED:
		return "Local Disk"
	case windows.DRIVE_REMOTE:
		return "Network Drive"
	case windows.DRIVE_CDROM:
		return "CD/DVD Drive"
	case windows.DRIVE_RAMDISK:
		return "RAM Disk"
	default:
		return "Drive"
	}
}

// Entry is one item (subdirectory or media file) within a browsed directory.
type Entry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"` // "dir", "video", or "audio"
	Size        int64  `json:"size"`
	ModTime     int64  `json:"modTime"`     // unix millis
	CreatedTime int64  `json:"createdTime"` // unix millis
}

// creationTime extracts a file or directory's Windows creation timestamp
// (unix millis) from fi, or 0 if unavailable (including a zeroed FILETIME,
// which some filesystems report when creation time isn't tracked — treating
// that as 0 rather than converting it avoids surfacing the ~1601 date that
// Nanoseconds() would otherwise produce from the Unix epoch offset).
func creationTime(fi os.FileInfo) int64 {
	winFI, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok || (winFI.CreationTime.HighDateTime == 0 && winFI.CreationTime.LowDateTime == 0) {
		return 0
	}
	return time.Unix(0, winFI.CreationTime.Nanoseconds()).UnixMilli()
}

// Listing is the result of browsing one directory.
type Listing struct {
	Path    string  `json:"path"`
	Parent  string  `json:"parent,omitempty"`
	Entries []Entry `json:"entries"`
}

// List returns dir's subdirectories and media files (everything else is
// filtered out — this tool only ever needs to find a video or audio file),
// directories first, then alphabetically.
//
// dir must already be an absolute, cleaned, existing directory path —
// callers (the HTTP handler, via ValidateAbsDir) are responsible for
// normalizing/validating untrusted input before it reaches here.
func List(dir string) (Listing, error) {
	return ListFiltered(dir, MediaKind)
}

// ListFiltered returns dir's subdirectories plus files for which kindOf
// returns a non-empty tag (used as that entry's Type), directories first
// then alphabetical. Pass a kindOf func that always returns "" to get a
// directories-only listing, e.g. for picking the models directory override
// rather than a file within it.
//
// dir must already be an absolute, cleaned, existing directory path —
// callers (the HTTP handler, via ValidateAbsDir) are responsible for
// normalizing/validating untrusted input before it reaches here.
func ListFiltered(dir string, kindOf func(name string) string) (Listing, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return Listing{}, fmt.Errorf("read dir %s: %w", dir, err)
	}

	listing := Listing{Path: dir}
	if parent := filepath.Dir(dir); parent != dir {
		listing.Parent = parent
	}

	for _, de := range dirEntries {
		name := de.Name()
		full := filepath.Join(dir, name)

		if de.IsDir() {
			entry := Entry{Name: name, Path: full, Type: "dir"}
			if fi, err := de.Info(); err == nil {
				entry.ModTime = fi.ModTime().UnixMilli()
				entry.CreatedTime = creationTime(fi)
			}
			listing.Entries = append(listing.Entries, entry)
			continue
		}
		kind := kindOf(name)
		if kind == "" {
			continue
		}
		fi, err := de.Info()
		if err != nil {
			continue
		}
		listing.Entries = append(listing.Entries, Entry{
			Name:        name,
			Path:        full,
			Type:        kind,
			Size:        fi.Size(),
			ModTime:     fi.ModTime().UnixMilli(),
			CreatedTime: creationTime(fi),
		})
	}

	sort.SliceStable(listing.Entries, func(i, j int) bool {
		a, b := listing.Entries[i], listing.Entries[j]
		if (a.Type == "dir") != (b.Type == "dir") {
			return a.Type == "dir"
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})

	return listing, nil
}

// ValidateAbsDir normalizes p (handling mixed / and \ separators from a
// hand-typed custom-output-directory field) and ensures it's an absolute
// path to an existing directory.
func ValidateAbsDir(p string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(p))
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("path %q is not absolute", p)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", clean)
	}
	return clean, nil
}

// ValidateMediaFile normalizes p and ensures it's an absolute path to an
// existing regular file with a recognized video or audio extension.
func ValidateMediaFile(p string) (string, error) {
	return validateFile(p, IsMediaFile, "does not have a recognized video or audio extension")
}

// ValidateExeFile normalizes p and ensures it's an absolute path to an
// existing regular file with a .exe extension — used to validate the
// ffmpeg.exe/whisper-cli.exe path overrides saved from the Settings panel.
func ValidateExeFile(p string) (string, error) {
	return validateFile(p, IsExeFile, "is not a .exe file")
}

// validateFile is the shared normalize-then-stat body behind
// ValidateMediaFile/ValidateExeFile: clean p (handling mixed / and \
// separators from a hand-typed field), require it to be absolute, existing,
// a regular file (not a directory), and satisfy match — otherwise return an
// error using what to describe the mismatch.
func validateFile(p string, match func(string) bool, what string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(p))
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("path %q is not absolute", p)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory, not a file", clean)
	}
	if !match(clean) {
		return "", fmt.Errorf("%q %s", clean, what)
	}
	return clean, nil
}

// EnsureWritable probes whether path can be opened for writing — creating
// it if it doesn't exist yet, but never truncating or holding it open — so
// callers can fail fast before starting an expensive transcription job if
// the target SRT path is locked by another process (e.g. a media player
// has it open). Any error is returned as-is: Windows reports a locked file
// as a sharing violation, not os.ErrPermission, so callers should treat any
// error here as "not writable" rather than pattern-matching a specific one.
func EnsureWritable(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
	if err != nil {
		return err
	}
	return f.Close()
}

// CopyFile copies src to dst, creating or truncating dst. Used by the job
// "export" endpoint to duplicate an already-generated SRT into a
// user-chosen folder on demand, without touching the original.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

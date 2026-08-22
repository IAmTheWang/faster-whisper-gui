// Package fsbrowse exposes the local filesystem to the frontend: listing
// drives and directories, filtering to video files, and validating/
// normalizing paths that arrive from the client (which is the only way this
// tool can get an absolute path to a video, since browsers won't hand one
// over — see the project plan for why).
package fsbrowse

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows"
)

// Drive is one logical drive available for browsing.
type Drive struct {
	Name  string `json:"name"`  // e.g. "C:\\"
	Label string `json:"label"` // e.g. "Local Disk"
}

// videoExtensions lists file extensions treated as "video" for both
// directory-listing filtering and job-submission validation.
var videoExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".mov": true, ".avi": true, ".wmv": true,
	".flv": true, ".webm": true, ".m4v": true, ".ts": true, ".mpg": true,
	".mpeg": true,
}

// IsVideoFile reports whether path's extension looks like a video container.
func IsVideoFile(path string) bool {
	return videoExtensions[strings.ToLower(filepath.Ext(path))]
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

// Entry is one item (subdirectory or video file) within a browsed directory.
type Entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Type    string `json:"type"` // "dir" or "video"
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"` // unix millis
}

// Listing is the result of browsing one directory.
type Listing struct {
	Path    string  `json:"path"`
	Parent  string  `json:"parent,omitempty"`
	Entries []Entry `json:"entries"`
}

// List returns dir's subdirectories and video files (everything else is
// filtered out — this tool only ever needs to find a video), directories
// first, then alphabetically.
//
// dir must already be an absolute, cleaned, existing directory path —
// callers (the HTTP handler, via ValidateAbsDir) are responsible for
// normalizing/validating untrusted input before it reaches here.
func List(dir string) (Listing, error) {
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
			listing.Entries = append(listing.Entries, Entry{Name: name, Path: full, Type: "dir"})
			continue
		}
		if !IsVideoFile(name) {
			continue
		}
		fi, err := de.Info()
		if err != nil {
			continue
		}
		listing.Entries = append(listing.Entries, Entry{
			Name:    name,
			Path:    full,
			Type:    "video",
			Size:    fi.Size(),
			ModTime: fi.ModTime().UnixMilli(),
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

// ValidateVideoFile normalizes p and ensures it's an absolute path to an
// existing regular file with a recognized video extension.
func ValidateVideoFile(p string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(p))
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("path %q is not absolute", p)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory, not a video file", clean)
	}
	if !IsVideoFile(clean) {
		return "", fmt.Errorf("%q does not have a recognized video extension", clean)
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

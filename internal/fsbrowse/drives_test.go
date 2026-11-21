package fsbrowse

import (
	"os"
	"regexp"
	"testing"

	"golang.org/x/sys/windows"
)

var driveNameRe = regexp.MustCompile(`^[A-Z]:\\$`)

func TestListDrives(t *testing.T) {
	drives, err := ListDrives()
	if err != nil {
		t.Fatalf("ListDrives: %v", err)
	}
	if len(drives) == 0 {
		t.Fatal("ListDrives returned no drives; a Windows machine always has at least one")
	}

	seen := map[string]bool{}
	for _, d := range drives {
		if !driveNameRe.MatchString(d.Name) {
			t.Errorf("drive name %q, want a root like C:\\", d.Name)
		}
		if seen[d.Name] {
			t.Errorf("drive %q listed twice", d.Name)
		}
		seen[d.Name] = true
		if d.Label == "" {
			t.Errorf("drive %q has an empty label", d.Name)
		}
	}

	// Drive letters come out in alphabetical order (the bitmask is walked A..Z).
	for i := 1; i < len(drives); i++ {
		if drives[i-1].Name >= drives[i].Name {
			t.Errorf("drives not in ascending order: %q before %q", drives[i-1].Name, drives[i].Name)
		}
	}

	// The drive holding the system root must be reachable and listed.
	if sysDrive := os.Getenv("SystemDrive"); sysDrive != "" {
		if want := sysDrive + `\`; !seen[want] {
			t.Errorf("system drive %q missing from %v", want, drives)
		}
	}
}

func TestDriveTypeLabel(t *testing.T) {
	tests := []struct {
		driveType uint32
		want      string
	}{
		{windows.DRIVE_REMOVABLE, "Removable Disk"},
		{windows.DRIVE_FIXED, "Local Disk"},
		{windows.DRIVE_REMOTE, "Network Drive"},
		{windows.DRIVE_CDROM, "CD/DVD Drive"},
		{windows.DRIVE_RAMDISK, "RAM Disk"},
		{windows.DRIVE_UNKNOWN, "Drive"},
	}
	for _, tt := range tests {
		if got := driveTypeLabel(tt.driveType); got != tt.want {
			t.Errorf("driveTypeLabel(%d) = %q, want %q", tt.driveType, got, tt.want)
		}
	}
}

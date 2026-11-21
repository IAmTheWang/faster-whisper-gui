package fsbrowse

import (
	"fmt"
	"strings"
)

// checkSafePath rejects absolute Windows paths that must never be touched on
// behalf of the (browser-facing) API: UNC shares, \?\ long-path and \.\
// device namespaces — statting a UNC path makes Windows authenticate to the
// remote host and can hang on a dead one — and reserved device names (CON,
// NUL, COM1, ...) in any segment, which Windows resolves to devices even with
// an extension ("con.txt"). clean must already be filepath.Clean'd.
func checkSafePath(clean string) error {
	if strings.HasPrefix(clean, `\`) {
		return fmt.Errorf("network (UNC) and device paths are not allowed: %q", clean)
	}
	rest := clean
	if len(rest) >= 2 && rest[1] == ':' {
		rest = rest[2:]
	}
	// Only the drive letter may carry a colon: "file.srt:stream" is an NTFS
	// alternate data stream, which the file checks would otherwise treat as the
	// ordinary file it is attached to.
	if strings.Contains(rest, ":") {
		return fmt.Errorf("a colon is only allowed in the drive letter: %q", clean)
	}
	for _, seg := range strings.Split(rest, `\`) {
		if isReservedDeviceName(seg) {
			return fmt.Errorf("reserved device name %q is not allowed in a path", seg)
		}
	}
	return nil
}

func isReservedDeviceName(seg string) bool {
	if i := strings.IndexByte(seg, '.'); i >= 0 {
		seg = seg[:i]
	}
	seg = strings.ToUpper(strings.TrimRight(seg, " "))
	switch seg {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	// COM1-9 / LPT1-9, plus the superscript digits (COM¹, COM², COM³) that
	// Windows also reserves.
	if r := []rune(seg); len(r) == 4 && (strings.HasPrefix(seg, "COM") || strings.HasPrefix(seg, "LPT")) {
		return (r[3] >= '1' && r[3] <= '9') || r[3] == '\u00b9' || r[3] == '\u00b2' || r[3] == '\u00b3'
	}
	return false
}

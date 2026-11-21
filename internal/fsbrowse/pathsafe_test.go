package fsbrowse

import "testing"

func TestCheckSafePath(t *testing.T) {
	bad := []string{
		`\server\share\x.mp4`,
		`\?\C:\videos\x.mp4`,
		`\.\pipe\foo`,
		`C:\videos\CON`,
		`C:\videos\con.txt`,
		`C:\NUL.srt`,
		`D:\a\com1\b.mp4`,
		`D:\a\LPT9.mp4`,
		`C:\videos\aux .mp4`,
		`C:\videos\clip.srt:hidden`,       // NTFS alternate data stream
		`C:\videos\x.mp4:Zone.Identifier`, // same
		"C:\\videos\\COM\u00b9.txt",       // superscript digits are reserved too
		"C:\\videos\\lpt\u00b3",
	}
	for _, p := range bad {
		if err := checkSafePath(p); err == nil {
			t.Errorf("checkSafePath(%q) = nil, want error", p)
		}
	}
	good := []string{
		`C:\`,
		`C:\videos\x.mp4`,
		`E:\my.video.v2\clip.mkv`,
		`C:\console\com10.txt`,
		`C:\coma\lpt.srt`,
	}
	for _, p := range good {
		if err := checkSafePath(p); err != nil {
			t.Errorf("checkSafePath(%q) = %v, want nil", p, err)
		}
	}
}

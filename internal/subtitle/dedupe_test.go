package subtitle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollapseRepeats(t *testing.T) {
	tests := []struct{ in, want string }{
		// the motivating example
		{"Womp, womp, womp, womp, womp, womp, womp, womp, womp, womp, womp, womp, womp, womp, womp.", "Womp, womp, womp, womp."},
		{"Oh, no, no, no, no, no, no, no, no, no, no, no, no, no.", "Oh, no, no, no, no."},
		{"Yes, yes, yes, yes, yes, yes.", "Yes, yes, yes, yes."},
		{"Yes, come here, yes, yes, yes, yes, yes, yes, yes, yes.", "Yes, come here, yes, yes, yes, yes."},
		// exactly MaxRepeat is left alone
		{"Yes, yes, yes, yes.", "Yes, yes, yes, yes."},
		{"Mm-hmm, mm-hmm, mm-hmm, mm-hmm, mm-hmm.", "Mm-hmm, mm-hmm, mm-hmm, mm-hmm."},
		// whitespace-separated, no commas (whisper often omits them)
		{"yeah yeah yeah yeah yeah yeah suck them", "yeah yeah yeah yeah suck them"},
		// phrases
		{"right there, right there, right there, right there, right there.", "right there, right there, right there, right there."},
		{"Right there baby, right there, right there, right there, right there, right there.", "Right there baby, right there, right there, right there, right there."},
		// Chinese clauses, fullwidth punctuation, dialogue dash
		{"- 噢，不，不，不，不，不，不，不。", "- 噢，不，不，不，不。"},
		{"对，对，对，对，对，对。", "对，对，对，对。"},
		// markup is preserved
		{`<font color="#c3cf47">Fuck, fuck, fuck, fuck, fuck, fuck, fuck.</font>`, `<font color="#c3cf47">Fuck, fuck, fuck, fuck.</font>`},
		// ordinary text and short repeats are untouched
		{"Oh yeah, look at all these big fat cogs.", "Oh yeah, look at all these big fat cogs."},
		{"No, no, no.", "No, no, no."},
		{"yesssssssssssssss", "yesssssssssssssss"},
		{"", ""},
	}
	for _, tt := range tests {
		// CollapseRepeats works on plain text; markup goes through collapseTextLine.
		if !strings.Contains(tt.in, "<font") {
			if got := CollapseRepeats(tt.in); got != tt.want {
				t.Errorf("CollapseRepeats(%q) = %q, want %q", tt.in, got, tt.want)
			}
		}
		if got := collapseTextLine(tt.in); got != tt.want {
			t.Errorf("collapseTextLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func srt(cues ...[3]string) string {
	var b strings.Builder
	for i, c := range cues {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%d\n%s\n%s\n", i+1, c[0], c[1]+c[2])
	}
	return b.String()
}

func TestCleanSRTDropsConsecutiveDuplicateCues(t *testing.T) {
	var cues [][3]string
	for i := 0; i < 19; i++ { // "Open that mouth." x19, one second apart
		cues = append(cues, [3]string{fmt.Sprintf("00:08:%02d,000 --> 00:08:%02d,900", 40+i, 40+i), "Open that mouth.", ""})
	}
	cues = append(cues, [3]string{"00:09:10,000 --> 00:09:11,000", "Okay.", ""})
	in := srt(cues...)

	out, stats, err := CleanSRT([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if stats.CuesDropped != 15 {
		t.Fatalf("CuesDropped = %d, want 15", stats.CuesDropped)
	}
	got := string(out)
	if n := strings.Count(got, "Open that mouth."); n != MaxRepeat {
		t.Errorf("kept %d repeats, want %d", n, MaxRepeat)
	}
	if !strings.Contains(got, "5\n00:09:10,000 --> 00:09:11,000\nOkay.") {
		t.Errorf("surviving cues must be renumbered; got:\n%s", got)
	}
}

func TestCleanSRTKeepsFarApartAndSeparatedCues(t *testing.T) {
	cues := [][3]string{
		{"00:00:01,000 --> 00:00:02,000", "Okay.", ""},
		{"00:00:03,000 --> 00:00:04,000", "Okay.", ""},
		{"00:00:05,000 --> 00:00:06,000", "okay", ""}, // case/punctuation-insensitive
		{"00:00:07,000 --> 00:00:08,000", "- Okay!", ""},
		{"00:00:09,000 --> 00:00:10,000", "Okay.", ""}, // 5th in a row -> dropped
		{"00:00:11,000 --> 00:00:12,000", "Something else.", ""},
		{"00:00:13,000 --> 00:00:14,000", "Okay.", ""},
		{"00:02:00,000 --> 00:02:01,000", "Okay.", ""}, // a minute later: not part of a run
	}
	out, stats, err := CleanSRT([]byte(srt(cues...)))
	if err != nil {
		t.Fatal(err)
	}
	if stats.CuesDropped != 1 {
		t.Fatalf("CuesDropped = %d, want 1 (only the 5th consecutive Okay)", stats.CuesDropped)
	}
	if n := strings.Count(string(out), "\n\n"); n != 6 { // 7 cues -> 6 separators
		t.Errorf("expected 7 surviving cues, got separators=%d:\n%s", n, out)
	}

	// a long gap breaks a run even when the text matches
	var far [][3]string
	for i := 0; i < 8; i++ {
		far = append(far, [3]string{fmt.Sprintf("00:%02d:00,000 --> 00:%02d:01,000", i, i), "Okay.", ""})
	}
	if _, stats, _ := CleanSRT([]byte(srt(far...))); stats.Changed() {
		t.Errorf("identical cues a minute apart must not be treated as a loop: %+v", stats)
	}
}

func TestCleanSRTCollapsesInsideCues(t *testing.T) {
	in := srt([3]string{"00:00:00,000 --> 00:00:05,000", "Womp, womp, womp, womp, womp, womp, womp, womp.", ""})
	out, stats, err := CleanSRT([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if stats.LinesCollapsed != 1 || stats.CuesDropped != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if !strings.Contains(string(out), "Womp, womp, womp, womp.\n") {
		t.Errorf("got %q", out)
	}
}

func TestCleanSRTPreservesFormatting(t *testing.T) {
	base := srt(
		[3]string{"00:00:00,000 --> 00:00:01,000", "No, no, no, no, no, no.", ""},
		[3]string{"00:00:01,000 --> 00:00:02,000", "Fine.", ""},
	)
	crlf := strings.ReplaceAll(base, "\n", "\r\n")
	out, _, err := CleanSRT([]byte("\xef\xbb\xbf" + crlf))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "\xef\xbb\xbf") || strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\n") || !strings.HasSuffix(s, "\r\n") {
		t.Errorf("BOM / CRLF / trailing newline not preserved: %q", s)
	}

	noTrailing := strings.TrimRight(base, "\n")
	out, _, _ = CleanSRT([]byte(noTrailing))
	if strings.HasSuffix(string(out), "\n") {
		t.Errorf("must not add a trailing newline that wasn't there: %q", out)
	}
}

func TestCleanSRTUnchangedInputIsReturnedAsIs(t *testing.T) {
	in := []byte(srt([3]string{"00:00:00,000 --> 00:00:01,000", "Hello there.", ""}))
	out, stats, err := CleanSRT(in)
	if err != nil || stats.Changed() || string(out) != string(in) {
		t.Fatalf("out=%q stats=%+v err=%v", out, stats, err)
	}
}

func TestCleanSRTMalformedInput(t *testing.T) {
	for _, in := range []string{"1\nnot a timing line\nhello\n", "just text without structure"} {
		out, _, err := CleanSRT([]byte(in))
		if err == nil || string(out) != in {
			t.Errorf("input %q: want original returned with an error, got %q, %v", in, out, err)
		}
	}
	if out, _, err := CleanSRT(nil); err != nil || len(out) != 0 {
		t.Errorf("empty input: %q, %v", out, err)
	}
}

func TestCleanFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.srt")
	in := srt([3]string{"00:00:00,000 --> 00:00:05,000", "Fine, fine, fine, fine, fine, fine, fine.", ""})
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	stats, err := CleanFile(path)
	if err != nil || stats.LinesCollapsed != 1 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "Fine, fine, fine, fine.") {
		t.Errorf("file not rewritten: %q", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}

	// an already-clean file is not rewritten at all
	before, _ := os.Stat(path)
	if stats, err := CleanFile(path); err != nil || stats.Changed() {
		t.Fatalf("second pass: %+v %v", stats, err)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("a clean file must not be rewritten")
	}

	if _, err := CleanFile(filepath.Join(dir, "missing.srt")); err == nil {
		t.Error("missing file must error")
	}
}

func TestUncensor(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Fuck those cogs, yeah, f*cking tits.", "Fuck those cogs, yeah, fucking tits."},
		{"What the f**k?", "What the fuck?"},
		{"F*CKING hell", "FUCKING hell"},
		{"F*cking hell", "Fucking hell"},
		{"This is sh*t.", "This is shit."},
		{"b*tch, d*ck, p*ssy, c*nt", "bitch, dick, pussy, cunt"},
		{"motherf*cker", "motherfucker"},
		// sound annotations and unknown masks are left alone
		{"*sobs* and *Humming*", "*sobs* and *Humming*"},
		{"noises* and *makes", "noises* and *makes"},
		{"a*b", "a*b"},
		{"f*ck*ng", "fucking"},
		// ambiguous (fits both "dick" and "dock"-like? only dick is listed) / wrong length stays
		{"f*cking*", "fucking*"},
		{"s****t", "s****t"},
		{"plain text", "plain text"},
	}
	for _, tt := range tests {
		if got := Uncensor(tt.in); got != tt.want {
			t.Errorf("Uncensor(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCleanSRTUncensorsBeforeCollapsing(t *testing.T) {
	// masked and unmasked spellings are the same word once restored, so the
	// run of 6 is detected and cut to 4.
	in := srt([3]string{"00:00:00,000 --> 00:00:05,000", "f*cking, fucking, f*cking, fucking, f*cking, fucking.", ""})
	out, stats, err := CleanSRT([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if stats.LinesUncensored != 1 || stats.LinesCollapsed != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	if !strings.Contains(string(out), "fucking, fucking, fucking, fucking.\n") || strings.Contains(string(out), "*") {
		t.Errorf("got %q", out)
	}
}

func TestCleanSRTDropsMusicSymbolCues(t *testing.T) {
	in := srt(
		[3]string{"00:51:30,140 --> 00:51:40,140", "♪♪", ""},
		[3]string{"00:51:40,140 --> 00:51:50,140", "♪", ""},
		[3]string{"00:52:00,000 --> 00:52:02,000", "♪ I love you ♪", ""},
		[3]string{"00:52:03,000 --> 00:52:04,000", "<font color=\"#fff\">♫♫</font>", ""},
		[3]string{"00:52:05,000 --> 00:52:06,000", "Fine.", ""},
	)
	out, stats, err := CleanSRT([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if stats.MusicCuesDropped != 3 || stats.MusicLinesCleaned != 4 {
		t.Fatalf("stats = %+v", stats)
	}
	want := "1\n00:52:00,000 --> 00:52:02,000\nI love you\n\n2\n00:52:05,000 --> 00:52:06,000\nFine.\n"
	if string(out) != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}

func TestCleanSRTDropsCuesLongerThanSevenSeconds(t *testing.T) {
	in := srt(
		[3]string{"00:00:01,000 --> 00:00:03,000", "Hello there.", ""},
		[3]string{"00:56:00,140 --> 00:56:30,120", "Thank you.", ""},                                                                      // 30s hallucination
		[3]string{"00:12:21,340 --> 00:12:43,600", "I want to see you fucking drooling, I want to see you fucking drooling, so good", ""}, // 22s
		[3]string{"00:57:00,000 --> 00:57:07,000", "Exactly seven seconds is fine.", ""},
		[3]string{"00:58:00,000 --> 00:58:07,500", "Seven and a half is not.", ""},
	)
	out, stats, err := CleanSRT([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if stats.LongCuesDropped != 3 {
		t.Fatalf("stats = %+v", stats)
	}
	for _, gone := range []string{"Thank you", "drooling", "Seven and a half"} {
		if strings.Contains(string(out), gone) {
			t.Errorf("%q should have been dropped:\n%s", gone, out)
		}
	}
	if !strings.Contains(string(out), "Hello there.") || !strings.Contains(string(out), "Exactly seven seconds is fine.") {
		t.Errorf("short cues must survive:\n%s", out)
	}
}

func TestStripFillers(t *testing.T) {
	tests := []struct{ in, want string }{
		// pure-filler lines vanish
		{"Yeah.", ""},
		{"Mm-hmm.", ""},
		{"Uh-huh, yeah.", ""},
		{"Oh, yeah.", ""},
		{"Ahh!", ""},
		{"- Yeah!", ""},
		{"Mmm, mmm, ooh.", ""},
		{"Wow.", ""},
		{`<font color="#c3cf47">Oh.</font>`, ""},
		{"对。", ""},
		{"- 噢，对。", ""},
		{"嗯哼。", ""},
		{"啊啊啊！", ""},
		// leading / trailing fillers are cut, punctuation and capitals fixed up
		{"Yeah, look at me.", "Look at me."},
		{"Fuck, yeah.", "Fuck."},
		{"Oh, fuck, yeah.", "Fuck."},
		{"Yes, baby, good girl.", "Baby, good girl."},
		{"Good girl, yeah!", "Good girl!"},
		{"- Oh, that's good.", "- That's good."},
		{`<font color="#c3cf47">Oh, I fucking love it.</font>`, `<font color="#c3cf47">I fucking love it.</font>`},
		{"对，宝贝，乖女孩。", "宝贝，乖女孩。"},
		{"噢，我他妈爱死了。", "我他妈爱死了。"},
		// fillers inside a sentence, and ordinary words, are left alone
		{"I said yeah and left.", "I said yeah and left."},
		{"Oh my God.", "My God."},
		{"A good girl.", "A good girl."},
		{"Mama said so.", "Mama said so."},
		{"No, no, no.", "No, no, no."},
		{"Hello there.", "Hello there."},
	}
	for _, tt := range tests {
		if got := stripFillers(tt.in); got != tt.want {
			t.Errorf("stripFillers(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCleanSRTDropsFillerCuesAndTrimsMixedOnes(t *testing.T) {
	in := srt(
		[3]string{"00:00:01,000 --> 00:00:02,000", "Yeah.", ""},
		[3]string{"00:00:03,000 --> 00:00:04,000", "Mm-hmm.", ""},
		[3]string{"00:00:05,000 --> 00:00:06,000", "Yeah, look at me.", ""},
		[3]string{"00:00:07,000 --> 00:00:08,000", "- Oh, yeah.\n- Fine.", ""},
		[3]string{"00:00:09,000 --> 00:00:10,000", "Oh, yeah.", ""},
	)
	out, stats, err := CleanSRT([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if stats.FillerCuesDropped != 3 || stats.FillerLinesTrimmed != 5 {
		t.Fatalf("stats = %+v", stats)
	}
	want := "1\n00:00:05,000 --> 00:00:06,000\nLook at me.\n\n2\n00:00:07,000 --> 00:00:08,000\n- Fine.\n"
	if string(out) != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

func TestCleanLineMatchesWhatCleanSRTDoes(t *testing.T) {
	for in, want := range map[string]string{
		"f*cking, yeah, yeah, yeah, yeah, yeah, yeah.": "fucking.",
		"♪ Oh yeah ♪":                 "",
		"Oh, no, no, no, no, no, no.": "No, no, no, no.",
	} {
		if got := CleanLine(in); got != want {
			t.Errorf("CleanLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanSRTRunDetectionIgnoresDroppedCuesInBetween(t *testing.T) {
	// A dropped music cue between two identical cues must not break the run.
	var cues [][3]string
	for i := 0; i < 6; i++ {
		cues = append(cues, [3]string{fmt.Sprintf("00:00:%02d,000 --> 00:00:%02d,500", i*2, i*2), "Okay.", ""})
		cues = append(cues, [3]string{fmt.Sprintf("00:00:%02d,600 --> 00:00:%02d,900", i*2, i*2), "♪", ""})
	}
	_, stats, err := CleanSRT([]byte(srt(cues...)))
	if err != nil || stats.MusicCuesDropped != 6 || stats.CuesDropped != 2 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
}

func TestStripSoundTags(t *testing.T) {
	tests := []struct{ in, want string }{
		{"*Mumbling*", ""},
		{"*sobs*", ""},
		{"*Humming*", ""},
		{"*mocking noises*", ""},
		{"*makes a noise*", ""},
		{"*Sings* Yes, baby. Fuck yeah.", "Yes, baby. Fuck yeah."},
		{"Yes *sobs* baby", "Yes baby"},
		{"[Music]", ""},
		{"[BLANK_AUDIO]", ""},
		{"(laughs)", ""},
		{"（叹气）", ""},
		{"【音乐】", ""},
		{"*抽泣*", ""},
		{"*哼哼*", ""},
		{`<font color="#c3cf47">*sobs*</font>`, `<font color="#c3cf47"></font>`},
		// not annotations
		{"Fill my f*cking mouth, f*cking hard.", "Fill my f*cking mouth, f*cking hard."},
		{"Just a normal line.", "Just a normal line."},
	}
	for _, tt := range tests {
		if got := stripSoundTags(tt.in); got != tt.want {
			t.Errorf("stripSoundTags(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCleanSRTDropsSoundAnnotationCues(t *testing.T) {
	in := srt(
		[3]string{"00:27:58,000 --> 00:28:03,000", "*Mumbling*", ""},
		[3]string{"00:28:04,000 --> 00:28:06,000", "*sobs*", ""},
		[3]string{"00:28:07,000 --> 00:28:09,000", "*Sings* Come here, baby.", ""},
		[3]string{"00:28:10,000 --> 00:28:12,000", "Fine.", ""},
	)
	out, stats, err := CleanSRT([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if stats.SoundCuesDropped != 2 || stats.SoundLinesCleaned != 3 || stats.FillerCuesDropped != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	want := "1\n00:28:07,000 --> 00:28:09,000\nCome here, baby.\n\n2\n00:28:10,000 --> 00:28:12,000\nFine.\n"
	if string(out) != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

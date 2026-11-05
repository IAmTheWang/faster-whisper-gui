// Package subtitle post-processes the SRT files whisper.cpp writes:
//
//   - Whisper regularly hallucinates runaway repetition on silence, music or
//     noise ("Womp, womp, womp, ..." x15, or the same cue 20 times in a row);
//     CleanSRT trims that down to at most MaxRepeat repeats.
//   - It also emits masked profanity ("f*cking"); CleanSRT spells those out
//     (see Uncensor).
//   - On silence/music it invents cues: "♪♪" music symbols, and a "Thank you."
//     stretched over 30 seconds. CleanSRT drops music-symbol cues and any cue
//     longer than maxCueDuration (nobody speaks one subtitle for that long).
//   - It strips interjections ("Yeah.", "Mm-hmm.", "Oh, yeah.") that add noise
//     but no content (see stripFillers), and non-speech annotations such as
//     "*Mumbling*", "*sobs*" or "[Music]" (see stripSoundTags).
//
// It has no dependency on anything else in this repo.
package subtitle

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// MaxRepeat is the most times a word/phrase, or a whole cue, may repeat in a
// row before the surplus is dropped.
const MaxRepeat = 4

// maxCueGap is the largest silence between two identical consecutive cues for
// them still to count as one run. Identical cues far apart in time are
// ordinary speech ("Yeah." a minute later), not a hallucination loop.
const maxCueGap = 5.0

// maxCueDuration is the longest a single cue may last, in seconds. Whisper
// emits stretched cues on silence and noise ("Thank you." held for 30s, a
// sentence spread over 22s); real people rarely speak one subtitle for longer
// than this, so anything over it is treated as a recognition error.
const maxCueDuration = 7.0

// Stats reports what CleanSRT changed.
type Stats struct {
	CuesDropped     int // whole cues removed as consecutive duplicates
	LinesCollapsed  int // text lines whose in-line repetition was shortened
	LinesUncensored int // text lines in which masked words were spelled out

	MusicLinesCleaned int // text lines from which ♪ symbols were stripped
	MusicCuesDropped  int // cues that held nothing but music symbols
	LongCuesDropped   int // cues longer than maxCueDuration

	FillerLinesTrimmed int // text lines from which interjections were cut
	FillerCuesDropped  int // cues that held nothing but interjections

	SoundLinesCleaned int // text lines from which *sobs*-style annotations were cut
	SoundCuesDropped  int // cues that held nothing but such annotations
}

// Changed reports whether anything was modified.
func (s Stats) Changed() bool {
	return s.CuesDropped > 0 || s.LinesCollapsed > 0 || s.LinesUncensored > 0 ||
		s.MusicLinesCleaned > 0 || s.MusicCuesDropped > 0 || s.LongCuesDropped > 0 ||
		s.FillerLinesTrimmed > 0 || s.FillerCuesDropped > 0 ||
		s.SoundLinesCleaned > 0 || s.SoundCuesDropped > 0
}

const trimPunct = ".。!！?？…~—\"'“”‘’*"

var (
	tokenRE    = regexp.MustCompile(`([^\s,，、]+)([\s,，、]*)`)
	markupRE   = regexp.MustCompile(`<[^>]+>|\{[^}]*\}`)
	leadDashRE = regexp.MustCompile(`^\s*-\s*`)
	keySepRE   = regexp.MustCompile(`[\s.。!！?？…~—"'“”‘’*,，、\-]+`)
	lineRE     = regexp.MustCompile(`(?s)^(\s*(?:<[^>]+>)*)(.*?)((?:</[^>]+>)*\s*)$`)
	blockSepRE = regexp.MustCompile(`\n[ \t]*\n+`)
	musicRE    = regexp.MustCompile(`[♪♫♬♩🎵🎶]+`)
	timeRE     = regexp.MustCompile(`^\s*(\d+):(\d+):(\d+)[,.](\d+)\s*-->\s*(\d+):(\d+):(\d+)[,.](\d+)`)
)

type token struct{ word, sep string }

func splitPunct(word string) (core, trail string) {
	core = strings.TrimRight(word, trimPunct)
	return core, word[len(core):]
}

func normWord(word string) string {
	if core := strings.ToLower(strings.Trim(word, trimPunct)); core != "" {
		return core
	}
	return word
}

// CollapseRepeats shortens any run of more than MaxRepeat identical
// words/clauses (or identical 2-3 word phrases) in line to MaxRepeat. Words
// are split on whitespace and on , ， 、 so it works for English and Chinese
// alike; the punctuation that ended the run (e.g. the final "." or "。") is
// kept on the last surviving repeat.
func CollapseRepeats(line string) string {
	locs := tokenRE.FindAllStringSubmatchIndex(line, -1)
	if len(locs) == 0 {
		return line
	}
	toks := make([]token, len(locs))
	for i, m := range locs {
		toks[i] = token{line[m[2]:m[3]], line[m[4]:m[5]]}
	}
	lead := line[:locs[0][0]]

	for {
		norms := make([]string, len(toks))
		for i, t := range toks {
			norms[i] = normWord(t.word)
		}
		collapsed := false
	scan:
		for i := 0; i < len(toks); i++ {
			for n := 1; n <= 3 && i+n <= len(toks); n++ {
				k := 1
				for equalRun(norms, i, i+k*n, n) {
					k++
				}
				if k <= MaxRepeat {
					continue
				}
				keepEnd, runEnd := i+MaxRepeat*n, i+k*n
				lastKept, lastRun := toks[keepEnd-1], toks[runEnd-1]
				core, _ := splitPunct(lastKept.word)
				_, trail := splitPunct(lastRun.word)
				toks[keepEnd-1] = token{core + trail, lastRun.sep}
				toks = append(toks[:keepEnd], toks[runEnd:]...)
				collapsed = true
				break scan
			}
		}
		if !collapsed {
			break
		}
	}

	var b strings.Builder
	b.WriteString(lead)
	for _, t := range toks {
		b.WriteString(t.word)
		b.WriteString(t.sep)
	}
	return b.String()
}

// equalRun reports whether norms[b:b+n] exists and equals norms[a:a+n].
func equalRun(norms []string, a, b, n int) bool {
	if b+n > len(norms) {
		return false
	}
	for j := 0; j < n; j++ {
		if norms[a+j] != norms[b+j] {
			return false
		}
	}
	return true
}

// collapseTextLine applies CollapseRepeats to a subtitle text line while
// leaving surrounding markup such as <font ...> ... </font> untouched.
func collapseTextLine(line string) string {
	m := lineRE.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	return m[1] + CollapseRepeats(m[2]) + m[3]
}

type cue struct {
	index      string
	timing     string
	lines      []string
	start, end float64
	timed      bool
}

// stripMusic removes music symbols and tidies the whitespace they leave behind.
func stripMusic(line string) string {
	if !musicRE.MatchString(line) {
		return line
	}
	return strings.Join(strings.Fields(musicRE.ReplaceAllString(line, " ")), " ")
}

type lineChange struct{ uncensored, collapsed, music, sound, filler bool }

// cleanLine applies every per-line cleanup, in order, and reports which ones
// changed the line. An empty result means nothing worth showing is left.
func cleanLine(l string) (string, lineChange) {
	var ch lineChange
	if nl := Uncensor(l); nl != l {
		l, ch.uncensored = nl, true
	}
	if nl := collapseTextLine(l); nl != l {
		l, ch.collapsed = nl, true
	}
	if nl := stripMusic(l); nl != l {
		l, ch.music = nl, true
	}
	if nl := stripSoundTags(l); nl != l {
		l, ch.sound = nl, true
	}
	if nl := stripFillers(l); nl != l {
		l, ch.filler = nl, true
	}
	if strings.TrimSpace(markupRE.ReplaceAllString(l, "")) == "" {
		l = ""
	}
	return l, ch
}

// CleanLine applies the per-line cleanups of CleanSRT (un-masking profanity,
// trimming repetition, stripping music symbols and interjections) to a single
// subtitle text line. It returns "" if nothing worth showing is left. Callers
// that process several aligned files (e.g. translations) use it to keep their
// text consistent with what CleanSRT does.
func CleanLine(l string) string {
	out, _ := cleanLine(l)
	return out
}

func cueKey(lines []string) string {
	t := strings.ToLower(markupRE.ReplaceAllString(strings.Join(lines, " "), ""))
	t = leadDashRE.ReplaceAllString(t, "")
	return strings.TrimSpace(keySepRE.ReplaceAllString(t, " "))
}

func parseSeconds(m []string, off int) float64 {
	h, _ := strconv.Atoi(m[off])
	mi, _ := strconv.Atoi(m[off+1])
	s, _ := strconv.Atoi(m[off+2])
	frac, _ := strconv.Atoi(m[off+3])
	f := float64(frac)
	for range m[off+3] {
		f /= 10
	}
	return float64(h*3600+mi*60+s) + f
}

// CleanSRT cleans up SRT data:
//
//  0. masked profanity is spelled out first ("f*cking" -> "fucking"), so the
//     repetition checks below see the real words;
//  1. inside a cue, a word/phrase repeated more than MaxRepeat times in a row
//     is cut to MaxRepeat repeats (see CollapseRepeats);
//  2. music symbols (♪) and sound annotations (*sobs*, [Music]) are stripped;
//  3. interjections are stripped (see stripFillers); a cue left with no text
//     is dropped;
//  4. a cue longer than maxCueDuration is dropped as a recognition error;
//  5. more than MaxRepeat consecutive cues with identical text (ignoring case,
//     punctuation and a leading "- ", and no more than maxCueGap seconds
//     apart) keep only the first MaxRepeat.
//
// Surviving cues are renumbered. A BOM, CRLF line endings and the presence of
// a trailing newline are preserved. Input that doesn't parse as SRT is
// returned unchanged.
func CleanSRT(src []byte) ([]byte, Stats, error) {
	var stats Stats
	bom := bytes.HasPrefix(src, []byte("\xef\xbb\xbf"))
	body := bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	crlf := bytes.Contains(body, []byte("\r\n"))
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	finalNL := strings.HasSuffix(text, "\n")
	text = strings.Trim(text, "\n")
	if strings.TrimSpace(text) == "" {
		return src, stats, nil
	}

	var cues []cue
	for _, blk := range blockSepRE.Split(text, -1) {
		lines := strings.Split(blk, "\n")
		if len(lines) < 2 {
			return src, stats, fmt.Errorf("subtitle: block without a timing line: %q", blk)
		}
		c := cue{index: lines[0], timing: lines[1], lines: lines[2:]}
		if m := timeRE.FindStringSubmatch(lines[1]); m != nil {
			c.start, c.end, c.timed = parseSeconds(m, 1), parseSeconds(m, 5), true
		} else {
			return src, stats, fmt.Errorf("subtitle: bad timing line %q", lines[1])
		}
		cues = append(cues, c)
	}

	emptied := make([]lineChange, len(cues)) // what emptied a cue, if anything
	for i := range cues {
		var all lineChange
		var lines []string
		for _, l := range cues[i].lines {
			nl, ch := cleanLine(l)
			if ch.uncensored {
				stats.LinesUncensored++
			}
			if ch.collapsed {
				stats.LinesCollapsed++
			}
			if ch.music {
				stats.MusicLinesCleaned++
			}
			if ch.filler {
				stats.FillerLinesTrimmed++
			}
			if ch.sound {
				stats.SoundLinesCleaned++
			}
			all.music = all.music || ch.music
			all.sound = all.sound || ch.sound
			all.filler = all.filler || ch.filler
			if nl != "" {
				lines = append(lines, nl)
			}
		}
		cues[i].lines = lines
		emptied[i] = all
	}

	var kept []cue
	run := 0
	prevKey := ""
	var prev cue
	for i, c := range cues {
		if len(c.lines) == 0 {
			switch {
			case emptied[i].music:
				stats.MusicCuesDropped++
			case emptied[i].sound:
				stats.SoundCuesDropped++
			case emptied[i].filler:
				stats.FillerCuesDropped++
			default:
				stats.MusicCuesDropped++ // already empty
			}
			continue
		}
		if c.end-c.start > maxCueDuration {
			stats.LongCuesDropped++
			continue
		}
		key := cueKey(c.lines)
		if len(kept) > 0 && key != "" && key == prevKey && c.start-prev.end <= maxCueGap {
			run++
		} else {
			run = 1
		}
		prevKey, prev = key, c
		if run > MaxRepeat {
			stats.CuesDropped++
			continue
		}
		kept = append(kept, c)
	}
	if !stats.Changed() {
		return src, stats, nil
	}

	blocks := make([]string, len(kept))
	for i, c := range kept {
		parts := append([]string{strconv.Itoa(i + 1), c.timing}, c.lines...)
		blocks[i] = strings.Join(parts, "\n")
	}
	out := strings.Join(blocks, "\n\n")
	if finalNL {
		out += "\n"
	}
	if crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	res := []byte(out)
	if bom {
		res = append([]byte("\xef\xbb\xbf"), res...)
	}
	return res, stats, nil
}

// CleanFile runs CleanSRT over the file at path and rewrites it only if
// something changed. The rewrite goes through a temp file + rename, so a
// failure never leaves a half-written subtitle behind.
func CleanFile(path string) (Stats, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Stats{}, err
	}
	out, stats, err := CleanSRT(src)
	if err != nil || !stats.Changed() {
		return stats, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dedupe-*.tmp")
	if err != nil {
		return stats, err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return stats, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return stats, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return stats, err
	}
	return stats, nil
}

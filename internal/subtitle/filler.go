package subtitle

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// fillerEN matches English interjections, hesitation sounds and bare
// affirmations ("oh", "ahh", "mm-hmm", "uh-huh", "wow", "yeah", "yes", ...).
// It is matched against a single lower-cased word with surrounding
// punctuation already stripped.
var fillerEN = regexp.MustCompile(`^(?:` +
	`o+h+|a+h+|u+h+|u+m+|e+h+|e+r+m*|` + // oh, ah, uh, um, eh, er
	`h*m+|m+h+m*|hm+|` + // mm, mmm, hm, mhm
	`(?:ha)+h?|he+y+|wo+w+|wh?o+a+h*|woo+|aw+|ow+|ew+|ugh+|oof|huh|` +
	`yeah|yea|yes|yep|yup|` +
	`uh-huh|uh-uh|mm-hmm|mm-mm` +
	`)$`)

// fillerCJK matches Chinese interjections / modal particles and "对" (yes).
var fillerCJK = regexp.MustCompile(`^(?:[噢哦喔啊嗯呃额哇嘿唉哎呀哟哈呵嘻哼嗷呜咦]+|对)$`)

func isFiller(word string) bool {
	w := normWord(word)
	return fillerEN.MatchString(strings.ToLower(w)) || fillerCJK.MatchString(w)
}

// stripFillers removes interjections from line:
//
//   - a line made up of nothing but fillers ("Mm-hmm.", "Oh, yeah.", "- 对。")
//     becomes "";
//   - fillers at the start or end of a longer line are cut ("Yeah, look at
//     me." -> "Look at me."; "Fuck, yeah." -> "Fuck."), keeping the line's
//     closing punctuation and re-capitalising a new first word.
//
// Fillers in the middle of a sentence are left alone — cutting them there
// tends to leave broken text. Markup such as <font ...> and a leading "- " are
// preserved.
func stripFillers(line string) string {
	m := lineRE.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	lead, text, trail := m[1], m[2], m[3]
	dash := leadDashRE.FindString(text)
	body := text[len(dash):]

	locs := tokenRE.FindAllStringSubmatchIndex(body, -1)
	if len(locs) == 0 {
		return line
	}
	toks := make([]token, len(locs))
	for i, loc := range locs {
		toks[i] = token{body[loc[2]:loc[3]], body[loc[4]:loc[5]]}
	}
	pre := body[:locs[0][0]]

	first, last := 0, len(toks)
	for first < last && isFiller(toks[first].word) {
		first++
	}
	if first == last {
		return "" // nothing but fillers
	}
	for last > first && isFiller(toks[last-1].word) {
		last--
	}
	if first == 0 && last == len(toks) {
		return line
	}

	kept := append([]token(nil), toks[first:last]...)
	if last < len(toks) {
		// The dropped tail carried the line's closing punctuation; move it
		// onto the last surviving word.
		lastKept := kept[len(kept)-1]
		core, ownTrail := splitPunct(lastKept.word)
		_, tailTrail := splitPunct(toks[len(toks)-1].word)
		if tailTrail == "" {
			tailTrail = ownTrail
		}
		kept[len(kept)-1] = token{core + tailTrail, toks[len(toks)-1].sep}
	}
	if first > 0 {
		kept[0].word = recapitalise(toks[0].word, kept[0].word)
	}

	var b strings.Builder
	b.WriteString(lead)
	b.WriteString(dash)
	b.WriteString(pre)
	for _, t := range kept {
		b.WriteString(t.word)
		b.WriteString(t.sep)
	}
	b.WriteString(trail)
	return b.String()
}

// recapitalise upper-cases the first letter of word if the line used to start
// with a capitalised word (oldFirst).
func recapitalise(oldFirst, word string) string {
	r, _ := utf8.DecodeRuneInString(oldFirst)
	w, size := utf8.DecodeRuneInString(word)
	if unicode.IsUpper(r) && unicode.IsLower(w) {
		return string(unicode.ToUpper(w)) + word[size:]
	}
	return word
}

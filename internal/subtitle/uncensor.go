package subtitle

import (
	"regexp"
	"strings"
	"unicode"
)

// censoredWords are the profanities whisper tends to emit with letters masked
// by '*' ("f*cking", "sh*t"). Only words in this list are ever restored, so
// sound annotations like "*sobs*" or "noises*" are left alone.
var censoredWords = []string{
	"fuck", "fucks", "fucked", "fucker", "fuckers", "fucking", "motherfucker", "motherfuckers", "motherfucking",
	"shit", "shits", "shitty", "bullshit",
	"bitch", "bitches", "bitchy",
	"dick", "dicks", "cock", "cocks", "pussy", "pussies", "cunt", "cunts",
	"asshole", "assholes", "bastard", "bastards", "whore", "whores", "slut", "sluts",
	"damn", "goddamn", "dammit", "piss", "pissed",
}

// censoredRE matches a word with at least one '*' between letters and letters
// on both ends ("f*cking", "f**k", "sh*t") — not "*sobs" or "noises*".
var censoredRE = regexp.MustCompile(`[A-Za-z]+(?:\*+[A-Za-z]+)+`)

// Uncensor spells out masked profanity: "f*cking" -> "fucking". Each '*'
// stands for exactly one missing letter, and a mask is only restored when it
// fits exactly one word in censoredWords (same length, same visible letters),
// so an ambiguous mask is left as it was. The original capitalisation style
// (lower, Capitalised, UPPER) is kept.
func Uncensor(line string) string {
	return censoredRE.ReplaceAllStringFunc(line, func(masked string) string {
		lower := strings.ToLower(masked)
		match := ""
		for _, w := range censoredWords {
			if fits(lower, w) {
				if match != "" && match != w {
					return masked // ambiguous
				}
				match = w
			}
		}
		if match == "" {
			return masked
		}
		return applyCase(masked, match)
	})
}

// fits reports whether word has the same length as masked and agrees with it
// on every non-'*' position.
func fits(masked, word string) bool {
	if len(masked) != len(word) {
		return false
	}
	for i := 0; i < len(masked); i++ {
		if masked[i] != '*' && masked[i] != word[i] {
			return false
		}
	}
	return true
}

func applyCase(masked, word string) string {
	letters := strings.ReplaceAll(masked, "*", "")
	switch {
	case letters == strings.ToUpper(letters):
		return strings.ToUpper(word)
	case unicode.IsUpper(rune(masked[0])):
		return strings.ToUpper(word[:1]) + word[1:]
	}
	return word
}

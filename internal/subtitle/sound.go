package subtitle

import (
	"regexp"
	"strings"
)

// soundTagRE matches non-speech annotations whisper writes for noises:
// *sobs*, *Humming*, *Mumbling*, [Music], [BLANK_AUDIO], (laughs), （叹气）,
// 【音乐】. The leading group (kept via $1) makes sure the opening bracket does
// not sit inside a word, so "f*cking" can never look like an annotation.
var soundTagRE = regexp.MustCompile(`(^|[^\p{L}\p{N}*])(?:` +
	`\*[^*\n]{1,60}\*|` +
	`\[[^\]\n]{1,60}\]|` +
	`\([^)\n]{1,60}\)|` +
	`（[^）\n]{1,60}）|` +
	`【[^】\n]{1,60}】` +
	`)`)

// stripSoundTags removes sound annotations from line and tidies the spacing
// they leave behind: "*Sings* Yes, baby." -> "Yes, baby.". A line that was
// only an annotation ("*Mumbling*") ends up empty, which drops its cue.
func stripSoundTags(line string) string {
	if !soundTagRE.MatchString(line) {
		return line
	}
	out := soundTagRE.ReplaceAllString(line, "$1")
	return strings.Join(strings.Fields(out), " ")
}

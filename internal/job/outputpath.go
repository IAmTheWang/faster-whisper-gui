package job

import (
	"path/filepath"
	"strings"
)

// OutputPrefix computes the whisper-cli "-of" prefix (an absolute path with
// no extension) for req: the source file's own directory and basename by
// default, or OutputDir with the source file's basename when a custom
// output directory was chosen.
//
// Both the media path and a custom output directory may arrive with mixed
// path separators (a user can type "E:/videos" into the custom-directory
// field), so both are normalized via filepath.Clean(filepath.FromSlash(...))
// here rather than trusting the caller to have done it. Only the source
// file's final extension is stripped, so multi-dot filenames like
// "my.video.v2.mp4" become ".../my.video.v2", not ".../my".
func OutputPrefix(req Request) string {
	mediaPath := filepath.Clean(filepath.FromSlash(req.MediaPath))

	dir := filepath.Dir(mediaPath)
	if req.OutputMode == OutputCustom && req.OutputDir != "" {
		dir = filepath.Clean(filepath.FromSlash(req.OutputDir))
	}

	base := filepath.Base(mediaPath)
	nameNoExt := strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(dir, nameNoExt)
}

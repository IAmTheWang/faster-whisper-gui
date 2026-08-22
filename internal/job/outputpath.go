package job

import (
	"path/filepath"
	"strings"
)

// OutputPrefix computes the whisper-cli "-of" prefix (an absolute path with
// no extension) for req: the video's own directory and basename by default,
// or OutputDir with the video's basename when a custom output directory was
// chosen.
//
// Both the video path and a custom output directory may arrive with mixed
// path separators (a user can type "E:/videos" into the custom-directory
// field), so both are normalized via filepath.Clean(filepath.FromSlash(...))
// here rather than trusting the caller to have done it. Only the video's
// final extension is stripped, so multi-dot filenames like
// "my.video.v2.mp4" become ".../my.video.v2", not ".../my".
func OutputPrefix(req Request) string {
	videoPath := filepath.Clean(filepath.FromSlash(req.VideoPath))

	dir := filepath.Dir(videoPath)
	if req.OutputMode == OutputCustom && req.OutputDir != "" {
		dir = filepath.Clean(filepath.FromSlash(req.OutputDir))
	}

	base := filepath.Base(videoPath)
	nameNoExt := strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(dir, nameNoExt)
}

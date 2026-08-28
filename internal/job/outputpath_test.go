package job

import "testing"

func TestOutputPrefix(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{
			name: "default same-as-source directory",
			req:  Request{MediaPath: `E:\videos\lecture1.mp4`, OutputMode: OutputSameAsSource},
			want: `E:\videos\lecture1`,
		},
		{
			name: "custom output directory",
			req: Request{
				MediaPath:  `E:\videos\lecture1.mp4`,
				OutputMode: OutputCustom,
				OutputDir:  `E:\srt-out`,
			},
			want: `E:\srt-out\lecture1`,
		},
		{
			name: "custom mode but empty OutputDir falls back to source directory",
			req: Request{
				MediaPath:  `E:\videos\lecture1.mp4`,
				OutputMode: OutputCustom,
				OutputDir:  "",
			},
			want: `E:\videos\lecture1`,
		},
		{
			name: "path with spaces",
			req:  Request{MediaPath: `E:\test videos\my clip.mp4`, OutputMode: OutputSameAsSource},
			want: `E:\test videos\my clip`,
		},
		{
			name: "non-ASCII path",
			req:  Request{MediaPath: `E:\test videos\日本語 sample (1).mp4`, OutputMode: OutputSameAsSource},
			want: `E:\test videos\日本語 sample (1)`,
		},
		{
			name: "multi-dot filename keeps all but the final extension",
			req:  Request{MediaPath: `E:\videos\my.video.v2.mp4`, OutputMode: OutputSameAsSource},
			want: `E:\videos\my.video.v2`,
		},
		{
			name: "uppercase extension",
			req:  Request{MediaPath: `E:\videos\lecture1.MP4`, OutputMode: OutputSameAsSource},
			want: `E:\videos\lecture1`,
		},
		{
			name: "forward slashes in media path get normalized",
			req:  Request{MediaPath: `E:/videos/lecture1.mp4`, OutputMode: OutputSameAsSource},
			want: `E:\videos\lecture1`,
		},
		{
			name: "forward slashes in custom output dir get normalized",
			req: Request{
				MediaPath:  `E:\videos\lecture1.mp4`,
				OutputMode: OutputCustom,
				OutputDir:  `E:/srt-out`,
			},
			want: `E:\srt-out\lecture1`,
		},
		{
			name: "mixed slashes and redundant separators",
			req:  Request{MediaPath: `E:/videos//sub\.\lecture1.mp4`, OutputMode: OutputSameAsSource},
			want: `E:\videos\sub\lecture1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OutputPrefix(tt.req); got != tt.want {
				t.Errorf("OutputPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

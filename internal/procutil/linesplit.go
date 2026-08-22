// Package procutil provides Windows-specific helpers for managing whisper-cli
// and ffmpeg subprocesses: line-buffered output splitting and process-tree
// cleanup via Job Objects.
package procutil

import (
	"bufio"
	"io"
)

// ScanCRLines is a bufio.SplitFunc like bufio.ScanLines, except it also
// treats a bare '\r' (without a following '\n') as a line terminator.
//
// CLI progress bars (whisper-cli, ffmpeg) commonly redraw a line in place by
// writing '\r' instead of '\n'. bufio.ScanLines only splits on '\n', so those
// updates sit in the buffer until the next real '\n' arrives, making progress
// appear to stall and then jump.
func ScanCRLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	for i, b := range data {
		switch b {
		case '\n':
			return i + 1, dropTrailingCR(data[:i]), nil
		case '\r':
			// A '\r' followed by '\n' is a single CRLF terminator; let the
			// '\n' branch above handle it so we don't emit an empty token.
			if i+1 < len(data) {
				if data[i+1] == '\n' {
					continue
				}
				return i + 1, data[:i], nil
			}
			// '\r' is the last byte we have so far — it might be the start
			// of a CRLF pair that hasn't arrived yet. Ask for more data
			// unless we're at EOF, in which case treat it as a terminator.
			if atEOF {
				return i + 1, data[:i], nil
			}
		}
	}

	if atEOF {
		return len(data), data, nil
	}

	return 0, nil, nil
}

func dropTrailingCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[:len(data)-1]
	}
	return data
}

// NewCRLineScanner returns a bufio.Scanner configured with ScanCRLines, ready
// to read line-oriented (and '\r'-redrawn) output from a subprocess pipe.
func NewCRLineScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Split(ScanCRLines)
	// whisper-cli/ffmpeg lines are short, but be generous in case a build
	// prints a long diagnostic line without a newline for a while.
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return s
}

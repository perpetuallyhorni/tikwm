package logging

import (
	"io"
	"regexp"
	"strings"

	"github.com/perpetuallyhorni/tikwm/pkg/client"
)

var (
	// videoIDRegex matches long numeric strings typical of TikTok video IDs.
	videoIDRegex = regexp.MustCompile(`\b\d{18,}\b`)
)

// RedactingWriter is an io.Writer that redacts sensitive information before
// writing to an underlying writer.
type RedactingWriter struct {
	underlying   io.Writer                 // The underlying writer to write to.
	replacements map[*regexp.Regexp]string // Map of regex patterns to their replacements.
}

// NewRedactingWriter creates a new writer that redacts specified patterns.
func NewRedactingWriter(w io.Writer, downloadPath string, targets []string) io.Writer {
	replacements := make(map[*regexp.Regexp]string)

	// Add static redactions
	replacements[videoIDRegex] = "[VIDEO_ID]"

	// Add dynamic redactions
	if downloadPath != "" {
		// Quote meta characters in path and handle path separators for different OS
		sanitizedPath := strings.ReplaceAll(regexp.QuoteMeta(downloadPath), `\\`, `[/\\]`)
		replacements[regexp.MustCompile(sanitizedPath)] = "[DOWNLOAD_PATH]"
	}

	for _, target := range targets {
		username := client.ExtractUsername(target) // Extract username from the target string.
		if username != "" {
			replacements[regexp.MustCompile(regexp.QuoteMeta(username))] = "[USERNAME]"
		}
	}

	return &RedactingWriter{
		underlying:   w,
		replacements: replacements,
	}
}

// Write redacts the input byte slice and writes it to the underlying writer.
func (rw *RedactingWriter) Write(p []byte) (n int, err error) {
	originalLen := len(p) // Store the original length of the input.
	message := string(p)  // Convert the byte slice to a string.
	for re, repl := range rw.replacements {
		message = re.ReplaceAllString(message, repl) // Replace all occurrences of the pattern with the replacement string.
	}

	_, err = rw.underlying.Write([]byte(message)) // Write the redacted message to the underlying writer.
	if err != nil {
		return 0, err
	}

	// We return the original length to satisfy the io.Writer contract,
	// even if the written length is different. The caller is interested
	// in whether the original buffer was processed.
	return originalLen, nil
}

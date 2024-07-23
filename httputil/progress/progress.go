// Package progress makes it possible to display download progress.
package progress

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/bazelbuild/bazelisk/config"
)

// Progress shows a download progress bar.
type progress struct {
	header      string
	total       int64
	current     int64
	lastMessage string
}

func showProgress(config config.Config) bool {
	// If stdout is a terminal, don't show progress.
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}

	// Check the config variable.
	showProgress := config.Get("BAZELISK_SHOW_PROGRESS")
	if len(showProgress) == 0 {
		// Default to showing progress
		return true
	}
	switch strings.ToLower(showProgress) {
	case "":
		return true // Default to on
	case "yes":
		return true
	case "y":
		return true
	case "true":
		return true
	case "1":
		return true
	case "no":
		return false
	case "n":
		return false
	case "false":
		return false
	case "0":
		return false
	}
	// TODO: default: error

	return true
}

// Writer creates an io.Writer to print the progress.
func Writer(w io.Writer, header string, total int64, config config.Config) io.Writer {
	if !showProgress(config) {
		return w
	}
	prog := &progress{
		header: header,
		total:  total,
	}
	out := io.MultiWriter(w, prog)
	return out
}

// Finish writes final output after the progress bar is complete. 
func Finish(config config.Config) {
	if showProgress(config) {
		// Add a newline after the progress bar
		fmt.Println()
	}
}

func (p *progress) Write(buf []byte) (int, error) {
	l := len(buf)
	p.current += int64(l)
	p.ShowProgress()
	return l, nil
}

// Writes the current download progress to stdout.
func (p *progress) ShowProgress() {
	var message string

	if p.total > 0 {
		message = fmt.Sprintf("%s: %s out of %s (%s)",
			p.header,
			formatMb(p.current),
			formatMb(p.total),
			formatPercentage(p.current, p.total))
	} else {
		message = fmt.Sprintf("%s: %s",
			p.header,
			formatMb(p.current))
	}

	if message == p.lastMessage {
		return
	}

	// Clear the line.
	fmt.Printf("\r%s", strings.Repeat(" ", 39))

	// Show a message, don't add a newline
	fmt.Printf("\r%s", message)
	p.lastMessage = message
}

func formatMb(size int64) string {
	// TODO: Use units other than MB
	inMb := size / (1024 * 1024)
	return fmt.Sprintf("%d MB", inMb)
}

func formatPercentage(current, size int64) string {
	percentage := current * 100 / size
	return fmt.Sprintf("%d%%", percentage)
}

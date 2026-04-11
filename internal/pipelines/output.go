package pipelines

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleCyan    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00E5FF")).Bold(true)
	styleWhite   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	styleGreen   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00E676")).Bold(true)
	styleRed     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5252")).Bold(true)
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("#546E7A"))
	styleMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("#B0BEC5"))
	styleYellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD740")).Bold(true)
	styleMagenta = lipgloss.NewStyle().Foreground(lipgloss.Color("#CE93D8"))
)

// Logger handles all styled terminal output for pipeline execution.
// All output is filtered through the Masker to prevent secret leaks.
type Logger struct {
	Masker *Masker
}

func (l *Logger) mask(msg string) string {
	if l.Masker != nil {
		return l.Masker.Mask(msg)
	}
	return msg
}

func (l *Logger) ts() string {
	return styleDim.Render(time.Now().Format("15:04:05"))
}

// Header prints a prominent section header.
func (l *Logger) Header(msg string) {
	fmt.Printf("  %s %s\n", l.ts(), styleCyan.Render(l.mask(msg)))
}

// Info prints an informational line.
func (l *Logger) Info(msg string) {
	fmt.Printf("  %s %s\n", l.ts(), styleMuted.Render(l.mask(msg)))
}

// Step prints a build step with an arrow prefix.
func (l *Logger) Step(msg string) {
	fmt.Printf("  %s %s %s\n", l.ts(), styleDim.Render("==>"), styleWhite.Render(l.mask(msg)))
}

// Command prints a shell command, styled differently for dry-run.
func (l *Logger) Command(command string, dryRun bool) {
	prefix := "$"
	if dryRun {
		prefix = styleYellow.Render("[dry-run]") + " $"
	}
	fmt.Printf("  %s     %s %s\n", l.ts(), prefix, styleMagenta.Render(l.mask(command)))
}

// Promote prints a promotion message.
func (l *Logger) Promote(msg string) {
	fmt.Printf("  %s %s %s\n", l.ts(), styleDim.Render("-->"), styleMuted.Render(l.mask(msg)))
}

// Success prints a success message.
func (l *Logger) Success(msg string) {
	fmt.Printf("  %s %s\n", l.ts(), styleGreen.Render(l.mask(msg)))
}

// Fail prints a failure message.
func (l *Logger) Fail(msg string) {
	fmt.Printf("  %s %s\n", l.ts(), styleRed.Render(l.mask(msg)))
}

// Separator prints an empty line.
func (l *Logger) Separator() {
	fmt.Println()
}

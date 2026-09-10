package cli

import (
	"fmt"
	"os"
	"strings"
)

// Terminal styling enabled check (respects NO_COLOR standard)
func isColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}

func style(code, s string) string {
	if !isColorEnabled() || s == "" {
		return s
	}
	return code + s + "\033[0m"
}

// Styles & Colors
func Bold(s string) string          { return style("\033[1m", s) }
func Dim(s string) string           { return style("\033[2m", s) }
func Underline(s string) string     { return style("\033[4m", s) }
func Cyan(s string) string          { return style("\033[36m", s) }
func BrightCyan(s string) string    { return style("\033[96m", s) }
func Green(s string) string         { return style("\033[32m", s) }
func BrightGreen(s string) string   { return style("\033[92m", s) }
func Yellow(s string) string        { return style("\033[33m", s) }
func BrightYellow(s string) string  { return style("\033[93m", s) }
func Magenta(s string) string       { return style("\033[35m", s) }
func BrightMagenta(s string) string { return style("\033[95m", s) }
func Blue(s string) string          { return style("\033[34m", s) }
func Red(s string) string           { return style("\033[31m", s) }
func BrightRed(s string) string     { return style("\033[91m", s) }
func Gray(s string) string          { return style("\033[90m", s) }

// Badges and Icons
func IconSuccess() string       { return BrightGreen("✔") }
func IconWarning() string       { return BrightYellow("▲") }
func IconError() string         { return BrightRed("✖") }
func IconInfo() string          { return BrightCyan("ℹ") }
func IconBullet() string        { return Gray("•") }
func IconArrow() string         { return BrightCyan("➔") }
func IconPointer() string       { return BrightCyan("▸") }
func IconRadioActive() string   { return BrightGreen("●") }
func IconRadioInactive() string { return Gray("○") }

func Badge(text, fgCode, bgCode string) string {
	if !isColorEnabled() {
		return "[" + text + "]"
	}
	return fmt.Sprintf("\033[%s;%sm %s \033[0m", fgCode, bgCode, text)
}

func BadgeCyan(text string) string {
	return Badge(text, "30", "46")
}

func BadgeGreen(text string) string {
	return Badge(text, "30", "42")
}

func BadgeYellow(text string) string {
	return Badge(text, "30", "43")
}

func BadgeRed(text string) string {
	return Badge(text, "37", "41")
}

func BadgeMagenta(text string) string {
	return Badge(text, "30", "45")
}

// BrandBanner renders a modern CLI banner
func BrandBanner(subtitle string) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + BrightCyan("⚡") + " " + Bold(BrightCyan("CAPACITYLAB")) + " " + Dim("v0.1.0-alpha") + "\n")
	if subtitle != "" {
		b.WriteString("  " + Dim(subtitle) + "\n")
	}
	b.WriteString("  " + Gray("─────────────────────────────────────────────────────────────") + "\n")
	return b.String()
}

// Card renders a modern bordered card
func Card(title string, rows [][2]string) string {
	var b strings.Builder
	width := 64

	// Header line
	b.WriteString("\n  " + Gray("┌─ ") + Bold(title) + " " + Gray(strings.Repeat("─", max(0, width-len(title)-5))) + Gray("┐") + "\n")
	b.WriteString("  " + Gray("│") + strings.Repeat(" ", width-2) + Gray("│") + "\n")

	for _, row := range rows {
		key := row[0]
		val := row[1]
		lineContent := fmt.Sprintf("    %-12s %s", Bold(key), val)
		// calculate visual length roughly
		padding := max(1, width-2-visualLen(lineContent))
		b.WriteString("  " + Gray("│") + lineContent + strings.Repeat(" ", padding) + Gray("│") + "\n")
	}

	b.WriteString("  " + Gray("│") + strings.Repeat(" ", width-2) + Gray("│") + "\n")
	b.WriteString("  " + Gray("└") + Gray(strings.Repeat("─", width-2)) + Gray("┘") + "\n")
	return b.String()
}

func visualLen(s string) int {
	inEscape := false
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if s[i] == 'm' {
				inEscape = false
			}
			continue
		}
		n++
	}
	return n
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

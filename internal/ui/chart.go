package ui

import (
	"fmt"
	"strings"
)

// sparkRunes index 0 is blank; 1..8 are increasing block heights.
var sparkRunes = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// heatRunes index 0 is "nothing happened"; 1..4 are increasing intensity.
// The calendar shades a day's cell with 1..4 and draws a blank for 0.
var heatRunes = []rune{'·', '▁', '▄', '▓', '█'}

// Sparkline renders one block rune per value, scaled to the series max.
func Sparkline(values []int) string {
	if len(values) == 0 {
		return ""
	}
	max := 0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	var b strings.Builder
	for _, v := range values {
		if v <= 0 || max == 0 {
			b.WriteRune(sparkRunes[0])
			continue
		}
		b.WriteRune(sparkRunes[1+(v*7)/max])
	}
	return b.String()
}

func heatLevel(v, max int) int {
	if v <= 0 || max == 0 {
		return 0
	}
	l := 1 + (v*3)/max
	if l > 4 {
		l = 4
	}
	return l
}

// HBar renders "label ████░░░░ 5" scaled to max across width cells. The
// label field fits the longest column name ("testing-review").
func HBar(label string, value, max, width int) string {
	filled := 0
	if max > 0 && value > 0 {
		filled = value * width / max
		if filled == 0 {
			filled = 1
		}
	}
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + MutedStyle.Render(strings.Repeat("░", width-filled))
	return fmt.Sprintf("%-14s %s %d", label, bar, value)
}

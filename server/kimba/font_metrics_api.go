package kimba

//go:generate sh -c "go run ./fontgen ../../references/sdk/hiptop/x/hiptop-sdk/libs/3.4/simulator.jar > font_metrics.go"

// fontMetrics are a device font's line metrics and advance widths.
type fontMetrics struct {
	ascent, descent int
	fallback        int // width of characters outside the table
	widths          [0x250]uint8
}

// defaultFontID is what the device uses when a style context sets no font
// (KStyleContext.mFontId).
const defaultFontID = fontBort9

func metricsFor(fontID int) *fontMetrics {
	if fontID < 0 || fontID >= len(fontMetricsTable) {
		fontID = defaultFontID
	}
	return &fontMetricsTable[fontID]
}

// textWidth is the width in pixels of s drawn in the given font.
func textWidth(s string, fontID int) int {
	m := metricsFor(fontID)
	w := 0
	for _, r := range s {
		if r >= 0 && int(r) < len(m.widths) {
			w += int(m.widths[r])
		} else {
			w += m.fallback
		}
	}
	return w
}

func fontLineHeight(fontID int) int {
	m := metricsFor(fontID)
	return m.ascent + m.descent
}

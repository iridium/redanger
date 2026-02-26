package kimba

import (
	"strconv"
	"strings"
)

const (
	displayBlock  = 0 // block-level element
	displayInline = 1 // inline-level element
	displayNone   = 3 // not rendered
)

const (
	textDecorationNone          = 0
	textDecorationUnderline     = 1
	textDecorationStrikethrough = 2
)

const (
	textTransformNone       = 0
	textTransformCapitalize = 1
	textTransformUppercase  = 2
	textTransformLowercase  = 3
)

// Text align values for styleContext.textAlign (internal representation).
// Note: these differ from the KBC wire format; serialize() remaps them.
const (
	textAlignCenter  = 1
	textAlignRight   = 2
	textAlignLeft    = 3 // explicit left
	textAlignJustify = 4
)

const (
	whiteSpaceNormal = 0
	whiteSpacePre    = 1
	whiteSpaceNowrap = 2
)

const (
	floatNone  = 0
	floatLeft  = 1
	floatRight = 2
)

const (
	listStyleDisc    = 0
	listStyleCircle  = 1
	listStyleSquare  = 2
	listStyleDecimal = 3
	listStyleNone    = 5
)

// Border style enum values (from KBC.java).
const (
	borderStyleNone   = 0
	borderStyleDotted = 1
	borderStyleDashed = 2
	borderStyleSolid  = 3
	borderStyleDouble = 4
	borderStyleGroove = 5
	borderStyleRidge  = 6
	borderStyleInset  = 7
	borderStyleOutset = 8
)

const (
	borderSideTop    = 0x1
	borderSideRight  = 0x2
	borderSideBottom = 0x4
	borderSideLeft   = 0x8
	borderSideAll    = 0xF
)

const (
	borderWidthThin   = 1
	borderWidthMedium = 3
	borderWidthThick  = 5
)

const linkColorBlue = 0x0000FF

const emToPx = 12

type styleContext struct {
	fontID         int // -1=not set, 0+=font ID from KFontMgr
	color          int32
	colorSet       bool
	bgColor        int32
	bgColorSet     bool
	textDecoration int // -1=not set, 0=none, 1=underline, 2=strikethrough
	textTransform  int // -1=not set, 0=none, 1=capitalize, 2=uppercase, 3=lowercase
	textAlign      int // -1=not set, 0=left(default), 1=center, 2=right, 3=left(explicit), 4=justify
	marginLeft     int // sentinel -1=not set
	marginTop      int
	marginRight    int
	marginBottom   int
	paddingLeft    int // -1=not set
	display        int // -1=not set, 0=block, 1=inline, 3=none
	paddingTop     int
	listStyleType  int // -1=not set, 0=disc, 1=circle, 2=square, 3=decimal
	paddingRight   int
	paddingBottom  int
	whiteSpace     int // -1=not set, 0=normal, 1=pre, 2=nowrap
	lineHeight     int // -1=not set
	// Border properties
	borderTopWidth    int // -1=not set
	borderRightWidth  int
	borderBottomWidth int
	borderLeftWidth   int
	borderTopStyle    int // -1=not set, 0=none, 1=dotted, 2=dashed, 3=solid, ...
	borderRightStyle  int
	borderBottomStyle int
	borderLeftStyle   int
	borderTopColor    int32
	borderRightColor  int32
	borderBottomColor int32
	borderLeftColor   int32
	borderColorSet    [4]bool // top, right, bottom, left
	// Float
	float int // -1=not set, 0=none, 1=left, 2=right
	// Width/height (CSS-level)
	cssWidth  int // -1=not set, pixel value
	cssHeight int // -1=not set, pixel value
}

func (ctx *styleContext) serialize(b *kbcBuilder) {
	if ctx.fontID >= 0 {
		b.writeByte(cssPropFont)
		b.writeUVL(ctx.fontID)
	}
	if ctx.colorSet {
		b.writeByte(cssPropColor)
		b.writeInt32(ctx.color)
	}
	if ctx.bgColorSet {
		b.writeByte(cssPropBgColor)
		b.writeInt32(ctx.bgColor)
	}
	if ctx.textDecoration >= 0 {
		b.writeByte(cssPropTextDecoration)
		b.writeByte(byte(ctx.textDecoration))
	}
	if ctx.textTransform >= 0 {
		b.writeByte(cssPropTextTransform)
		b.writeByte(byte(ctx.textTransform))
	}
	if ctx.marginLeft != -1 {
		b.writeByte(cssPropMarginLeft)
		b.writeSVL(ctx.marginLeft)
	}
	if ctx.marginTop != -1 {
		b.writeByte(cssPropMarginTop)
		b.writeSVL(ctx.marginTop)
	}
	if ctx.marginRight != -1 {
		b.writeByte(cssPropMarginRight)
		b.writeSVL(ctx.marginRight)
	}
	if ctx.marginBottom != -1 {
		b.writeByte(cssPropMarginBottom)
		b.writeSVL(ctx.marginBottom)
	}
	if ctx.paddingLeft != -1 {
		b.writeByte(cssPropPaddingLeft)
		b.writeUVL(ctx.paddingLeft)
	}
	if ctx.display >= 0 {
		b.writeByte(cssPropDisplay)
		b.writeByte(byte(ctx.display))
	}
	if ctx.paddingTop != -1 {
		b.writeByte(cssPropPaddingTop)
		b.writeUVL(ctx.paddingTop)
	}
	if ctx.listStyleType >= 0 {
		b.writeByte(cssPropListStyleType)
		b.writeByte(byte(ctx.listStyleType))
	}
	if ctx.paddingRight != -1 {
		b.writeByte(cssPropPaddingRight)
		b.writeUVL(ctx.paddingRight)
	}
	if ctx.paddingBottom != -1 {
		b.writeByte(cssPropPaddingBottom)
		b.writeUVL(ctx.paddingBottom)
	}
	if ctx.whiteSpace >= 0 {
		b.writeByte(cssPropWhiteSpace)
		b.writeByte(byte(ctx.whiteSpace))
	}
	if ctx.textAlign >= 0 {
		// KBC text-align: 0=center, 1=left, 2=right, 3=justify
		var kbcAlign byte
		switch ctx.textAlign {
		case textAlignCenter:
			kbcAlign = 0
		case textAlignRight:
			kbcAlign = 2
		case textAlignLeft:
			kbcAlign = 1
		case textAlignJustify:
			kbcAlign = 3
		default:
			kbcAlign = 1 // left
		}
		b.writeByte(cssPropTextAlign)
		b.writeByte(kbcAlign)
	}
	if ctx.float >= 0 {
		b.writeByte(cssPropFloat)
		b.writeByte(byte(ctx.float))
	}
	// Border properties
	if ctx.borderTopWidth >= 0 {
		b.writeByte(cssPropBorderTopWidth)
		b.writeUVL(ctx.borderTopWidth)
	}
	if ctx.borderRightWidth >= 0 {
		b.writeByte(cssPropBorderRightWidth)
		b.writeUVL(ctx.borderRightWidth)
	}
	if ctx.borderBottomWidth >= 0 {
		b.writeByte(cssPropBorderBottomWidth)
		b.writeUVL(ctx.borderBottomWidth)
	}
	if ctx.borderLeftWidth >= 0 {
		b.writeByte(cssPropBorderLeftWidth)
		b.writeUVL(ctx.borderLeftWidth)
	}
	if ctx.borderTopStyle >= 0 {
		b.writeByte(cssPropBorderTopStyle)
		b.writeByte(byte(ctx.borderTopStyle))
	}
	if ctx.borderRightStyle >= 0 {
		b.writeByte(cssPropBorderRightStyle)
		b.writeByte(byte(ctx.borderRightStyle))
	}
	if ctx.borderBottomStyle >= 0 {
		b.writeByte(cssPropBorderBottomStyle)
		b.writeByte(byte(ctx.borderBottomStyle))
	}
	if ctx.borderLeftStyle >= 0 {
		b.writeByte(cssPropBorderLeftStyle)
		b.writeByte(byte(ctx.borderLeftStyle))
	}
	if ctx.borderColorSet[0] {
		b.writeByte(cssPropBorderTopColor)
		b.writeInt32(ctx.borderTopColor)
	}
	if ctx.borderColorSet[1] {
		b.writeByte(cssPropBorderRightColor)
		b.writeInt32(ctx.borderRightColor)
	}
	if ctx.borderColorSet[2] {
		b.writeByte(cssPropBorderBottomColor)
		b.writeInt32(ctx.borderBottomColor)
	}
	if ctx.borderColorSet[3] {
		b.writeByte(cssPropBorderLeftColor)
		b.writeInt32(ctx.borderLeftColor)
	}
	if ctx.lineHeight >= 0 {
		b.writeByte(cssPropLineHeight)
		b.writeUVL(ctx.lineHeight)
	}
	b.writeByte(0) // end marker
}

type styleContextTable struct {
	entries []styleContext
}

func (t *styleContextTable) intern(ctx styleContext) int {
	for i := range t.entries {
		if t.entries[i] == ctx {
			return i + 1
		}
	}
	t.entries = append(t.entries, ctx)
	return len(t.entries)
}

func (b *kbcBuilder) writeStyleContextTableChunk(table *styleContextTable) {
	var ctxBuf kbcBuilder
	for i := range table.entries {
		table.entries[i].serialize(&ctxBuf)
	}
	b.writeByte(chunkStyleContextTable)
	dataLen := int32(2 + ctxBuf.buf.Len())
	b.writeInt32(dataLen)
	count := len(table.entries)
	b.writeByte(byte(count >> 8))
	b.writeByte(byte(count))
	b.writeBytes(ctxBuf.buf.Bytes())
}

var namedColors = map[string]int32{
	"black":   0x000000,
	"white":   0xFFFFFF,
	"red":     0xFF0000,
	"green":   0x008000,
	"blue":    0x0000FF,
	"yellow":  0xFFFF00,
	"orange":  0xFFA500,
	"purple":  0x800080,
	"gray":    0x808080,
	"grey":    0x808080,
	"silver":  0xC0C0C0,
	"navy":    0x000080,
	"teal":    0x008080,
	"maroon":  0x800000,
	"olive":   0x808000,
	"lime":    0x00FF00,
	"aqua":    0x00FFFF,
	"fuchsia": 0xFF00FF,
	"pink":    0xFFC0CB,
}

func parseColor(s string) (int32, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, false
	}
	// Handle transparent
	if s == "transparent" {
		return 0xFFFFFF, true // approximate as white
	}
	// Handle inherit/initial/unset
	if s == "inherit" || s == "initial" || s == "unset" || s == "currentcolor" {
		return 0, false
	}
	if c, ok := namedColors[s]; ok {
		return c, true
	}
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		// Handle 8-digit and 4-digit hex (with alpha) by ignoring the alpha
		if len(hex) == 8 {
			hex = hex[:6]
		}
		if len(hex) == 4 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) != 6 {
			return 0, false
		}
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return 0, false
		}
		return int32(v), true
	}
	if strings.HasPrefix(s, "rgba(") && strings.HasSuffix(s, ")") {
		inner := s[5 : len(s)-1]
		parts := splitCSSArgs(inner)
		if len(parts) < 3 {
			return 0, false
		}
		r, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, false
		}
		g, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, false
		}
		bv, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil {
			return 0, false
		}
		r = clamp(r, 0, 255)
		g = clamp(g, 0, 255)
		bv = clamp(bv, 0, 255)
		// Ignore alpha (4th arg)
		return int32((r << 16) | (g << 8) | bv), true
	}
	if strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")") {
		inner := s[4 : len(s)-1]
		parts := splitCSSArgs(inner)
		if len(parts) < 3 {
			return 0, false
		}
		r, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, false
		}
		g, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, false
		}
		bv, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil {
			return 0, false
		}
		r = clamp(r, 0, 255)
		g = clamp(g, 0, 255)
		bv = clamp(bv, 0, 255)
		return int32((r << 16) | (g << 8) | bv), true
	}
	if strings.HasPrefix(s, "hsl(") && strings.HasSuffix(s, ")") {
		return parseHSL(s[4:len(s)-1], false)
	}
	if strings.HasPrefix(s, "hsla(") && strings.HasSuffix(s, ")") {
		return parseHSL(s[5:len(s)-1], true)
	}
	return 0, false
}

func splitCSSArgs(s string) []string {
	// Try comma-separated first
	parts := strings.Split(s, ",")
	if len(parts) >= 3 {
		return parts
	}
	// Try space-separated (modern CSS syntax: rgb(255 0 0))
	parts = strings.Fields(s)
	// Filter out "/" which separates alpha in modern syntax
	var result []string
	for _, p := range parts {
		if p != "/" {
			result = append(result, p)
		}
	}
	return result
}

func parseHSL(args string, hasAlpha bool) (int32, bool) {
	parts := splitCSSArgs(args)
	if len(parts) < 3 {
		return 0, false
	}
	// Parse hue (degrees)
	hStr := strings.TrimSpace(parts[0])
	hStr = strings.TrimSuffix(hStr, "deg")
	h, err := strconv.ParseFloat(hStr, 64)
	if err != nil {
		return 0, false
	}
	// Parse saturation (percentage)
	sStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[1]), "%"))
	sVal, err := strconv.ParseFloat(sStr, 64)
	if err != nil {
		return 0, false
	}
	// Parse lightness (percentage)
	lStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[2]), "%"))
	lVal, err := strconv.ParseFloat(lStr, 64)
	if err != nil {
		return 0, false
	}

	// Normalize
	h = h / 360.0
	for h < 0 {
		h += 1.0
	}
	for h > 1.0 {
		h -= 1.0
	}
	s := sVal / 100.0
	l := lVal / 100.0

	// HSL to RGB
	var r, g, b float64
	if s == 0 {
		r, g, b = l, l, l
	} else {
		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}
		p := 2*l - q
		r = hueToRGB(p, q, h+1.0/3.0)
		g = hueToRGB(p, q, h)
		b = hueToRGB(p, q, h-1.0/3.0)
	}

	ri := clamp(int(r*255+0.5), 0, 255)
	gi := clamp(int(g*255+0.5), 0, 255)
	bi := clamp(int(b*255+0.5), 0, 255)
	return int32((ri << 16) | (gi << 8) | bi), true
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1.0
	}
	if t > 1.0 {
		t -= 1.0
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6.0*t
	}
	if t < 0.5 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6.0
	}
	return p
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func newDefaultStyleContext() styleContext {
	return styleContext{
		fontID:            -1,
		textDecoration:    -1,
		textTransform:     -1,
		textAlign:         -1,
		marginLeft:        -1,
		marginTop:         -1,
		marginRight:       -1,
		marginBottom:      -1,
		paddingLeft:       -1,
		display:           -1,
		paddingTop:        -1,
		listStyleType:     -1,
		paddingRight:      -1,
		paddingBottom:     -1,
		whiteSpace:        -1,
		lineHeight:        -1,
		borderTopWidth:    -1,
		borderRightWidth:  -1,
		borderBottomWidth: -1,
		borderLeftWidth:   -1,
		borderTopStyle:    -1,
		borderRightStyle:  -1,
		borderBottomStyle: -1,
		borderLeftStyle:   -1,
		float:             -1,
		cssWidth:          -1,
		cssHeight:         -1,
	}
}

func inheritFromParent(ctx *styleContext, parent *styleContext) {
	// color inherits
	if !ctx.colorSet && parent.colorSet {
		ctx.color = parent.color
		ctx.colorSet = true
	}
	// font inherits
	if ctx.fontID < 0 && parent.fontID >= 0 {
		ctx.fontID = parent.fontID
	}
	// text-decoration does NOT inherit in CSS spec, but we need it for practical purposes
	// (e.g. <a> containing <span>). Actually per spec it doesn't inherit, skip it.

	// text-transform inherits
	if ctx.textTransform < 0 && parent.textTransform >= 0 {
		ctx.textTransform = parent.textTransform
	}
	// white-space inherits
	if ctx.whiteSpace < 0 && parent.whiteSpace >= 0 {
		ctx.whiteSpace = parent.whiteSpace
	}
	// line-height inherits
	if ctx.lineHeight < 0 && parent.lineHeight >= 0 {
		ctx.lineHeight = parent.lineHeight
	}
	// list-style-type inherits
	if ctx.listStyleType < 0 && parent.listStyleType >= 0 {
		ctx.listStyleType = parent.listStyleType
	}
	// text-align inherits
	if ctx.textAlign < 0 && parent.textAlign >= 0 {
		ctx.textAlign = parent.textAlign
	}
}

// parseBorderStyleValue parses a CSS border-style keyword into a KBC enum.
// Values from KBC.java: 0=none, 1=dotted, 2=dashed, 3=solid, 4=double,
// 5=groove, 6=ridge, 7=inset, 8=outset, 9=blank
func parseBorderStyleValue(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none":
		return borderStyleNone
	case "dotted":
		return borderStyleDotted
	case "dashed":
		return borderStyleDashed
	case "solid":
		return borderStyleSolid
	case "double":
		return borderStyleDouble
	case "groove":
		return borderStyleGroove
	case "ridge":
		return borderStyleRidge
	case "inset":
		return borderStyleInset
	case "outset":
		return borderStyleOutset
	case "hidden":
		return borderStyleNone
	}
	return -1
}

func parsePx(s string) int {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "0" {
		return 0
	}
	// Try px
	if numStr, ok := strings.CutSuffix(s, "px"); ok {
		if v, err := strconv.Atoi(strings.TrimSpace(numStr)); err == nil {
			return v
		}
	}
	// Try em (approximate: 1em ≈ 12px)
	if numStr, ok := strings.CutSuffix(s, "em"); ok {
		if v, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64); err == nil {
			return int(v * emToPx)
		}
	}
	// Try pt (1pt ≈ 1.33px)
	if numStr, ok := strings.CutSuffix(s, "pt"); ok {
		if v, err := strconv.Atoi(strings.TrimSpace(numStr)); err == nil {
			return v * 4 / 3
		}
	}
	// Try bare number
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return 0
}

func fontSizeToFontID(value string, currentFontID int) int {
	v := strings.ToLower(strings.TrimSpace(value))

	// Determine target size bucket
	type sizeBucket int
	const (
		sizeSmall sizeBucket = iota
		sizeMedium
		sizeLarge
		sizeUnknown
	)

	bucket := sizeUnknown

	switch v {
	case "xx-small", "x-small", "small", "smaller":
		bucket = sizeSmall
	case "medium":
		bucket = sizeMedium
	case "large", "larger", "x-large", "xx-large":
		bucket = sizeLarge
	default:
		// Try to parse px value
		if strings.HasSuffix(v, "px") {
			numStr := strings.TrimSuffix(v, "px")
			if px, err := strconv.Atoi(numStr); err == nil {
				switch {
				case px <= 8:
					bucket = sizeSmall
				case px <= 10:
					bucket = sizeMedium
				default:
					bucket = sizeLarge
				}
			}
		} else if strings.HasSuffix(v, "pt") {
			numStr := strings.TrimSuffix(v, "pt")
			if pt, err := strconv.Atoi(numStr); err == nil {
				// Approximate: 1pt ≈ 1.33px
				px := pt * 4 / 3
				switch {
				case px <= 8:
					bucket = sizeSmall
				case px <= 10:
					bucket = sizeMedium
				default:
					bucket = sizeLarge
				}
			}
		}
		// Relative units (em, rem, %); keep current font
	}

	if bucket == sizeUnknown {
		return currentFontID
	}

	// Determine current bold/italic state
	isBold := currentFontID == fontBortBold9 || currentFontID == fontBortBold12 || currentFontID == fontBortBoldItalic9
	isItalic := currentFontID == fontBortItalic9 || currentFontID == fontBortBoldItalic9

	switch bucket {
	case sizeSmall:
		return fontBort7 // no bold/italic variant for size 7
	case sizeMedium:
		if isBold && isItalic {
			return fontBortBoldItalic9
		} else if isBold {
			return fontBortBold9
		} else if isItalic {
			return fontBortItalic9
		}
		return fontBort9
	case sizeLarge:
		if isBold {
			return fontBortBold12
		}
		return fontBort12 // no italic variant for size 12
	}
	return currentFontID
}

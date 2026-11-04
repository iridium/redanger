package kimba

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// lengthContext carries what's needed to resolve relative lengths.
type lengthContext struct {
	fontSize     float64 // element's font size (parent's when resolving font-size)
	rootFontSize float64
	percentBase  float64 // what 100% means
	hasPercent   bool    // false: percentages are invalid
}

// parseLength resolves a CSS length (or calc()/min()/max()/clamp()) to px.
func parseLength(s string, ctx lengthContext) (float64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, false
	}
	if fn, args, ok := cssFunction(s); ok {
		switch fn {
		case "calc", "-webkit-calc", "-moz-calc":
			return evalCalc(args, ctx)
		case "min", "max", "clamp":
			var vals []float64
			for _, a := range splitTopLevel(args, ',') {
				v, ok := parseLength(a, ctx)
				if !ok {
					return 0, false
				}
				vals = append(vals, v)
			}
			if len(vals) == 0 {
				return 0, false
			}
			switch fn {
			case "min":
				return minFloat(vals), true
			case "max":
				return maxFloat(vals), true
			default:
				if len(vals) != 3 {
					return 0, false
				}
				return math.Max(vals[0], math.Min(vals[1], vals[2])), true
			}
		}
		return 0, false
	}
	num, unit := splitNumberUnit(s)
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, false
	}
	switch unit {
	case "px":
		return v, true
	case "":
		return v, v == 0 // unitless is only valid for zero
	case "em":
		return v * ctx.fontSize, true
	case "rem":
		return v * ctx.rootFontSize, true
	case "ex", "ch":
		return v * ctx.fontSize / 2, true
	case "%":
		if !ctx.hasPercent {
			return 0, false
		}
		return v * ctx.percentBase / 100, true
	case "pt":
		return v * 4 / 3, true
	case "pc":
		return v * 16, true
	case "in":
		return v * 96, true
	case "cm":
		return v * 96 / 2.54, true
	case "mm":
		return v * 96 / 25.4, true
	case "q":
		return v * 96 / 101.6, true
	case "vw", "svw", "lvw", "dvw":
		return v * screenWidth / 100, true
	case "vh", "svh", "lvh", "dvh":
		return v * screenHeight / 100, true
	case "vmin":
		return v * math.Min(screenWidth, screenHeight) / 100, true
	case "vmax":
		return v * math.Max(screenWidth, screenHeight) / 100, true
	}
	return 0, false
}

func splitNumberUnit(s string) (num, unit string) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	// Exponent, e.g. 1e3 (but not the "e" of "em"/"ex").
	if i < len(s) && s[i] == 'e' && i+1 < len(s) && (s[i+1] >= '0' && s[i+1] <= '9' || s[i+1] == '-' || s[i+1] == '+') {
		j := i + 1
		if s[j] == '-' || s[j] == '+' {
			j++
		}
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		i = j
	}
	return s[:i], s[i:]
}

// cssFunction splits "name(args)" into its parts.
func cssFunction(s string) (name, args string, ok bool) {
	open := strings.IndexByte(s, '(')
	if open <= 0 || !strings.HasSuffix(s, ")") {
		return "", "", false
	}
	for i := 0; i < open; i++ {
		if !isIdentChar(s[i]) {
			return "", "", false
		}
	}
	return s[:open], s[open+1 : len(s)-1], true
}

// calcValue is a calc() operand: a length in px, or a plain number.
type calcValue struct {
	v        float64
	isLength bool
}

func evalCalc(expr string, ctx lengthContext) (float64, bool) {
	p := calcParser{s: expr, ctx: ctx}
	v, ok := p.sum()
	if !ok || strings.TrimSpace(p.s[p.i:]) != "" {
		return 0, false
	}
	if !v.isLength && v.v != 0 {
		return 0, false
	}
	return v.v, true
}

type calcParser struct {
	s   string
	i   int
	ctx lengthContext
}

func (p *calcParser) sum() (calcValue, bool) {
	left, ok := p.product()
	if !ok {
		return left, false
	}
	for {
		p.i = skipSpace(p.s, p.i)
		if p.i >= len(p.s) || (p.s[p.i] != '+' && p.s[p.i] != '-') {
			return left, true
		}
		op := p.s[p.i]
		p.i++
		right, ok := p.product()
		if !ok {
			return left, false
		}
		if left.v != 0 && right.v != 0 && left.isLength != right.isLength {
			return left, false
		}
		if op == '+' {
			left.v += right.v
		} else {
			left.v -= right.v
		}
		left.isLength = left.isLength || right.isLength
	}
}

func (p *calcParser) product() (calcValue, bool) {
	left, ok := p.operand()
	if !ok {
		return left, false
	}
	for {
		p.i = skipSpace(p.s, p.i)
		if p.i >= len(p.s) || (p.s[p.i] != '*' && p.s[p.i] != '/') {
			return left, true
		}
		op := p.s[p.i]
		p.i++
		right, ok := p.operand()
		if !ok {
			return left, false
		}
		switch {
		case op == '*' && !(left.isLength && right.isLength):
			left = calcValue{left.v * right.v, left.isLength || right.isLength}
		case op == '/' && !right.isLength && right.v != 0:
			left.v /= right.v
		default:
			return left, false
		}
	}
}

func (p *calcParser) operand() (calcValue, bool) {
	p.i = skipSpace(p.s, p.i)
	if p.i >= len(p.s) {
		return calcValue{}, false
	}
	if p.s[p.i] == '(' {
		p.i++
		v, ok := p.sum()
		p.i = skipSpace(p.s, p.i)
		if !ok || p.i >= len(p.s) || p.s[p.i] != ')' {
			return v, false
		}
		p.i++
		return v, true
	}
	// A token runs to the next space, operator (other than a leading sign
	// or an exponent sign) or closing paren; nested functions stay whole.
	start := p.i
	j := start
	if j < len(p.s) && (p.s[j] == '-' || p.s[j] == '+') {
		j++
	}
	depth := 0
	for j < len(p.s) {
		c := p.s[j]
		if c == '(' {
			depth++
		} else if c == ')' {
			if depth == 0 {
				break
			}
			depth--
		} else if depth == 0 && (c == ' ' || c == '*' || c == '/' || c == '\t' || c == '\n') {
			break
		}
		j++
	}
	tok := p.s[start:j]
	p.i = j
	if v, err := strconv.ParseFloat(tok, 64); err == nil {
		return calcValue{v: v}, true
	}
	v, ok := parseLength(tok, p.ctx)
	return calcValue{v: v, isLength: true}, ok
}

func minFloat(v []float64) float64 {
	m := v[0]
	for _, x := range v[1:] {
		m = math.Min(m, x)
	}
	return m
}

func maxFloat(v []float64) float64 {
	m := v[0]
	for _, x := range v[1:] {
		m = math.Max(m, x)
	}
	return m
}

func pxString(v float64) string {
	return fmt.Sprintf("%dpx", int(math.Round(v)))
}

func rgbString(c int32) string {
	return fmt.Sprintf("rgb(%d, %d, %d)", (c>>16)&0xFF, (c>>8)&0xFF, c&0xFF)
}

// cssColorValue resolves a color to the "rgb(r, g, b)" form the box builder
// expects, or transparentRGBA for fully transparent colors. currentcolor
// resolves to current when haveCurrent is set.
func cssColorValue(s string, current int32, haveCurrent bool) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "transparent":
		return transparentRGBA, true
	case "currentcolor":
		if haveCurrent {
			return rgbString(current), true
		}
		return "", false
	case "inherit", "initial", "unset":
		return "", false
	}
	if fn, args, ok := cssFunction(v); ok && fn == "light-dark" {
		light, _, _ := strings.Cut(args, ",")
		return cssColorValue(light, current, haveCurrent)
	}
	if colorAlphaIsZero(v) {
		return transparentRGBA, true
	}
	c, ok := parseColor(normalizeColorArgs(v))
	if !ok {
		return "", false
	}
	return rgbString(c), true
}

// normalizeColorArgs rewrites modern rgb()/hsl() syntax ("rgb(0 0 0 / 50%)",
// percentages, fractional channels) into the integer comma form parseColor
// understands.
func normalizeColorArgs(v string) string {
	fn, args, ok := cssFunction(v)
	if !ok {
		return v
	}
	args = strings.ReplaceAll(args, "/", " ")
	args = strings.ReplaceAll(args, ",", " ")
	parts := strings.Fields(args)
	if len(parts) < 3 {
		return v
	}
	switch fn {
	case "rgb", "rgba":
		ch := make([]string, 3)
		for i := range 3 {
			p := parts[i]
			var f float64
			var err error
			if n, ok := strings.CutSuffix(p, "%"); ok {
				f, err = strconv.ParseFloat(n, 64)
				f = f * 255 / 100
			} else {
				f, err = strconv.ParseFloat(p, 64)
			}
			if err != nil {
				return v
			}
			ch[i] = strconv.Itoa(int(math.Round(f)))
		}
		return "rgb(" + strings.Join(ch, ", ") + ")"
	case "hsl", "hsla":
		h := strings.TrimSuffix(parts[0], "deg")
		return "hsl(" + h + ", " + parts[1] + ", " + parts[2] + ")"
	}
	return v
}

func colorAlphaIsZero(v string) bool {
	if strings.HasPrefix(v, "#") {
		h := v[1:]
		switch len(h) {
		case 4:
			return h[3] == '0'
		case 8:
			return h[6:] == "00"
		}
		return false
	}
	fn, args, ok := cssFunction(v)
	if !ok || (fn != "rgb" && fn != "rgba" && fn != "hsl" && fn != "hsla") {
		return false
	}
	args = strings.ReplaceAll(args, "/", " ")
	args = strings.ReplaceAll(args, ",", " ")
	parts := strings.Fields(args)
	if len(parts) != 4 {
		return false
	}
	a := parts[3]
	if n, ok := strings.CutSuffix(a, "%"); ok {
		a = n
	}
	f, err := strconv.ParseFloat(a, 64)
	return err == nil && f == 0
}

func hasVar(s string) bool {
	return strings.Contains(strings.ToLower(s), "var(")
}

const maxVarDepth = 16

// substituteVars replaces var(--name[, fallback]) references. It fails when a
// reference has no value and no fallback (the declaration is then invalid).
func substituteVars(s string, vars map[string]string, depth int) (string, bool) {
	if depth > maxVarDepth {
		return "", false
	}
	var b strings.Builder
	for {
		k := indexFold(s, "var(")
		if k < 0 {
			b.WriteString(s)
			return b.String(), true
		}
		b.WriteString(s[:k])
		open := k + 3
		end := scanUntil(s, open+1, ")")
		if end >= len(s) {
			return "", false
		}
		inner := s[open+1 : end]
		name, fallback, hasFallback := strings.Cut(inner, ",")
		name = strings.TrimSpace(name)
		val, ok := vars[name]
		if ok {
			val, ok = substituteVars(val, vars, depth+1)
		}
		if !ok {
			if !hasFallback {
				return "", false
			}
			val, ok = substituteVars(strings.TrimSpace(fallback), vars, depth+1)
			if !ok {
				return "", false
			}
		}
		b.WriteString(val)
		s = s[end+1:]
	}
}

func indexFold(s, sub string) int {
	n := len(sub)
	for i := 0; i+n <= len(s); i++ {
		if strings.EqualFold(s[i:i+n], sub) {
			return i
		}
	}
	return -1
}

// unescapeCSSString decodes the backslash escapes in a CSS string's body:
// \XXXXXX hex code points (with an optional trailing space) and escaped
// characters.
func unescapeCSSString(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		j := i + 1
		for j < len(s) && j < i+7 && isHexDigit(s[j]) {
			j++
		}
		if j == i+1 {
			if s[j] != '\n' { // an escaped newline is a line continuation
				b.WriteByte(s[j])
			}
			i = j
			continue
		}
		cp, _ := strconv.ParseUint(s[i+1:j], 16, 32)
		if cp == 0 || cp > 0x10FFFF {
			cp = 0xFFFD
		}
		b.WriteRune(rune(cp))
		if j < len(s) && s[j] == ' ' {
			j++
		}
		i = j - 1
	}
	return b.String()
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

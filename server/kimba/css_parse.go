package kimba

import (
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/cascadia"
)

// cssDecl is one longhand declaration. Shorthands are expanded at parse time,
// except when their value uses var(): then every longhand keeps the raw
// shorthand and is expanded after substitution (see resolveDecl).
type cssDecl struct {
	prop      string // internal (camelCase) longhand name, or "--custom"
	value     string
	important bool
	shorthand string // CSS shorthand name when value is a deferred var() shorthand
}

type cssRule struct {
	sel    cascadia.Sel
	spec   cascadia.Specificity
	bucket ruleBucket
	order  int
	decls  []cssDecl
}

type stylesheet struct {
	rules []cssRule

	// Rules for ::before and ::after, matched against the originating element.
	pseudo map[string]*stylesheet

	// Rule indexes by bucket, built on first use (sheets are cached and
	// shared between requests, so this must be safe for concurrent use).
	indexOnce sync.Once
	anyRules  []int
	byTag     map[string][]int
	byID      map[string][]int
	byClass   map[string][]int
}

// cssImporter fetches the text of an @import-ed stylesheet ("" on failure).
type cssImporter func(url string) string

const maxImportDepth = 3

// longhandNames maps CSS property names to the internal names the box builder
// reads. Anything else (except custom properties) is dropped at parse time.
var longhandNames = map[string]string{
	"color":                "color",
	"background-color":     "backgroundColor",
	"font-size":            "fontSize",
	"font-weight":          "fontWeight",
	"font-style":           "fontStyle",
	"display":              "display",
	"visibility":           "visibility",
	"text-decoration-line": "textDecoration",
	"text-transform":       "textTransform",
	"white-space":          "whiteSpace",
	"text-align":           "textAlign",
	"margin-top":           "marginTop",
	"margin-right":         "marginRight",
	"margin-bottom":        "marginBottom",
	"margin-left":          "marginLeft",
	"padding-top":          "paddingTop",
	"padding-right":        "paddingRight",
	"padding-bottom":       "paddingBottom",
	"padding-left":         "paddingLeft",
	"border-top-width":     "borderTopWidth",
	"border-right-width":   "borderRightWidth",
	"border-bottom-width":  "borderBottomWidth",
	"border-left-width":    "borderLeftWidth",
	"border-top-style":     "borderTopStyle",
	"border-right-style":   "borderRightStyle",
	"border-bottom-style":  "borderBottomStyle",
	"border-left-style":    "borderLeftStyle",
	"border-top-color":     "borderTopColor",
	"border-right-color":   "borderRightColor",
	"border-bottom-color":  "borderBottomColor",
	"border-left-color":    "borderLeftColor",
	"line-height":          "lineHeight",
	"float":                "float",
	"list-style-type":      "listStyleType",
	"width":                "width",
	"height":               "height",
	"content":              "content", // only used for ::before/::after
}

var sides = [4]string{"Top", "Right", "Bottom", "Left"}

func sideProps(prefix, suffix string) []string {
	out := make([]string, 4)
	for i, s := range sides {
		out[i] = prefix + s + suffix
	}
	return out
}

// shorthandLonghands lists the internal longhands each supported shorthand sets.
var shorthandLonghands = map[string][]string{
	"margin":          sideProps("margin", ""),
	"padding":         sideProps("padding", ""),
	"border-width":    sideProps("border", "Width"),
	"border-style":    sideProps("border", "Style"),
	"border-color":    sideProps("border", "Color"),
	"border":          append(append(sideProps("border", "Width"), sideProps("border", "Style")...), sideProps("border", "Color")...),
	"border-top":      {"borderTopWidth", "borderTopStyle", "borderTopColor"},
	"border-right":    {"borderRightWidth", "borderRightStyle", "borderRightColor"},
	"border-bottom":   {"borderBottomWidth", "borderBottomStyle", "borderBottomColor"},
	"border-left":     {"borderLeftWidth", "borderLeftStyle", "borderLeftColor"},
	"background":      {"backgroundColor"},
	"font":            {"fontStyle", "fontWeight", "fontSize", "lineHeight"},
	"text-decoration": {"textDecoration"},
	"list-style":      {"listStyleType"},
}

// parseStylesheet parses CSS source. Rule order numbers start at 0 per sheet;
// the cascade breaks ties by (sheet index, order).
func parseStylesheet(src string, importer cssImporter) *stylesheet {
	ss := &stylesheet{}
	p := &cssParser{importer: importer}
	p.parseRules(stripCSSComments(src), ss, 0)
	return ss
}

type cssParser struct {
	importer cssImporter
	order    int
}

func (p *cssParser) parseRules(s string, ss *stylesheet, depth int) {
	i := 0
	for i < len(s) {
		i = skipSpace(s, i)
		if i >= len(s) {
			return
		}
		// Stray HTML comment markers are allowed at the top level of a sheet.
		if strings.HasPrefix(s[i:], "<!--") {
			i += 4
			continue
		}
		if strings.HasPrefix(s[i:], "-->") {
			i += 3
			continue
		}
		if s[i] == '@' {
			i = p.parseAtRule(s, i, ss, depth)
			continue
		}
		// Qualified rule: prelude up to '{', then a balanced block.
		open := scanUntil(s, i, "{;")
		if open >= len(s) {
			return
		}
		if s[open] == ';' { // garbage; skip it
			i = open + 1
			continue
		}
		end := matchBrace(s, open)
		prelude := strings.TrimSpace(s[i:open])
		block := s[open+1 : min(end, len(s))]
		p.addRule(ss, prelude, block)
		i = end + 1
	}
}

func (p *cssParser) parseAtRule(s string, i int, ss *stylesheet, depth int) int {
	j := i + 1
	for j < len(s) && (isIdentChar(s[j])) {
		j++
	}
	name := strings.ToLower(s[i+1 : j])
	stop := scanUntil(s, j, "{;")
	prelude := strings.TrimSpace(s[j:min(stop, len(s))])
	if stop >= len(s) {
		return len(s)
	}
	if s[stop] == ';' {
		if name == "import" && p.importer != nil && depth < maxImportDepth {
			url, media := parseImportPrelude(prelude)
			if url != "" && mediaMatches(media) {
				if src := p.importer(url); src != "" {
					p.parseRules(stripCSSComments(src), ss, depth+1)
				}
			}
		}
		return stop + 1
	}
	end := matchBrace(s, stop)
	block := s[stop+1 : min(end, len(s))]
	switch name {
	case "media":
		if mediaMatches(prelude) {
			p.parseRules(block, ss, depth)
		}
	case "supports", "layer", "container", "document", "-moz-document", "scope":
		// We can't evaluate these properly; most real-world uses guard
		// progressive enhancements, so including the contents is the
		// better guess. (@layer ordering is ignored.)
		p.parseRules(block, ss, depth)
	}
	// Everything else (@font-face, @keyframes, @page, ...) is skipped.
	return end + 1
}

func parseImportPrelude(prelude string) (url, media string) {
	s := strings.TrimSpace(prelude)
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "url("):
		end := strings.IndexByte(s, ')')
		if end < 0 {
			return "", ""
		}
		url = unquoteCSS(strings.TrimSpace(s[4:end]))
		s = s[end+1:]
	case len(s) > 0 && (s[0] == '"' || s[0] == '\''):
		end := strings.IndexByte(s[1:], s[0])
		if end < 0 {
			return "", ""
		}
		url = s[1 : end+1]
		s = s[end+2:]
	default:
		return "", ""
	}
	// Drop layer / layer(...) / supports(...) conditions; the rest is a media query.
	var mq []string
	for _, t := range splitSpaces(s) {
		lt := strings.ToLower(t)
		if lt == "layer" || strings.HasPrefix(lt, "layer(") || strings.HasPrefix(lt, "supports(") {
			continue
		}
		mq = append(mq, t)
	}
	return url, strings.Join(mq, " ")
}

func (p *cssParser) addRule(ss *stylesheet, prelude, block string) {
	decls := parseDeclarations(block)
	if len(decls) == 0 {
		return
	}
	for _, selText := range splitTopLevel(prelude, ',') {
		selText = strings.TrimSpace(selText)
		if selText == "" {
			continue
		}
		sel, err := cascadia.ParseWithPseudoElement(selText)
		if err != nil {
			// Unsupported selectors simply never match, like a browser
			// dropping a rule it can't parse.
			continue
		}
		target := ss
		switch pe := sel.PseudoElement(); pe {
		case "":
		case "before", "after":
			if ss.pseudo == nil {
				ss.pseudo = map[string]*stylesheet{}
			}
			if ss.pseudo[pe] == nil {
				ss.pseudo[pe] = &stylesheet{}
			}
			target = ss.pseudo[pe]
		default:
			continue // ::first-line, ::marker, ... aren't supported
		}
		target.rules = append(target.rules, cssRule{
			sel:    sel,
			spec:   sel.Specificity(),
			bucket: bucketForSelector(selText),
			order:  p.order,
			decls:  decls,
		})
		p.order++
	}
}

// parseDeclarations parses the inside of a declaration block (or a style
// attribute) into longhand declarations.
func parseDeclarations(block string) []cssDecl {
	var out []cssDecl
	for _, part := range splitTopLevel(block, ';') {
		colon := strings.IndexByte(part, ':')
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(part[:colon])
		value := strings.TrimSpace(part[colon+1:])
		important := false
		if k := strings.LastIndexByte(value, '!'); k >= 0 &&
			strings.EqualFold(strings.TrimSpace(value[k+1:]), "important") {
			important = true
			value = strings.TrimSpace(value[:k])
		}
		if value == "" {
			continue
		}
		if strings.HasPrefix(name, "--") {
			out = append(out, cssDecl{prop: name, value: value, important: important})
			continue
		}
		name = strings.ToLower(name)
		out = appendDecl(out, name, value, important)
	}
	return out
}

func appendDecl(out []cssDecl, name, value string, important bool) []cssDecl {
	if prop, ok := longhandNames[name]; ok {
		return append(out, cssDecl{prop: prop, value: value, important: important})
	}
	longhands, ok := shorthandLonghands[name]
	if !ok {
		return out
	}
	if hasVar(value) || isCSSWideKeyword(value) {
		for _, lh := range longhands {
			d := cssDecl{prop: lh, value: value, important: important}
			if hasVar(value) {
				d.shorthand = name
			}
			out = append(out, d)
		}
		return out
	}
	expanded, ok := expandShorthand(name, value)
	if !ok {
		return out
	}
	for _, lh := range longhands {
		if v, ok := expanded[lh]; ok {
			out = append(out, cssDecl{prop: lh, value: v, important: important})
		}
	}
	return out
}

func isCSSWideKeyword(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "inherit", "initial", "unset", "revert", "revert-layer":
		return true
	}
	return false
}

// expandShorthand splits a shorthand value into internal longhands. Longhands
// the shorthand resets but the value doesn't mention get their initial value.
func expandShorthand(name, value string) (map[string]string, bool) {
	toks := splitSpaces(value)
	if len(toks) == 0 {
		return nil, false
	}
	out := map[string]string{}
	switch name {
	case "margin", "padding", "border-width", "border-style", "border-color":
		vals, ok := boxValues(toks)
		if !ok {
			return nil, false
		}
		props := shorthandLonghands[name]
		for i := range props {
			out[props[i]] = vals[i]
		}
	case "border", "border-top", "border-right", "border-bottom", "border-left":
		w, st, c := "medium", "none", "currentcolor"
		for _, t := range toks {
			switch {
			case isBorderStyle(t):
				st = t
			case isBorderWidth(t):
				w = t
			default:
				if _, ok := cssColorValue(t, 0, false); ok || strings.EqualFold(t, "currentcolor") {
					c = t
				} else {
					return nil, false
				}
			}
		}
		for _, lh := range shorthandLonghands[name] {
			switch {
			case strings.HasSuffix(lh, "Width"):
				out[lh] = w
			case strings.HasSuffix(lh, "Style"):
				out[lh] = st
			default:
				out[lh] = c
			}
		}
	case "background":
		// The color can only appear in the final layer.
		layers := splitTopLevel(value, ',')
		out["backgroundColor"] = "transparent"
		for _, t := range splitSpaces(layers[len(layers)-1]) {
			if _, ok := cssColorValue(t, 0, false); ok || strings.EqualFold(t, "currentcolor") {
				out["backgroundColor"] = t
			}
		}
	case "font":
		if len(toks) == 1 { // system font keyword (caption, menu, ...)
			return nil, false
		}
		out["fontStyle"], out["fontWeight"], out["lineHeight"] = "normal", "normal", "normal"
		sized := false
		for _, t := range toks {
			lt := strings.ToLower(t)
			if sized {
				break // the rest is the family list
			}
			switch {
			case lt == "italic" || lt == "oblique":
				out["fontStyle"] = lt
			case lt == "bold" || lt == "bolder" || lt == "lighter" || isNumber(lt):
				out["fontWeight"] = lt
			case lt == "normal" || lt == "small-caps":
			default:
				size, lh, _ := strings.Cut(t, "/")
				out["fontSize"] = size
				if lh != "" {
					out["lineHeight"] = lh
				}
				sized = true
			}
		}
		if !sized {
			return nil, false
		}
	case "text-decoration":
		out["textDecoration"] = value
	case "list-style":
		out["listStyleType"] = "disc"
		for _, t := range toks {
			lt := strings.ToLower(t)
			if lt == "inside" || lt == "outside" || strings.HasPrefix(lt, "url(") {
				continue
			}
			out["listStyleType"] = lt
		}
	default:
		return nil, false
	}
	return out, true
}

// boxValues expands 1-4 values into top, right, bottom, left.
func boxValues(t []string) ([4]string, bool) {
	switch len(t) {
	case 1:
		return [4]string{t[0], t[0], t[0], t[0]}, true
	case 2:
		return [4]string{t[0], t[1], t[0], t[1]}, true
	case 3:
		return [4]string{t[0], t[1], t[2], t[1]}, true
	case 4:
		return [4]string{t[0], t[1], t[2], t[3]}, true
	}
	return [4]string{}, false
}

func isBorderStyle(s string) bool {
	return parseBorderStyleValue(s) >= 0
}

func isBorderWidth(s string) bool {
	switch strings.ToLower(s) {
	case "thin", "medium", "thick":
		return true
	}
	_, ok := parseLength(s, lengthContext{fontSize: 16, rootFontSize: 16})
	return ok
}

func isNumber(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// mediaWidth is the screen width media queries see. Layout is 240px, but
// mobile stylesheets assume at least 320 (e.g. HN's min-width: 300px), and
// the desktop styles that apply otherwise are far worse on this screen.
const mediaWidth = 320

// mediaMatches evaluates a media query list for the Sidekick screen.
func mediaMatches(q string) bool {
	q = strings.Join(strings.Fields(strings.ToLower(q)), " ")
	if q == "" {
		return true
	}
	for _, one := range splitTopLevel(q, ',') {
		if mediaQueryMatches(strings.TrimSpace(one)) {
			return true
		}
	}
	return false
}

func mediaQueryMatches(q string) bool {
	negate := false
	if rest, ok := strings.CutPrefix(q, "not "); ok {
		negate, q = true, rest
	}
	q = strings.TrimPrefix(q, "only ")
	result := true
	for _, part := range strings.Split(q, " and ") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !mediaTermMatches(part) {
			result = false
			break
		}
	}
	return result != negate
}

func mediaTermMatches(t string) bool {
	switch t {
	case "all", "screen", "handheld":
		return true
	case "print", "speech", "tv", "projection", "aural", "braille", "embossed", "tty":
		return false
	}
	t = strings.TrimSuffix(strings.TrimPrefix(t, "("), ")")
	name, val, hasVal := strings.Cut(t, ":")
	name, val = strings.TrimSpace(name), strings.TrimSpace(val)
	px := func() (float64, bool) {
		return parseLength(val, lengthContext{fontSize: 16, rootFontSize: 16})
	}
	switch name {
	case "min-width", "min-device-width":
		v, ok := px()
		return ok && mediaWidth >= v
	case "max-width", "max-device-width":
		v, ok := px()
		return ok && mediaWidth <= v
	case "min-height", "min-device-height":
		v, ok := px()
		return ok && float64(screenHeight) >= v
	case "max-height", "max-device-height":
		v, ok := px()
		return ok && float64(screenHeight) <= v
	case "orientation":
		return val == "landscape"
	case "prefers-color-scheme":
		return val == "light"
	case "color", "hover", "pointer", "any-hover", "any-pointer", "prefers-reduced-motion":
		if !hasVal {
			return name == "color"
		}
		return val == "none" || val == "no-preference" || val == "reduce"
	}
	return false
}

// ruleBucket is a cheap pre-filter: the id, class or tag that the rightmost
// compound selector requires. Rules whose bucket can't match an element are
// skipped without running the full selector.
type ruleBucket struct {
	kind byte // 0 = any, '#' = id, '.' = class, 't' = tag
	name string
}

func bucketForSelector(sel string) ruleBucket {
	// Find the start of the rightmost compound selector.
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(sel); i++ {
		c := sel[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			depth--
		case depth == 0 && (c == ' ' || c == '>' || c == '+' || c == '~' || c == '\t' || c == '\n'):
			start = i + 1
		}
	}
	compound := sel[start:]
	if strings.ContainsAny(compound, "\\") {
		return ruleBucket{}
	}
	var tag string
	var best ruleBucket
	depth = 0
	for i := 0; i < len(compound); {
		c := compound[i]
		switch {
		case c == '(' || c == '[':
			depth++
			i++
		case c == ')' || c == ']':
			depth--
			i++
		case depth == 0 && (c == '#' || c == '.'):
			j := i + 1
			for j < len(compound) && isIdentChar(compound[j]) {
				j++
			}
			if j > i+1 && (best.kind != '#') {
				best = ruleBucket{kind: c, name: compound[i+1 : j]}
			}
			i = j
		case depth == 0 && i == 0 && isIdentChar(c):
			j := i
			for j < len(compound) && isIdentChar(compound[j]) {
				j++
			}
			tag = strings.ToLower(compound[:j])
			i = j
		default:
			i++
		}
	}
	if best.kind != 0 {
		return best
	}
	if tag != "" {
		return ruleBucket{kind: 't', name: tag}
	}
	return ruleBucket{}
}

func isIdentChar(c byte) bool {
	return c == '-' || c == '_' || c >= 0x80 ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func stripCSSComments(s string) string {
	if !strings.Contains(s, "/*") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == '\\' && i+1 < len(s) {
				i++
				b.WriteByte(s[i])
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			b.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				break
			}
			i += end + 3
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func skipSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '\f') {
		i++
	}
	return i
}

// scanUntil returns the index of the first byte in stops outside strings,
// parentheses and brackets, or len(s).
func scanUntil(s string, i int, stops string) int {
	depth := 0
	var quote byte
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case depth == 0 && strings.IndexByte(stops, c) >= 0:
			return i
		case c == '"' || c == '\'':
			quote = c
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			if depth > 0 {
				depth--
			}
		}
	}
	return len(s)
}

// matchBrace returns the index of the '}' matching the '{' at open, or len(s).
func matchBrace(s string, open int) int {
	depth := 0
	var quote byte
	for i := open; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(s)
}

// splitTopLevel splits on sep outside strings, parentheses and brackets.
func splitTopLevel(s string, sep byte) []string {
	var out []string
	for {
		i := scanUntil(s, 0, string(sep))
		out = append(out, s[:i])
		if i >= len(s) {
			return out
		}
		s = s[i+1:]
	}
}

// splitSpaces splits a value into whitespace-separated tokens, keeping
// functions like rgb(1, 2, 3) together.
func splitSpaces(s string) []string {
	var out []string
	for {
		s = strings.TrimSpace(s)
		if s == "" {
			return out
		}
		i := scanUntil(s, 0, " \t\n\r\f")
		out = append(out, s[:i])
		if i >= len(s) {
			return out
		}
		s = s[i:]
	}
}

func unquoteCSS(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

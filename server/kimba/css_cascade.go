package kimba

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

// uaCSS is the built-in default stylesheet: a trimmed version of the HTML
// spec's rendering section, with scripting disabled (so <noscript> renders).
const uaCSS = `
html, address, blockquote, body, center, dialog, div, figure, figcaption,
footer, form, header, hr, legend, listing, main, p, plaintext, pre, search,
xmp, article, aside, h1, h2, h3, h4, h5, h6, hgroup, nav, section, dd, dl,
dt, fieldset, details, summary, menu, ol, ul, dir, optgroup, frameset, frame { display: block }
area, base, basefont, datalist, head, link, meta, noembed, noframes, param,
rp, script, style, template, title, [hidden], dialog:not([open]),
input[type=hidden] { display: none }
li { display: list-item }
table { display: table }
caption { display: table-caption; text-align: center }
thead { display: table-header-group }
tbody { display: table-row-group }
tfoot { display: table-footer-group }
tr { display: table-row }
td, th { display: table-cell; padding: 1px }
th { font-weight: bold; text-align: center }
body { margin: 2px }
p, blockquote, figure, dl, ol, ul, menu, dir, pre, xmp, listing, plaintext { margin-top: 1em; margin-bottom: 1em }
blockquote, figure { margin-left: 40px; margin-right: 40px }
dd { margin-left: 40px }
ol, ul, menu, dir { padding-left: 40px }
ol ol, ol ul, ul ol, ul ul, menu ul, ul menu, dir ul { margin-top: 0; margin-bottom: 0 }
ol { list-style-type: decimal }
ul ul, ol ul, menu ul { list-style-type: circle }
ul ul ul, ol ul ul, ul ol ul, ol ol ul { list-style-type: square }
h1 { font-size: 2em; margin-top: 0.67em; margin-bottom: 0.67em }
h2 { font-size: 1.5em; margin-top: 0.83em; margin-bottom: 0.83em }
h3 { font-size: 1.17em; margin-top: 1em; margin-bottom: 1em }
h4 { margin-top: 1.33em; margin-bottom: 1.33em }
h5 { font-size: 0.83em; margin-top: 1.67em; margin-bottom: 1.67em }
h6 { font-size: 0.67em; margin-top: 2.33em; margin-bottom: 2.33em }
h1, h2, h3, h4, h5, h6, b, strong, dt { font-weight: bold }
i, cite, em, var, dfn, address { font-style: italic }
u, ins { text-decoration: underline }
s, strike, del { text-decoration: line-through }
a:link { color: #0000ee; text-decoration: underline }
pre, xmp, listing, plaintext { white-space: pre }
textarea { white-space: pre-wrap }
nobr { white-space: nowrap }
td[nowrap], th[nowrap] { white-space: nowrap }
center { text-align: -webkit-center }
small, sub, sup { font-size: smaller }
big { font-size: larger }
hr { border-style: inset; border-width: 1px; color: gray; margin-top: 0.5em; margin-bottom: 0.5em }
fieldset { margin-left: 2px; margin-right: 2px; border: 2px groove; padding: 0.35em 0.75em 0.625em }
mark { background-color: yellow; color: black }
`

// quirksCSS applies to documents without a standards doctype (like HN):
// tables don't inherit text and font properties.
const quirksCSS = `
table { text-align: start; white-space: normal; line-height: normal;
  font-weight: normal; font-size: medium; font-style: normal }
`

var (
	uaSheetOnce sync.Once
	uaSheet     *stylesheet
	quirksSheet *stylesheet
)

func uaStylesheet() *stylesheet {
	uaSheetOnce.Do(func() {
		uaSheet = parseStylesheet(uaCSS, nil)
		quirksSheet = parseStylesheet(quirksCSS, nil)
	})
	return uaSheet
}

// Cascade levels, lowest first. Presentational hints (bgcolor=, <font>...)
// rank below every author rule, and style="" above them.
const (
	levelUA = iota
	levelHint
	levelAuthor
	levelInline
)

type declPriority struct {
	important bool
	level     int
	spec      cascadia.Specificity
	sheet     int
	order     int // rule order within the sheet
	index     int // declaration order within the rule
}

func (a declPriority) less(b declPriority) bool {
	if a.important != b.important {
		return !a.important
	}
	if a.level != b.level {
		return a.level < b.level
	}
	if a.spec != b.spec {
		return a.spec.Less(b.spec)
	}
	if a.sheet != b.sheet {
		return a.sheet < b.sheet
	}
	if a.order != b.order {
		return a.order < b.order
	}
	return a.index < b.index
}

type candidate struct {
	decl cssDecl
	prio declPriority
}

// computedStyle holds an element's computed values in getComputedStyle's
// string form ("rgb(0, 0, 0)", "8px"), which is what the box builder parses.
type computedStyle struct {
	props        map[string]string
	fontSize     float64
	rootFontSize float64
	color        int32
	fontWeight   int
	vars         map[string]string
	contentWidth float64 // width available to children
	isInitial    bool    // the root element's parent: initial values only
}

const defaultFontSize = 16

func initialStyle() *computedStyle {
	cs := &computedStyle{
		props:        map[string]string{},
		fontSize:     defaultFontSize,
		rootFontSize: defaultFontSize,
		fontWeight:   400,
		contentWidth: screenWidth,
		isInitial:    true,
	}
	for name, spec := range propSpecs {
		cs.props[name] = spec.initial
	}
	cs.props["color"] = rgbString(0)
	return cs
}

type propSpec struct {
	inherited bool
	initial   string
	compute   func(v string, cs, parent *computedStyle) (string, bool)
}

var propSpecs map[string]propSpec

func init() {
	keywords := func(allowed ...string) func(string, *computedStyle, *computedStyle) (string, bool) {
		return func(v string, _, _ *computedStyle) (string, bool) {
			v = strings.ToLower(v)
			return v, slices.Contains(allowed, v)
		}
	}
	margin := func(v string, cs, parent *computedStyle) (string, bool) {
		if strings.EqualFold(v, "auto") {
			return "auto", true // parsePx reads it as 0; layout uses it to center
		}
		px, ok := parseLength(v, cs.lengthCtx(parent.contentWidth))
		return pxString(math.Max(px, 0)), ok
	}
	padding := func(v string, cs, parent *computedStyle) (string, bool) {
		px, ok := parseLength(v, cs.lengthCtx(parent.contentWidth))
		return pxString(math.Max(px, 0)), ok
	}
	borderWidth := func(v string, cs, _ *computedStyle) (string, bool) {
		switch strings.ToLower(v) {
		case "thin":
			return "1px", true
		case "medium":
			return "3px", true
		case "thick":
			return "5px", true
		}
		ctx := cs.lengthCtx(0)
		ctx.hasPercent = false
		px, ok := parseLength(v, ctx)
		return pxString(math.Max(px, 0)), ok
	}
	borderStyle := func(v string, _, _ *computedStyle) (string, bool) {
		v = strings.ToLower(v)
		return v, isBorderStyle(v)
	}
	ownColor := func(v string, cs, _ *computedStyle) (string, bool) {
		return cssColorValue(v, cs.color, true)
	}

	propSpecs = map[string]propSpec{
		"color": {inherited: true, compute: func(v string, _, parent *computedStyle) (string, bool) {
			return cssColorValue(v, parent.color, true)
		}},
		"backgroundColor": {initial: transparentRGBA, compute: ownColor},
		"fontSize":        {inherited: true, initial: pxString(defaultFontSize), compute: computeFontSize},
		"fontWeight":      {inherited: true, initial: "400", compute: computeFontWeight},
		"fontStyle": {inherited: true, initial: "normal", compute: func(v string, _, _ *computedStyle) (string, bool) {
			f := strings.Fields(strings.ToLower(v))
			if len(f) == 0 {
				return "", false
			}
			return f[0], f[0] == "normal" || f[0] == "italic" || f[0] == "oblique"
		}},
		"display":        {initial: "inline", compute: computeDisplay},
		"visibility":     {inherited: true, initial: "visible", compute: keywords("visible", "hidden", "collapse")},
		"textDecoration": {initial: "none", compute: computeTextDecoration},
		"textTransform":  {inherited: true, initial: "none", compute: keywords("none", "capitalize", "uppercase", "lowercase", "full-width")},
		"whiteSpace": {inherited: true, initial: "normal", compute: keywords(
			"normal", "pre", "nowrap", "pre-wrap", "pre-line", "break-spaces")},
		"textAlign":     {inherited: true, initial: "start", compute: computeTextAlign},
		"lineHeight":    {inherited: true, initial: "normal", compute: computeLineHeight},
		"float":         {initial: "none", compute: computeFloat},
		"listStyleType": {inherited: true, initial: "disc", compute: computeListStyleType},
		"width":         {initial: "auto", compute: computeWidth},
		"height":        {initial: "auto", compute: computeHeight},
	}
	for _, s := range sides {
		propSpecs["margin"+s] = propSpec{initial: "0px", compute: margin}
		propSpecs["padding"+s] = propSpec{initial: "0px", compute: padding}
		propSpecs["border"+s+"Width"] = propSpec{initial: "3px", compute: borderWidth}
		propSpecs["border"+s+"Style"] = propSpec{initial: "none", compute: borderStyle}
		propSpecs["border"+s+"Color"] = propSpec{initial: "currentcolor", compute: ownColor}
	}

	computeOrder = []string{"fontSize", "color", "fontWeight"}
	var rest []string
	for name := range propSpecs {
		if !slices.Contains(computeOrder, name) {
			rest = append(rest, name)
		}
	}
	slices.Sort(rest)
	computeOrder = append(computeOrder, rest...)
}

// computeOrder: font-size first (em lengths need it), then color
// (currentcolor needs it), then everything else.
var computeOrder []string

func (cs *computedStyle) lengthCtx(percentBase float64) lengthContext {
	return lengthContext{
		fontSize:     cs.fontSize,
		rootFontSize: cs.rootFontSize,
		percentBase:  percentBase,
		hasPercent:   true,
	}
}

var fontSizeKeywords = map[string]float64{
	"xx-small": 9, "x-small": 10, "small": 13, "medium": 16,
	"large": 18, "x-large": 24, "xx-large": 32, "xxx-large": 48,
}

func computeFontSize(v string, cs, parent *computedStyle) (string, bool) {
	lv := strings.ToLower(v)
	var px float64
	if k, ok := fontSizeKeywords[lv]; ok {
		px = k
	} else if lv == "smaller" {
		px = parent.fontSize / 1.2
	} else if lv == "larger" {
		px = parent.fontSize * 1.2
	} else {
		ctx := lengthContext{fontSize: parent.fontSize, rootFontSize: cs.rootFontSize,
			percentBase: parent.fontSize, hasPercent: true}
		var ok bool
		if px, ok = parseLength(v, ctx); !ok || px < 0 {
			return "", false
		}
	}
	cs.fontSize = px
	return pxString(px), true
}

func computeFontWeight(v string, cs, parent *computedStyle) (string, bool) {
	w := 0
	switch strings.ToLower(v) {
	case "normal":
		w = 400
	case "bold":
		w = 700
	case "bolder":
		switch {
		case parent.fontWeight < 350:
			w = 400
		case parent.fontWeight < 550:
			w = 700
		default:
			w = 900
		}
	case "lighter":
		switch {
		case parent.fontWeight < 550:
			w = 100
		case parent.fontWeight < 750:
			w = 400
		default:
			w = 700
		}
	default:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 1 || f > 1000 {
			return "", false
		}
		w = int(f)
	}
	cs.fontWeight = w
	return strconv.Itoa(w), true
}

func computeDisplay(v string, _, _ *computedStyle) (string, bool) {
	f := strings.Fields(strings.ToLower(v))
	if len(f) == 0 {
		return "", false
	}
	if len(f) > 1 { // two-value syntax: "inline flex", "block flow-root"...
		switch {
		case f[0] == "inline" && f[1] == "flow":
			return "inline", true
		case f[0] == "inline":
			return "inline-block", true
		case f[0] == "block":
			return "block", true
		}
		return f[0], true
	}
	switch f[0] {
	case "block", "inline", "inline-block", "flex", "inline-flex", "grid", "inline-grid",
		"flow-root", "list-item", "table", "inline-table", "table-row-group",
		"table-header-group", "table-footer-group", "table-row", "table-cell",
		"table-caption", "table-column", "table-column-group", "contents", "none", "run-in":
		return f[0], true
	case "-webkit-box", "-webkit-flex", "-ms-flexbox":
		return "flex", true
	case "-webkit-inline-box", "-webkit-inline-flex", "-ms-inline-flexbox":
		return "inline-flex", true
	}
	return "", false
}

func computeTextDecoration(v string, _, _ *computedStyle) (string, bool) {
	var lines []string
	for _, t := range splitSpaces(strings.ToLower(v)) {
		switch t {
		case "underline", "line-through", "overline":
			lines = append(lines, t)
		}
	}
	if len(lines) == 0 {
		return "none", true
	}
	return strings.Join(lines, " "), true
}

func computeTextAlign(v string, _, parent *computedStyle) (string, bool) {
	switch strings.ToLower(v) {
	case "left", "-webkit-left", "-moz-left":
		return "left", true
	case "right", "end", "-webkit-right", "-moz-right":
		return "right", true
	case "center":
		return "center", true
	case "-webkit-center", "-moz-center":
		// Like center, but also centers block children (<center>, align=).
		return "-webkit-center", true
	case "justify", "justify-all":
		return "justify", true
	case "start":
		return "start", true
	case "match-parent":
		return parent.props["textAlign"], true
	}
	return "", false
}

func computeLineHeight(v string, cs, _ *computedStyle) (string, bool) {
	if strings.EqualFold(v, "normal") {
		return "normal", true
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return pxString(f * cs.fontSize), f >= 0
	}
	px, ok := parseLength(v, cs.lengthCtx(cs.fontSize))
	return pxString(px), ok && px >= 0
}

func computeFloat(v string, _, _ *computedStyle) (string, bool) {
	switch strings.ToLower(v) {
	case "left", "inline-start":
		return "left", true
	case "right", "inline-end":
		return "right", true
	case "none":
		return "none", true
	}
	return "", false
}

func computeListStyleType(v string, _, _ *computedStyle) (string, bool) {
	v = strings.ToLower(v)
	for i := 0; i < len(v); i++ {
		if !isIdentChar(v[i]) {
			return "", false // strings and symbols() aren't supported
		}
	}
	return v, v != ""
}

func computeWidth(v string, cs, parent *computedStyle) (string, bool) {
	switch strings.ToLower(v) {
	case "auto", "max-content", "min-content", "fit-content", "-webkit-fill-available", "-moz-available", "stretch":
		return "auto", true
	}
	px, ok := parseLength(v, cs.lengthCtx(parent.contentWidth))
	if !ok || px < 0 {
		return "", false
	}
	// Desktop layouts are much wider than the Sidekick screen; let anything
	// that wouldn't fit just fill the available width.
	if px > parent.contentWidth {
		return "auto", true
	}
	return pxString(px), true
}

func computeHeight(v string, cs, _ *computedStyle) (string, bool) {
	if strings.EqualFold(v, "auto") || strings.HasSuffix(strings.TrimSpace(v), "%") {
		return "auto", true
	}
	ctx := cs.lengthCtx(0)
	ctx.hasPercent = false
	px, ok := parseLength(v, ctx)
	if !ok || px < 0 {
		return "", false
	}
	return pxString(px), true
}

// styleResolver matches rules against elements and computes their styles.
type styleResolver struct {
	sheets   []*stylesheet // in cascade order, UA sheets first
	uaSheets int
}

func newStyleResolver(author []*stylesheet, quirks bool) *styleResolver {
	r := &styleResolver{sheets: []*stylesheet{uaStylesheet()}}
	if quirks {
		r.sheets = append(r.sheets, quirksSheet)
	}
	r.uaSheets = len(r.sheets)
	r.sheets = append(r.sheets, author...)
	return r
}

func (r *styleResolver) compute(n *html.Node, parent *computedStyle) *computedStyle {
	byProp := map[string][]candidate{}
	add := func(d cssDecl, p declPriority) {
		p.important = d.important
		byProp[d.prop] = append(byProp[d.prop], candidate{d, p})
	}
	r.collect(n, "", add)
	for i, d := range presentationalHints(n) {
		add(d, declPriority{level: levelHint, index: i})
	}
	if style := attrValue(n, "style"); style != "" {
		for i, d := range parseDeclarations(style) {
			add(d, declPriority{level: levelInline, index: i})
		}
	}
	return r.computeFrom(byProp, parent)
}

// computePseudo returns the style and generated text of n's ::before or
// ::after, or nil when it has no content.
func (r *styleResolver) computePseudo(n *html.Node, pseudo string, cs *computedStyle) (*computedStyle, string) {
	byProp := map[string][]candidate{}
	r.collect(n, pseudo, func(d cssDecl, p declPriority) {
		p.important = d.important
		byProp[d.prop] = append(byProp[d.prop], candidate{d, p})
	})
	cands := byProp["content"]
	if len(cands) == 0 {
		return nil, ""
	}
	text, ok := "", false
	r.sortCandidates(cands)
	for _, c := range cands {
		v, vok := resolveDecl(c.decl, cs.vars)
		if !vok {
			continue
		}
		if text, ok = parseContent(v, n); ok {
			break
		}
	}
	if text == "" {
		return nil, ""
	}
	pcs := r.computeFrom(byProp, cs)
	if pcs.props["display"] == "none" {
		return nil, ""
	}
	return pcs, text
}

// collect adds the declarations of every rule matching n (or its pseudo
// element) in cascade order.
func (r *styleResolver) collect(n *html.Node, pseudo string, add func(cssDecl, declPriority)) {
	for si, ss := range r.sheets {
		if pseudo != "" {
			if ss = ss.pseudo[pseudo]; ss == nil {
				continue
			}
		}
		level := levelAuthor
		if si < r.uaSheets {
			level = levelUA
		}
		ss.matching(n, func(rule *cssRule) {
			for i, d := range rule.decls {
				add(d, declPriority{level: level, spec: rule.spec, sheet: si, order: rule.order, index: i})
			}
		})
	}
}

// parseContent evaluates a content value to text. Strings and attr() are
// supported; counters, quotes and images aren't.
func parseContent(v string, n *html.Node) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "none", "normal", "":
		return "", true
	}
	var b strings.Builder
	any := false
	for _, t := range splitSpaces(v) {
		switch {
		case len(t) >= 2 && (t[0] == '"' || t[0] == '\'') && t[len(t)-1] == t[0]:
			b.WriteString(unescapeCSSString(t[1 : len(t)-1]))
			any = true
		case strings.HasPrefix(strings.ToLower(t), "attr(") && strings.HasSuffix(t, ")"):
			b.WriteString(attrValue(n, strings.TrimSpace(t[5:len(t)-1])))
			any = true
		case t == "/": // alt text follows; ignore it
			return b.String(), any
		}
	}
	return b.String(), any
}

func (r *styleResolver) computeFrom(byProp map[string][]candidate, parent *computedStyle) *computedStyle {
	cs := &computedStyle{
		props:        make(map[string]string, len(propSpecs)),
		rootFontSize: parent.rootFontSize,
		vars:         parent.vars,
	}

	// Custom properties inherit, and are resolved lazily where they're used.
	var customs []string
	for prop := range byProp {
		if strings.HasPrefix(prop, "--") {
			customs = append(customs, prop)
		}
	}
	if len(customs) > 0 {
		vars := make(map[string]string, len(parent.vars)+len(customs))
		for k, v := range parent.vars {
			vars[k] = v
		}
		for _, prop := range customs {
			best := winner(byProp[prop])
			if isCSSWideKeyword(best.value) {
				if v, ok := parent.vars[prop]; ok && !strings.EqualFold(best.value, "initial") {
					vars[prop] = v
				} else {
					delete(vars, prop)
				}
				continue
			}
			vars[prop] = best.value
		}
		cs.vars = vars
	}

	for _, name := range computeOrder {
		spec := propSpecs[name]
		val, ok := r.cascadeValue(byProp[name], spec, cs, parent)
		if !ok {
			if spec.inherited {
				val = parent.props[name]
			} else {
				val, _ = r.initialValue(name, spec, cs, parent)
			}
		}
		cs.props[name] = val
		r.afterCompute(name, cs, parent)
	}

	r.fixup(cs, parent)
	return cs
}

// cascadeValue tries candidates from highest priority down, taking the first
// one that's valid after var() substitution.
func (r *styleResolver) cascadeValue(cands []candidate, spec propSpec, cs, parent *computedStyle) (string, bool) {
	if len(cands) == 0 {
		return "", false
	}
	r.sortCandidates(cands)
	for _, c := range cands {
		v, ok := resolveDecl(c.decl, cs.vars)
		if !ok {
			continue
		}
		switch strings.ToLower(v) {
		case "inherit":
			return parent.props[c.decl.prop], true
		case "initial":
			return r.initialValue(c.decl.prop, spec, cs, parent)
		case "unset", "revert", "revert-layer":
			if spec.inherited {
				return parent.props[c.decl.prop], true
			}
			return r.initialValue(c.decl.prop, spec, cs, parent)
		}
		if out, ok := spec.compute(v, cs, parent); ok {
			return out, true
		}
	}
	return "", false
}

func (r *styleResolver) initialValue(name string, spec propSpec, cs, parent *computedStyle) (string, bool) {
	switch name {
	case "color":
		return rgbString(0), true
	case "fontSize":
		return computeFontSize("medium", cs, parent)
	}
	if spec.initial == "currentcolor" {
		return rgbString(cs.color), true
	}
	return spec.initial, true
}

// afterCompute records the numeric values later properties depend on.
// computeFontSize sets fontSize itself; inherited values leave it zero.
func (r *styleResolver) afterCompute(name string, cs, parent *computedStyle) {
	switch name {
	case "fontSize":
		if cs.fontSize == 0 {
			cs.fontSize = parent.fontSize
		}
		if parent.isInitial {
			cs.rootFontSize = cs.fontSize // the root element defines rem
		}
	case "color":
		if c, ok := parseColor(cs.props["color"]); ok {
			cs.color = c
		}
	case "fontWeight":
		cs.fontWeight, _ = strconv.Atoi(cs.props["fontWeight"])
	}
}

func (r *styleResolver) fixup(cs, parent *computedStyle) {
	// A border with no style has no width.
	for _, s := range sides {
		switch cs.props["border"+s+"Style"] {
		case "none", "hidden":
			cs.props["border"+s+"Width"] = "0px"
		}
	}
	// Floated elements are blockified.
	if cs.props["float"] != "none" {
		switch cs.props["display"] {
		case "inline", "inline-block", "run-in":
			cs.props["display"] = "block"
		case "inline-flex":
			cs.props["display"] = "flex"
		case "inline-table":
			cs.props["display"] = "table"
		}
	}

	avail := parent.contentWidth
	if w := cs.props["width"]; w != "auto" {
		avail = float64(parsePx(w))
	} else {
		for _, p := range []string{"marginLeft", "marginRight", "borderLeftWidth", "borderRightWidth"} {
			avail -= float64(parsePx(cs.props[p]))
		}
	}
	avail -= float64(parsePx(cs.props["paddingLeft"]) + parsePx(cs.props["paddingRight"]))
	cs.contentWidth = math.Max(avail, 0)
}

// sortCandidates orders candidates from highest priority down.
func (r *styleResolver) sortCandidates(cands []candidate) {
	slices.SortStableFunc(cands, func(a, b candidate) int {
		switch {
		case b.prio.less(a.prio):
			return -1
		case a.prio.less(b.prio):
			return 1
		}
		return 0
	})
}

// winner returns the highest-priority declaration.
func winner(cands []candidate) cssDecl {
	best := cands[0]
	for _, c := range cands[1:] {
		if !c.prio.less(best.prio) {
			best = c
		}
	}
	return best.decl
}

// resolveDecl substitutes var() references, then expands a deferred shorthand.
func resolveDecl(d cssDecl, vars map[string]string) (string, bool) {
	v := d.value
	if hasVar(v) {
		var ok bool
		if v, ok = substituteVars(v, vars, 0); !ok {
			return "", false
		}
	}
	v = strings.TrimSpace(v)
	if d.shorthand == "" || isCSSWideKeyword(v) {
		return v, v != ""
	}
	expanded, ok := expandShorthand(d.shorthand, v)
	if !ok {
		return "", false
	}
	if lv, ok := expanded[d.prop]; ok {
		return lv, true
	}
	return "initial", true
}

// matching calls fn for every rule in ss that matches n, using the bucket
// index to skip rules that can't.
func (ss *stylesheet) matching(n *html.Node, fn func(*cssRule)) {
	ss.indexOnce.Do(ss.buildIndex)
	try := func(idx []int) {
		for _, i := range idx {
			rule := &ss.rules[i]
			if rule.sel.Match(n) {
				fn(rule)
			}
		}
	}
	try(ss.anyRules)
	try(ss.byTag[n.Data])
	if id := attrValue(n, "id"); id != "" {
		try(ss.byID[id])
	}
	if class := attrValue(n, "class"); class != "" {
		seen := map[string]bool{}
		for _, c := range strings.Fields(class) {
			if !seen[c] {
				seen[c] = true
				try(ss.byClass[c])
			}
		}
	}
}

func (ss *stylesheet) buildIndex() {
	ss.byTag = map[string][]int{}
	ss.byID = map[string][]int{}
	ss.byClass = map[string][]int{}
	for i, rule := range ss.rules {
		switch rule.bucket.kind {
		case 't':
			ss.byTag[rule.bucket.name] = append(ss.byTag[rule.bucket.name], i)
		case '#':
			ss.byID[rule.bucket.name] = append(ss.byID[rule.bucket.name], i)
		case '.':
			ss.byClass[rule.bucket.name] = append(ss.byClass[rule.bucket.name], i)
		default:
			ss.anyRules = append(ss.anyRules, i)
		}
	}
}

func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return true
		}
	}
	return false
}

// presentationalHints maps legacy HTML attributes to declarations.
func presentationalHints(n *html.Node) []cssDecl {
	var out []cssDecl
	set := func(name, value string) {
		out = appendDecl(out, name, value, false)
	}
	tag := n.Data

	if c, ok := legacyColor(attrValue(n, "bgcolor")); ok {
		set("background-color", c)
	}
	switch tag {
	case "body":
		if c, ok := legacyColor(attrValue(n, "text")); ok {
			set("color", c)
		}
	case "font":
		if c, ok := legacyColor(attrValue(n, "color")); ok {
			set("color", c)
		}
		if s, ok := legacyFontSize(attrValue(n, "size")); ok {
			set("font-size", s)
		}
	}
	switch tag {
	case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "td", "th", "tr", "tbody",
		"thead", "tfoot", "caption", "legend":
		switch a := strings.ToLower(attrValue(n, "align")); a {
		case "left", "right", "justify":
			set("text-align", a)
		case "center", "middle":
			set("text-align", "-webkit-center")
		}
	}
	switch tag {
	case "table", "td", "th", "col", "hr", "img", "iframe", "object", "embed", "video", "canvas":
		if w, ok := legacyLength(attrValue(n, "width")); ok {
			set("width", w)
		}
	}
	switch tag {
	case "td", "th", "tr", "img", "iframe", "object", "embed", "video", "canvas":
		if h, ok := legacyLength(attrValue(n, "height")); ok {
			set("height", h)
		}
	}
	if (tag == "td" || tag == "th") && hasAttr(n, "nowrap") {
		set("white-space", "nowrap")
	}
	if tag == "table" && strings.EqualFold(attrValue(n, "align"), "center") {
		set("margin-left", "auto")
		set("margin-right", "auto")
	}
	if tag == "table" {
		if b, err := strconv.Atoi(strings.TrimSpace(attrValue(n, "border"))); err == nil && b > 0 {
			set("border", strconv.Itoa(b)+"px outset")
		}
	}
	return out
}

// legacyColor parses an HTML color attribute, accepting the common
// unprefixed hex form (bgcolor="ffffff").
func legacyColor(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || strings.ContainsAny(v, ";{}") {
		return "", false
	}
	if _, ok := parseColor(v); ok {
		return v, true
	}
	if len(v) == 3 || len(v) == 6 {
		if _, err := strconv.ParseUint(v, 16, 32); err == nil {
			return "#" + v, true
		}
	}
	return "", false
}

func legacyLength(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	num, pct := strings.CutSuffix(v, "%")
	num = strings.TrimSuffix(num, "px")
	f, err := strconv.ParseFloat(num, 64)
	if err != nil || f < 0 {
		return "", false
	}
	if pct {
		return strconv.FormatFloat(f, 'f', -1, 64) + "%", true
	}
	return strconv.FormatFloat(f, 'f', -1, 64) + "px", true
}

var legacyFontSizes = [...]string{"x-small", "x-small", "small", "medium", "large", "x-large", "xx-large", "xxx-large"}

func legacyFontSize(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(v, "+"))
	if err != nil {
		return "", false
	}
	if v[0] == '+' || v[0] == '-' {
		n += 3
	}
	n = clamp(n, 1, 7)
	return legacyFontSizes[n], true
}

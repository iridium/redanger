package kimba

import "strings"

type layoutState struct {
	title     string
	styles    styleContextTable
	pageURL   string
	hasImages bool
	formCount int // number of <form> elements encountered
}

type layoutBox struct {
	kbcTag      int    // KBC tag ID (-1 for text nodes)
	text        string // text content (text nodes only)
	x, y        int    // position relative to parent
	width       int    // content width
	height      int    // total height
	styleCtxIdx int
	style       styleContext // cached style for layout
	children    []*layoutBox

	// Element-specific attributes
	href       string // <a>
	imgSrc     string // <img>
	imgAlt     string // <img>
	formAction string // <form>
	formMethod int    // <form> method byte (-1 = not set, 0=GET, 1=POST)
	inputType  int    // <input> type byte (-1 = not input)
	inputName  string // <input>/<textarea>/<select> name
	inputValue string // <input>/<textarea> value
	formID     int    // form index for form controls (-1 = none)
	olStart    int    // <ol> start attribute (-1 = not set)
	selectIdx  int    // <select> selected index (-1 = not set)
	cols       int    // <textarea> cols
	rows       int    // <textarea> rows
	selected   bool   // <option> selected flag
	colspan    int    // <td>/<th> columns spanned (>= 1)
	attrW      int    // <img> width/height attributes (0 = not given)
	attrH      int

	// Layout classification
	isBlock      bool
	isText       bool
	preformatted bool // inherited white-space: pre
	nowrap       bool // inherited white-space: nowrap
	inlineBlock  bool // display: inline-block (and inline-flex/grid/table)
	autoMarginL  bool // margin-left: auto
	centerBlocks bool // text-align: -webkit-center (<center>): centers block children too
	autoMarginR  bool // margin-right: auto
	fontID       int  // font the text is drawn in (text nodes: the parent's)

	// Text node extent data (filled during layout)
	extents []textExtent
}

type textExtent struct {
	x, y       int // relative to parent
	textOffset int // byte offset into the text string
	text       string
}

// lineLeading is added to a font's height to get the line height.
const lineLeading = 2

func lineHeightFor(fontID int) int {
	return fontLineHeight(fontID) + lineLeading
}

// boxFont is the font an element's own text (labels, options) is drawn in.
func boxFont(box *layoutBox) int {
	if box.style.fontID >= 0 {
		return box.style.fontID
	}
	return defaultFontID
}

func horizontalExtras(box *layoutBox) (padL, padR, borderL, borderR int) {
	ctx := &box.style
	padL, padR = max(ctx.paddingLeft, 0), max(ctx.paddingRight, 0)
	borderL, borderR = max(ctx.borderLeftWidth, 0), max(ctx.borderRightWidth, 0)
	return
}

// layoutBoxTree lays out box to fill availWidth (or its CSS width). Children
// are positioned relative to box.
func layoutBoxTree(box *layoutBox, availWidth int) {
	if box.isText {
		// Text nodes are laid out by the parent inline flow
		return
	}

	// Replaced elements: compute intrinsic size
	switch box.kbcTag {
	case tagImg:
		layoutImage(box, availWidth)
		return
	case tagInput:
		layoutInput(box)
		return
	case tagSelect:
		layoutSelect(box)
		return
	case tagTextarea:
		layoutTextarea(box)
		return
	case tagHR:
		box.width = availWidth
		box.height = hrHeight
		return
	case tagBR:
		box.width = 0
		box.height = 0
		return
	}

	box.width = availWidth

	if box.style.cssWidth >= 0 && box.kbcTag != tagTD && box.kbcTag != tagTH {
		box.width = min(box.style.cssWidth, availWidth)
	}

	if box.kbcTag == tagTable {
		layoutTable(box, availWidth)
		return
	}

	padL, padR, borderL, borderR := horizontalExtras(box)
	padTop := max(box.style.paddingTop, 0) + max(box.style.borderTopWidth, 0)
	padBottom := max(box.style.paddingBottom, 0) + max(box.style.borderBottomWidth, 0)
	left := padL + borderL
	contentWidth := max(box.width-left-padR-borderR, 1)

	cursorY := padTop

	// Partition children into runs of inline and block
	i := 0
	for i < len(box.children) {
		child := box.children[i]
		if child.isBlock && child.kbcTag != tagBR {
			marginTop := max(child.style.marginTop, 0)
			marginBottom := max(child.style.marginBottom, 0)
			marginLeft := max(child.style.marginLeft, 0)
			marginRight := max(child.style.marginRight, 0)

			cursorY += marginTop
			child.x = left + marginLeft
			child.y = cursorY
			avail := max(contentWidth-marginLeft-marginRight, 1)
			layoutBoxTree(child, avail)
			applyAutoMargins(child, avail)
			if box.centerBlocks && !child.autoMarginL && !child.autoMarginR && child.width < avail {
				child.x += (avail - child.width) / 2
			}
			cursorY += child.height + marginBottom
			i++
			continue
		}
		// Collect run of consecutive inline children
		runStart := i
		for i < len(box.children) && (!box.children[i].isBlock || box.children[i].kbcTag == tagBR) {
			i++
		}
		f := newInlineFlow(box, left, cursorY, contentWidth)
		f.layoutChildren(box.children[runStart:i])
		cursorY = f.finish()
	}

	box.height = cursorY + padBottom

	// Apply CSS height if set (use as minimum height)
	if box.style.cssHeight >= 0 && box.height < box.style.cssHeight {
		box.height = box.style.cssHeight
	}
}

// applyAutoMargins shifts a block narrower than avail by its auto margins.
func applyAutoMargins(box *layoutBox, avail int) {
	if slack := avail - box.width; slack > 0 {
		switch {
		case box.autoMarginL && box.autoMarginR:
			box.x += slack / 2
		case box.autoMarginL:
			box.x += slack
		}
	}
}

// inlineFlow lays out a run of inline content into lines, in the
// coordinates of the containing block.
type inlineFlow struct {
	left, width int // content box of the containing block
	align       int // textAlign of the containing block
	baseFont    int // the block's font: the minimum line height ("strut")

	x, y         int  // cursor; x is relative to left
	lineH        int  // height of the current line
	pendingSpace bool // a collapsible space precedes the next word
	lineFrags    []lineFrag
	inlines      []*layoutBox // inline elements, sized once the flow is done
}

// lineFrag is something placed on the current line, shifted by text-align:
// a box, or one extent of a text box (by index, since extents can grow).
type lineFrag struct {
	box *layoutBox
	ext int // extent index, or -1 for the box itself
}

func (f *inlineFlow) addFrag(box *layoutBox, ext int) {
	f.lineFrags = append(f.lineFrags, lineFrag{box, ext})
}

func newInlineFlow(block *layoutBox, left, y, width int) *inlineFlow {
	f := &inlineFlow{left: left, width: width, y: y, align: block.style.textAlign, baseFont: boxFont(block)}
	f.lineH = lineHeightFor(f.baseFont)
	return f
}

// newLine ends the current line (aligning it) and starts another.
func (f *inlineFlow) newLine() {
	f.alignLine()
	f.y += f.lineH
	f.x = 0
	f.lineH = lineHeightFor(f.baseFont)
	f.pendingSpace = false
}

func (f *inlineFlow) alignLine() {
	slack := f.width - f.x
	shift := 0
	switch f.align {
	case textAlignCenter:
		shift = slack / 2
	case textAlignRight:
		shift = slack
	}
	if shift > 0 {
		for _, fr := range f.lineFrags {
			if fr.ext >= 0 {
				fr.box.extents[fr.ext].x += shift
			} else {
				fr.box.x += shift
			}
		}
	}
	f.lineFrags = f.lineFrags[:0]
}

// finish ends the last line and returns the y below the flow.
func (f *inlineFlow) finish() int {
	if f.x > 0 {
		f.alignLine()
		f.y += f.lineH
	}
	f.lineFrags = nil
	f.boundInlines()
	return f.y
}

func (f *inlineFlow) layoutChildren(children []*layoutBox) {
	for _, child := range children {
		switch {
		case child.kbcTag == tagBR:
			if f.x == 0 {
				// An empty line still takes up a line.
				f.y += f.lineH
			} else {
				f.newLine()
			}
		case child.isText:
			f.layoutText(child)
		case isReplacedElement(child.kbcTag) || child.inlineBlock:
			f.layoutAtomic(child)
		case child.isBlock:
			f.layoutBlockInInline(child)
		default:
			f.layoutInlineElement(child)
		}
	}
}

// layoutAtomic places an unbreakable box: an image, a form control, or an
// inline-block (laid out as a block at its CSS or shrink-to-fit width).
func (f *inlineFlow) layoutAtomic(box *layoutBox) {
	ml, mr, mt, mb := 0, 0, 0, 0
	if box.inlineBlock {
		ml, mr = max(box.style.marginLeft, 0), max(box.style.marginRight, 0)
		mt, mb = max(box.style.marginTop, 0), max(box.style.marginBottom, 0)
		avail := max(f.width-ml-mr, 1)
		w := avail
		if box.style.cssWidth < 0 {
			_, maxW := intrinsicWidths(box)
			w = min(avail, max(maxW-ml-mr, 1))
		}
		layoutBoxTree(box, w)
	} else {
		layoutBoxTree(box, f.width)
	}
	outer := ml + box.width + mr
	if f.pendingSpace && f.x > 0 {
		f.x += textWidth(" ", f.baseFont)
	}
	f.pendingSpace = false
	if f.x > 0 && f.x+outer > f.width {
		f.newLine()
	}
	box.x = f.left + f.x + ml
	box.y = f.y + mt
	f.addFrag(box, -1)
	f.x += outer
	f.lineH = max(f.lineH, mt+box.height+mb)
}

// layoutBlockInInline lays out a block inside an inline element (<a><p>):
// it breaks the line and takes the full width, as in CSS.
func (f *inlineFlow) layoutBlockInInline(box *layoutBox) {
	if f.x > 0 {
		f.newLine()
	}
	mt, mb := max(box.style.marginTop, 0), max(box.style.marginBottom, 0)
	ml, mr := max(box.style.marginLeft, 0), max(box.style.marginRight, 0)
	box.x = f.left + ml
	box.y = f.y + mt
	avail := max(f.width-ml-mr, 1)
	layoutBoxTree(box, avail)
	applyAutoMargins(box, avail)
	f.y = box.y + box.height + mb
	f.pendingSpace = false
}

func (f *inlineFlow) layoutInlineElement(box *layoutBox) {
	// A space before the element belongs outside it.
	if f.pendingSpace && f.x > 0 {
		f.x += textWidth(" ", f.baseFont)
		f.pendingSpace = false
	}
	// Horizontal margins, borders and padding take room on the line.
	padL, padR, borderL, borderR := horizontalExtras(box)
	f.x += max(box.style.marginLeft, 0) + borderL + padL
	box.x, box.y = f.left+f.x, f.y
	box.width, box.height = 0, f.lineH
	f.inlines = append(f.inlines, box)
	f.layoutChildren(box.children)
	f.x += padR + borderR + max(box.style.marginRight, 0)
}

// boundInlines sizes each inline element to the union of the content laid
// out inside it (after alignment, so it covers the final positions).
func (f *inlineFlow) boundInlines() {
	for _, box := range f.inlines {
		x0, y0, x1, y1, ok := contentBounds(box)
		if ok {
			box.x, box.y = x0, y0
			box.width, box.height = x1-x0, y1-y0
		}
	}
	f.inlines = nil
}

// contentBounds is the block-space bounding box of an inline box's text and
// atomic descendants.
func contentBounds(box *layoutBox) (x0, y0, x1, y1 int, ok bool) {
	add := func(ax0, ay0, ax1, ay1 int) {
		if !ok {
			x0, y0, x1, y1, ok = ax0, ay0, ax1, ay1, true
			return
		}
		x0, y0, x1, y1 = min(x0, ax0), min(y0, ay0), max(x1, ax1), max(y1, ay1)
	}
	for _, c := range box.children {
		switch {
		case c.isText:
			lh := lineHeightFor(c.fontID)
			for _, e := range c.extents {
				add(e.x, e.y, e.x+textWidth(e.text, c.fontID), e.y+lh)
			}
		case isReplacedElement(c.kbcTag) || c.inlineBlock || c.isBlock:
			add(c.x, c.y, c.x+c.width, c.y+c.height)
		default:
			if cx0, cy0, cx1, cy1, cok := contentBounds(c); cok {
				add(cx0, cy0, cx1, cy1)
			}
		}
	}
	return
}

func (f *inlineFlow) layoutText(box *layoutBox) {
	box.extents = nil
	if box.text == "" {
		return
	}
	font := box.fontID
	f.lineH = max(f.lineH, lineHeightFor(font))

	if box.preformatted {
		f.layoutPreText(box, font)
		box.width, box.height = f.width, max(f.lineH, 1)
		return
	}

	space := textWidth(" ", font)
	text := box.text
	if text[0] == ' ' {
		f.pendingSpace = true
	}

	// Each extent is a run of words on one line: [start, end) of box.text.
	start, end := -1, -1
	flush := func() {
		if start >= 0 {
			ext := textExtent{x: f.left + f.x - textWidth(text[start:end], font), y: f.y, textOffset: start, text: text[start:end]}
			box.extents = append(box.extents, ext)
			f.addFrag(box, len(box.extents)-1)
			start = -1
		}
	}

	i := 0
	for i < len(text) {
		if text[i] == ' ' {
			f.pendingSpace = true
			i++
			continue
		}
		j := strings.IndexByte(text[i:], ' ')
		if j < 0 {
			j = len(text)
		} else {
			j += i
		}
		word := text[i:j]
		w := textWidth(word, font)
		lead := 0
		if f.pendingSpace && f.x > 0 {
			lead = space
		}

		if f.x > 0 && f.x+lead+w > f.width && !box.nowrap {
			flush()
			f.newLine()
			lead = 0
		}
		if f.x == 0 && w > f.width && !box.nowrap {
			// A word wider than the line: break it between characters.
			flush()
			for _, part := range splitToWidth(word, font, f.width) {
				if f.x > 0 {
					flush()
					f.newLine()
				}
				start, end = i, i+len(part)
				f.x += textWidth(part, font)
				i += len(part)
			}
			flush()
			f.pendingSpace = false
			continue
		}

		// A new extent starts after the space; an open one runs across it.
		f.x += lead
		if start < 0 {
			start = i
		}
		end = j
		f.x += w
		f.pendingSpace = false
		i = j
	}
	flush()

	// Text node bounds, in block coordinates (the extents carry the layout).
	if len(box.extents) > 0 {
		first := box.extents[0]
		box.x, box.y = first.x, first.y
		box.width = f.width
		box.height = f.y - first.y + f.lineH
	}
}

func (f *inlineFlow) layoutPreText(box *layoutBox, font int) {
	offset := 0
	for i, line := range strings.Split(box.text, "\n") {
		if i > 0 {
			f.newLine()
			f.lineH = max(f.lineH, lineHeightFor(font))
		}
		if line != "" {
			box.extents = append(box.extents, textExtent{x: f.left + f.x, y: f.y, textOffset: offset, text: line})
			f.addFrag(box, len(box.extents)-1)
			f.x += textWidth(line, font)
		}
		offset += len(line) + 1
	}
	if len(box.extents) > 0 {
		box.x, box.y = box.extents[0].x, box.extents[0].y
	}
}

// splitToWidth breaks s into pieces no wider than width (at least one
// character each).
func splitToWidth(s string, font, width int) []string {
	var parts []string
	start, w := 0, 0
	for i, r := range s {
		cw := textWidth(string(r), font)
		if i > start && w+cw > width {
			parts = append(parts, s[start:i])
			start, w = i, 0
		}
		w += cw
	}
	return append(parts, s[start:])
}

func isReplacedElement(tag int) bool {
	return tag == tagImg || tag == tagInput || tag == tagSelect || tag == tagTextarea
}

func layoutImage(box *layoutBox, availWidth int) {
	w, h := box.attrW, box.attrH
	if w <= 0 {
		w = min(defaultImageWidth, availWidth)
	}
	if h <= 0 {
		h = defaultImageHeight
	}
	// Clamp width to available space, preserving aspect ratio
	if w > availWidth {
		if box.attrW > 0 && box.attrH > 0 {
			h = max(h*availWidth/w, 1)
		}
		w = availWidth
	}
	box.width = w
	box.height = h
}

func layoutInput(box *layoutBox) {
	font := boxFont(box)
	lh := lineHeightFor(font)
	switch box.inputType {
	case inputTypeHidden:
		box.width = 0
		box.height = 0
	case inputTypeCheckbox, inputTypeRadio:
		box.width = checkboxSize
		box.height = checkboxSize
	case inputTypeSubmit, inputTypeReset, inputTypeButton:
		label := box.inputValue
		if label == "" {
			switch box.inputType {
			case inputTypeSubmit:
				label = defaultSubmitLabel
			case inputTypeReset:
				label = defaultResetLabel
			default:
				label = defaultButtonLabel
			}
		}
		box.width = textWidth(label, font) + buttonPaddingH
		box.height = lh + buttonPaddingV
	default: // text, password, file
		box.width = defaultInputSize * textWidth("n", font)
		box.height = lh + buttonPaddingV
	}
}

func layoutSelect(box *layoutBox) {
	font := boxFont(box)
	w := selectMinOptionWidth * textWidth("n", font)
	for _, opt := range box.children {
		w = max(w, textWidth(opt.text, font))
	}
	box.width = w + selectArrowSpace
	box.height = lineHeightFor(font) + buttonPaddingV
}

func layoutTextarea(box *layoutBox) {
	font := boxFont(box)
	box.width = box.cols*textWidth("n", font) + textareaPadding
	box.height = box.rows*lineHeightFor(font) + textareaPadding
}

// tableRows returns a table's rows, looking inside row groups.
func tableRows(table *layoutBox) []*layoutBox {
	var rows []*layoutBox
	for _, child := range table.children {
		switch child.kbcTag {
		case tagTR:
			rows = append(rows, child)
		case tagTBody:
			for _, gc := range child.children {
				if gc.kbcTag == tagTR {
					rows = append(rows, gc)
				}
			}
		}
	}
	return rows
}

func isCell(b *layoutBox) bool {
	return b.kbcTag == tagTD || b.kbcTag == tagTH
}

func cellSpan(b *layoutBox) int {
	return max(b.colspan, 1)
}

// tableColumns measures each column's min-content and max-content width, and
// which columns have a fixed (CSS or width=) width.
func tableColumns(rows []*layoutBox) (colMin, colMax []int, fixed []bool) {
	ncols := 0
	for _, tr := range rows {
		n := 0
		for _, td := range tr.children {
			if isCell(td) {
				n += cellSpan(td)
			}
		}
		ncols = max(ncols, n)
	}
	colMin, colMax, fixed = make([]int, ncols), make([]int, ncols), make([]bool, ncols)

	type spanning struct{ col, span, min, max int }
	var multi []spanning
	for _, tr := range rows {
		col := 0
		for _, td := range tr.children {
			if !isCell(td) {
				continue
			}
			span := cellSpan(td)
			mn, mx := intrinsicWidths(td)
			if span == 1 {
				colMin[col] = max(colMin[col], mn)
				colMax[col] = max(colMax[col], mx)
				fixed[col] = fixed[col] || td.style.cssWidth >= 0
			} else {
				multi = append(multi, spanning{col, span, mn, mx})
			}
			col += span
		}
	}
	// Widen spanned columns when a spanning cell needs more room: the auto
	// ones if there are any, else all of them.
	for _, m := range multi {
		grow := func(cols []int, need int) {
			have := 0
			var targets []int
			for i := m.col; i < m.col+m.span; i++ {
				have += cols[i]
				if !fixed[i] {
					targets = append(targets, i)
				}
			}
			if len(targets) == 0 {
				for i := m.col; i < m.col+m.span; i++ {
					targets = append(targets, i)
				}
			}
			if need > have {
				per := (need - have + len(targets) - 1) / len(targets)
				for _, i := range targets {
					cols[i] += per
				}
			}
		}
		grow(colMin, m.min)
		grow(colMax, m.max)
	}
	for i := range colMax {
		colMax[i] = max(colMax[i], colMin[i])
	}
	return colMin, colMax, fixed
}

// distributeColumns picks column widths summing to width: max-content when
// it fits (extra space going to the auto-width columns), otherwise
// interpolated between min- and max-content.
func distributeColumns(colMin, colMax []int, fixed []bool, width int) []int {
	n := len(colMin)
	widths := make([]int, n)
	sumMin, sumMax := 0, 0
	for i := range n {
		sumMin += colMin[i]
		sumMax += colMax[i]
	}
	switch {
	case sumMax <= width:
		extra := width - sumMax
		autoMax, autoCount := 0, 0
		for i := range n {
			if !fixed[i] {
				autoMax += colMax[i]
				autoCount++
			}
		}
		for i := range n {
			widths[i] = colMax[i]
			switch {
			case autoCount == 0:
				widths[i] += extra / n
			case fixed[i]:
			case autoMax > 0:
				widths[i] += extra * colMax[i] / autoMax
			default:
				widths[i] += extra / autoCount
			}
		}
	case sumMin >= width:
		// Even min-content doesn't fit: shrink proportionally (the cells'
		// text will break mid-word) rather than run off the screen.
		for i := range n {
			if sumMin > 0 {
				widths[i] = colMin[i] * width / sumMin
			}
		}
	default:
		for i := range n {
			widths[i] = colMin[i] + (colMax[i]-colMin[i])*(width-sumMin)/(sumMax-sumMin)
		}
	}
	// Rounding leftovers go to the last auto column.
	total := 0
	for _, w := range widths {
		total += w
	}
	last := n - 1
	for i := n - 1; i >= 0; i-- {
		if !fixed[i] {
			last = i
			break
		}
	}
	if n > 0 {
		widths[last] += width - total
	}
	return widths
}

func layoutTable(box *layoutBox, availWidth int) {
	rows := tableRows(box)
	colMin, colMax, fixed := tableColumns(rows)
	sumMin, sumMax := 0, 0
	for i := range colMin {
		sumMin += colMin[i]
		sumMax += colMax[i]
	}

	width := availWidth
	if box.style.cssWidth >= 0 {
		width = min(box.style.cssWidth, availWidth)
	} else {
		width = min(availWidth, max(sumMax, sumMin))
	}
	if len(rows) == 0 || len(colMin) == 0 {
		box.width, box.height = width, 0
		return
	}
	widths := distributeColumns(colMin, colMax, fixed, width)

	cursorY := 0
	for _, tr := range rows {
		x, col, rowHeight := 0, 0, 0
		var cells []*layoutBox
		for _, td := range tr.children {
			if !isCell(td) || col >= len(widths) {
				continue
			}
			span := min(cellSpan(td), len(widths)-col)
			w := 0
			for _, cw := range widths[col : col+span] {
				w += cw
			}
			td.x, td.y = x, 0 // relative to TR
			layoutBoxTree(td, max(w, 1))
			td.width = w
			rowHeight = max(rowHeight, td.height)
			cells = append(cells, td)
			x += w
			col += span
		}
		for _, td := range cells {
			td.height = rowHeight
		}
		tr.x, tr.y = 0, cursorY
		tr.width, tr.height = width, rowHeight
		cursorY += rowHeight
	}

	box.width = width
	box.height = cursorY

	// Row groups share the table's coordinate space (rows are positioned
	// relative to the table), so give them the table's box.
	for _, child := range box.children {
		if child.kbcTag == tagTBody {
			child.x, child.y = 0, 0
			child.width, child.height = width, cursorY
		}
	}
}

// intrinsicWidths returns box's min-content width (its widest unbreakable
// piece) and max-content width (its width with no line wrapping), including
// its padding, borders and horizontal margins.
func intrinsicWidths(box *layoutBox) (minW, maxW int) {
	switch {
	case box.isText:
		return textIntrinsic(box)
	case box.kbcTag == tagBR:
		return 0, 0
	case box.kbcTag == tagImg:
		w := box.attrW
		if w <= 0 {
			w = defaultImageWidth
		}
		return w, w
	case isReplacedElement(box.kbcTag):
		layoutBoxTree(box, screenWidth)
		return box.width, box.width
	case box.kbcTag == tagHR:
		return 0, 0
	case box.kbcTag == tagTable:
		colMin, colMax, _ := tableColumns(tableRows(box))
		for i := range colMin {
			minW += colMin[i]
			maxW += colMax[i]
		}
	default:
		line := 0 // max-content width of the current inline run
		for _, c := range box.children {
			mn, mx := intrinsicWidths(c)
			minW = max(minW, mn)
			if c.kbcTag == tagBR || (c.isBlock && !c.isText) {
				maxW = max(maxW, line)
				line = 0
				if c.kbcTag != tagBR {
					maxW = max(maxW, mx)
				}
				continue
			}
			line += mx
		}
		maxW = max(maxW, line)
	}

	if box.style.cssWidth >= 0 {
		minW = max(minW, box.style.cssWidth)
		maxW = minW
	}
	padL, padR, borderL, borderR := horizontalExtras(box)
	extra := padL + padR + borderL + borderR +
		max(box.style.marginLeft, 0) + max(box.style.marginRight, 0)
	return minW + extra, max(maxW, minW) + extra
}

func textIntrinsic(box *layoutBox) (minW, maxW int) {
	font := box.fontID
	if box.preformatted {
		for _, line := range strings.Split(box.text, "\n") {
			maxW = max(maxW, textWidth(line, font))
		}
		return maxW, maxW
	}
	for _, word := range strings.Fields(box.text) {
		minW = max(minW, textWidth(word, font))
	}
	maxW = textWidth(box.text, font)
	if box.nowrap {
		minW = maxW
	}
	return minW, maxW
}

// Layout positions everything inside an inline formatting context in the
// containing block's coordinates. KBC wants each element relative to its
// parent, and text extents as deltas: the first from the parent element's
// origin, each later one from the previous extent (KDText accumulates them).

// relativizeBlock fixes up the children of a box laid out by layoutBoxTree,
// whose children are already relative to it.
func relativizeBlock(box *layoutBox) {
	for _, c := range box.children {
		switch {
		case c.isText:
			extentsToDeltas(c, 0, 0)
		case c.isBlock || c.inlineBlock:
			relativizeBlock(c)
		default:
			relativizeInline(c, 0, 0)
		}
	}
}

// relativizeInline converts an inline-flowed box and its descendants from
// block coordinates; (ox, oy) is the block-space position of its parent.
func relativizeInline(box *layoutBox, ox, oy int) {
	bx, by := box.x, box.y
	box.x -= ox
	box.y -= oy
	for _, c := range box.children {
		switch {
		case c.isText:
			extentsToDeltas(c, bx, by)
		case (c.isBlock && c.kbcTag != tagBR) || c.inlineBlock:
			// Positioned in block space; its own children are relative to it.
			c.x -= bx
			c.y -= by
			relativizeBlock(c)
		default:
			relativizeInline(c, bx, by)
		}
	}
}

func extentsToDeltas(box *layoutBox, ox, oy int) {
	px, py := ox, oy
	for i := range box.extents {
		e := &box.extents[i]
		ax, ay := e.x, e.y
		e.x, e.y = ax-px, ay-py
		px, py = ax, ay
	}
}

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
	kbcTag      int          // KBC tag ID (-1 for text nodes)
	text        string       // text content (text nodes only)
	x, y        int          // position relative to parent
	width       int          // content width
	height      int          // total height
	styleCtxIdx int          // style context table index
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

	// Layout classification
	isBlock      bool
	isText       bool
	preformatted bool // inherited white-space: pre

	// Text node extent data (filled during layout)
	extents []textExtent
}

type textExtent struct {
	x, y       int // relative to parent
	textOffset int // byte offset into the text string
	text       string
}

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

	// Apply CSS width if set (clamp to available width)
	if box.style.cssWidth >= 0 {
		box.width = box.style.cssWidth
		if box.width > availWidth {
			box.width = availWidth
		}
	}

	if box.kbcTag == tagTable {
		layoutTable(box, box.width)
		return
	}

	// Get margin/padding from style context
	ctx := box.style
	padLeft := max(ctx.paddingLeft, 0)
	padTop := max(ctx.paddingTop, 0)
	padRight := max(ctx.paddingRight, 0)
	padBottom := max(ctx.paddingBottom, 0)

	contentWidth := availWidth - padLeft - padRight
	if contentWidth < charWidth {
		contentWidth = charWidth
	}

	cursorY := padTop

	// Partition children into runs of inline and block
	i := 0
	for i < len(box.children) {
		child := box.children[i]
		if child.isBlock {
			// Block child
			marginTop := max(child.style.marginTop, 0)
			marginBottom := max(child.style.marginBottom, 0)
			marginLeft := max(child.style.marginLeft, 0)

			cursorY += marginTop
			child.x = padLeft + marginLeft
			child.y = cursorY
			childWidth := contentWidth - marginLeft
			if childWidth < charWidth {
				childWidth = charWidth
			}
			layoutBoxTree(child, childWidth)
			cursorY += child.height + marginBottom
			i++
		} else {
			// Collect run of consecutive inline children
			runStart := i
			for i < len(box.children) && !box.children[i].isBlock {
				i++
			}
			inlineRun := box.children[runStart:i]
			cursorY = layoutInlineFlow(inlineRun, padLeft, cursorY, contentWidth, box.preformatted)
		}
	}

	box.height = cursorY + padBottom

	// Apply CSS height if set (use as minimum height)
	if box.style.cssHeight >= 0 && box.height < box.style.cssHeight {
		box.height = box.style.cssHeight
	}
}

func layoutInlineFlow(children []*layoutBox, padLeft, startY, contentWidth int, parentPre bool) int {
	cursorX := 0
	cursorY := startY
	currentLineHeight := lineHeight

	for _, child := range children {
		if child.kbcTag == tagBR {
			// Line break
			cursorX = 0
			cursorY += currentLineHeight
			currentLineHeight = lineHeight
			continue
		}

		if child.isText {
			cursorX, cursorY, currentLineHeight = layoutTextNode(child, padLeft, cursorX, cursorY, contentWidth, currentLineHeight)
			continue
		}

		// Replaced inline elements (img, input, select, textarea)
		if isReplacedElement(child.kbcTag) {
			layoutBoxTree(child, contentWidth-cursorX)
			if cursorX > 0 && cursorX+child.width > contentWidth {
				cursorX = 0
				cursorY += currentLineHeight
				currentLineHeight = lineHeight
			}
			child.x = padLeft + cursorX
			child.y = cursorY
			cursorX += child.width
			if child.height > currentLineHeight {
				currentLineHeight = child.height
			}
			continue
		}

		// Non-replaced inline element (a, b, span, etc.): recursively collect atoms
		cursorX, cursorY, currentLineHeight = layoutInlineElement(child, padLeft, cursorX, cursorY, contentWidth, currentLineHeight, parentPre)
	}

	// Account for last line
	if cursorX > 0 {
		cursorY += currentLineHeight
	}

	return cursorY
}

func layoutInlineElement(box *layoutBox, padLeft, cursorX, cursorY, contentWidth, currentLineHeight int, parentPre bool) (int, int, int) {
	box.x = padLeft + cursorX
	box.y = cursorY
	startX := cursorX
	startY := cursorY
	maxW := 0
	pre := parentPre || box.preformatted

	for _, child := range box.children {
		if child.kbcTag == tagBR {
			cursorX = 0
			cursorY += currentLineHeight
			currentLineHeight = lineHeight
			continue
		}
		if child.isText {
			cursorX, cursorY, currentLineHeight = layoutTextNode(child, padLeft, cursorX, cursorY, contentWidth, currentLineHeight)
		} else if isReplacedElement(child.kbcTag) {
			layoutBoxTree(child, contentWidth-cursorX)
			if cursorX > 0 && cursorX+child.width > contentWidth {
				cursorX = 0
				cursorY += currentLineHeight
				currentLineHeight = lineHeight
			}
			child.x = padLeft + cursorX
			child.y = cursorY
			cursorX += child.width
			if child.height > currentLineHeight {
				currentLineHeight = child.height
			}
		} else {
			cursorX, cursorY, currentLineHeight = layoutInlineElement(child, padLeft, cursorX, cursorY, contentWidth, currentLineHeight, pre)
		}
		if cursorX > maxW {
			maxW = cursorX
		}
	}

	// Compute this inline element's dimensions relative to its start
	box.x = padLeft + startX
	box.y = startY
	box.width = maxW - startX
	if box.width < 0 {
		box.width = 0
	}
	if cursorY > startY {
		box.height = cursorY - startY + currentLineHeight
	} else {
		box.height = currentLineHeight
	}

	return cursorX, cursorY, currentLineHeight
}

func layoutTextNode(box *layoutBox, padLeft, cursorX, cursorY, contentWidth, currentLineHeight int) (int, int, int) {
	if box.text == "" {
		return cursorX, cursorY, currentLineHeight
	}

	box.x = padLeft + cursorX
	box.y = cursorY
	box.extents = nil

	charsAvail := (contentWidth - cursorX) / charWidth
	if charsAvail < 1 {
		charsAvail = 1
	}

	if box.preformatted {
		// Preformatted: split on newlines, preserve whitespace
		lines := strings.Split(box.text, "\n")
		offset := 0
		for i, line := range lines {
			if i > 0 {
				// New line
				cursorX = 0
				cursorY += currentLineHeight
				currentLineHeight = lineHeight
			}
			box.extents = append(box.extents, textExtent{
				x:          padLeft + cursorX,
				y:          cursorY,
				textOffset: offset,
				text:       line,
			})
			cursorX += len(line) * charWidth
			offset += len(line) + 1 // +1 for the \n
		}
	} else {
		// Normal: word-wrap
		words := strings.Fields(box.text)
		offset := 0
		lineStart := offset
		var lineBuf strings.Builder

		flushLine := func() {
			if lineBuf.Len() > 0 {
				box.extents = append(box.extents, textExtent{
					x:          padLeft + cursorX,
					y:          cursorY,
					textOffset: lineStart,
					text:       lineBuf.String(),
				})
				cursorX += lineBuf.Len() * charWidth
				lineBuf.Reset()
			}
		}

		totalChars := (contentWidth) / charWidth
		if totalChars < 1 {
			totalChars = 1
		}

		for _, word := range words {
			wordLen := len(word)
			availChars := max((contentWidth-cursorX)/charWidth, 1)

			if lineBuf.Len() == 0 {
				// First word on current position
				if cursorX > 0 && wordLen > availChars {
					// Wrap to next line
					cursorX = 0
					cursorY += currentLineHeight
					currentLineHeight = lineHeight
				}
				lineStart = offset
				lineBuf.WriteString(word)
			} else if lineBuf.Len()+1+wordLen <= availChars {
				lineBuf.WriteByte(' ')
				lineBuf.WriteString(word)
			} else {
				// Flush current line, start new line
				flushLine()
				cursorX = 0
				cursorY += currentLineHeight
				currentLineHeight = lineHeight
				lineStart = offset
				lineBuf.WriteString(word)
			}
			offset += wordLen + 1 // +1 for space
		}
		flushLine()
	}

	// Compute text node bounds
	if len(box.extents) > 0 {
		first := box.extents[0]
		last := box.extents[len(box.extents)-1]
		box.x = first.x
		box.y = first.y
		if last.y > first.y {
			box.height = last.y - first.y + lineHeight
		} else {
			box.height = lineHeight
		}
		box.width = max(cursorX-(box.x-padLeft), 0)
	}

	return cursorX, cursorY, currentLineHeight
}

func isReplacedElement(tag int) bool {
	return tag == tagImg || tag == tagInput || tag == tagSelect || tag == tagTextarea
}

func layoutImage(box *layoutBox, availWidth int) {
	origW := box.width
	origH := box.height
	if origW <= 0 {
		origW = availWidth
	}
	if origH <= 0 {
		origH = defaultImageHeight
	}
	// Clamp width to available space, preserving aspect ratio
	if origW > availWidth {
		if box.width > 0 && box.height > 0 {
			// We have both original dimensions, so scale proportionally
			origH = max(origH*availWidth/origW, 1)
		}
		origW = availWidth
	}
	box.width = origW
	box.height = origH
}

func layoutInput(box *layoutBox) {
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
		box.width = len(label)*charWidth + buttonPaddingH
		box.height = lineHeight + buttonPaddingV
	default: // text, password, file
		box.width = defaultInputSize * charWidth
		box.height = lineHeight + buttonPaddingV
	}
}

func layoutSelect(box *layoutBox) {
	maxLen := selectMinOptionWidth
	for _, opt := range box.children {
		if len(opt.text) > maxLen {
			maxLen = len(opt.text)
		}
	}
	box.width = maxLen*charWidth + selectArrowSpace
	box.height = lineHeight + buttonPaddingV
}

func layoutTextarea(box *layoutBox) {
	box.width = box.cols*charWidth + textareaPadding
	box.height = box.rows*lineHeight + textareaPadding
}

func layoutTable(box *layoutBox, availWidth int) {
	// Collect TR children (may be inside TBODY/THEAD/TFOOT)
	var trBoxes []*layoutBox
	for _, child := range box.children {
		switch child.kbcTag {
		case tagTR:
			trBoxes = append(trBoxes, child)
		case tagTBody:
			for _, gc := range child.children {
				if gc.kbcTag == tagTR {
					trBoxes = append(trBoxes, gc)
				}
			}
		}
	}

	if len(trBoxes) == 0 {
		box.width = availWidth
		box.height = 0
		return
	}

	// Determine max columns
	maxCols := 0
	for _, tr := range trBoxes {
		cols := 0
		for _, td := range tr.children {
			if td.kbcTag == tagTD || td.kbcTag == tagTH {
				cols++
			}
		}
		if cols > maxCols {
			maxCols = cols
		}
	}
	if maxCols == 0 {
		maxCols = 1
	}
	cellWidth := availWidth / maxCols

	cursorY := 0
	for _, tr := range trBoxes {
		tr.x = 0
		tr.y = cursorY
		tr.width = availWidth

		colIdx := 0
		rowHeight := lineHeight
		for _, td := range tr.children {
			if td.kbcTag != tagTD && td.kbcTag != tagTH {
				continue
			}
			td.x = colIdx * cellWidth
			td.y = 0 // relative to TR
			layoutBoxTree(td, cellWidth)
			if td.height > rowHeight {
				rowHeight = td.height
			}
			colIdx++
		}
		tr.height = rowHeight
		cursorY += rowHeight
	}

	box.width = availWidth
	box.height = cursorY
}

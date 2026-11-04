package kimba

import (
	"context"
	"log"
	"net/url"
	"strconv"
	"strings"
)

// htmlToKBC renders an HTML document to KBC. pageURL is the URL the document
// was served from (after redirects).
func htmlToKBC(pageURL string, body []byte, contentType string) []byte {
	page, err := renderHTML(context.Background(), pageURL, body, contentType)
	if err != nil {
		log.Printf("Kimba: render failed for %s: %v", pageURL, err)
		return nil
	}

	ls := &layoutState{pageURL: page.baseURL}

	// Intern the default (empty) style context as entry 1.
	defaultCtx := newDefaultStyleContext()
	ls.styles.intern(defaultCtx)

	ls.title = page.title

	bodyBox := buildBoxTree(page.body, ls, false, nil)
	if bodyBox == nil {
		log.Printf("Kimba: empty box tree for %s", pageURL)
		return nil
	}

	// Body-level background/text colors.
	var colors bodyColors
	if c, ok := parseColor(page.body.Styles["backgroundColor"]); ok &&
		page.body.Styles["backgroundColor"] != transparentRGBA {
		colors.bgColor = c
		colors.bgColorSet = true
	}
	if c, ok := parseColor(page.body.Styles["color"]); ok {
		colors.textColor = c
		colors.textColorSet = true
	}

	return buildKBCOutput(ls, bodyBox, colors)
}

func buildBoxTree(node *domNode, ls *layoutState, parentPre bool, parentStyle *styleContext) *layoutBox {
	if node == nil {
		return nil
	}

	if node.Tag == textNodeTag {
		text := node.Text
		if !parentPre {
			// Collapse runs of spaces but keep one at either edge: it
			// separates this text from its inline neighbours.
			text = collapseSpaces(text)
		}
		text = stripInvisible(text)
		if text == "" {
			return nil
		}
		box := &layoutBox{
			kbcTag:       tagText,
			text:         text,
			isText:       true,
			preformatted: parentPre,
			fontID:       defaultFontID,
			inputType:    -1,
			formID:       -1,
			formMethod:   -1,
			olStart:      -1,
			selectIdx:    -1,
		}
		if parentStyle != nil {
			if parentStyle.fontID >= 0 {
				box.fontID = parentStyle.fontID
			}
			box.nowrap = parentStyle.whiteSpace == whiteSpaceNowrap
		}
		return box
	}

	tag := node.Tag

	ctx := styleContextFromComputed(node, parentStyle)

	// display:none elements never reach here (renderHTML drops them).
	if node.Styles != nil {
		if node.Styles["visibility"] == "hidden" {
			return nil
		}
	}

	// Only list items get markers; the device draws one for any <li>.
	if tag == "li" && node.Styles["display"] != "list-item" {
		ctx.listStyleType = listStyleNone
	}

	ctxIdx := ls.styles.intern(ctx)

	info, ok := tagByName[tag]
	kbcTag := info.kbcTag
	if !ok {
		// Unknown tag: guess block vs inline based on computed display.
		if ctx.display == displayBlock {
			kbcTag = tagDiv
		} else {
			kbcTag = tagSpan
		}
	}

	pre := parentPre || ctx.whiteSpace == whiteSpacePre

	box := &layoutBox{
		kbcTag:       kbcTag,
		styleCtxIdx:  ctxIdx,
		style:        ctx,
		preformatted: pre,
		inputType:    -1,
		formID:       -1,
		formMethod:   -1,
		olStart:      -1,
		selectIdx:    -1,
	}

	switch node.Styles["display"] {
	case "inline-block", "inline-flex", "inline-grid", "inline-table":
		box.inlineBlock = true
	}
	box.centerBlocks = node.Styles["textAlign"] == "-webkit-center"
	box.autoMarginL = node.Styles["marginLeft"] == "auto"
	box.autoMarginR = node.Styles["marginRight"] == "auto"

	// Block/inline classification from computed display.
	switch ctx.display {
	case displayBlock:
		box.isBlock = true
	case displayInline:
		box.isBlock = false
	default:
		box.isBlock = tagByName[tag].isBlock
	}

	switch tag {
	case "a":
		box.href = nodeAttr(node, "href")
		if box.href != "" {
			box.href = resolveURL(ls.pageURL, box.href)
		}
	case "img":
		box.imgSrc = nodeAttr(node, "src")
		if box.imgSrc != "" {
			box.imgSrc = resolveURL(ls.pageURL, box.imgSrc)
		}
		box.imgAlt = nodeAttr(node, "alt")
		if isSVGURL(box.imgSrc) {
			// The device can't decode SVG: show the alt text instead.
			return altTextBox(box.imgAlt, ctx)
		}
		w, werr := strconv.Atoi(nodeAttr(node, "width"))
		h, herr := strconv.Atoi(nodeAttr(node, "height"))
		if (werr == nil && w <= 0) || (herr == nil && h <= 0) {
			return nil // zero-size (spacer/tracking) image
		}
		if werr == nil {
			box.attrW = w
		}
		if herr == nil {
			box.attrH = h
		}
		// Skip spacer images
		if box.attrW > 0 && box.attrH > 0 && box.attrW <= spacerMaxDim && box.attrH <= spacerMaxDim {
			return nil
		}
		if box.imgAlt == "" && box.imgSrc == "" {
			return nil
		}
		ls.hasImages = true
	case "form":
		box.formAction = nodeAttr(node, "action")
		if box.formAction != "" {
			box.formAction = resolveURL(ls.pageURL, box.formAction)
		}
		box.formMethod = formMethodGET
		if strings.EqualFold(nodeAttr(node, "method"), "post") {
			box.formMethod = formMethodPOST
		}
		box.formID = ls.formCount
		ls.formCount++
	case "input":
		box.inputType = parseInputType(nodeAttr(node, "type"))
		box.inputName = nodeAttr(node, "name")
		box.inputValue = nodeAttr(node, "value")
		box.formID = findFormID(node, ls)
	case "select":
		box.inputName = nodeAttr(node, "name")
		box.selectIdx = 0
		box.formID = findFormID(node, ls)
	case "textarea":
		box.inputName = nodeAttr(node, "name")
		box.inputValue = extractAllText(node)
		box.cols, _ = strconv.Atoi(nodeAttr(node, "cols"))
		if box.cols <= 0 {
			box.cols = defaultTextareaCols
		}
		box.rows, _ = strconv.Atoi(nodeAttr(node, "rows"))
		if box.rows <= 0 {
			box.rows = defaultTextareaRows
		}
		box.formID = findFormID(node, ls)
	case "td", "th":
		box.colspan, _ = strconv.Atoi(nodeAttr(node, "colspan"))
	case "ol":
		if s := nodeAttr(node, "start"); s != "" {
			box.olStart, _ = strconv.Atoi(s)
		}
	case "br":
		box.isBlock = false
	case "button":
		box.kbcTag = tagInput
		box.inputType = inputTypeSubmit
		box.inputName = nodeAttr(node, "name")
		box.inputValue = collapseWhitespace(extractAllText(node))
		if box.inputValue == "" {
			box.inputValue = defaultSubmitLabel
		}
		box.formID = findFormID(node, ls)
		box.isBlock = false
	}

	// Build children (skip for replaced/leaf elements).
	isLeaf := tag == "img" || tag == "input" || tag == "br" ||
		tag == "hr" || tag == "textarea" || tag == "button"

	if !isLeaf && tag == "select" {
		box.children = buildSelectOptions(node, ls)
	} else if !isLeaf {
		for i := range node.Children {
			child := buildBoxTree(&node.Children[i], ls, pre, &ctx)
			if child != nil {
				box.children = append(box.children, child)
			}
		}
	}

	return box
}

func buildSelectOptions(node *domNode, ls *layoutState) []*layoutBox {
	var opts []*layoutBox
	idx := 0
	for i := range node.Children {
		child := &node.Children[i]
		if child.Tag != "option" {
			continue
		}
		ctx := styleContextFromComputed(child, nil)
		ctxIdx := ls.styles.intern(ctx)
		text := collapseWhitespace(extractAllText(child))
		value := nodeAttr(child, "value")
		if value == "" {
			value = text
		}
		selected := nodeAttr(child, "selected") != ""
		opt := &layoutBox{
			kbcTag:      tagOption,
			styleCtxIdx: ctxIdx,
			text:        text,
			inputValue:  value,
			selectIdx:   idx,
			selected:    selected,
			inputType:   -1,
			formID:      -1,
			formMethod:  -1,
			olStart:     -1,
		}
		opts = append(opts, opt)
		idx++
	}
	return opts
}

// styleContextFromComputed maps computed style values (already cascaded and
// resolved by renderHTML, e.g. "rgb(0, 0, 255)", "8px") to a styleContext.
func styleContextFromComputed(node *domNode, parent *styleContext) styleContext {
	ctx := newDefaultStyleContext()
	s := node.Styles
	if s == nil {
		if parent != nil {
			inheritFromParent(&ctx, parent)
		}
		return ctx
	}

	if v, ok := s["color"]; ok {
		if c, cok := parseColor(v); cok {
			ctx.color = c
			ctx.colorSet = true
		}
	}

	if v, ok := s["backgroundColor"]; ok {
		if c, cok := parseColor(v); cok {
			// Don't set transparent/white backgrounds that are just defaults
			if v != transparentRGBA {
				ctx.bgColor = c
				ctx.bgColorSet = true
			}
		}
	}

	if v, ok := s["fontSize"]; ok {
		ctx.fontID = fontSizeToFontID(v, ctx.fontID)
	}

	if v, ok := s["fontWeight"]; ok {
		weight, err := strconv.Atoi(v)
		if err == nil && weight >= fontWeightBoldThreshold {
			switch ctx.fontID {
			case fontBortItalic9:
				ctx.fontID = fontBortBoldItalic9
			case fontBort12:
				ctx.fontID = fontBortBold12
			default:
				ctx.fontID = fontBortBold9
			}
		}
	}

	if v, ok := s["fontStyle"]; ok {
		if v == "italic" || v == "oblique" {
			if ctx.fontID == fontBortBold9 {
				ctx.fontID = fontBortBoldItalic9
			} else {
				ctx.fontID = fontBortItalic9
			}
		}
	}

	if v, ok := s["display"]; ok {
		switch v {
		case "block", "flex", "grid", "table", "list-item",
			"table-row-group", "table-header-group", "table-footer-group",
			"table-row", "table-cell", "table-caption":
			ctx.display = displayBlock
		case "inline", "inline-block", "inline-flex", "inline-grid", "inline-table":
			ctx.display = displayInline
		case "none":
			ctx.display = displayNone
		}
	}

	if v, ok := s["textDecoration"]; ok {
		// e.g. "none", "underline", "underline line-through"
		vl := strings.ToLower(v)
		if strings.Contains(vl, "line-through") {
			ctx.textDecoration = textDecorationStrikethrough
		} else if strings.Contains(vl, "underline") {
			ctx.textDecoration = textDecorationUnderline
		} else if strings.HasPrefix(vl, "none") {
			ctx.textDecoration = textDecorationNone
		}
	}

	if v, ok := s["textTransform"]; ok {
		switch v {
		case "none":
			ctx.textTransform = textTransformNone
		case "capitalize":
			ctx.textTransform = textTransformCapitalize
		case "uppercase":
			ctx.textTransform = textTransformUppercase
		case "lowercase":
			ctx.textTransform = textTransformLowercase
		}
	}

	if v, ok := s["textAlign"]; ok {
		switch v {
		case "center", "-webkit-center":
			ctx.textAlign = textAlignCenter
		case "right":
			ctx.textAlign = textAlignRight
		case "left", "start":
			ctx.textAlign = textAlignLeft
		case "justify":
			ctx.textAlign = textAlignJustify
		}
	}

	if v, ok := s["whiteSpace"]; ok {
		switch v {
		case "normal":
			ctx.whiteSpace = whiteSpaceNormal
		case "pre", "pre-wrap", "pre-line":
			ctx.whiteSpace = whiteSpacePre
		case "nowrap":
			ctx.whiteSpace = whiteSpaceNowrap
		}
	}

	// Margins (computed as "0px", "8px", etc.)
	if v, ok := s["marginTop"]; ok {
		ctx.marginTop = parsePx(v)
	}
	if v, ok := s["marginRight"]; ok {
		ctx.marginRight = parsePx(v)
	}
	if v, ok := s["marginBottom"]; ok {
		ctx.marginBottom = parsePx(v)
	}
	if v, ok := s["marginLeft"]; ok {
		ctx.marginLeft = parsePx(v)
	}

	if v, ok := s["paddingTop"]; ok {
		ctx.paddingTop = parsePx(v)
	}
	if v, ok := s["paddingRight"]; ok {
		ctx.paddingRight = parsePx(v)
	}
	if v, ok := s["paddingBottom"]; ok {
		ctx.paddingBottom = parsePx(v)
	}
	if v, ok := s["paddingLeft"]; ok {
		ctx.paddingLeft = parsePx(v)
	}

	if v, ok := s["lineHeight"]; ok {
		if v != "normal" {
			ctx.lineHeight = parsePx(v)
		}
	}

	if v, ok := s["float"]; ok {
		switch v {
		case "none":
			ctx.float = floatNone
		case "left":
			ctx.float = floatLeft
		case "right":
			ctx.float = floatRight
		}
	}

	if v, ok := s["listStyleType"]; ok {
		switch v {
		case "disc":
			ctx.listStyleType = listStyleDisc
		case "circle":
			ctx.listStyleType = listStyleCircle
		case "square":
			ctx.listStyleType = listStyleSquare
		case "decimal", "decimal-leading-zero":
			ctx.listStyleType = listStyleDecimal
		case "lower-roman":
			ctx.listStyleType = listStyleLowerRoman
		case "upper-roman":
			ctx.listStyleType = listStyleUpperRoman
		case "lower-alpha", "lower-latin":
			ctx.listStyleType = listStyleLowerAlpha
		case "upper-alpha", "upper-latin":
			ctx.listStyleType = listStyleUpperAlpha
		case "lower-greek":
			ctx.listStyleType = listStyleLowerGreek
		case "none":
			ctx.listStyleType = listStyleNone
		}
	}

	if v, ok := s["width"]; ok {
		if px := parsePx(v); px > 0 {
			ctx.cssWidth = px
		}
	}
	if v, ok := s["height"]; ok {
		if px := parsePx(v); px > 0 {
			ctx.cssHeight = px
		}
	}

	parseBorders(&ctx, s)

	if parent != nil {
		inheritFromParent(&ctx, parent)
	}

	return ctx
}

func parseBorders(ctx *styleContext, s map[string]string) {
	if v, ok := s["borderTopWidth"]; ok {
		ctx.borderTopWidth = parsePx(v)
	}
	if v, ok := s["borderRightWidth"]; ok {
		ctx.borderRightWidth = parsePx(v)
	}
	if v, ok := s["borderBottomWidth"]; ok {
		ctx.borderBottomWidth = parsePx(v)
	}
	if v, ok := s["borderLeftWidth"]; ok {
		ctx.borderLeftWidth = parsePx(v)
	}

	if v, ok := s["borderTopStyle"]; ok {
		ctx.borderTopStyle = parseBorderStyleValue(v)
	}
	if v, ok := s["borderRightStyle"]; ok {
		ctx.borderRightStyle = parseBorderStyleValue(v)
	}
	if v, ok := s["borderBottomStyle"]; ok {
		ctx.borderBottomStyle = parseBorderStyleValue(v)
	}
	if v, ok := s["borderLeftStyle"]; ok {
		ctx.borderLeftStyle = parseBorderStyleValue(v)
	}

	if v, ok := s["borderTopColor"]; ok {
		if c, cok := parseColor(v); cok {
			ctx.borderTopColor = c
			ctx.borderColorSet[0] = true
		}
	}
	if v, ok := s["borderRightColor"]; ok {
		if c, cok := parseColor(v); cok {
			ctx.borderRightColor = c
			ctx.borderColorSet[1] = true
		}
	}
	if v, ok := s["borderBottomColor"]; ok {
		if c, cok := parseColor(v); cok {
			ctx.borderBottomColor = c
			ctx.borderColorSet[2] = true
		}
	}
	if v, ok := s["borderLeftColor"]; ok {
		if c, cok := parseColor(v); cok {
			ctx.borderLeftColor = c
			ctx.borderColorSet[3] = true
		}
	}
}

func nodeAttr(node *domNode, key string) string {
	if node.Attrs == nil {
		return ""
	}
	return node.Attrs[key]
}

func extractAllText(node *domNode) string {
	if node.Tag == textNodeTag {
		return node.Text
	}
	var sb strings.Builder
	for i := range node.Children {
		sb.WriteString(extractAllText(&node.Children[i]))
	}
	return sb.String()
}

// findFormID finds the form index for a form control. Since we don't have
// parent pointers in the styled DOM tree, we track form IDs during tree
// construction. Form controls that aren't inside a <form> get formID -1.
// The actual form assignment happens during buildBoxTree via the
// ls.formCount that gets incremented when we encounter a <form>.
func findFormID(node *domNode, ls *layoutState) int {
	// Since we process the tree depth-first, the most recently seen form is
	// ls.formCount - 1. If no forms have been seen, return -1.
	if ls.formCount > 0 {
		return ls.formCount - 1
	}
	return -1
}

func isSVGURL(u string) bool {
	if pu, err := url.Parse(u); err == nil {
		return strings.HasSuffix(strings.ToLower(pu.Path), ".svg")
	}
	return false
}

// altTextBox stands in for an image the device can't show.
func altTextBox(alt string, ctx styleContext) *layoutBox {
	alt = collapseWhitespace(alt)
	if alt == "" {
		return nil
	}
	font := defaultFontID
	if ctx.fontID >= 0 {
		font = ctx.fontID
	}
	return &layoutBox{
		kbcTag:     tagText,
		text:       alt,
		isText:     true,
		fontID:     font,
		inputType:  -1,
		formID:     -1,
		formMethod: -1,
		olStart:    -1,
		selectIdx:  -1,
	}
}

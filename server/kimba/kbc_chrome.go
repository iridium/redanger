package kimba

import (
	"log"
	"strconv"
	"strings"
)

func chromeHTMLToKBC(pageURL string) []byte {
	domTree, title, err := fetchPageViaChrome(pageURL)
	if err != nil {
		log.Printf("Kimba: Chrome fetch failed for %s: %v", pageURL, err)
		return nil
	}

	ls := &layoutState{pageURL: pageURL}

	// Intern the default (empty) style context as entry 1.
	defaultCtx := newDefaultStyleContext()
	ls.styles.intern(defaultCtx)

	ls.title = title

	// Build box tree from the Chrome DOM tree.
	bodyBox := chromeBuildBoxTree(domTree, ls, false, nil)
	if bodyBox == nil {
		log.Printf("Kimba: Chrome DOM produced empty box tree for %s", pageURL)
		return nil
	}

	// Extract body-level background/text colors from computed styles.
	var colors bodyColors
	if domTree.Styles != nil {
		if c, ok := parseColor(domTree.Styles["backgroundColor"]); ok {
			colors.bgColor = c
			colors.bgColorSet = true
		}
		if c, ok := parseColor(domTree.Styles["color"]); ok {
			colors.textColor = c
			colors.textColorSet = true
		}
	}

	return buildKBCOutput(ls, bodyBox, colors)
}

func chromeBuildBoxTree(node *chromeDOMNode, ls *layoutState, parentPre bool, parentStyle *styleContext) *layoutBox {
	if node == nil {
		return nil
	}

	// Text node
	if node.Tag == textNodeTag {
		text := node.Text
		if !parentPre {
			text = collapseWhitespace(text)
		}
		if text == "" {
			return nil
		}
		return &layoutBox{
			kbcTag:       tagText,
			text:         text,
			isText:       true,
			preformatted: parentPre,
			inputType:    -1,
			formID:       -1,
			formMethod:   -1,
			olStart:      -1,
			selectIdx:    -1,
		}
	}

	tag := node.Tag

	// Compute style context from Chrome's computed styles.
	ctx := chromeComputeStyleContext(node, parentStyle)

	// Chrome already filters display:none in the JS extraction, but
	// double-check visibility:hidden.
	if node.Styles != nil {
		if node.Styles["visibility"] == "hidden" {
			return nil
		}
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

	// Block/inline classification from computed display.
	switch ctx.display {
	case displayBlock:
		box.isBlock = true
	case displayInline:
		box.isBlock = false
	default:
		box.isBlock = tagByName[tag].isBlock
	}

	// Element-specific attribute extraction.
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
		if w, err := strconv.Atoi(nodeAttr(node, "width")); err == nil && w > 0 {
			box.width = w
		}
		if h, err := strconv.Atoi(nodeAttr(node, "height")); err == nil && h > 0 {
			box.height = h
		}
		// Skip spacer images
		if box.width > 0 && box.height > 0 && box.width <= spacerMaxDim && box.height <= spacerMaxDim {
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
		box.formID = chromeFindFormID(node, ls)
	case "select":
		box.inputName = nodeAttr(node, "name")
		box.selectIdx = 0
		box.formID = chromeFindFormID(node, ls)
	case "textarea":
		box.inputName = nodeAttr(node, "name")
		box.inputValue = chromeExtractAllText(node)
		box.cols, _ = strconv.Atoi(nodeAttr(node, "cols"))
		if box.cols <= 0 {
			box.cols = defaultTextareaCols
		}
		box.rows, _ = strconv.Atoi(nodeAttr(node, "rows"))
		if box.rows <= 0 {
			box.rows = defaultTextareaRows
		}
		box.formID = chromeFindFormID(node, ls)
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
		box.inputValue = collapseWhitespace(chromeExtractAllText(node))
		if box.inputValue == "" {
			box.inputValue = defaultSubmitLabel
		}
		box.formID = chromeFindFormID(node, ls)
		box.isBlock = false
	}

	// Build children (skip for replaced/leaf elements).
	isLeaf := tag == "img" || tag == "input" || tag == "br" ||
		tag == "hr" || tag == "textarea" || tag == "button"

	if !isLeaf && tag == "select" {
		// Build option children for <select>.
		box.children = chromeBuildSelectOptions(node, ls)
	} else if !isLeaf {
		for i := range node.Children {
			child := chromeBuildBoxTree(&node.Children[i], ls, pre, &ctx)
			if child != nil {
				box.children = append(box.children, child)
			}
		}
	}

	return box
}

func chromeBuildSelectOptions(node *chromeDOMNode, ls *layoutState) []*layoutBox {
	var opts []*layoutBox
	idx := 0
	for i := range node.Children {
		child := &node.Children[i]
		if child.Tag != "option" {
			continue
		}
		ctx := chromeComputeStyleContext(child, nil)
		ctxIdx := ls.styles.intern(ctx)
		text := collapseWhitespace(chromeExtractAllText(child))
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

// chromeComputeStyleContext maps Chrome's computed style values to a styleContext.
// Chrome gives us fully resolved, canonical values (e.g. "rgb(0, 0, 255)"),
// so we don't need CSS cascade, specificity, or inheritance logic. Chrome
// already did that. We just parse the final values.
func chromeComputeStyleContext(node *chromeDOMNode, parent *styleContext) styleContext {
	ctx := newDefaultStyleContext()
	s := node.Styles
	if s == nil {
		if parent != nil {
			inheritFromParent(&ctx, parent)
		}
		return ctx
	}

	// Color
	if v, ok := s["color"]; ok {
		if c, cok := parseColor(v); cok {
			ctx.color = c
			ctx.colorSet = true
		}
	}

	// Background color
	if v, ok := s["backgroundColor"]; ok {
		if c, cok := parseColor(v); cok {
			// Don't set transparent/white backgrounds that are just defaults
			if v != transparentRGBA {
				ctx.bgColor = c
				ctx.bgColorSet = true
			}
		}
	}

	// Font size to font ID
	if v, ok := s["fontSize"]; ok {
		ctx.fontID = fontSizeToFontID(v, ctx.fontID)
	}

	// Font weight
	if v, ok := s["fontWeight"]; ok {
		weight, err := strconv.Atoi(v)
		if err == nil && weight >= fontWeightBoldThreshold {
			// Bold
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

	// Font style
	if v, ok := s["fontStyle"]; ok {
		if v == "italic" || v == "oblique" {
			if ctx.fontID == fontBortBold9 {
				ctx.fontID = fontBortBoldItalic9
			} else {
				ctx.fontID = fontBortItalic9
			}
		}
	}

	// Display
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

	// Text decoration
	if v, ok := s["textDecoration"]; ok {
		// Chrome returns computed like "none solid rgb(0, 0, 0)" or "underline solid rgb(0, 0, 255)"
		vl := strings.ToLower(v)
		if strings.Contains(vl, "line-through") {
			ctx.textDecoration = textDecorationStrikethrough
		} else if strings.Contains(vl, "underline") {
			ctx.textDecoration = textDecorationUnderline
		} else if strings.HasPrefix(vl, "none") {
			ctx.textDecoration = textDecorationNone
		}
	}

	// Text transform
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

	// Text align
	if v, ok := s["textAlign"]; ok {
		switch v {
		case "center":
			ctx.textAlign = textAlignCenter
		case "right":
			ctx.textAlign = textAlignRight
		case "left", "start":
			ctx.textAlign = textAlignLeft
		case "justify":
			ctx.textAlign = textAlignJustify
		}
	}

	// White space
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

	// Margins (Chrome returns computed as "0px", "8px", etc.)
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

	// Padding
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

	// Line height
	if v, ok := s["lineHeight"]; ok {
		if v != "normal" {
			ctx.lineHeight = parsePx(v)
		}
	}

	// Float
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

	// List style type
	if v, ok := s["listStyleType"]; ok {
		switch v {
		case "disc":
			ctx.listStyleType = listStyleDisc
		case "circle":
			ctx.listStyleType = listStyleCircle
		case "square":
			ctx.listStyleType = listStyleSquare
		case "decimal":
			ctx.listStyleType = listStyleDecimal
		case "none":
			ctx.listStyleType = listStyleNone
		}
	}

	// Width/height from computed styles
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

	// Borders
	chromeParseBorder(&ctx, s)

	// Inherit inheritable properties from parent.
	if parent != nil {
		inheritFromParent(&ctx, parent)
	}

	return ctx
}

func chromeParseBorder(ctx *styleContext, s map[string]string) {
	// Border widths
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

	// Border styles
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

	// Border colors
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

func nodeAttr(node *chromeDOMNode, key string) string {
	if node.Attrs == nil {
		return ""
	}
	return node.Attrs[key]
}

func chromeExtractAllText(node *chromeDOMNode) string {
	if node.Tag == textNodeTag {
		return node.Text
	}
	var sb strings.Builder
	for i := range node.Children {
		sb.WriteString(chromeExtractAllText(&node.Children[i]))
	}
	return sb.String()
}

// chromeFindFormID finds the form index for a form control. Since we don't have
// parent pointers in the Chrome DOM tree, we track form IDs during tree
// construction. Form controls that aren't inside a <form> get formID -1.
// The actual form assignment happens during chromeBuildBoxTree via the
// ls.formCount that gets incremented when we encounter a <form>.
func chromeFindFormID(node *chromeDOMNode, ls *layoutState) int {
	// Since we process the tree depth-first, the most recently seen form is
	// ls.formCount - 1. If no forms have been seen, return -1.
	if ls.formCount > 0 {
		return ls.formCount - 1
	}
	return -1
}

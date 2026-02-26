package kimba

type bodyColors struct {
	bgColor      int32
	bgColorSet   bool
	textColor    int32
	textColorSet bool
}

func buildKBCOutput(ls *layoutState, bodyBox *layoutBox, colors bodyColors) []byte {
	layoutBoxTree(bodyBox, screenWidth)

	totalHeight := bodyBox.height
	if totalHeight < lineHeight {
		totalHeight = lineHeight
	}

	var b kbcBuilder
	b.writeHeader(ls.hasImages)
	b.writeStyleContextTableChunk(&ls.styles)

	var tree kbcBuilder

	// DOCUMENT tag
	tree.writeTagOpen(tagDocument, true, true)
	if ls.title != "" {
		tree.writeAttrString(attrDocumentTitle, ls.title)
	}
	tree.writeAttrString(attrDocumentURL, ls.pageURL)
	tree.writeAttrEnd()

	// BODY tag
	tree.writeTagOpen(tagBody, len(bodyBox.children) > 0, true)
	tree.writeAttrLayoutXYWH(0, 0, screenWidth, totalHeight)
	tree.writeAttrCSSContext(bodyBox.styleCtxIdx)
	if colors.bgColorSet {
		tree.writeAttrColor(attrBodyBgColor, colors.bgColor)
	}
	if colors.textColorSet {
		tree.writeAttrColor(attrBodyText, colors.textColor)
	}
	tree.writeAttrEnd()

	for _, child := range bodyBox.children {
		writeBoxTree(&tree, child)
	}

	if len(bodyBox.children) > 0 {
		tree.writeEndTag() // close BODY
	}
	tree.writeEndTag() // close DOCUMENT

	b.writeByte(chunkDOMTree)
	treeBytes := tree.buf.Bytes()
	b.writeInt32(int32(len(treeBytes)))
	b.writeBytes(treeBytes)
	b.writeByte(chunkEndChunk)

	return b.buf.Bytes()
}

func buildErrorKBC(pageURL, message string) []byte {
	var styles styleContextTable
	defaultCtx := newDefaultStyleContext()
	defaultCtxIdx := styles.intern(defaultCtx)

	var b kbcBuilder
	b.writeHeader(false)
	b.writeStyleContextTableChunk(&styles)

	var tree kbcBuilder
	tree.writeTagOpen(tagDocument, true, true)
	tree.writeAttrString(attrDocumentTitle, "Error")
	tree.writeAttrString(attrDocumentURL, pageURL)
	tree.writeAttrEnd()

	tree.writeTagOpen(tagBody, true, true)
	tree.writeAttrLayoutXYWH(0, 0, screenWidth, lineHeight)
	tree.writeAttrCSSContext(defaultCtxIdx)
	tree.writeAttrEnd()

	tree.writeTextNode(message, 0, 0, charsPerLine)

	tree.writeEndTag() // BODY
	tree.writeEndTag() // DOCUMENT

	b.writeByte(chunkDOMTree)
	treeBytes := tree.buf.Bytes()
	b.writeInt32(int32(len(treeBytes)))
	b.writeBytes(treeBytes)
	b.writeByte(chunkEndChunk)

	return b.buf.Bytes()
}

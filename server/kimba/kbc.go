package kimba

import "html"

type bodyColors struct {
	bgColor      int32
	bgColorSet   bool
	textColor    int32
	textColorSet bool
}

func buildKBCOutput(ls *layoutState, bodyBox *layoutBox, colors bodyColors) []byte {
	layoutBoxTree(bodyBox, screenWidth)
	relativizeBlock(bodyBox)

	totalHeight := max(bodyBox.height, lineHeightFor(defaultFontID))

	var b kbcBuilder
	b.writeHeader(ls.hasImages)
	b.writeStyleContextTableChunk(&ls.styles)

	var tree kbcBuilder

	tree.writeTagOpen(tagDocument, true, true)
	if ls.title != "" {
		tree.writeAttrString(attrDocumentTitle, ls.title)
	}
	tree.writeAttrString(attrDocumentURL, ls.pageURL)
	tree.writeAttrEnd()

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
	page := "<title>Error</title><p>" + html.EscapeString(message) + "</p>"
	return htmlToKBC(pageURL, []byte(page), "text/html; charset=utf-8")
}

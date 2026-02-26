package kimba

import "strings"

const (
	attrFormAction  = 24
	attrFormMethod  = 18 // byte
	attrInputType   = 18 // byte
	attrInputValue  = 24
	attrInputName   = 28
	attrInputFormID = 38 // UVL

	attrSelectName        = 24
	attrSelectSelectedIdx = 18 // UVL
	attrSelectFormID      = 34 // UVL

	attrTextareaName   = 28
	attrTextareaValue  = 32
	attrTextareaCols   = 18 // UVL
	attrTextareaRows   = 22 // UVL
	attrTextareaFormID = 30 // UVL

	attrOptionText     = 20
	attrOptionValue    = 24
	attrOptionIndex    = 18 // UVL
	attrOptionSelected = 5  // boolean

	attrOlStart = 22 // UVL
)

func writeBoxTree(b *kbcBuilder, box *layoutBox) {
	if box.isText {
		writeTextBoxNode(b, box)
		return
	}

	if box.kbcTag == tagOption {
		writeOptionNode(b, box)
		return
	}

	hasChildren := len(box.children) > 0

	b.writeTagOpen(box.kbcTag, hasChildren, true)
	b.writeAttrLayoutXYWH(box.x, box.y, box.width, box.height)
	b.writeAttrCSSContext(box.styleCtxIdx)

	// Element-specific attributes
	switch box.kbcTag {
	case tagA:
		if box.href != "" {
			b.writeAttrString(attrAnchorHREF, box.href)
		}
	case tagImg:
		if box.imgSrc != "" {
			b.writeAttrString(attrImgSrc, box.imgSrc)
		}
		if box.imgAlt != "" {
			b.writeAttrString(attrImgAlt, box.imgAlt)
		}
		// Note: image dimensions are communicated via LAYOUT_XYWH (attr 2).
		// KBC_ATTR_ID_HTMLIMAGEELEMENT_WIDTH (63) and HEIGHT (51) are defined
		// in KBC.java but NOT handled by the device's loadAttributeValue;
		// they fall through to KDNode which reads them as colors (id&3==3 =
		// 4 bytes), causing a byte desync since we write UVL.
	case tagForm:
		if box.formAction != "" {
			b.writeAttrString(attrFormAction, box.formAction)
		}
		if box.formMethod >= 0 {
			b.writeByte(attrFormMethod)
			b.writeByte(byte(box.formMethod))
		}
	case tagInput:
		if box.inputType >= 0 {
			b.writeByte(attrInputType)
			b.writeByte(byte(box.inputType))
		}
		if box.inputValue != "" {
			b.writeAttrString(attrInputValue, box.inputValue)
		}
		if box.inputName != "" {
			b.writeAttrString(attrInputName, box.inputName)
		}
		if box.formID >= 0 {
			b.writeByte(attrInputFormID)
			b.writeUVL(box.formID)
		}
	case tagSelect:
		if box.selectIdx >= 0 {
			b.writeByte(attrSelectSelectedIdx)
			b.writeUVL(box.selectIdx)
		}
		if box.inputName != "" {
			b.writeAttrString(attrSelectName, box.inputName)
		}
		if box.formID >= 0 {
			b.writeByte(attrSelectFormID)
			b.writeUVL(box.formID)
		}
	case tagTextarea:
		b.writeByte(attrTextareaCols)
		b.writeUVL(box.cols)
		b.writeByte(attrTextareaRows)
		b.writeUVL(box.rows)
		if box.inputName != "" {
			b.writeAttrString(attrTextareaName, box.inputName)
		}
		if box.inputValue != "" {
			b.writeAttrString(attrTextareaValue, box.inputValue)
		}
		if box.formID >= 0 {
			b.writeByte(attrTextareaFormID)
			b.writeUVL(box.formID)
		}
	case tagOL:
		if box.olStart >= 0 {
			b.writeByte(attrOlStart)
			b.writeUVL(box.olStart)
		}
	case tagBody:
		// Body colors are handled separately by the caller
	}

	b.writeAttrEnd()

	if hasChildren {
		for _, child := range box.children {
			writeBoxTree(b, child)
		}
		b.writeEndTag()
	}
}

func writeTextBoxNode(b *kbcBuilder, box *layoutBox) {
	if len(box.extents) == 0 {
		return
	}

	// TEXT tag: no children, no attributes
	b.writeTagOpen(tagText, false, false)

	// Number of extents
	b.writeUVL(len(box.extents))

	// Build full text and track offsets
	var fullText strings.Builder
	offsets := make([]int, len(box.extents))
	for i, ext := range box.extents {
		offsets[i] = fullText.Len()
		fullText.WriteString(ext.text)
	}

	// Write extent data: x(SVL), y(SVL), textOffset(UVL)
	for i, ext := range box.extents {
		b.writeSVL(ext.x)
		b.writeSVL(ext.y)
		b.writeUVL(offsets[i])
	}

	// Null-terminated text content
	b.writeNullString(fullText.String())
}

func writeOptionNode(b *kbcBuilder, box *layoutBox) {
	b.writeTagOpen(tagOption, false, true)
	b.writeByte(attrOptionIndex)
	b.writeUVL(box.selectIdx)
	if box.selected {
		b.writeByte(attrOptionSelected)
	}
	if box.text != "" {
		b.writeAttrString(attrOptionText, box.text)
	}
	if box.inputValue != "" {
		b.writeAttrString(attrOptionValue, box.inputValue)
	}
	b.writeAttrEnd()
}

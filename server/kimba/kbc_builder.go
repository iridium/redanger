package kimba

import (
	"bytes"
	"strings"
)

const (
	tagEndTag     = 0
	tagA          = 1
	tagB          = 3
	tagBR         = 6
	tagFont       = 8
	tagForm       = 9
	tagI          = 12
	tagImg        = 14
	tagInput      = 15
	tagLI         = 16
	tagOL         = 23
	tagOption     = 24
	tagP          = 25
	tagPre        = 27
	tagSelect     = 29
	tagSpan       = 30
	tagStrike     = 31
	tagTable      = 33
	tagTBody      = 34
	tagTD         = 35
	tagTextarea   = 36
	tagTH         = 37
	tagTitle      = 38
	tagTR         = 39
	tagTT         = 40
	tagU          = 41
	tagUL         = 42
	tagText       = 43
	tagBig        = 44
	tagBlockquote = 45
	tagBody       = 46
	tagDiv        = 47
	tagCenter     = 49
	tagH1         = 51
	tagH2         = 52
	tagH3         = 53
	tagH4         = 54
	tagH5         = 55
	tagH6         = 56
	tagHead       = 57
	tagHR         = 58
	tagHTML       = 59
	tagEm         = 79
	tagSmall      = 87
	tagStrong     = 88
	tagDocument   = 94
)

const (
	attrEndAttr       = 0
	attrLayoutXYWH    = 2
	attrDocumentTitle = 4
	attrDocumentURL   = 8
	attrCSSContext    = 14
	attrAnchorHREF    = 20
	attrImgSrc        = 20 // same ID, different element context
	attrImgAlt        = 24
)

const (
	attrBodyBgColor = 3 // type COLOR (id & 3 == 3)
	attrBodyText    = 7 // type COLOR
)

const (
	chunkEndChunk          = 0
	chunkStyleContextTable = 3
	chunkDOMTree           = 4
)

// CSS property IDs from KBC.java (propID & 3 determines wire type)
const (
	cssPropFont           = 2  // type 2 = number (UVL)
	cssPropColor          = 3  // type 3 = color (int32)
	cssPropBgColor        = 7  // type 3 = color (int32)
	cssPropTextDecoration = 9  // type 1 = byte
	cssPropTextTransform  = 13 // type 1 = byte
	cssPropMarginLeft     = 14 // type 2 = coord (SVL)
	cssPropMarginTop      = 18 // type 2 = coord (SVL)
	cssPropMarginRight    = 22 // type 2 = coord (SVL)
	cssPropMarginBottom   = 26 // type 2 = coord (SVL)
	cssPropPaddingLeft    = 34 // type 2 = dimension (UVL)
	cssPropDisplay        = 37 // type 1 = byte
	cssPropPaddingTop     = 38 // type 2 = dimension (UVL)
	cssPropListStyleType  = 41 // type 1 = byte
	cssPropPaddingRight   = 42 // type 2 = dimension (UVL)
	cssPropPaddingBottom  = 46 // type 2 = dimension (UVL)
	cssPropWhiteSpace     = 61 // type 1 = byte
	cssPropTextAlign      = 85 // type 1 = byte (0=center, 1=left, 2=right, 3=justify)
	cssPropLineHeight     = 78 // type 2 = number (UVL)
	cssPropFloat          = 57 // type 1 = byte (0=none, 1=left, 2=right)
	// Border style properties
	cssPropBorderTopStyle    = 25 // type 1 = byte
	cssPropBorderRightStyle  = 29 // type 1 = byte
	cssPropBorderBottomStyle = 33 // type 1 = byte
	cssPropBorderLeftStyle   = 21 // type 1 = byte
	// Border width properties
	cssPropBorderTopWidth    = 58 // type 2 = dimension (UVL)
	cssPropBorderRightWidth  = 62 // type 2 = dimension (UVL)
	cssPropBorderBottomWidth = 66 // type 2 = dimension (UVL)
	cssPropBorderLeftWidth   = 54 // type 2 = dimension (UVL)
	// Border color properties
	cssPropBorderTopColor    = 19 // type 3 = color (int32)
	cssPropBorderRightColor  = 23 // type 3 = color (int32)
	cssPropBorderBottomColor = 27 // type 3 = color (int32)
	cssPropBorderLeftColor   = 15 // type 3 = color (int32)
)

// Font IDs from KFontMgr.java sFontNames[]
const (
	fontBort7           = 0
	fontBort9           = 3 // default
	fontBortBold9       = 4
	fontBortItalic9     = 5
	fontBort12          = 6
	fontBortBold12      = 7
	fontBortBoldItalic9 = 11
)

const (
	screenWidth  = 240
	charWidth    = 8
	lineHeight   = 16
	charsPerLine = screenWidth / charWidth // 30
)

// Input type constants for KBC (from KDHTMLInputElementGenerated).
const (
	inputTypeButton   = 0
	inputTypeText     = 1
	inputTypePassword = 2
	inputTypeCheckbox = 3
	inputTypeRadio    = 4
	inputTypeSubmit   = 5
	inputTypeReset    = 6
	inputTypeHidden   = 7
	inputTypeImage    = 8
	inputTypeFile     = 9
)

const (
	formMethodGET  = 0
	formMethodPOST = 1
)

const (
	defaultTextareaCols = 20
	defaultTextareaRows = 3
)

const spacerMaxDim = 2

const (
	defaultSubmitLabel = "Submit"
	defaultResetLabel  = "Reset"
	defaultButtonLabel = "Button"
)

const textNodeTag = "#text"

// transparentRGBA is the Chrome computed style value for a fully transparent
// background, used as a sentinel to skip default backgrounds.
const transparentRGBA = "rgba(0, 0, 0, 0)"

const fontWeightBoldThreshold = 700

const (
	defaultImageHeight   = 60 // fallback height when image dimensions unknown
	hrHeight             = 4
	checkboxSize         = 12
	buttonPaddingH       = 16 // horizontal padding for submit/reset/button
	buttonPaddingV       = 4  // vertical padding added to lineHeight
	defaultInputSize     = 20 // character width for text/password inputs
	selectMinOptionWidth = 8  // minimum character width for select options
	selectArrowSpace     = 20 // extra width for dropdown arrow
	textareaPadding      = 4  // padding for textarea elements
)

type tagInfo struct {
	name    string
	kbcTag  int
	isBlock bool
}

var tagRegistry = []tagInfo{
	{"a", tagA, false},
	{"b", tagB, false},
	{"br", tagBR, true},
	{"font", tagFont, false},
	{"form", tagForm, true},
	{"i", tagI, false},
	{"img", tagImg, false},
	{"input", tagInput, false},
	{"li", tagLI, true},
	{"ol", tagOL, true},
	{"option", tagOption, false},
	{"p", tagP, true},
	{"pre", tagPre, true},
	{"select", tagSelect, false},
	{"span", tagSpan, false},
	{"strike", tagStrike, false},
	{"s", tagStrike, false},
	{"table", tagTable, true},
	{"tbody", tagTBody, true},
	{"td", tagTD, true},
	{"textarea", tagTextarea, false},
	{"th", tagTH, true},
	{"title", tagTitle, false},
	{"tr", tagTR, true},
	{"tt", tagTT, false},
	{"u", tagU, false},
	{"ul", tagUL, true},
	{"big", tagBig, false},
	{"blockquote", tagBlockquote, true},
	{"body", tagBody, true},
	{"div", tagDiv, true},
	{"center", tagCenter, true},
	{"h1", tagH1, true},
	{"h2", tagH2, true},
	{"h3", tagH3, true},
	{"h4", tagH4, true},
	{"h5", tagH5, true},
	{"h6", tagH6, true},
	{"head", tagHead, true},
	{"hr", tagHR, true},
	{"html", tagHTML, true},
	{"em", tagEm, false},
	{"small", tagSmall, false},
	{"strong", tagStrong, false},
	// HTML5 semantic elements mapped to div
	{"nav", tagDiv, true},
	{"main", tagDiv, true},
	{"footer", tagDiv, true},
	{"header", tagDiv, true},
	{"section", tagDiv, true},
	{"article", tagDiv, true},
	{"aside", tagDiv, true},
	{"figure", tagDiv, true},
	{"figcaption", tagDiv, true},
	{"details", tagDiv, true},
	{"summary", tagDiv, true},
	{"fieldset", tagDiv, true},
	{"legend", tagSpan, true},
	{"dl", tagDiv, true},
	{"dt", tagDiv, true},
	{"dd", tagDiv, true},
	{"address", tagDiv, true},
	{"caption", tagDiv, true},
	{"thead", tagTBody, true},
	{"tfoot", tagTBody, true},
	// Additional inline/monospace mappings
	{"button", tagInput, false},
	{"label", tagSpan, false},
	{"abbr", tagSpan, false},
	{"code", tagTT, false},
	{"kbd", tagTT, false},
	{"samp", tagTT, false},
	{"var", tagI, false},
	{"cite", tagI, false},
	{"dfn", tagI, false},
	{"mark", tagSpan, false},
	{"time", tagSpan, false},
	{"q", tagSpan, false},
	{"sub", tagSpan, false},
	{"sup", tagSpan, false},
	{"ins", tagU, false},
	{"del", tagStrike, false},
}

var tagByName map[string]tagInfo

func init() {
	tagByName = make(map[string]tagInfo, len(tagRegistry))
	for _, t := range tagRegistry {
		tagByName[t.name] = t
	}
}

type kbcBuilder struct {
	buf bytes.Buffer
}

func (b *kbcBuilder) writeByte(v byte) {
	b.buf.WriteByte(v)
}

func (b *kbcBuilder) writeBytes(v []byte) {
	b.buf.Write(v)
}

func (b *kbcBuilder) writeInt32(v int32) {
	b.writeByte(byte(v >> 24))
	b.writeByte(byte(v >> 16))
	b.writeByte(byte(v >> 8))
	b.writeByte(byte(v))
}

func (b *kbcBuilder) writeUVL(value int) {
	if value < 0 {
		value = 0
	}
	if value < 128 {
		b.buf.WriteByte(byte(value))
		return
	}
	// Collect 7-bit groups, then write MSB first with continuation bits
	var parts [5]byte
	n := 0
	v := value
	for {
		parts[n] = byte(v & 0x7F)
		n++
		v >>= 7
		if v == 0 {
			break
		}
	}
	// Write in reverse (MSB first), setting continuation bit on all but last
	for i := n - 1; i >= 0; i-- {
		b2 := parts[i]
		if i > 0 {
			b2 |= 0x80
		}
		b.buf.WriteByte(b2)
	}
}

func (b *kbcBuilder) writeSVL(value int) {
	var uvl int
	if value < 0 {
		uvl = (-value << 1) | 1
	} else {
		uvl = value << 1
	}
	b.writeUVL(uvl)
}

func (b *kbcBuilder) writeNullString(s string) {
	b.buf.WriteString(s)
	b.buf.WriteByte(0)
}

func (b *kbcBuilder) writeHeader(hasImages bool) {
	b.writeByte(1) // major version
	b.writeByte(6) // minor version
	b.writeByte(0) // flags high
	if hasImages {
		b.writeByte(2) // KBC_HEADER_FLAGS_HAS_IMAGES
	} else {
		b.writeByte(0) // flags low
	}
}

func (b *kbcBuilder) writeTagOpen(tagID int, hasChildren, hasAttributes bool) {
	var tagByte byte
	if !hasChildren {
		tagByte |= 0x80 // NO_CHILDREN
	}
	if hasAttributes {
		tagByte |= 0x40 // HAS_ATTRIBUTES
	}
	if tagID < 63 {
		tagByte |= byte(tagID)
		b.writeByte(tagByte)
	} else {
		tagByte |= 63 // extended tag marker
		b.writeByte(tagByte)
		b.writeByte(byte(tagID))
	}
}

func (b *kbcBuilder) writeEndTag() {
	b.writeByte(tagEndTag)
}

func (b *kbcBuilder) writeAttrEnd() {
	b.writeByte(attrEndAttr)
}

func (b *kbcBuilder) writeAttrLayoutXYWH(x, y, w, h int) {
	b.writeByte(attrLayoutXYWH)
	b.writeSVL(x)
	b.writeSVL(y)
	b.writeUVL(w)
	b.writeUVL(h)
}

func (b *kbcBuilder) writeAttrCSSContext(index int) {
	b.writeByte(attrCSSContext)
	b.writeUVL(index)
}

func (b *kbcBuilder) writeAttrString(attrID int, value string) {
	b.writeByte(byte(attrID))
	b.writeNullString(value)
}

func (b *kbcBuilder) writeAttrColor(attrID int, color int32) {
	b.writeByte(byte(attrID))
	b.writeInt32(color)
}

func (b *kbcBuilder) writeTextNode(text string, x, y, wrapChars int) {
	if wrapChars <= 0 {
		wrapChars = charsPerLine
	}
	lines := wrapText(text, wrapChars)
	if len(lines) == 0 {
		return
	}

	// TEXT tag: no children, no attributes
	b.writeTagOpen(tagText, false, false)

	// Number of extents (one per line)
	b.writeUVL(len(lines))

	// Build the full text string and track offsets
	var fullText strings.Builder
	offsets := make([]int, len(lines))
	for i, line := range lines {
		offsets[i] = fullText.Len()
		fullText.WriteString(line)
	}

	// Write extent data: x(SVL), y(SVL), textOffset(UVL)
	for i := range lines {
		b.writeSVL(x)
		b.writeSVL(y + i*lineHeight)
		b.writeUVL(offsets[i])
	}

	// Null-terminated text content
	b.writeNullString(fullText.String())
}

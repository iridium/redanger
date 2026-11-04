package kimba

import (
	"net/url"
	"strings"
	"unicode"
)

func parseInputType(t string) int {
	switch strings.ToLower(t) {
	case "button":
		return inputTypeButton
	case "", "text":
		return inputTypeText
	case "password":
		return inputTypePassword
	case "checkbox":
		return inputTypeCheckbox
	case "radio":
		return inputTypeRadio
	case "submit":
		return inputTypeSubmit
	case "reset":
		return inputTypeReset
	case "hidden":
		return inputTypeHidden
	case "image":
		return inputTypeImage
	case "file":
		return inputTypeFile
	}
	return inputTypeText
}

func collapseWhitespace(s string) string {
	var b strings.Builder
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace && b.Len() > 0 {
				b.WriteByte(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(r)
			inSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// collapseSpaces collapses runs of HTML whitespace to a single space without
// trimming. (Non-breaking spaces are kept.)
func collapseSpaces(s string) string {
	var b strings.Builder
	inSpace := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ', '\t', '\n', '\r', '\f':
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
		default:
			b.WriteByte(c)
			inSpace = false
		}
	}
	return b.String()
}

func resolveURL(base, ref string) string {
	if base == "" {
		return ref
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return ref
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return baseURL.ResolveReference(refURL).String()
}

// stripInvisible removes zero-width and formatting characters that browsers
// don't draw but the device would show as missing-glyph boxes.
func stripInvisible(s string) string {
	if strings.IndexFunc(s, isInvisible) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isInvisible(r) {
			return -1
		}
		return r
	}, s)
}

func isInvisible(r rune) bool {
	switch {
	case r == 0xAD, r == 0x34F, r == 0x61C, r == 0xFEFF:
		return true
	case r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2060 && r <= 0x206F:
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
		return true
	}
	return false
}

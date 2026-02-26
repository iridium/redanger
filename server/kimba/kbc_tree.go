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

func wrapText(text string, maxChars int) []string {
	if text == "" {
		return nil
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var line strings.Builder
	for _, word := range words {
		if line.Len() == 0 {
			line.WriteString(word)
		} else if line.Len()+1+len(word) <= maxChars {
			line.WriteByte(' ')
			line.WriteString(word)
		} else {
			lines = append(lines, line.String())
			line.Reset()
			line.WriteString(word)
		}
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return lines
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

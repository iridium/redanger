package kimba

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// domNode is an element or text node with its computed styles: the input to
// the KBC box builder.
type domNode struct {
	Tag      string            // lowercase tag name, or "#text"
	Text     string            // text content (for #text nodes)
	Styles   map[string]string // computed CSS values
	Attrs    map[string]string
	Children []domNode
}

const (
	stylesheetBudget    = 6 * time.Second // total time spent fetching CSS for a page
	maxStylesheets      = 24
	maxStylesheetBytes  = 2 << 20
	stylesheetCacheTTL  = 10 * time.Minute
	stylesheetCacheSize = 128
)

// Elements that are never rendered. (<noscript> isn't here: scripts never
// run, so its fallback content is what the page wants shown.)
var skippedTags = map[string]bool{
	"script": true, "style": true, "link": true, "meta": true, "head": true,
	"svg": true, "math": true, "iframe": true, "video": true, "audio": true,
	"canvas": true, "template": true, "object": true, "embed": true,
}

// keptAttrs are the attributes the box builder looks at.
var keptAttrs = []string{
	"href", "src", "alt", "action", "method",
	"type", "name", "value", "selected", "checked",
	"start", "cols", "rows", "multiple", "size",
	"maxlength", "placeholder", "id", "class",
	"width", "height", "colspan",
}

// booleanAttrs get their own name as value when present, so a bare
// <option selected> reads as set.
var booleanAttrs = map[string]bool{"selected": true, "checked": true, "multiple": true}

// styledPage is a parsed and styled HTML document.
type styledPage struct {
	body    *domNode
	title   string
	baseURL string
}

// renderHTML parses an HTML document, applies its stylesheets and returns the
// styled body. pageURL is the URL the document was served from (after
// redirects); contentType is the response Content-Type, used for charset
// detection.
func renderHTML(ctx context.Context, pageURL string, body []byte, contentType string) (*styledPage, error) {
	r, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		r = bytes.NewReader(body)
	}
	doc, err := html.ParseWithOptions(r, html.ParseOptionEnableScripting(false))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	page := &styledPage{baseURL: pageURL}
	var htmlEl, bodyEl *html.Node
	walkElements(doc, func(n *html.Node) bool {
		switch n.Data {
		case "html":
			if htmlEl == nil {
				htmlEl = n
			}
		case "body", "frameset":
			if bodyEl == nil {
				bodyEl = n
			}
		case "title":
			if page.title == "" && !inForeignContent(n) {
				page.title = collapseWhitespace(textContent(n))
			}
		case "base":
			if href := attrValue(n, "href"); href != "" && page.baseURL == pageURL {
				page.baseURL = resolveURL(pageURL, href)
			}
		case "svg", "math":
			return false
		}
		return true
	})
	if htmlEl == nil || bodyEl == nil {
		return nil, fmt.Errorf("no body element")
	}

	sheets := collectStylesheets(ctx, doc, page.baseURL)
	res := newStyleResolver(sheets, isQuirksMode(doc))

	htmlStyle := res.compute(htmlEl, initialStyle())
	bodyParent := htmlStyle
	// Style any wrappers between <html> and <body> (there normally aren't).
	for p := bodyEl.Parent; p != nil && p != htmlEl; p = p.Parent {
		bodyParent = res.compute(p, bodyParent)
	}
	bodyStyle := res.compute(bodyEl, bodyParent)
	page.body = buildDOMNode(bodyEl, bodyStyle, res)

	// A background set on <html> paints the whole canvas; since the device
	// only knows the body's, carry it over.
	if page.body.Styles["backgroundColor"] == transparentRGBA {
		page.body.Styles["backgroundColor"] = htmlStyle.props["backgroundColor"]
	}
	return page, nil
}

func buildDOMNode(n *html.Node, cs *computedStyle, res *styleResolver) *domNode {
	node := &domNode{Tag: n.Data, Styles: cs.props}
	for _, a := range n.Attr {
		if a.Namespace != "" {
			continue
		}
		for _, k := range keptAttrs {
			if a.Key == k {
				if node.Attrs == nil {
					node.Attrs = map[string]string{}
				}
				v := a.Val
				if booleanAttrs[k] && v == "" {
					v = k
				}
				node.Attrs[k] = v
				break
			}
		}
	}
	before := generatedNode(n, "before", cs, res)
	if before != nil {
		node.Children = append(node.Children, *before)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			// Whitespace-only text still separates inline content.
			node.Children = append(node.Children, domNode{Tag: textNodeTag, Text: c.Data})
		case html.ElementNode:
			if skippedTags[c.Data] {
				continue
			}
			ccs := res.compute(c, cs)
			if ccs.props["display"] == "none" {
				continue
			}
			node.Children = append(node.Children, *buildDOMNode(c, ccs, res))
		}
	}
	if after := generatedNode(n, "after", cs, res); after != nil {
		node.Children = append(node.Children, *after)
	}
	return node
}

// generatedNode builds n's ::before or ::after box: a span holding the
// generated text.
func generatedNode(n *html.Node, pseudo string, cs *computedStyle, res *styleResolver) *domNode {
	switch n.Data {
	case "img", "input", "br", "hr", "select", "textarea", "option":
		return nil // replaced elements have no generated content
	}
	pcs, text := res.computePseudo(n, pseudo, cs)
	if pcs == nil {
		return nil
	}
	return &domNode{Tag: "span", Styles: pcs.props, Children: []domNode{{Tag: textNodeTag, Text: text}}}
}

// isQuirksMode reports whether a document renders in quirks mode: no
// doctype, or a legacy one (HTML 4 transitional without a system ID, ...).
func isQuirksMode(doc *html.Node) bool {
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.DoctypeNode {
			continue
		}
		if !strings.EqualFold(c.Data, "html") {
			return true
		}
		var public string
		hasSystem := false
		for _, a := range c.Attr {
			switch a.Key {
			case "public":
				public = strings.ToLower(a.Val)
			case "system":
				hasSystem = true
			}
		}
		switch {
		case strings.HasPrefix(public, "-//w3c//dtd html 3"), strings.HasPrefix(public, "-//w3c//dtd html 2"),
			strings.HasPrefix(public, "-//ietf//dtd html"), strings.HasPrefix(public, "-//netscape comm. corp.//dtd"),
			strings.HasPrefix(public, "-//microsoft//dtd internet explorer"):
			return true
		case !hasSystem && (strings.HasPrefix(public, "-//w3c//dtd html 4.01 transitional") ||
			strings.HasPrefix(public, "-//w3c//dtd html 4.01 frameset")):
			return true
		}
		return false
	}
	return true
}

// walkElements visits elements in document order; fn returns false to skip
// an element's children.
func walkElements(n *html.Node, fn func(*html.Node) bool) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			if !fn(c) {
				continue
			}
		}
		walkElements(c, fn)
	}
}

func inForeignContent(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Data == "svg" || p.Data == "math" {
			return true
		}
	}
	return false
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// collectStylesheets gathers <style> and <link rel=stylesheet> sheets in
// document order. Linked sheets are fetched in parallel.
func collectStylesheets(ctx context.Context, doc *html.Node, baseURL string) []*stylesheet {
	type slot struct {
		inline string
		url    string
		sheet  *stylesheet
	}
	var slots []*slot
	walkElements(doc, func(n *html.Node) bool {
		switch n.Data {
		case "style":
			if mediaMatches(attrValue(n, "media")) {
				slots = append(slots, &slot{inline: textContent(n)})
			}
		case "link":
			rel := " " + strings.ToLower(attrValue(n, "rel")) + " "
			href := attrValue(n, "href")
			if strings.Contains(rel, " stylesheet ") && !strings.Contains(rel, " alternate ") &&
				href != "" && !hasAttr(n, "disabled") && mediaMatches(attrValue(n, "media")) {
				slots = append(slots, &slot{url: resolveURL(baseURL, href)})
			}
		case "svg", "math", "template":
			return false
		}
		return true
	})

	ctx, cancel := context.WithTimeout(ctx, stylesheetBudget)
	defer cancel()
	var wg sync.WaitGroup
	fetched := 0
	for _, s := range slots {
		if s.url != "" {
			if fetched >= maxStylesheets {
				continue
			}
			fetched++
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.sheet = loadStylesheet(ctx, s.url, baseURL)
			}()
			continue
		}
		s.sheet = parseStylesheet(s.inline, newImporter(ctx, baseURL, baseURL))
	}
	wg.Wait()

	var sheets []*stylesheet
	for _, s := range slots {
		if s.sheet != nil {
			sheets = append(sheets, s.sheet)
		}
	}
	return sheets
}

type cachedSheet struct {
	sheet   *stylesheet
	fetched time.Time
}

var (
	sheetCacheMu sync.Mutex
	sheetCache   = map[string]cachedSheet{}
)

// loadStylesheet fetches and parses a stylesheet (with its @imports),
// caching the result. Parsed sheets are immutable and shared.
func loadStylesheet(ctx context.Context, url, referer string) *stylesheet {
	sheetCacheMu.Lock()
	if c, ok := sheetCache[url]; ok && time.Since(c.fetched) < stylesheetCacheTTL {
		sheetCacheMu.Unlock()
		return c.sheet
	}
	sheetCacheMu.Unlock()

	src, ok := fetchText(ctx, url, referer)
	if !ok {
		return nil
	}
	sheet := parseStylesheet(src, newImporter(ctx, url, referer))

	sheetCacheMu.Lock()
	defer sheetCacheMu.Unlock()
	if len(sheetCache) >= stylesheetCacheSize {
		var oldestURL string
		var oldest time.Time
		for u, c := range sheetCache {
			if oldestURL == "" || c.fetched.Before(oldest) {
				oldestURL, oldest = u, c.fetched
			}
		}
		delete(sheetCache, oldestURL)
	}
	sheetCache[url] = cachedSheet{sheet: sheet, fetched: time.Now()}
	return sheet
}

func newImporter(ctx context.Context, sheetURL, referer string) cssImporter {
	return func(ref string) string {
		src, _ := fetchText(ctx, resolveURL(sheetURL, ref), referer)
		return src
	}
}

func fetchText(ctx context.Context, url, referer string) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("Kimba: stylesheet %s: %v", url, err)
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("Kimba: stylesheet %s: HTTP %d", url, resp.StatusCode)
		return "", false
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxStylesheetBytes))
	if err != nil {
		return "", false
	}
	return string(b), true
}

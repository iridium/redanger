package kimba

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// render serves page (and any extra files) from a test server and renders it.
func render(t *testing.T, page string, files map[string]string) *styledPage {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if body, ok := files[r.URL.Path]; ok {
			w.Header().Set("Content-Type", "text/css")
			w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p, err := renderHTML(context.Background(), srv.URL+"/page.html", []byte(page), "text/html; charset=utf-8")
	if err != nil {
		t.Fatalf("renderHTML: %v", err)
	}
	return p
}

// find returns the first element with the given id (or tag, if no element has
// that id) in document order.
func find(n *domNode, key string) *domNode {
	var byTag *domNode
	var walk func(*domNode) *domNode
	walk = func(n *domNode) *domNode {
		if n.Attrs["id"] == key {
			return n
		}
		if byTag == nil && n.Tag == key {
			byTag = n
		}
		for i := range n.Children {
			if f := walk(&n.Children[i]); f != nil {
				return f
			}
		}
		return nil
	}
	if f := walk(n); f != nil {
		return f
	}
	return byTag
}

func expectStyles(t *testing.T, p *styledPage, key string, want map[string]string) {
	t.Helper()
	n := find(p.body, key)
	if n == nil {
		t.Fatalf("%s: not found", key)
	}
	for prop, v := range want {
		if got := n.Styles[prop]; got != v {
			t.Errorf("%s %s = %q, want %q", key, prop, got, v)
		}
	}
}

func TestUADefaults(t *testing.T) {
	p := render(t, `<title> Hello
		World </title><h1>Head</h1><p>Para <a href="/x">link</a> <b>bold</b></p>
		<ul><li>one<ul><li id=nested>two</ul></ul><table><tr><td>cell</table>`, nil)
	if p.title != "Hello World" {
		t.Errorf("title = %q", p.title)
	}
	expectStyles(t, p, "body", map[string]string{"display": "block", "marginTop": "2px", "fontSize": "16px"})
	expectStyles(t, p, "h1", map[string]string{"fontSize": "32px", "fontWeight": "700", "marginTop": "21px"})
	expectStyles(t, p, "p", map[string]string{"marginTop": "16px", "marginBottom": "16px"})
	expectStyles(t, p, "a", map[string]string{"color": "rgb(0, 0, 238)", "textDecoration": "underline", "display": "inline"})
	expectStyles(t, p, "b", map[string]string{"fontWeight": "700"})
	expectStyles(t, p, "li", map[string]string{"display": "list-item", "listStyleType": "disc"})
	expectStyles(t, p, "nested", map[string]string{"listStyleType": "circle"})
	expectStyles(t, p, "td", map[string]string{"display": "table-cell", "paddingLeft": "1px"})
}

func TestCascadePriority(t *testing.T) {
	p := render(t, `<style>
		p { color: red }
		.c { color: green }
		#i { color: blue }
		p.later { color: green }
		p.later { color: purple }
		#fallback { font-size: 10px; font-size: max(1em, 20px) }
		.imp { color: red !important }
		div p { margin-left: 4px }
	</style>
	<p class=c id=i>id beats class</p>
	<p class=later id=order>later wins at equal specificity</p>
	<p class=imp id=imp style="color: blue">important beats inline</p>
	<p id=inline class=c style="color: #ff0">inline beats id</p>
	<div><p id=desc>descendant</p></div><p id=fallback>later declaration in a rule wins</p>`, nil)
	expectStyles(t, p, "fallback", map[string]string{"fontSize": "20px"})
	expectStyles(t, p, "i", map[string]string{"color": "rgb(0, 0, 255)"})
	expectStyles(t, p, "order", map[string]string{"color": "rgb(128, 0, 128)"})
	expectStyles(t, p, "imp", map[string]string{"color": "rgb(255, 0, 0)"})
	expectStyles(t, p, "inline", map[string]string{"color": "rgb(255, 255, 0)"})
	expectStyles(t, p, "desc", map[string]string{"marginLeft": "4px"})
}

func TestInheritanceAndUnits(t *testing.T) {
	p := render(t, `<style>
		html { font-size: 10px }
		#outer { font-size: 2em; color: #123456; margin: 1em 2em; padding: 5%; border: 2px solid; width: 50% }
		#inner { font-size: 1.5rem; margin-left: calc(10px + 1em); line-height: 1.5 }
		#wide { width: 2000px }
	</style>
	<div id=outer><span id=inner>x</span><div id=wide>w</div></div>`, nil)
	expectStyles(t, p, "outer", map[string]string{
		"fontSize": "20px", "marginTop": "20px", "marginLeft": "40px", "paddingLeft": "12px",
		"borderTopWidth": "2px", "borderTopStyle": "solid", "borderTopColor": "rgb(18, 52, 86)", "width": "118px",
	})
	expectStyles(t, p, "inner", map[string]string{
		"fontSize": "15px", "color": "rgb(18, 52, 86)", "marginLeft": "25px", "lineHeight": "23px",
		// margin/border don't inherit
		"marginTop": "0px", "borderTopWidth": "0px",
	})
	expectStyles(t, p, "wide", map[string]string{"width": "auto"})
}

func TestCustomProperties(t *testing.T) {
	p := render(t, `<style>
		:root { --fg: #00ff00; --gap: 3px }
		#a { color: var(--fg); margin: var(--gap) 0 }
		#b { color: var(--missing, rgb(1 2 3 / 50%)) }
		#c { color: blue; color: var(--missing) }
		#d { --fg: red }
		#d p { color: var(--fg) }
	</style>
	<p id=a>a</p><p id=b>b</p><p id=c>c</p><div id=d><p id=dp>d</p></div>`, nil)
	expectStyles(t, p, "a", map[string]string{"color": "rgb(0, 255, 0)", "marginTop": "3px", "marginLeft": "0px"})
	expectStyles(t, p, "b", map[string]string{"color": "rgb(1, 2, 3)"})
	// An invalid var() falls back to the next-best declaration.
	expectStyles(t, p, "c", map[string]string{"color": "rgb(0, 0, 255)"})
	expectStyles(t, p, "dp", map[string]string{"color": "rgb(255, 0, 0)"})
}

func TestLinkedStylesheetsAndMedia(t *testing.T) {
	p := render(t, `<link rel=stylesheet href="/main.css">
		<link rel=stylesheet href="/print.css" media=print>
		<link rel="alternate stylesheet" href="/alt.css">
		<p id=a>a</p><p id=b>b</p><p id=c>c</p>`, map[string]string{
		"/main.css": `@import url("imported.css");
			@media (max-width: 600px) { #a { color: red } }
			@media screen and (min-width: 800px) { #b { color: red } }
			@font-face { font-family: x; src: url(x.woff) }
			#b { color: green }`,
		"/imported.css": `#c { color: blue }`,
		"/print.css":    `#a, #b, #c { color: yellow !important }`,
		"/alt.css":      `#a, #b, #c { color: yellow !important }`,
	})
	expectStyles(t, p, "a", map[string]string{"color": "rgb(255, 0, 0)"})
	expectStyles(t, p, "b", map[string]string{"color": "rgb(0, 128, 0)"})
	expectStyles(t, p, "c", map[string]string{"color": "rgb(0, 0, 255)"})
}

func TestHiddenAndScripting(t *testing.T) {
	p := render(t, `<p id=keep>keep</p><div style="display:none"><p id=gone>gone</p></div>
		<p hidden id=hid>hidden attr</p><script>document.write("x")</script>
		<noscript><p id=ns>no js</p></noscript><input type=hidden name=h>`, nil)
	for _, id := range []string{"gone", "hid"} {
		if find(p.body, id) != nil {
			t.Errorf("%s should not be rendered", id)
		}
	}
	if find(p.body, "keep") == nil || find(p.body, "ns") == nil {
		t.Error("visible content and <noscript> should be rendered")
	}
	if find(p.body, "script") != nil || find(p.body, "input") != nil {
		t.Error("script and hidden input should be dropped")
	}
}

func TestPresentationalHints(t *testing.T) {
	p := render(t, `<html style="background: #eee url(x.png) no-repeat"><body text="333333">
		<table bgcolor=ff0000 width=50%><tr><td align=center id=td>x</td></tr></table>
		<font size=+2 color=navy id=f>big</font><font size=1 id=small>small</font>
		<style>#td { text-align: right }</style><p align=center id=p>p</p>`, nil)
	expectStyles(t, p, "body", map[string]string{"color": "rgb(51, 51, 51)", "backgroundColor": "rgb(238, 238, 238)"})
	expectStyles(t, p, "table", map[string]string{"backgroundColor": "rgb(255, 0, 0)", "width": "118px"})
	// Author CSS beats presentational hints.
	expectStyles(t, p, "td", map[string]string{"textAlign": "right"})
	expectStyles(t, p, "p", map[string]string{"textAlign": "-webkit-center"})
	expectStyles(t, p, "f", map[string]string{"fontSize": "24px", "color": "rgb(0, 0, 128)"})
	expectStyles(t, p, "small", map[string]string{"fontSize": "10px"})
}

func TestCharsetAndAttributes(t *testing.T) {
	// "café" in windows-1252, declared only in a meta tag.
	page := []byte("<meta charset=windows-1252><p id=p>caf\xe9</p><select><option>a<option selected value=2>b</select>")
	p, err := renderHTML(context.Background(), "http://example.test/", page, "text/html")
	if err != nil {
		t.Fatal(err)
	}
	if got := find(p.body, "p").Children[0].Text; got != "café" {
		t.Errorf("text = %q", got)
	}
	opts := find(p.body, "select").Children
	if len(opts) != 2 || opts[1].Attrs["selected"] == "" || opts[1].Attrs["value"] != "2" {
		t.Errorf("options = %+v", opts)
	}
}

func TestHTMLToKBC(t *testing.T) {
	kbc := htmlToKBC("http://example.test/", []byte(`<title>T</title><style>p{color:red}</style>
		<h1>Hi</h1><p>Some <a href="/next">text</a></p><form action=/go><input name=q><input type=submit></form>`),
		"text/html")
	if len(kbc) == 0 {
		t.Fatal("no KBC output")
	}
	if !strings.Contains(string(kbc), "http://example.test/next") {
		t.Error("link href should be resolved against the page URL")
	}
}

func TestParseLength(t *testing.T) {
	ctx := lengthContext{fontSize: 10, rootFontSize: 16, percentBase: 200, hasPercent: true}
	cases := map[string]float64{
		"0": 0, "12px": 12, "1.5em": 15, "2rem": 32, "50%": 100, "12pt": 16, "1in": 96,
		"calc(100% - 2em)": 180, "calc(2 * (3px + 1px))": 8, "min(10px, 5em)": 10,
		"clamp(1px, 50%, 20px)": 20, "10vw": 24,
	}
	for in, want := range cases {
		got, ok := parseLength(in, ctx)
		if !ok || got != want {
			t.Errorf("parseLength(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"12", "auto", "calc(1px + 2)", "red", ""} {
		if _, ok := parseLength(bad, ctx); ok {
			t.Errorf("parseLength(%q) should fail", bad)
		}
	}
}

func TestColorValues(t *testing.T) {
	cases := map[string]string{
		"red": "rgb(255, 0, 0)", "#0f0": "rgb(0, 255, 0)", "#00f8": "rgb(0, 0, 255)",
		"rgb(10 20 30)": "rgb(10, 20, 30)", "rgb(100%, 0%, 50%)": "rgb(255, 0, 128)",
		"rgba(1, 2, 3, 0)": transparentRGBA, "transparent": transparentRGBA, "#11223300": transparentRGBA,
		"hsl(120deg 100% 50%)": "rgb(0, 255, 0)",
	}
	for in, want := range cases {
		if got, ok := cssColorValue(in, 0, false); !ok || got != want {
			t.Errorf("cssColorValue(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestMediaQueries(t *testing.T) {
	cases := map[string]bool{
		"": true, "screen": true, "print": false, "screen, print": true, "not print": true,
		"(max-width: 480px)": true, "(min-width: 768px)": false, "only screen and (max-width: 20em)": true,
		"only screen\nand (min-width : 300px)\nand (max-width : 750px)": true,
		"(orientation: portrait)": false, "(prefers-color-scheme: dark)": false,
	}
	for q, want := range cases {
		if got := mediaMatches(q); got != want {
			t.Errorf("mediaMatches(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestGeneratedContent(t *testing.T) {
	p := render(t, `<style>
		li:not(:last-child)::after { content: " | "; color: black }
		.q::before { content: "\201C" attr(data-x) }
		.none::before { content: none }
	</style><ul><li id=a>A</li><li id=b>B</li></ul><p class=q data-x="hi" id=q>x</p><p class=none id=n>n</p>`, nil)
	a := find(p.body, "a")
	if last := a.Children[len(a.Children)-1]; last.Tag != "span" || last.Children[0].Text != " | " ||
		last.Styles["color"] != "rgb(0, 0, 0)" {
		t.Errorf("li::after = %+v", last)
	}
	if b := find(p.body, "b"); len(b.Children) != 1 {
		t.Errorf("last li should have no ::after, got %+v", b.Children)
	}
	if q := find(p.body, "q"); q.Children[0].Tag != "span" || q.Children[0].Children[0].Text != "“hi" {
		t.Errorf("::before = %+v", q.Children[0])
	}
	if n := find(p.body, "n"); len(n.Children) != 1 {
		t.Errorf("content: none should generate nothing, got %+v", n.Children)
	}
}

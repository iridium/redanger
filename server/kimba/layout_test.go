package kimba

import (
	"context"
	"strings"
	"testing"
)

// layoutPage renders and lays out a page, returning the body box in layout
// (block-space) coordinates, before conversion to KBC's relative ones.
// bodyMargin is the UA stylesheet's body margin.
const bodyMargin = 2

func layoutPage(t *testing.T, page string) *layoutBox {
	t.Helper()
	p, err := renderHTML(context.Background(), "http://example.test/", []byte(page), "text/html")
	if err != nil {
		t.Fatal(err)
	}
	ls := &layoutState{pageURL: p.baseURL}
	ls.styles.intern(newDefaultStyleContext())
	body := buildBoxTree(p.body, ls, false, nil)
	layoutBoxTree(body, screenWidth)
	return body
}

func findBox(b *layoutBox, pred func(*layoutBox) bool) *layoutBox {
	if pred(b) {
		return b
	}
	for _, c := range b.children {
		if f := findBox(c, pred); f != nil {
			return f
		}
	}
	return nil
}

func textBox(b *layoutBox, contains string) *layoutBox {
	return findBox(b, func(b *layoutBox) bool { return b.isText && strings.Contains(b.text, contains) })
}

func TestTableColumnsFitContent(t *testing.T) {
	// The shape of a Hacker News comment: indent spacer, vote arrow, text.
	body := layoutPage(t, `<table><tr>
		<td><img src="s.gif" height="1" width="0"></td>
		<td><a href="vote">^</a></td>
		<td id=text>The quick brown fox jumps over the lazy dog, and then keeps on running for quite a while longer.</td>
	</tr></table>`)
	cell := findBox(body, func(b *layoutBox) bool { return b.kbcTag == tagTD && textBox(b, "quick") != nil })
	if cell == nil {
		t.Fatal("text cell not found")
	}
	if cell.width < 180 {
		t.Errorf("text column is %dpx wide; it should get nearly all of the %dpx", cell.width, screenWidth-2*bodyMargin)
	}
}

func TestTableColspanAndExplicitWidths(t *testing.T) {
	body := layoutPage(t, `<table width=200><tr><td width=50>a</td><td>b</td></tr>
		<tr><td colspan=2>spanning both columns</td></tr></table>`)
	table := findBox(body, func(b *layoutBox) bool { return b.kbcTag == tagTable })
	if table.width != 200 {
		t.Errorf("table width = %d, want 200", table.width)
	}
	rows := tableRows(table)
	if got := rows[1].children[0].width; got != 200 {
		t.Errorf("colspan=2 cell width = %d, want 200", got)
	}
	if got := rows[0].children[0].width; got < 50 || got > 100 {
		t.Errorf("width=50 column = %d", got)
	}
}

func TestInlineSpacingAndWrapping(t *testing.T) {
	body := layoutPage(t, `<p><b>Hacker News</b> <a href="/new">new</a></p>`)
	b := textBox(body, "Hacker")
	a := textBox(body, "new")
	bEnd := b.extents[len(b.extents)-1]
	gotGap := a.extents[0].x - (bEnd.x + textWidth(bEnd.text, b.fontID))
	if bEnd.y == a.extents[0].y && gotGap <= 0 {
		t.Errorf("no space between adjacent inline elements (gap %d)", gotGap)
	}

	long := layoutPage(t, `<p>`+strings.Repeat("word ", 100)+`</p>`)
	tb := textBox(long, "word")
	if len(tb.extents) < 5 {
		t.Fatalf("expected wrapping, got %d lines", len(tb.extents))
	}
	limit := screenWidth - 2*bodyMargin
	for _, e := range tb.extents {
		if w := e.x + textWidth(e.text, tb.fontID); w > bodyMargin+limit {
			t.Errorf("line %q ends at %d, past %d", e.text, w, bodyMargin+limit)
		}
		if !strings.HasPrefix(tb.text[e.textOffset:], e.text) {
			t.Errorf("extent text %q doesn't match text at offset %d", e.text, e.textOffset)
		}
	}
}

func TestTextAlignCenter(t *testing.T) {
	body := layoutPage(t, `<p style="text-align:center">hi</p>`)
	tb := textBox(body, "hi")
	w := textWidth("hi", tb.fontID)
	want := bodyMargin + (screenWidth-2*bodyMargin-w)/2
	if got := tb.extents[0].x; got != want {
		t.Errorf("centered text x = %d, want %d", got, want)
	}
}

func TestExtentsBecomeDeltas(t *testing.T) {
	body := layoutPage(t, `<p>`+strings.Repeat("word ", 40)+`</p>`)
	relativizeBlock(body)
	tb := textBox(body, "word")
	lh := lineHeightFor(tb.fontID)
	for i, e := range tb.extents[1:] {
		if e.y != lh {
			t.Errorf("extent %d dy = %d, want %d", i+1, e.y, lh)
		}
	}
}

func TestLongWordBreaks(t *testing.T) {
	body := layoutPage(t, `<p>`+strings.Repeat("x", 200)+`</p>`)
	tb := textBox(body, "xxx")
	if len(tb.extents) < 2 {
		t.Fatalf("a word wider than the screen should break, got %d extents", len(tb.extents))
	}
}

func TestInlineBlockAndBlockInInline(t *testing.T) {
	body := layoutPage(t, `<style>.p { display: inline-block; width: 80px; margin: 5px 10px }
		.p img { display: block }</style>
		<div><div class=p><a href=1><p>Alice</p></a><img src=a.jpg></div><div class=p><a href=2><p>Bob</p></a><img src=b.jpg></div></div>`)
	var people []*layoutBox
	findBox(body, func(b *layoutBox) bool {
		if b.inlineBlock {
			people = append(people, b)
		}
		return false
	})
	if len(people) != 2 {
		t.Fatalf("found %d inline-blocks", len(people))
	}
	if people[0].width != 80 || people[0].y != people[1].y || people[1].x <= people[0].x+80 {
		t.Errorf("inline-blocks should sit side by side at 80px: %+v / %+v", *people[0], *people[1])
	}
	img := findBox(people[0], func(b *layoutBox) bool { return b.kbcTag == tagImg })
	name := textBox(people[0], "Alice")
	if img.y < name.extents[0].y+lineHeightFor(name.fontID) {
		t.Errorf("block image (y=%d) should sit below the name (y=%d)", img.y, name.extents[0].y)
	}
}

func TestAutoMarginsCenter(t *testing.T) {
	body := layoutPage(t, `<div style="width:100px;margin:0 auto">x</div><table align=center><tr><td>t</td></tr></table>`)
	div := findBox(body, func(b *layoutBox) bool { return b.kbcTag == tagDiv })
	if want := bodyMargin + (screenWidth-2*bodyMargin-100)/2; div.x != want {
		t.Errorf("centered div x = %d, want %d", div.x, want)
	}
	table := findBox(body, func(b *layoutBox) bool { return b.kbcTag == tagTable })
	if table.x <= bodyMargin {
		t.Errorf("align=center table x = %d, should be centered", table.x)
	}
}

func TestQuirksTablesAndCenter(t *testing.T) {
	// No doctype: quirks mode, so the table doesn't inherit <center>'s
	// alignment, but <center> still centers the table itself.
	body := layoutPage(t, `<center><table width=50%><tr><td>left</td></tr></table></center>`)
	table := findBox(body, func(b *layoutBox) bool { return b.kbcTag == tagTable })
	if table.x <= bodyMargin {
		t.Errorf("table in <center> x = %d, should be centered", table.x)
	}
	if tb := textBox(body, "left"); tb.extents[0].x != 1 { // the cell's 1px padding
		t.Errorf("quirks-mode cell text x = %d, want 1 (left-aligned)", tb.extents[0].x)
	}
}

package kimba

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func dump(b *layoutBox, d int) {
	name := "?"
	for _, t := range tagRegistry {
		if t.kbcTag == b.kbcTag {
			name = t.name
		}
	}
	if b.isText {
		fmt.Printf("%s#text %q ext=%v\n", strings.Repeat("  ", d), b.text, b.extents)
		return
	}
	fmt.Printf("%s<%s> x=%d y=%d w=%d h=%d block=%v disp=%d\n", strings.Repeat("  ", d), name, b.x, b.y, b.width, b.height, b.isBlock, b.style.display)
	for _, c := range b.children {
		dump(c, d+1)
	}
}
func TestDbg(t *testing.T) {
	page, _ := os.ReadFile(os.Getenv("PAGE"))
	body := layoutPage(t, string(page))
	nav := findBox(body, func(b *layoutBox) bool { return b.kbcTag == tagUL })
	dump(nav, 0)
}

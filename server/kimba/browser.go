package kimba

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	pagePoolSize       = 4
	viewportHeight     = 480
	navigateTimeout    = 15 * time.Second
	domStableTimeout   = 3 * time.Second
	domStableInterval  = 500 * time.Millisecond
	domStableThreshold = 0.1
	evalTimeout        = 10 * time.Second
)

var (
	browserOnce     sync.Once
	browserInstance *rod.Browser
	browserRouter   *rod.HijackRouter
	pagePool        rod.Pool[rod.Page]
)

// No default device emulation so we can set a custom 240px viewport.
func getBrowser() *rod.Browser {
	browserOnce.Do(func() {
		u := launcher.New().
			Headless(true).
			NoSandbox(true).
			MustLaunch()

		browserInstance = rod.New().
			ControlURL(u).
			NoDefaultDevice().
			MustConnect()

		// Set up a browser-level hijack router to block heavy resources.
		// We ALLOW: documents, stylesheets, scripts (needed for CSS-in-JS).
		// We BLOCK: images, fonts, media, websockets, etc.
		browserRouter = browserInstance.HijackRequests()
		browserRouter.MustAdd("*", func(ctx *rod.Hijack) {
			switch ctx.Request.Type() {
			case proto.NetworkResourceTypeImage,
				proto.NetworkResourceTypeFont,
				proto.NetworkResourceTypeMedia,
				proto.NetworkResourceTypeWebSocket,
				proto.NetworkResourceTypeManifest,
				proto.NetworkResourceTypePing:
				ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			default:
				ctx.ContinueRequest(&proto.FetchContinueRequest{})
			}
		})
		go browserRouter.Run()

		// Page pool: reuse up to 4 tabs concurrently.
		pagePool = rod.NewPagePool(pagePoolSize)

		log.Printf("Kimba: headless Chrome started at %s", u)
	})
	return browserInstance
}

func closeBrowser() {
	if browserRouter != nil {
		browserRouter.MustStop()
	}
	if pagePool != nil {
		pagePool.Cleanup(func(p *rod.Page) { p.MustClose() })
	}
	if browserInstance != nil {
		browserInstance.MustClose()
	}
}

type chromeDOMNode struct {
	Tag      string            `json:"tag"`      // lowercase tag name, or "#text"
	Text     string            `json:"text"`     // text content (for #text nodes)
	Styles   map[string]string `json:"styles"`   // computed CSS values
	Attrs    map[string]string `json:"attrs"`    // HTML attributes
	Children []chromeDOMNode   `json:"children"` // child nodes
}

const extractDOMJS = `() => {
	const STYLE_PROPS = [
		'color', 'backgroundColor',
		'fontSize', 'fontWeight', 'fontStyle',
		'display', 'visibility',
		'textDecoration', 'textTransform', 'whiteSpace', 'textAlign',
		'marginTop', 'marginRight', 'marginBottom', 'marginLeft',
		'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
		'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
		'borderTopStyle', 'borderRightStyle', 'borderBottomStyle', 'borderLeftStyle',
		'borderTopColor', 'borderRightColor', 'borderBottomColor', 'borderLeftColor',
		'lineHeight', 'float', 'listStyleType',
		'width', 'height'
	];

	const ATTR_NAMES = [
		'href', 'src', 'alt', 'action', 'method',
		'type', 'name', 'value', 'selected', 'checked',
		'start', 'cols', 'rows', 'multiple', 'size',
		'maxlength', 'placeholder', 'id', 'class'
	];

	function walkNode(node) {
		if (node.nodeType === Node.TEXT_NODE) {
			let text = node.textContent;
			if (!text || text.trim() === '') return null;
			return { tag: '#text', text: text };
		}

		if (node.nodeType !== Node.ELEMENT_NODE) return null;

		let tag = node.tagName.toLowerCase();

		// Skip non-renderable elements
		if (tag === 'script' || tag === 'style' || tag === 'link' ||
		    tag === 'meta' || tag === 'noscript' || tag === 'head' ||
		    tag === 'svg' || tag === 'iframe' || tag === 'video' ||
		    tag === 'audio' || tag === 'canvas' || tag === 'template') {
			return null;
		}

		let cs = window.getComputedStyle(node);

		// Skip display:none elements
		if (cs.display === 'none') return null;

		let styles = {};
		for (let prop of STYLE_PROPS) {
			let val = cs[prop];
			if (val !== undefined && val !== '') {
				styles[prop] = val;
			}
		}

		let attrs = {};
		for (let name of ATTR_NAMES) {
			if (node.hasAttribute(name)) {
				attrs[name] = node.getAttribute(name);
			}
		}

		let children = [];
		for (let child of node.childNodes) {
			let r = walkNode(child);
			if (r) children.push(r);
		}

		return { tag: tag, styles: styles, attrs: attrs, children: children };
	}

	let title = document.title || '';
	let body = walkNode(document.body);
	return { title: title, body: body };
}`

type chromePageResult struct {
	Title string         `json:"title"`
	Body  *chromeDOMNode `json:"body"`
}

func fetchPageViaChrome(pageURL string) (*chromeDOMNode, string, error) {
	browser := getBrowser()

	// Get a page from the pool (or create a new one).
	page := pagePool.MustGet(func() *rod.Page {
		return browser.MustPage()
	})
	defer pagePool.Put(page)

	// Set 240px viewport (Sidekick screen width).
	err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             screenWidth,
		Height:            viewportHeight,
		DeviceScaleFactor: 1,
	})
	if err != nil {
		return nil, "", fmt.Errorf("set viewport: %w", err)
	}

	// Navigate with a timeout.
	err = page.Timeout(navigateTimeout).Navigate(pageURL)
	if err != nil {
		return nil, "", fmt.Errorf("navigate to %s: %w", pageURL, err)
	}

	// Wait for the page to be loaded.
	err = page.Timeout(navigateTimeout).WaitLoad()
	if err != nil {
		return nil, "", fmt.Errorf("wait load %s: %w", pageURL, err)
	}

	// Give a brief moment for late CSS/JS to settle.
	err = page.Timeout(domStableTimeout).WaitDOMStable(domStableInterval, domStableThreshold)
	if err != nil {
		// Not fatal, continue with what we have.
		log.Printf("Kimba: DOM not fully stable for %s (continuing): %v", pageURL, err)
	}

	// Extract the DOM tree with computed styles.
	result, err := page.Timeout(evalTimeout).Eval(extractDOMJS)
	if err != nil {
		return nil, "", fmt.Errorf("eval DOM extraction for %s: %w", pageURL, err)
	}

	// Parse the JSON result.
	var pageResult chromePageResult
	if err := result.Value.Unmarshal(&pageResult); err != nil {
		return nil, "", fmt.Errorf("unmarshal DOM tree for %s: %w", pageURL, err)
	}

	if pageResult.Body == nil {
		return nil, "", fmt.Errorf("no body element for %s", pageURL)
	}

	return pageResult.Body, pageResult.Title, nil
}

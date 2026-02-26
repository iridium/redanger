package kimba

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"server/proto"
	"strings"
	"time"
)

// Kimba protocol class IDs (separate namespace from main Danger protocol)
const (
	ClassFetchRequest    = 1
	ClassImageRequest    = 2
	ClassFormSubmit      = 3
	ClassAuthentication  = 4
	ClassCacheUpdate     = 6
	ClassKimbaDoc        = 9
	ClassDocBegin        = 11
	ClassDocEnd          = 12
	ClassError           = 13
	ClassDataMessage     = 59
	ClassProtocolVersion = 73
	ClassGeneric         = 74
)

var kimbaClassName = map[uint16]string{
	ClassFetchRequest:    "FetchRequest",
	ClassImageRequest:    "ImageRequest",
	ClassFormSubmit:      "FormSubmit",
	ClassAuthentication:  "Authentication",
	ClassCacheUpdate:     "CacheUpdate",
	ClassKimbaDoc:        "KimbaDoc",
	ClassDocBegin:        "DocBegin",
	ClassDocEnd:          "DocEnd",
	ClassError:           "Error",
	ClassDataMessage:     "DataMessage",
	ClassProtocolVersion: "ProtocolVersion",
	ClassGeneric:         "Generic",
}

func kimbaClassStr(id uint16) string {
	if name, ok := kimbaClassName[id]; ok {
		return fmt.Sprintf("%s(%d)", name, id)
	}
	return fmt.Sprintf("Unknown(%d)", id)
}

const userAgent = "Mozilla/5.0 (Danger hiptop 3.4; U; AvantGo 3.2)"

const (
	errorCodeFetch      = 1000 // network/fetch error
	errorCodeBadRequest = 1001 // malformed request
)

const httpClientTimeout = 30 * time.Second

const httpMaxRedirects = 10

var httpClient = &http.Client{
	Timeout: httpClientTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= httpMaxRedirects {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

func ListenAndServe(addr string) error {
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	log.Printf("Kimba proxy listening on %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Kimba: accept error: %v", err)
			continue
		}
		log.Printf("Kimba: connection from %s", conn.RemoteAddr())
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Perform Danger secure handshake
	sc, err := proto.Handshake(conn)
	if err != nil {
		log.Printf("Kimba: handshake failed: %v", err)
		return
	}

	r := proto.NewReader(sc)

	for {
		hdr, err := r.ReadHeader()
		if err != nil {
			if err != io.EOF {
				log.Printf("Kimba: error reading header: %v", err)
			}
			return
		}

		body := make([]byte, hdr.BodyLength)
		if hdr.BodyLength > 0 {
			if _, err := io.ReadFull(r, body); err != nil {
				log.Printf("Kimba: error reading body: %v", err)
				return
			}
		}

		log.Printf("KIMBA RECV %s ver=%d bodyLen=%d\n%s",
			kimbaClassStr(hdr.ClassID), hdr.Version, hdr.BodyLength, hex.Dump(body))

		switch hdr.ClassID {
		case ClassProtocolVersion:
			br := proto.NewBodyReader(body)
			version, _ := br.ReadShort()
			log.Printf("Kimba: client protocol version=%d", version)

		case ClassAuthentication:
			log.Printf("Kimba: auth message (v=%d, bodyLen=%d), accepted", hdr.Version, hdr.BodyLength)

		case ClassGeneric:
			br := proto.NewBodyReader(body)
			count, _ := br.ReadShort()
			log.Printf("Kimba: generic message with %d key-value pairs", count)
			for range count {
				k, _ := br.ReadString()
				v, _ := br.ReadString()
				log.Printf("  %s = %s", k, v)
			}

		case ClassCacheUpdate:
			log.Printf("Kimba: cache update (bodyLen=%d), ignored", hdr.BodyLength)

		case ClassFetchRequest, ClassImageRequest:
			handleFetchRequest(sc, hdr, body)

		case ClassFormSubmit:
			handleFormSubmit(sc, hdr, body)

		default:
			log.Printf("Kimba: unknown classID=%d v=%d bodyLen=%d", hdr.ClassID, hdr.Version, hdr.BodyLength)
		}
	}
}

func handleFetchRequest(w io.Writer, hdr proto.Header, body []byte) {
	br := proto.NewBodyReader(body)

	url, _ := br.ReadString()
	referer, _ := br.ReadString()
	var charset string
	if hdr.Version >= 5 {
		charset, _ = br.ReadString()
	}
	flags, _ := br.ReadShort()
	seqID, _ := br.ReadShort()

	log.Printf("Kimba: FetchRequest url=%q referer=%q charset=%q flags=0x%04x seqID=%d",
		url, referer, charset, flags, seqID)

	proxyHTTP(w, url, referer, "GET", seqID)
}

func handleFormSubmit(w io.Writer, hdr proto.Header, body []byte) {
	br := proto.NewBodyReader(body)

	url, _ := br.ReadString()
	referer, _ := br.ReadString()
	var charset string
	if hdr.Version >= 3 {
		charset, _ = br.ReadString()
	}
	flags, _ := br.ReadShort()
	seqID, _ := br.ReadShort()
	methodByte, _ := br.ReadByte()
	_, _ = br.ReadByte() // encType

	method := "POST"
	if methodByte == 1 {
		method = "PUT"
	}

	formCharset, _ := br.ReadString()
	controlCount, _ := br.ReadShort()

	// Build form data
	var formParts []string
	for range controlCount {
		name, _ := br.ReadString()
		value, _ := br.ReadString()
		// Skip file controls for now
		if name == "X-Danger-Form-Submit-File-Indicator" {
			continue
		}
		formParts = append(formParts, name+"="+value)
	}
	formBody := strings.Join(formParts, "&")

	log.Printf("Kimba: FormSubmit method=%s url=%q charset=%q formCharset=%q seqID=%d controls=%d",
		method, url, charset, formCharset, seqID, controlCount)

	_ = flags // flags are for rendering hints, not HTTP

	proxyHTTPWithBody(w, url, referer, method, seqID, formBody, "application/x-www-form-urlencoded")
}

func proxyHTTP(w io.Writer, url, referer, method string, seqID int16) {
	proxyHTTPWithBody(w, url, referer, method, seqID, "", "")
}

func proxyHTTPWithBody(w io.Writer, url, referer, method string, seqID int16, reqBody, contentType string) {
	var bodyReader io.Reader
	if reqBody != "" {
		bodyReader = strings.NewReader(reqBody)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		log.Printf("Kimba: bad request for %s: %v", url, err)
		sendError(w, url, seqID, errorCodeBadRequest, fmt.Sprintf("Bad request: %v", err))
		return
	}

	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("Kimba: fetch error for %s: %v", url, err)
		sendError(w, url, seqID, errorCodeFetch, fmt.Sprintf("Fetch error: %v", err))
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Kimba: read body error for %s: %v", url, err)
		sendError(w, url, seqID, errorCodeFetch, fmt.Sprintf("Read error: %v", err))
		return
	}

	log.Printf("Kimba: fetched %s -> %d (%d bytes)", url, resp.StatusCode, len(respBody))

	respContentType := resp.Header.Get("Content-Type")
	respCharset := ""
	if ct := respContentType; ct != "" {
		if _, params, ok := strings.Cut(ct, ";"); ok {
			params = strings.TrimSpace(params)
			if strings.HasPrefix(strings.ToLower(params), "charset=") {
				respCharset = params[8:]
			}
		}
		// Strip parameters from content type
		if base, _, ok := strings.Cut(respContentType, ";"); ok {
			respContentType = strings.TrimSpace(base)
		}
	}

	if strings.HasPrefix(respContentType, "text/html") {
		// Convert HTML to KBC via headless Chrome and send as KimbaDoc.
		kbc := chromeHTMLToKBC(url)
		if kbc == nil {
			log.Printf("Kimba: Chrome rendering failed for %s", url)
			kbc = buildErrorKBC(url, "Failed to render page")
		}
		sendDocBegin(w, int32(len(kbc)), 0, seqID)
		sendKimbaDoc(w, url, seqID, int16(resp.StatusCode), kbc)
		sendDocEnd(w, seqID)
	} else {
		// Send non-HTML content as DataMessage
		sendDocBegin(w, int32(len(respBody)), 0, seqID)
		sendDataMessage(w, url, seqID, respContentType, respCharset, int16(resp.StatusCode), string(respBody))
		sendDocEnd(w, seqID)
	}
}

func sendDocBegin(w io.Writer, totalSize int32, subfiles int32, seqID int16) {
	var b proto.Body
	b.WriteInt(totalSize)
	b.WriteInt(subfiles)
	b.WriteShort(seqID)
	writeKimbaObject(w, 1, ClassDocBegin, b.Bytes())
}

func sendDocEnd(w io.Writer, seqID int16) {
	var b proto.Body
	b.WriteShort(seqID)
	writeKimbaObject(w, 1, ClassDocEnd, b.Bytes())
}

func sendDataMessage(w io.Writer, url string, seqID int16, contentType, charset string, status int16, content string) {
	var b proto.Body
	b.WriteString(url)
	b.WriteShort(seqID)
	b.WriteString(contentType)
	b.WriteString(charset)
	b.WriteShort(status)
	b.WriteInt(0) // expiration
	b.WriteInt(0) // checksum
	b.WriteString(content)
	writeKimbaObject(w, 3, ClassDataMessage, b.Bytes())
}

func sendKimbaDoc(w io.Writer, url string, seqID int16, status int16, kbcContent []byte) {
	var b proto.Body
	b.WriteString(url)
	b.WriteShort(seqID)
	b.WriteShort(status)
	b.WriteInt(0) // expiry
	b.WriteInt(0) // checksum
	b.WriteInt(0) // lastModified
	b.WriteString(string(kbcContent))
	writeKimbaObject(w, 2, ClassKimbaDoc, b.Bytes())
}

func sendError(w io.Writer, url string, seqID int16, errorCode int32, message string) {
	var b proto.Body
	b.WriteString(url)
	b.WriteInt(errorCode)
	b.WriteString(message)
	b.WriteString("") // system message
	b.WriteShort(seqID)
	writeKimbaObject(w, 1, ClassError, b.Bytes())
}

func writeKimbaObject(w io.Writer, classVersion int, classID uint16, body []byte) {
	err := proto.WriteObject(w, classVersion, classID, false, 0, body)
	if err != nil {
		log.Printf("Kimba: write error: %v", err)
	}
}

package proto

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
)

const (
	ClassIMYahoo             = 6
	ClassContactsTransaction = 7
	ClassAck                 = 14
	ClassPing                = 15
	ClassPong                = 16
	ClassServiceTime         = 27
	ClassSystemVersionInfo   = 38
	ClassServiceInfo         = 42
	ClassServiceLogin        = 43
	ClassServiceLoginResult  = 44
	ClassIdentityCheck       = 45
	ClassIdentityCheckResult = 46
	ClassDropBoxInfo         = 47
	ClassIMAIM               = 53
	ClassIMJabber            = 54
	ClassServiceEventLog     = 57
	ClassIMSay2              = 61
	ClassServiceTimeSettings = 66
	ClassCellUpdate          = 68
	ClassIMMSN               = 89
	ClassIMICQ               = 97
)

var className = map[uint16]string{
	ClassIMYahoo:             "IMYahoo",
	ClassContactsTransaction: "ContactsTransaction",
	ClassAck:                 "Ack",
	ClassPing:                "Ping",
	ClassPong:                "Pong",
	ClassServiceTime:         "ServiceTime",
	ClassSystemVersionInfo:   "SystemVersionInfo",
	ClassServiceInfo:         "ServiceInfo",
	ClassServiceLogin:        "ServiceLogin",
	ClassServiceLoginResult:  "ServiceLoginResult",
	ClassIdentityCheck:       "IdentityCheck",
	ClassIdentityCheckResult: "IdentityCheckResult",
	ClassDropBoxInfo:         "DropBoxInfo",
	ClassIMAIM:               "IMAIM",
	ClassIMJabber:            "IMJabber",
	ClassServiceEventLog:     "ServiceEventLog",
	ClassIMSay2:              "IMSay2",
	ClassServiceTimeSettings: "ServiceTimeSettings",
	ClassCellUpdate:          "CellUpdate",
	ClassIMMSN:               "IMMSN",
	ClassIMICQ:               "IMICQ",
}

func ClassNameStr(id uint16) string {
	if name, ok := className[id]; ok {
		return fmt.Sprintf("%s(%d)", name, id)
	}
	return fmt.Sprintf("Unknown(%d)", id)
}

type Header struct {
	Version     int
	BodyLength  int
	HasSequence bool
	SequenceID  uint16
	ClassID     uint16
}

type Reader struct {
	*bufio.Reader
}

func NewReader(r io.Reader) *Reader {
	return &Reader{bufio.NewReader(r)}
}

func NewBodyReader(data []byte) *Reader {
	return &Reader{bufio.NewReader(bytes.NewReader(data))}
}

func (r *Reader) ReadShort() (int16, error) {
	var v int16
	err := binary.Read(r, binary.BigEndian, &v)
	return v, err
}

func (r *Reader) ReadInt() (int32, error) {
	var v int32
	err := binary.Read(r, binary.BigEndian, &v)
	return v, err
}

func (r *Reader) ReadSize() (int, error) {
	result := 0
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		result = (result << 7) | int(b&0x7F)
		if b&0x80 == 0 {
			break
		}
	}
	return result, nil
}

func (r *Reader) ReadString() (string, error) {
	size, err := r.ReadSize()
	if err != nil {
		return "", err
	}
	if size == 0 {
		return "", nil
	}
	buf := make([]byte, size)
	_, err = io.ReadFull(r, buf)
	return string(buf), err
}

func (r *Reader) ReadBinary() ([]byte, error) {
	size, err := r.ReadSize()
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	_, err = io.ReadFull(r, buf)
	return buf, err
}

func (r *Reader) ReadHeader() (Header, error) {
	var hdr Header
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return hdr, err
	}
	b0, b1, b2, b3 := buf[0], buf[1], buf[2], buf[3]
	hdr.Version = int((b0 >> 4) & 0x0F)
	hdr.BodyLength = int(b0&0x0F)<<23 | int(b1)<<15 | int(b2)<<7 | int((b3&0xFE)>>1)
	hdr.HasSequence = (b3 & 0x01) != 0
	if hdr.HasSequence {
		seq, err := r.ReadShort()
		if err != nil {
			return hdr, err
		}
		hdr.SequenceID = uint16(seq)
	}
	classID, err := r.ReadShort()
	if err != nil {
		return hdr, err
	}
	hdr.ClassID = uint16(classID)
	return hdr, nil
}

type Body struct {
	bytes.Buffer
}

func (b *Body) WriteShort(v int16) {
	binary.Write(&b.Buffer, binary.BigEndian, v)
}

func (b *Body) WriteInt(v int32) {
	binary.Write(&b.Buffer, binary.BigEndian, v)
}

func (b *Body) WriteString(s string) {
	b.writeSize(len(s))
	b.Buffer.WriteString(s)
}

func (b *Body) WriteBinary(data []byte) {
	b.writeSize(len(data))
	b.Buffer.Write(data)
}

func (b *Body) writeSize(value int) {
	if value < 128 {
		b.Buffer.WriteByte(byte(value))
		return
	}
	var buf [5]byte
	i := 0
	v := value
	for {
		buf[i] = byte(v & 0x7F)
		if i > 0 {
			buf[i] |= 0x80
		}
		i++
		v >>= 7
		if v == 0 {
			break
		}
	}
	for j := i - 1; j >= 0; j-- {
		b.Buffer.WriteByte(buf[j])
	}
}

func WriteObject(w io.Writer, classVersion int, classID uint16, hasSeq bool, seqID uint16, body []byte) error {
	bodyLen := len(body)
	seqBit := 0
	if hasSeq {
		seqBit = 1
	}

	var out bytes.Buffer
	out.WriteByte(byte((classVersion << 4) | ((bodyLen >> 23) & 0x0F)))
	out.WriteByte(byte((bodyLen >> 15) & 0xFF))
	out.WriteByte(byte((bodyLen >> 7) & 0xFF))
	out.WriteByte(byte(((bodyLen << 1) & 0xFE) | seqBit))
	if hasSeq {
		binary.Write(&out, binary.BigEndian, seqID)
	}
	binary.Write(&out, binary.BigEndian, classID)
	out.Write(body)

	raw := out.Bytes()
	log.Printf("SEND %s ver=%d seq=%v bodyLen=%d\n%s",
		ClassNameStr(classID), classVersion, hasSeq, bodyLen, hex.Dump(raw))
	_, err := w.Write(raw)
	return err
}

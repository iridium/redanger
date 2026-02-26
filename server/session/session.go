package session

import (
	"encoding/hex"
	"io"
	"log"
	"net"
	"server/proto"
	"sync"

	"github.com/bwmarrin/discordgo"
)

const channelBotName = "ChannelBot"

func ListenAndServe(addr string, dg *discordgo.Session, guildID string) error {
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	log.Printf("Listening on %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		log.Printf("Connection from %s", conn.RemoteAddr())
		go New(conn, dg, guildID).Run()
	}
}

type Session struct {
	conn            net.Conn
	r               *proto.Reader
	done            chan struct{}
	lastSeqReceived int16 // highest sequence received from client
	lastAckSent     int16 // highest sequence we've ACKed

	mu                 sync.Mutex
	discord            *discordgo.Session
	discordGuild       string
	handlers           []func()
	discordChannels    map[string]string    // roomName -> Discord channel ID (subscribed only)
	availableChannels  []*discordgo.Channel // all text channels in order (for numbered list)
	subscribedChannels map[string]bool      // Discord channel ID -> true (channels user has joined)
	screenName         string
}

func New(conn net.Conn, discord *discordgo.Session, guildID string) *Session {
	return &Session{
		conn:               conn,
		r:                  proto.NewReader(conn),
		done:               make(chan struct{}),
		discord:            discord,
		discordGuild:       guildID,
		discordChannels:    make(map[string]string),
		subscribedChannels: make(map[string]bool),
	}
}

func (s *Session) Run() {
	defer s.conn.Close()
	defer close(s.done)
	defer func() {
		for _, remove := range s.handlers {
			remove()
		}
	}()

	protocolVersion, err := s.r.ReadShort()
	if err != nil {
		log.Printf("Error reading protocol version: %v", err)
		return
	}
	locale, err := s.r.ReadString()
	if err != nil {
		log.Printf("Error reading locale: %v", err)
		return
	}
	log.Printf("Client connected, protocol=%d locale=%s", protocolVersion, locale)

	for {
		hdr, err := s.r.ReadHeader()
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading header: %v", err)
			}
			return
		}

		body := make([]byte, hdr.BodyLength)
		if hdr.BodyLength > 0 {
			if _, err := io.ReadFull(s.r, body); err != nil {
				log.Printf("Error reading body: %v", err)
				return
			}
		}

		log.Printf("RECV %s ver=%d seq=%v(%d) bodyLen=%d\n%s",
			proto.ClassNameStr(hdr.ClassID), hdr.Version, hdr.HasSequence, hdr.SequenceID, hdr.BodyLength, hex.Dump(body))

		if err := s.dispatch(hdr, body); err != nil {
			log.Printf("Error handling classID=%d: %v", hdr.ClassID, err)
			return
		}

		if hdr.HasSequence {
			s.lastSeqReceived = int16(hdr.SequenceID)
			if err := s.sendAckIfNeeded(); err != nil {
				log.Printf("Error sending ack: %v", err)
				return
			}
		}
	}
}

func (s *Session) dispatch(hdr proto.Header, body []byte) error {
	br := proto.NewBodyReader(body)

	switch hdr.ClassID {
	case proto.ClassIdentityCheck:
		return s.handleIdentityCheck(hdr, br)
	case proto.ClassServiceLogin:
		return s.handleServiceLogin(hdr, br)
	case proto.ClassSystemVersionInfo:
		return s.handleSystemVersionInfo(hdr, br)
	case proto.ClassDropBoxInfo:
		return s.handleDropBoxInfo(hdr, br)
	case proto.ClassAck:
		return s.handleAck(hdr, br)
	case proto.ClassPong:
		return s.handlePong(hdr, br)
	case proto.ClassServiceTimeSettings:
		return s.handleServiceTimeSettings(hdr, br)
	case proto.ClassServiceEventLog:
		return s.handleServiceEventLog(hdr, br)
	case proto.ClassCellUpdate:
		return s.handleCellUpdate(hdr, br)
	case proto.ClassContactsTransaction:
		return s.handleContactsTransaction(hdr, br)
	case proto.ClassIMYahoo, proto.ClassIMAIM, proto.ClassIMJabber, proto.ClassIMSay2, proto.ClassIMMSN, proto.ClassIMICQ:
		return s.handleIM(hdr, br)
	default:
		log.Printf("Unknown classID=%d version=%d bodyLen=%d hasSeq=%v seqID=%d",
			hdr.ClassID, hdr.Version, hdr.BodyLength, hdr.HasSequence, hdr.SequenceID)
		return nil
	}
}

func (s *Session) send(classVersion int, classID uint16, body *proto.Body) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return proto.WriteObject(s.conn, classVersion, classID, false, 0, body.Bytes())
}

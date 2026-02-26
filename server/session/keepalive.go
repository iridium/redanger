package session

import (
	"log"
	"server/proto"
	"time"
)

func (s *Session) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			if err := s.send(1, proto.ClassPing, proto.Marshal(&proto.Ping{})); err != nil {
				return
			}
		}
	}
}

func (s *Session) sendAckIfNeeded() error {
	if int16(s.lastSeqReceived-s.lastAckSent) >= 10 {
		return s.sendAck()
	}
	return nil
}

func (s *Session) sendAck() error {
	resp := proto.Marshal(&proto.Ack{HighestSequence: s.lastSeqReceived})
	s.lastAckSent = s.lastSeqReceived
	return s.send(1, proto.ClassAck, resp)
}

func (s *Session) handleAck(hdr proto.Header, br *proto.Reader) error {
	var msg proto.Ack
	if err := proto.Unmarshal(br, &msg); err != nil {
		return err
	}
	log.Printf("Ack: seq=%d", msg.HighestSequence)
	return nil
}

func (s *Session) handlePong(hdr proto.Header, br *proto.Reader) error {
	var msg proto.Pong
	if err := proto.Unmarshal(br, &msg); err != nil {
		return err
	}
	log.Printf("Pong: payloadSize=%d", len(msg.Payload))
	return nil
}

package session

import (
	"fmt"
	"log"
	"net"
	"server/proto"
	"time"
)

func (s *Session) handleSystemVersionInfo(hdr proto.Header, br *proto.Reader) error {
	log.Printf("Received SystemVersionInfo (version=%d bodyLen=%d)", hdr.Version, hdr.BodyLength)
	return nil
}

func (s *Session) handleDropBoxInfo(hdr proto.Header, br *proto.Reader) error {
	var msg proto.DropBoxInfo
	if err := proto.Unmarshal(br, &msg); err != nil {
		return err
	}
	if msg.Flags != 0 {
		log.Printf("DropBoxInfo: hasSeqInfo=true lastSent=%d lastRecv=%d", msg.LastSent, msg.LastRecv)
	} else {
		log.Printf("DropBoxInfo: no sequence info")
	}

	resp := proto.Marshal(&proto.DropBoxInfo{Flags: 1, LastSent: 0, LastRecv: 0})
	if err := s.send(2, proto.ClassDropBoxInfo, resp); err != nil {
		return err
	}
	log.Printf("Sent DropBoxInfo, client is now LOGGED_IN")

	// Send current time (seconds since Danger epoch: Jan 1, 2000 00:00:00 UTC)
	dangerEpoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	timeBody := proto.Marshal(&proto.ServiceTime{Timestamp: int32(time.Now().Unix() - dangerEpoch.Unix())})
	if err := s.send(1, proto.ClassServiceTime, timeBody); err != nil {
		return err
	}
	log.Printf("Sent ServiceTime")

	versionBody := proto.Marshal(&proto.ServiceTimeSettings{Command: -1000, ProtocolVersion: 2})
	if err := s.send(2, proto.ClassServiceTimeSettings, versionBody); err != nil {
		return err
	}
	log.Printf("Sent ServiceTimeSettings VERSION_PREF")

	settingsBody := proto.Marshal(&proto.ServiceTimeSettings{Command: -400, TimeZoneID: 0, TimeFormat: 0})
	if err := s.send(2, proto.ClassServiceTimeSettings, settingsBody); err != nil {
		return err
	}
	log.Printf("Sent ServiceTimeSettings SETTINGS")

	host, _, _ := net.SplitHostPort(s.conn.LocalAddr().String())
	serviceInfoBody := proto.Marshal(&proto.ServiceInfo{
		Settings: map[string]string{
			"kimba_proxy_machine": host,
			"kimba_proxy_port":    fmt.Sprintf("%d", 25026),
		},
	})
	if err := s.send(1, proto.ClassServiceInfo, serviceInfoBody); err != nil {
		return err
	}
	log.Printf("Sent ServiceInfo: kimba_proxy_machine=%s kimba_proxy_port=25026", host)

	if err := s.sendContacts(); err != nil {
		return err
	}

	go s.keepAlive()
	return nil
}

func (s *Session) handleServiceTimeSettings(hdr proto.Header, br *proto.Reader) error {
	var msg proto.ServiceTimeSettings
	if err := proto.Unmarshal(br, &msg); err != nil {
		return err
	}
	switch msg.Command {
	case -1000: // VERSION_PREF
		log.Printf("ServiceTimeSettings VERSION_PREF: protocolVersion=%d", msg.ProtocolVersion)
	case -400: // SETTINGS
		log.Printf("ServiceTimeSettings SETTINGS: timeZoneID=%d timeFormat=%d", msg.TimeZoneID, msg.TimeFormat)
	default:
		log.Printf("ServiceTimeSettings: unknown command=%d", msg.Command)
	}
	return nil
}

func (s *Session) handleServiceEventLog(hdr proto.Header, br *proto.Reader) error {
	var msg proto.ServiceEventLog
	if err := proto.Unmarshal(br, &msg); err != nil {
		return err
	}
	log.Printf("ServiceEventLog: date=%d category=%q message=%q", msg.Date, msg.Category, msg.Message)
	return nil
}

func (s *Session) handleContactsTransaction(hdr proto.Header, br *proto.Reader) error {
	var cmd proto.ContactsCommand
	if err := proto.Unmarshal(br, &cmd); err != nil {
		return err
	}
	switch cmd.Command {
	case -1000: // VERSION_PREF
		log.Printf("Contacts VERSION_PREF from client (ignored)")
	case -1002: // UPDATE_ALL (client requesting full sync)
		log.Printf("Contacts UPDATE_ALL request from client")
	default:
		log.Printf("Contacts command=%d (unhandled)", cmd.Command)
	}
	return nil
}

func (s *Session) handleCellUpdate(hdr proto.Header, br *proto.Reader) error {
	var msg proto.CellUpdate
	if err := proto.Unmarshal(br, &msg); err != nil {
		return err
	}
	log.Printf("CellUpdate: operator=%q lac=%q cid=%q", msg.Operator, msg.LAC, msg.CID)
	return nil
}

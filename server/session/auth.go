package session

import (
	"log"
	"server/proto"
)

func (s *Session) handleIdentityCheck(hdr proto.Header, br *proto.Reader) error {
	var msg proto.IdentityCheck
	if err := proto.Unmarshal(br, &msg); err != nil {
		return err
	}
	log.Printf("IdentityCheck: simIdType=%d simId=%q deviceIdType=%q deviceId=%q partnerId=%d",
		msg.SIMIDType, msg.SIMID, msg.DeviceIDType, msg.DeviceID, msg.PartnerID)

	resp := proto.Marshal(&proto.IdentityCheckResult{Action: 0, CurrentUser: "user"})
	if err := s.send(1, proto.ClassIdentityCheckResult, resp); err != nil {
		return err
	}
	log.Printf("Sent IdentityCheckResult: normal_login")
	return nil
}

func (s *Session) handleServiceLogin(hdr proto.Header, br *proto.Reader) error {
	var msg proto.ServiceLogin
	if err := proto.Unmarshal(br, &msg); err != nil {
		return err
	}
	log.Printf("ServiceLogin: loginType=%d username=%q simIdType=%d simId=%q deviceIdType=%q deviceId=%q locationKeyLen=%d partnerId=%d",
		msg.LoginType, msg.Username, msg.SIMIDType, msg.SIMID, msg.DeviceIDType, msg.DeviceID, len(msg.LocationKey), msg.PartnerID)

	resp := proto.Marshal(&proto.ServiceLoginResult{Result: 0, Explanation: "Welcome", NewLocationKey: []byte{}})
	if err := s.send(2, proto.ClassServiceLoginResult, resp); err != nil {
		return err
	}
	log.Printf("Sent ServiceLoginResult: success")
	return nil
}

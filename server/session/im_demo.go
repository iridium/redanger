package session

import (
	"log"
	"server/proto"
)

type demoMessage struct {
	sender string
	text   string
}

var demoConversation = []demoMessage{
	{"nicole_rawr", "omg have u seen Re:Danger??"},
	{"jecka_xo", "wait what is that"},
	{"iridium", "sidekick server revival :)"},
	{"nicole_rawr", "NO WAY"},
}

var demoMembers = []string{"iridium", "nicole_rawr", "jecka_xo"}

const demoRoom = "#redanger"

func (s *Session) demoLogin() error {
	log.Printf("Demo mode: logging in %q", s.screenName)

	// logonName, numGroups, numBlocked, numStatuses
	resp := proto.Marshal(&proto.IMMessage{
		Command: imCmdLogonInfo,
		Args:    []string{s.screenName, "0", "0", "0"},
	})
	if err := s.send(5, proto.ClassIMAIM, resp); err != nil {
		return err
	}

	if err := s.sendChatJoin(demoRoom); err != nil {
		return err
	}
	if err := s.sendChatMembers(demoRoom, "1", demoMembers); err != nil {
		return err
	}

	for _, msg := range demoConversation {
		if err := s.sendChatMessage(demoRoom, msg.sender, msg.text); err != nil {
			return err
		}
	}

	return nil
}

func (s *Session) handleDemoChatMessage(args []string) error {
	if len(args) < 2 {
		return nil
	}
	roomName := args[0]
	text := args[1]

	if err := s.sendChatMessage(roomName, s.screenName, text); err != nil {
		return err
	}

	return s.sendChatMessage(roomName, "iridium", "long live the hiptop!")
}

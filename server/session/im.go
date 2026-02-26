package session

import (
	"fmt"
	"log"
	"server/proto"
	"sort"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	imCmdMessage     int32 = -500
	imCmdStatus      int32 = -502
	imCmdLogon       int32 = -503
	imCmdDisconnect  int32 = -510
	imCmdLogonInfo   int32 = -525
	imCmdChatJoin    int32 = -531
	imCmdChatLeave   int32 = -532
	imCmdChatMembers int32 = -533
	imCmdChatMessage int32 = -534
	imCmdChatAccept  int32 = -535
)

func (s *Session) handleIM(hdr proto.Header, br *proto.Reader) error {
	var msg proto.IMMessage
	if err := proto.Unmarshal(br, &msg); err != nil {
		return err
	}

	log.Printf("%s: command=%d numStrings=%d args=%q",
		proto.ClassNameStr(hdr.ClassID), msg.Command, len(msg.Args), msg.Args)

	if hdr.ClassID == proto.ClassIMAIM {
		return s.handleIMAIM(msg.Command, msg.Args)
	}
	return nil
}

func (s *Session) handleIMAIM(command int32, args []string) error {
	switch command {
	case imCmdLogon:
		if len(args) > 0 {
			s.screenName = args[0]
		}
		if s.discord == nil {
			return s.demoLogin()
		}
		return s.aimLoginWithDiscord()
	case imCmdMessage:
		if s.discord == nil {
			return nil
		}
		return s.handleDirectMessageFromDevice(args)
	case imCmdChatMessage:
		if s.discord == nil {
			return s.handleDemoChatMessage(args)
		}
		return s.handleChatMessageFromDevice(args)
	case imCmdChatLeave:
		return s.handleChatLeave(args)
	case imCmdChatAccept:
	case imCmdDisconnect:
		log.Printf("IMAIM disconnect")
		s.removeAllHandlers()
	default:
		log.Printf("IMAIM unhandled command %d", command)
	}
	return nil
}

func (s *Session) removeAllHandlers() {
	for _, remove := range s.handlers {
		remove()
	}
	s.handlers = nil
}

func (s *Session) handleDirectMessageFromDevice(args []string) error {
	if len(args) < 2 {
		return nil
	}
	recipient := args[0]
	text := args[1]

	if strings.EqualFold(strings.ReplaceAll(recipient, " ", ""), channelBotName) {
		return s.handleChannelBotMessage(text)
	}

	log.Printf("IMAIM DM to %q ignored (no handler)", recipient)
	return nil
}

func (s *Session) handleChannelBotMessage(text string) error {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)

	switch lower {
	case "list":
		return s.sendChannelList()
	case "reset":
		return s.resetChannels()
	case "all":
		return s.subscribeAll()
	default:
		return s.subscribeByNumbers(text)
	}
}

func (s *Session) handleChatMessageFromDevice(args []string) error {
	if len(args) < 2 {
		return nil
	}
	roomName := args[0]
	text := s.screenName + ": " + args[1]

	channelID, ok := s.discordChannels[roomName]
	if !ok {
		log.Printf("Unknown chat room %q", roomName)
		return nil
	}

	_, err := s.discord.ChannelMessageSend(channelID, text)
	if err != nil {
		log.Printf("Discord send error: %v", err)
	}

	// Echo back so the sent message appears in the chat UI
	if err := s.sendChatMessage(roomName, s.screenName, args[1]); err != nil {
		log.Printf("Echo send error: %v", err)
	}

	return nil
}

func (s *Session) handleChatLeave(_ []string) error {
	log.Printf("IMAIM chat leave")
	s.removeAllHandlers()
	return nil
}

func (s *Session) aimLoginWithDiscord() error {
	s.removeAllHandlers()

	if err := s.sendIMAIMLogonInfo(s.screenName); err != nil {
		return err
	}

	guildChannels, err := s.discord.GuildChannels(s.discordGuild)
	if err != nil {
		log.Printf("Discord fetch guild channels error: %v", err)
		return nil
	}

	s.availableChannels = nil
	for _, ch := range guildChannels {
		if ch.Type == discordgo.ChannelTypeGuildText {
			s.availableChannels = append(s.availableChannels, ch)
		}
	}

	s.discordChannels = make(map[string]string)
	s.subscribedChannels = make(map[string]bool)

	return s.sendChannelList()
}

func (s *Session) sendChannelList() error {
	if len(s.availableChannels) == 0 {
		return s.sendDirectMessage(channelBotName, "No channels available.")
	}

	var b strings.Builder
	b.WriteString("Channels:\n")
	for i, ch := range s.availableChannels {
		marker := " "
		if s.subscribedChannels[ch.ID] {
			marker = "*"
		}
		fmt.Fprintf(&b, "%s%d. #%s\n", marker, i+1, ch.Name)
	}
	b.WriteString("\nReply with numbers to join (e.g. 1,3,5).\n")
	b.WriteString("Commands: list, reset, all")

	return s.sendDirectMessage(channelBotName, b.String())
}

func (s *Session) subscribeByNumbers(text string) error {
	text = strings.ReplaceAll(text, ",", " ")
	parts := strings.Fields(text)

	var toJoin []*discordgo.Channel
	var invalid []string
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > len(s.availableChannels) {
			invalid = append(invalid, p)
			continue
		}
		ch := s.availableChannels[n-1]
		if s.subscribedChannels[ch.ID] {
			continue
		}
		toJoin = append(toJoin, ch)
	}

	if len(toJoin) == 0 && len(invalid) > 0 {
		return s.sendDirectMessage(channelBotName,
			fmt.Sprintf("Invalid input: %s\nSend numbers (e.g. 1,3,5), list, reset, or all.", strings.Join(invalid, ", ")))
	}

	if len(toJoin) == 0 {
		return s.sendDirectMessage(channelBotName, "Those channels are already joined. Send 'list' to see all channels.")
	}

	if err := s.subscribeToChannels(toJoin); err != nil {
		return err
	}

	var names []string
	for _, ch := range toJoin {
		names = append(names, "#"+ch.Name)
	}
	msg := fmt.Sprintf("Joined: %s", strings.Join(names, ", "))
	if len(invalid) > 0 {
		msg += fmt.Sprintf("\nSkipped invalid: %s", strings.Join(invalid, ", "))
	}
	return s.sendDirectMessage(channelBotName, msg)
}

func (s *Session) subscribeAll() error {
	var toJoin []*discordgo.Channel
	for _, ch := range s.availableChannels {
		if !s.subscribedChannels[ch.ID] {
			toJoin = append(toJoin, ch)
		}
	}

	if len(toJoin) == 0 {
		return s.sendDirectMessage(channelBotName, "Already joined all channels.")
	}

	if err := s.subscribeToChannels(toJoin); err != nil {
		return err
	}

	return s.sendDirectMessage(channelBotName,
		fmt.Sprintf("Joined all %d channels.", len(toJoin)))
}

func (s *Session) resetChannels() error {
	s.removeAllHandlers()
	s.discordChannels = make(map[string]string)
	s.subscribedChannels = make(map[string]bool)

	guildChannels, err := s.discord.GuildChannels(s.discordGuild)
	if err != nil {
		log.Printf("Discord fetch guild channels error: %v", err)
		return s.sendDirectMessage(channelBotName, "Error refreshing channel list.")
	}

	s.availableChannels = nil
	for _, ch := range guildChannels {
		if ch.Type == discordgo.ChannelTypeGuildText {
			s.availableChannels = append(s.availableChannels, ch)
		}
	}

	if err := s.sendDirectMessage(channelBotName, "All channels cleared."); err != nil {
		return err
	}
	return s.sendChannelList()
}

func (s *Session) subscribeToChannels(channels []*discordgo.Channel) error {
	for _, ch := range channels {
		roomName := "#" + ch.Name
		s.discordChannels[roomName] = ch.ID
		s.subscribedChannels[ch.ID] = true

		if err := s.sendChatJoin(roomName); err != nil {
			return err
		}

		messages, err := s.discord.ChannelMessages(ch.ID, 50, "", "", "")
		if err != nil {
			log.Printf("Discord fetch messages error for %s: %v", roomName, err)
			continue
		}

		seen := map[string]bool{}
		var members []string
		for _, m := range messages {
			if seen[m.Author.Username] {
				continue
			}
			seen[m.Author.Username] = true
			members = append(members, m.Author.Username)
		}

		if err := s.sendChatMembers(roomName, "1", members); err != nil {
			return err
		}

		sort.Slice(messages, func(i, j int) bool {
			return messages[i].Timestamp.Before(messages[j].Timestamp)
		})

		for _, m := range messages {
			if err := s.sendChatMessage(roomName, m.Author.Username, m.Content); err != nil {
				return err
			}
		}
	}

	s.removeAllHandlers()
	s.registerDiscordHandler()

	return nil
}

func (s *Session) registerDiscordHandler() {
	if len(s.discordChannels) == 0 {
		return
	}

	reverseMap := make(map[string]string, len(s.discordChannels))
	for roomName, channelID := range s.discordChannels {
		reverseMap[channelID] = roomName
	}

	remove := s.discord.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		select {
		case <-s.done:
			return
		default:
		}
		if m.Author.ID == s.discord.State.User.ID {
			return
		}
		roomName, ok := reverseMap[m.ChannelID]
		if !ok {
			return
		}
		if err := s.sendChatMessage(roomName, m.Author.Username, m.Content); err != nil {
			log.Printf("Discord real-time send error: %v", err)
		}
	})
	s.handlers = append(s.handlers, remove)
}

func (s *Session) sendIMAIMLogonInfo(screenName string) error {
	// logonName, numGroups, [groupName, numBuddies, [displayName, nickname, extra]...],
	// numBlocked, numStatuses, [buddyName, status, deviceType, idleTime, iconChecksum, customMessage]...
	resp := proto.Marshal(&proto.IMMessage{
		Command: imCmdLogonInfo,
		Args: []string{
			screenName,
			"1",
			"Bots", "1",
			channelBotName, "", "",
			"0",
			"1",
			channelBotName, "0", "2", "0", "", "",
		},
	})
	log.Printf("Sent IMAIM LOGON_INFO for %q (with %s buddy)", screenName, channelBotName)
	return s.send(5, proto.ClassIMAIM, resp)
}

func (s *Session) sendDirectMessage(sender, text string) error {
	resp := proto.Marshal(&proto.IMMessage{
		Command: imCmdMessage,
		Args:    []string{sender, text, "0"},
	})
	log.Printf("Sent IMAIM DM from %q: %q", sender, text)
	return s.send(5, proto.ClassIMAIM, resp)
}

func (s *Session) sendChatJoin(roomName string) error {
	resp := proto.Marshal(&proto.IMMessage{
		Command: imCmdChatJoin,
		Args:    []string{roomName},
	})
	log.Printf("Sent IMAIM CHAT_JOIN for room %q", roomName)
	return s.send(5, proto.ClassIMAIM, resp)
}

func (s *Session) sendChatMembers(roomName string, direction string, members []string) error {
	args := make([]string, 0, 2+len(members))
	args = append(args, roomName, direction)
	args = append(args, members...)

	resp := proto.Marshal(&proto.IMMessage{
		Command: imCmdChatMembers,
		Args:    args,
	})
	log.Printf("Sent IMAIM CHAT_MEMBERS for room %q: %d members", roomName, len(members))
	return s.send(5, proto.ClassIMAIM, resp)
}

func (s *Session) sendChatMessage(roomName, sender, text string) error {
	resp := proto.Marshal(&proto.IMMessage{
		Command: imCmdChatMessage,
		Args:    []string{roomName, sender, text, "0"},
	})
	log.Printf("Sent IMAIM CHAT_MESSAGE in %q from %q: %q", roomName, sender, text)
	return s.send(5, proto.ClassIMAIM, resp)
}

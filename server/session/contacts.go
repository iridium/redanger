package session

import (
	"log"
	"server/proto"
)

var demoContacts []proto.Contact

func init() {
	type contactDef struct {
		uid       int32
		firstName string
		lastName  string
		nickName  string
		phone     string
		email     string
		im        string
	}

	defs := []contactDef{
		{1, "Andy", "Iridium", "iridium", "555-0001", "andy@redanger.net", "iridium"},
		{2, "Nicole", "Rawr", "nicole_rawr", "555-0002", "nicole@redanger.net", "nicole_rawr"},
		{3, "Jecka", "Xo", "jecka_xo", "555-0003", "jecka@redanger.net", "jecka_xo"},
	}

	for _, d := range defs {
		demoContacts = append(demoContacts, proto.Contact{
			UID:       d.uid,
			FirstName: d.firstName,
			LastName:  d.lastName,
			NickName:  d.nickName,
			Phones:    []proto.ContactPhone{{Label: "Mobile", Number: d.phone}},
			Emails:    []proto.ContactEmail{{Label: "Home", Address: d.email}},
			IMs:       []proto.ContactIM{{Label: "AIM", Account: d.im}},
		})
	}
}

const contactsProtocolVersion int32 = 1

func (s *Session) sendContacts() error {
	vp := proto.Marshal(&proto.ContactsVersionPref{ProtocolVersion: contactsProtocolVersion})
	if err := s.send(1, proto.ClassContactsTransaction, vp); err != nil {
		return err
	}
	log.Printf("Sent ContactsTransaction VERSION_PREF (version=%d)", contactsProtocolVersion)

	msg := &proto.ContactsUpdateAll{
		ProtocolVersion: contactsProtocolVersion,
		Categories:      []proto.LabelDef{{ID: 1, Name: "Unfiled"}},
		Labels: []proto.LabelDef{
			{ID: 1, Name: "Mobile"},
			{ID: 2, Name: "Home"},
			{ID: 3, Name: "Work"},
			{ID: 4, Name: "AIM"},
		},
		IMServices: []proto.LabelDef{{ID: 1, Name: "AIM"}},
		Contacts:   demoContacts,
	}
	b := proto.Marshal(msg)
	if err := s.send(1, proto.ClassContactsTransaction, b); err != nil {
		return err
	}
	log.Printf("Sent ContactsTransaction UPDATE_ALL (%d contacts, v%d)", len(demoContacts), contactsProtocolVersion)

	return nil
}

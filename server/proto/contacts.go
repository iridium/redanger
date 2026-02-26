package proto

// Contact types for the Danger Contacts protocol (classID 7).
//
// The wire format is a SyncObjectTransaction: int(command) + command-specific
// payload. ProtocolVersion controls sub-record formats (v1 uses int ringtones,
// v2+ uses string ringtones).

type ContactPhone struct {
	Label  string
	Number string
}

type ContactEmail struct {
	Label   string
	Address string
}

type ContactIM struct {
	Label   string
	Account string
}

type ContactAddress struct {
	Label   string
	Street  string
	City    string
	State   string
	Zip     string
	Country string
}

type ContactURL struct {
	Label string
	URL   string
}

type Contact struct {
	UID       int32
	FirstName string
	LastName  string
	NickName  string
	Icon      int32 // 0 = silhouette, >0 = gallery icon index
	Phones    []ContactPhone
	Emails    []ContactEmail
	IMs       []ContactIM
	Addresses []ContactAddress
	URLs      []ContactURL
}

type LabelDef struct {
	ID   int32
	Name string
}

type ContactsVersionPref struct {
	ProtocolVersion int32
}

func (m *ContactsVersionPref) Encode(b *Body) {
	b.WriteInt(-1000)
	b.WriteInt(m.ProtocolVersion)
}

func (m *ContactsVersionPref) Decode(r *Reader) error {
	var err error
	m.ProtocolVersion, err = r.ReadInt()
	return err
}

type ContactsUpdateAll struct {
	ProtocolVersion int32
	Categories      []LabelDef
	Labels          []LabelDef
	IMServices      []LabelDef
	Contacts        []Contact
}

func (m *ContactsUpdateAll) Encode(b *Body) {
	b.WriteInt(-1002)

	b.WriteInt(int32(len(m.Categories)))
	for _, c := range m.Categories {
		b.WriteInt(c.ID)
		b.WriteString(c.Name)
	}

	b.WriteInt(int32(len(m.Labels)))
	for _, l := range m.Labels {
		b.WriteInt(l.ID)
		b.WriteString(l.Name)
	}

	b.WriteInt(int32(len(m.IMServices)))
	for _, s := range m.IMServices {
		b.WriteInt(s.ID)
		b.WriteString(s.Name)
	}

	b.WriteInt(int32(len(m.Contacts)))
	for _, c := range m.Contacts {
		writeContact(b, &c, m.ProtocolVersion)
	}
}

func writeContact(b *Body, c *Contact, version int32) {
	b.WriteInt(c.UID) // uid (read by SyncObjectStore.readObject)
	b.WriteInt(0)     // deviceLastMod
	b.WriteInt(0)     // serviceLastMod
	b.WriteInt(0)     // categoryID
	b.WriteString(c.FirstName)
	b.WriteString(c.LastName)
	b.WriteString("") // middleName
	b.WriteString(c.NickName)
	b.WriteString("") // company
	b.WriteString("") // title
	b.WriteInt(0)     // birthYear
	b.WriteInt(0)     // birthMonth
	b.WriteInt(0)     // birthDate
	b.WriteString("") // note
	b.WriteInt(0)     // dangerID
	b.WriteString("") // dangerName
	b.WriteString("") // dangerDomain
	b.WriteInt(0)     // flags
	b.WriteString("") // callingCardNotes
	b.WriteString("") // callingCardTagLine
	b.WriteInt(0)     // cardLastMod
	b.WriteInt(c.Icon)

	// Addresses
	b.WriteInt(int32(len(c.Addresses)))
	for i, a := range c.Addresses {
		b.WriteInt(int32(i + 1))
		b.WriteString(a.Label)
		b.WriteString(a.Street)
		b.WriteString(a.City)
		b.WriteString(a.State)
		b.WriteString(a.Zip)
		b.WriteString(a.Country)
	}

	// Emails
	b.WriteInt(int32(len(c.Emails)))
	for i, e := range c.Emails {
		b.WriteInt(int32(i + 1))
		b.WriteString(e.Label)
		b.WriteString(e.Address)
	}

	// IMs
	b.WriteInt(int32(len(c.IMs)))
	for i, im := range c.IMs {
		b.WriteInt(int32(i + 1))
		b.WriteString(im.Label)
		b.WriteString(im.Account)
	}

	// Phone format depends on protocol version
	b.WriteInt(int32(len(c.Phones)))
	for i, p := range c.Phones {
		b.WriteInt(int32(i + 1))
		b.WriteString(p.Label)
		b.WriteString(p.Number)
		if version > 1 {
			b.WriteString("") // phone ringtone (empty = none)
			b.WriteString("") // SMS ringtone (empty = none)
		} else {
			b.WriteInt(0) // phone ringtone (0 = none)
			b.WriteInt(0) // SMS ringtone (0 = none)
		}
	}

	// URLs
	b.WriteInt(int32(len(c.URLs)))
	for i, u := range c.URLs {
		b.WriteInt(int32(i + 1))
		b.WriteString(u.Label)
		b.WriteString(u.URL)
	}
}

type ContactsCommand struct {
	Command int32
}

func (m *ContactsCommand) Decode(r *Reader) error {
	var err error
	m.Command, err = r.ReadInt()
	return err
}

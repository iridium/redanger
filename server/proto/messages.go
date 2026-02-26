package proto

type Ack struct {
	HighestSequence int16
}

type Ping struct{}

type Pong struct {
	Payload string
}

type IdentityCheck struct {
	SIMIDType    int16
	SIMID        string
	DeviceIDType string
	DeviceID     string
	PartnerID    int32
}

type IdentityCheckResult struct {
	Action      int16
	CurrentUser string
}

type ServiceLoginResult struct {
	Result         int16
	Explanation    string
	NewLocationKey []byte
}

type ServiceEventLog struct {
	Date     int32
	Category string
	Message  string
}

type CellUpdate struct {
	Operator string
	LAC      string
	CID      string
}

type ServiceLogin struct {
	LoginType    int16
	Username     string
	SIMIDType    int16
	Password     string
	SIMID        string
	DeviceIDType string
	DeviceID     string
	LocationKey  []byte
	PartnerID    int32
}

func (m *ServiceLogin) Decode(r *Reader) error {
	var err error
	if m.LoginType, err = r.ReadShort(); err != nil {
		return err
	}
	if m.LoginType == 1 {
		if m.Username, err = r.ReadString(); err != nil {
			return err
		}
	}
	if m.SIMIDType, err = r.ReadShort(); err != nil {
		return err
	}
	if m.Password, err = r.ReadString(); err != nil {
		return err
	}
	if m.SIMID, err = r.ReadString(); err != nil {
		return err
	}
	if m.DeviceIDType, err = r.ReadString(); err != nil {
		return err
	}
	if m.DeviceID, err = r.ReadString(); err != nil {
		return err
	}
	if m.LocationKey, err = r.ReadBinary(); err != nil {
		return err
	}
	m.PartnerID, err = r.ReadInt()
	return err
}

func (m *ServiceLogin) Encode(b *Body) {
	b.WriteShort(m.LoginType)
	if m.LoginType == 1 {
		b.WriteString(m.Username)
	}
	b.WriteShort(m.SIMIDType)
	b.WriteString(m.Password)
	b.WriteString(m.SIMID)
	b.WriteString(m.DeviceIDType)
	b.WriteString(m.DeviceID)
	b.WriteBinary(m.LocationKey)
	b.WriteInt(m.PartnerID)
}

type DropBoxInfo struct {
	Flags    int16
	LastSent int16
	LastRecv int16
}

func (m *DropBoxInfo) Decode(r *Reader) error {
	var err error
	if m.Flags, err = r.ReadShort(); err != nil {
		return err
	}
	if m.Flags != 0 {
		if m.LastSent, err = r.ReadShort(); err != nil {
			return err
		}
		m.LastRecv, err = r.ReadShort()
		return err
	}
	return nil
}

func (m *DropBoxInfo) Encode(b *Body) {
	b.WriteShort(m.Flags)
	if m.Flags != 0 {
		b.WriteShort(m.LastSent)
		b.WriteShort(m.LastRecv)
	}
}

type ServiceTime struct {
	Timestamp int32
}

type ServiceTimeSettings struct {
	Command         int32
	ProtocolVersion int32 // for command -1000
	TimeZoneID      int32 // for command -400
	TimeFormat      int32 // for command -400 (1=12h, 0=24h)
}

func (m *ServiceTimeSettings) Decode(r *Reader) error {
	var err error
	if m.Command, err = r.ReadInt(); err != nil {
		return err
	}
	switch m.Command {
	case -1000: // VERSION_PREF
		m.ProtocolVersion, err = r.ReadInt()
		return err
	case -400: // SETTINGS
		if m.TimeZoneID, err = r.ReadInt(); err != nil {
			return err
		}
		m.TimeFormat, err = r.ReadInt()
		return err
	}
	return nil
}

func (m *ServiceTimeSettings) Encode(b *Body) {
	b.WriteInt(m.Command)
	switch m.Command {
	case -1000: // VERSION_PREF
		b.WriteInt(m.ProtocolVersion)
	case -400: // SETTINGS
		b.WriteInt(m.TimeZoneID)
		b.WriteInt(m.TimeFormat)
	}
}

type IMMessage struct {
	Command int32
	Args    []string
}

func (m *IMMessage) Decode(r *Reader) error {
	var err error
	if m.Command, err = r.ReadInt(); err != nil {
		return err
	}
	count, err := r.ReadInt()
	if err != nil {
		return err
	}
	m.Args = make([]string, 0, count)
	for range count {
		s, err := r.ReadString()
		if err != nil {
			return err
		}
		m.Args = append(m.Args, s)
	}
	return nil
}

func (m *IMMessage) Encode(b *Body) {
	b.WriteInt(m.Command)
	b.WriteInt(int32(len(m.Args)))
	for _, s := range m.Args {
		b.WriteString(s)
	}
}

type ServiceInfo struct {
	Settings map[string]string
}

func (m *ServiceInfo) Encode(b *Body) {
	b.WriteShort(int16(len(m.Settings)))
	for k, v := range m.Settings {
		b.WriteString(k)
		b.WriteString(v)
	}
}

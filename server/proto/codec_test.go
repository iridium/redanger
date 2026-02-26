package proto

import (
	"reflect"
	"testing"
)

func roundTrip[T any](t *testing.T, orig *T) T {
	t.Helper()
	body := Marshal(orig)
	r := NewBodyReader(body.Bytes())
	var got T
	if err := Unmarshal(r, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	return got
}

func TestAck(t *testing.T) {
	orig := &Ack{HighestSequence: 42}
	got := roundTrip(t, orig)
	if got.HighestSequence != 42 {
		t.Errorf("HighestSequence = %d, want 42", got.HighestSequence)
	}
}

func TestPing(t *testing.T) {
	orig := &Ping{}
	body := Marshal(orig)
	if body.Len() != 0 {
		t.Errorf("Ping body length = %d, want 0", body.Len())
	}
}

func TestPong(t *testing.T) {
	orig := &Pong{Payload: "hello world"}
	got := roundTrip(t, orig)
	if got.Payload != "hello world" {
		t.Errorf("Payload = %q, want %q", got.Payload, "hello world")
	}
}

func TestPongEmpty(t *testing.T) {
	orig := &Pong{Payload: ""}
	got := roundTrip(t, orig)
	if got.Payload != "" {
		t.Errorf("Payload = %q, want empty", got.Payload)
	}
}

func TestIdentityCheck(t *testing.T) {
	orig := &IdentityCheck{
		SIMIDType:    1,
		SIMID:        "sim123",
		DeviceIDType: "IMEI",
		DeviceID:     "dev456",
		PartnerID:    99,
	}
	got := roundTrip(t, orig)
	if got != *orig {
		t.Errorf("got %+v, want %+v", got, *orig)
	}
}

func TestIdentityCheckResult(t *testing.T) {
	orig := &IdentityCheckResult{Action: 0, CurrentUser: "user"}
	got := roundTrip(t, orig)
	if got != *orig {
		t.Errorf("got %+v, want %+v", got, *orig)
	}
}

func TestServiceLoginResult(t *testing.T) {
	orig := &ServiceLoginResult{
		Result:         0,
		Explanation:    "Welcome",
		NewLocationKey: []byte{0xDE, 0xAD, 0xBE, 0xEF},
	}
	got := roundTrip(t, orig)
	if got.Result != orig.Result || got.Explanation != orig.Explanation {
		t.Errorf("scalar fields mismatch: got %+v, want %+v", got, *orig)
	}
	if !reflect.DeepEqual(got.NewLocationKey, orig.NewLocationKey) {
		t.Errorf("NewLocationKey = %x, want %x", got.NewLocationKey, orig.NewLocationKey)
	}
}

func TestServiceLoginResultEmptyKey(t *testing.T) {
	orig := &ServiceLoginResult{Result: 0, Explanation: "ok"}
	got := roundTrip(t, orig)
	if got.NewLocationKey != nil {
		t.Errorf("NewLocationKey = %x, want nil", got.NewLocationKey)
	}
}

func TestServiceEventLog(t *testing.T) {
	orig := &ServiceEventLog{Date: 1700000000, Category: "system", Message: "boot complete"}
	got := roundTrip(t, orig)
	if got != *orig {
		t.Errorf("got %+v, want %+v", got, *orig)
	}
}

func TestCellUpdate(t *testing.T) {
	orig := &CellUpdate{Operator: "T-Mobile", LAC: "1234", CID: "5678"}
	got := roundTrip(t, orig)
	if got != *orig {
		t.Errorf("got %+v, want %+v", got, *orig)
	}
}

func TestServiceLoginWithUsername(t *testing.T) {
	orig := &ServiceLogin{
		LoginType:    1,
		Username:     "admin",
		SIMIDType:    2,
		Password:     "secret",
		SIMID:        "sim789",
		DeviceIDType: "ESN",
		DeviceID:     "devABC",
		LocationKey:  []byte{0x01, 0x02},
		PartnerID:    50,
	}
	got := roundTrip(t, orig)
	if got.LoginType != orig.LoginType || got.Username != orig.Username ||
		got.SIMIDType != orig.SIMIDType || got.Password != orig.Password ||
		got.SIMID != orig.SIMID || got.DeviceIDType != orig.DeviceIDType ||
		got.DeviceID != orig.DeviceID || got.PartnerID != orig.PartnerID {
		t.Errorf("got %+v, want %+v", got, *orig)
	}
	if !reflect.DeepEqual(got.LocationKey, orig.LocationKey) {
		t.Errorf("LocationKey = %x, want %x", got.LocationKey, orig.LocationKey)
	}
}

func TestServiceLoginWithoutUsername(t *testing.T) {
	orig := &ServiceLogin{
		LoginType:    0,
		SIMIDType:    2,
		Password:     "secret",
		SIMID:        "sim789",
		DeviceIDType: "ESN",
		DeviceID:     "devABC",
		PartnerID:    50,
	}
	got := roundTrip(t, orig)
	if got.LoginType != 0 || got.Username != "" {
		t.Errorf("LoginType=%d Username=%q, want 0 and empty", got.LoginType, got.Username)
	}
	if got.SIMIDType != orig.SIMIDType || got.Password != orig.Password {
		t.Errorf("got %+v, want %+v", got, *orig)
	}
}

func TestDropBoxInfoWithSequence(t *testing.T) {
	orig := &DropBoxInfo{Flags: 1, LastSent: 10, LastRecv: 20}
	got := roundTrip(t, orig)
	if got != *orig {
		t.Errorf("got %+v, want %+v", got, *orig)
	}
}

func TestDropBoxInfoNoSequence(t *testing.T) {
	orig := &DropBoxInfo{Flags: 0}
	got := roundTrip(t, orig)
	if got.Flags != 0 || got.LastSent != 0 || got.LastRecv != 0 {
		t.Errorf("got %+v, want zeroed", got)
	}
}

func TestIMMessage(t *testing.T) {
	orig := &IMMessage{Command: -534, Args: []string{"#general", "alice", "hello", "0"}}
	got := roundTrip(t, orig)
	if got.Command != orig.Command {
		t.Errorf("Command = %d, want %d", got.Command, orig.Command)
	}
	if !reflect.DeepEqual(got.Args, orig.Args) {
		t.Errorf("Args = %q, want %q", got.Args, orig.Args)
	}
}

func TestIMMessageNoArgs(t *testing.T) {
	orig := &IMMessage{Command: -503, Args: []string{}}
	got := roundTrip(t, orig)
	if got.Command != -503 {
		t.Errorf("Command = %d, want -503", got.Command)
	}
	if len(got.Args) != 0 {
		t.Errorf("Args = %q, want empty", got.Args)
	}
}

func TestServiceTime(t *testing.T) {
	orig := &ServiceTime{Timestamp: 1700000000}
	got := roundTrip(t, orig)
	if got.Timestamp != orig.Timestamp {
		t.Errorf("Timestamp = %d, want %d", got.Timestamp, orig.Timestamp)
	}
}

func TestServiceTimeSettingsVersionPref(t *testing.T) {
	orig := &ServiceTimeSettings{Command: -1000, ProtocolVersion: 2}
	got := roundTrip(t, orig)
	if got.Command != -1000 {
		t.Errorf("Command = %d, want -1000", got.Command)
	}
	if got.ProtocolVersion != 2 {
		t.Errorf("ProtocolVersion = %d, want 2", got.ProtocolVersion)
	}
}

func TestServiceTimeSettingsSettings(t *testing.T) {
	orig := &ServiceTimeSettings{Command: -400, TimeZoneID: 42, TimeFormat: 1}
	got := roundTrip(t, orig)
	if got.Command != -400 {
		t.Errorf("Command = %d, want -400", got.Command)
	}
	if got.TimeZoneID != 42 {
		t.Errorf("TimeZoneID = %d, want 42", got.TimeZoneID)
	}
	if got.TimeFormat != 1 {
		t.Errorf("TimeFormat = %d, want 1", got.TimeFormat)
	}
}

func TestContactsVersionPref(t *testing.T) {
	orig := &ContactsVersionPref{ProtocolVersion: 3}
	body := Marshal(orig)
	r := NewBodyReader(body.Bytes())

	// Should encode as: int(-1000) + int(3)
	cmd, err := r.ReadInt()
	if err != nil {
		t.Fatal(err)
	}
	if cmd != -1000 {
		t.Errorf("command = %d, want -1000", cmd)
	}
	ver, err := r.ReadInt()
	if err != nil {
		t.Fatal(err)
	}
	if ver != 3 {
		t.Errorf("version = %d, want 3", ver)
	}
}

func TestContactsUpdateAllV1(t *testing.T) {
	msg := &ContactsUpdateAll{
		ProtocolVersion: 1,
		Categories:      []LabelDef{{ID: 1, Name: "Unfiled"}},
		Labels:          []LabelDef{{ID: 1, Name: "Mobile"}},
		IMServices:      []LabelDef{{ID: 1, Name: "AIM"}},
		Contacts: []Contact{{
			UID:       42,
			FirstName: "Alice",
			LastName:  "Test",
			NickName:  "alice",
			Icon:      0,
			Phones:    []ContactPhone{{Label: "Mobile", Number: "555-1234"}},
			Emails:    []ContactEmail{{Label: "Home", Address: "alice@test.com"}},
			IMs:       []ContactIM{{Label: "AIM", Account: "alice123"}},
		}},
	}
	body := Marshal(msg)
	r := NewBodyReader(body.Bytes())

	// Command
	cmd, _ := r.ReadInt()
	if cmd != -1002 {
		t.Fatalf("command = %d, want -1002", cmd)
	}

	// Categories
	catCount, _ := r.ReadInt()
	if catCount != 1 {
		t.Fatalf("categories = %d, want 1", catCount)
	}
	catID, _ := r.ReadInt()
	catName, _ := r.ReadString()
	if catID != 1 || catName != "Unfiled" {
		t.Errorf("category = (%d, %q), want (1, Unfiled)", catID, catName)
	}

	// Labels
	lblCount, _ := r.ReadInt()
	if lblCount != 1 {
		t.Fatalf("labels = %d, want 1", lblCount)
	}
	r.ReadInt()    // id
	r.ReadString() // name

	// IM services
	imSvcCount, _ := r.ReadInt()
	if imSvcCount != 1 {
		t.Fatalf("im services = %d, want 1", imSvcCount)
	}
	r.ReadInt()    // id
	r.ReadString() // name

	// Contacts
	contactCount, _ := r.ReadInt()
	if contactCount != 1 {
		t.Fatalf("contacts = %d, want 1", contactCount)
	}

	// Read through the contact fields to verify no stream misalignment
	uid, _ := r.ReadInt() // uid
	if uid != 42 {
		t.Errorf("uid = %d, want 42", uid)
	}
	r.ReadInt() // deviceLastMod
	r.ReadInt() // serviceLastMod
	r.ReadInt() // categoryID
	firstName, _ := r.ReadString()
	if firstName != "Alice" {
		t.Errorf("firstName = %q, want Alice", firstName)
	}
	r.ReadString() // lastName
	r.ReadString() // middleName
	r.ReadString() // nickName
	r.ReadString() // company
	r.ReadString() // title
	r.ReadInt()    // birthYear
	r.ReadInt()    // birthMonth
	r.ReadInt()    // birthDate
	r.ReadString() // note
	r.ReadInt()    // dangerID
	r.ReadString() // dangerName
	r.ReadString() // dangerDomain
	r.ReadInt()    // flags
	r.ReadString() // callingCardNotes
	r.ReadString() // callingCardTagLine
	r.ReadInt()    // cardLastMod
	icon, _ := r.ReadInt()
	if icon != 0 {
		t.Errorf("icon = %d, want 0", icon)
	}

	// Addresses
	addrCount, _ := r.ReadInt()
	if addrCount != 0 {
		t.Errorf("addresses = %d, want 0", addrCount)
	}

	// Emails
	emailCount, _ := r.ReadInt()
	if emailCount != 1 {
		t.Fatalf("emails = %d, want 1", emailCount)
	}
	r.ReadInt()    // sub-uid
	r.ReadString() // label
	emailAddr, _ := r.ReadString()
	if emailAddr != "alice@test.com" {
		t.Errorf("email = %q, want alice@test.com", emailAddr)
	}

	// IMs
	imCount, _ := r.ReadInt()
	if imCount != 1 {
		t.Fatalf("ims = %d, want 1", imCount)
	}
	r.ReadInt()    // sub-uid
	r.ReadString() // label
	r.ReadString() // account

	// Phones (v1 = int ringtones)
	phoneCount, _ := r.ReadInt()
	if phoneCount != 1 {
		t.Fatalf("phones = %d, want 1", phoneCount)
	}
	r.ReadInt()    // sub-uid
	r.ReadString() // label
	phoneNum, _ := r.ReadString()
	if phoneNum != "555-1234" {
		t.Errorf("phone = %q, want 555-1234", phoneNum)
	}
	phoneRT, _ := r.ReadInt() // v1: int ringtone
	if phoneRT != 0 {
		t.Errorf("phone ringtone = %d, want 0", phoneRT)
	}
	smsRT, _ := r.ReadInt() // v1: int SMS ringtone
	if smsRT != 0 {
		t.Errorf("sms ringtone = %d, want 0", smsRT)
	}

	// URLs
	urlCount, _ := r.ReadInt()
	if urlCount != 0 {
		t.Errorf("urls = %d, want 0", urlCount)
	}

	// No custom icon in v1, so the stream should be exhausted
	if r.Buffered() != 0 {
		t.Errorf("stream has %d extra bytes (should be empty for v1)", r.Buffered())
	}
}

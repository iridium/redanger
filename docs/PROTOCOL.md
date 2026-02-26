# Danger Service Network Protocol Specification

## 1. Overview

This document describes the binary protocol used for communication between the Danger Hiptop (T-Mobile Sidekick) device and the Danger service infrastructure. The specification is derived from decompiled simulator source code and is intended to be sufficient for a clean-room implementation.

- **Protocol version:** 7
- **Transport:** TCP stream
- **Byte order:** Big-endian throughout

### Source References

All paths are relative to `references/`.

| File                                                  | Purpose                                         |
| ----------------------------------------------------- | ----------------------------------------------- |
| `library/sources/danger/io/Service.java`              | Connection state machine, reader/writer threads |
| `library/sources/danger/io/ObjectInputStream.java`    | Deserialization, header parsing                 |
| `library/sources/danger/io/ObjectOutputStream.java`   | Serialization, header writing                   |
| `library/sources/danger/io/NetworkObject.java`        | Class ID registry, object dispatch              |
| `library/sources/danger/io/DropBox.java`              | Reliable delivery queue                         |
| `library/sources/danger/io/Response.java`             | Response matching                               |
| `library/sources/danger/io/ChunkedObjectHandler.java` | Large object chunking                           |
| `library/sources/danger/io/ServiceObjects/*.java`     | All message type definitions                    |
| `library/sources/danger/net/FunnelTransaction.java`   | Application-level multiplexing                  |
| `simulator/apps/registration/`                        | Authentication flow examples                    |

---

## 2. Connection Establishment

### 2.1 Initial Handshake

Immediately after establishing a TCP connection, the client writes:

```
short(7)              — protocol version
string(locale)        — device locale (e.g. "en_US")
```

Where `string` uses the variable-length size encoding described in Section 3.1.

Source: `Service.connect()` (line ~1091):

```java
mOut.writeShort((short) 7);
mOut.writeString(Settings.getLocale().toString());
```

### 2.2 Redirector Flow

The device does not initially know which server to connect to. The connection sequence is:

1. Connect to the **redirector** service (port 5001)
2. Perform the initial handshake (version + locale)
3. Send a `SessionLocationCheck` object (classID 64)
4. Receive a `SessionLocationResult` object (classID 65) containing:
   - Server IP address
   - Server port
   - Encryption flag
   - Retry count
5. Close the redirector connection
6. Reconnect to the real server using the provided IP/port
7. Perform the initial handshake again on the new connection

---

## 3. Object Serialization Wire Format

### 3.1 Primitive Types

All multi-byte integers are **big-endian**.

| Type    | Size    | Encoding                                                     |
| ------- | ------- | ------------------------------------------------------------ |
| `byte`  | 1 byte  | Signed 8-bit                                                 |
| `char`  | 2 bytes | Unsigned 16-bit: `(octet << 8) \| octet`                     |
| `short` | 2 bytes | Signed 16-bit: `(octet << 8) \| octet`                       |
| `int`   | 4 bytes | Signed 32-bit: `(b0 << 24) \| (b1 << 16) \| (b2 << 8) \| b3` |
| `long`  | 8 bytes | Signed 64-bit                                                |

### 3.2 Variable-Length Size Encoding

The `size` type is used as a length prefix for strings and binary data. It uses a 7-bit variable-length encoding with continuation bits.

#### Writing (`ObjectOutputStream.writeSize`, lines 76-111)

```
if value < 128:
    write single byte: value

otherwise:
    buffer = []
    i = 0
    do:
        buffer[i] = value & 0x7F
        if i > 0:
            buffer[i] |= 0x80     // set continuation bit on non-first bytes
        i++
        value >>= 7
    while value > 0

    // write buffer in REVERSE order (MSB first)
    for j = i-1 down to 0:
        write buffer[j]
```

The continuation bit (MSB, 0x80) indicates "more bytes follow". Bytes are written most-significant first. The final byte in the stream has the continuation bit clear.

#### Reading (`ObjectInputStream.readLengthHeader`, lines 88-96)

```
result = 0
do:
    octet = read_byte()
    result = (result << 7) | (octet & 0x7F)
while (octet & 0x80) != 0
return result
```

#### Examples

| Value | Encoded bytes    |
| ----- | ---------------- |
| 0     | `0x00`           |
| 127   | `0x7F`           |
| 128   | `0x81 0x00`      |
| 255   | `0x81 0x7F`      |
| 16384 | `0x81 0x80 0x00` |

### 3.3 String Encoding

```
size(byte_length) + UTF-8_bytes
```

- A null string is written as the empty string
- `size` is the byte length of the UTF-8 encoding, NOT the character count

### 3.4 Binary Encoding

```
size(byte_length) + raw_bytes
```

- A null byte array is written as `size(0)` (no payload bytes)

### 3.5 Object Header

Every serialized object begins with a fixed header. The header format differs based on whether the object carries a sequence number.

#### Header Layout

```
Byte 0: [version:4][bodyLength bits 26..23:4]
Byte 1: [bodyLength bits 22..15:8]
Byte 2: [bodyLength bits 14..7:8]
Byte 3: [bodyLength bits 6..0:7][hasSequence:1]
(if hasSequence == 1):
    Byte 4: [sequenceID high byte:8]
    Byte 5: [sequenceID low byte:8]
Next 2 bytes: [classID:16]
```

**Total header size:** 6 bytes (unsequenced) or 8 bytes (sequenced)

#### Field Descriptions

| Field         | Bits | Description                                                   |
| ------------- | ---- | ------------------------------------------------------------- |
| `version`     | 4    | Object class version (0-15)                                   |
| `bodyLength`  | 27   | Byte length of the body following the header                  |
| `hasSequence` | 1    | 1 if a sequence number follows, 0 otherwise                   |
| `sequenceID`  | 16   | Sequence number for reliable delivery (only if hasSequence=1) |
| `classID`     | 16   | Identifies the object type                                    |

#### Writing (`ObjectOutputStream.writeObject`, lines 152-219)

```java
// Reserve header space (6 or 8 bytes)
// Serialize body via storable.writeToStream()
// Calculate body size
size = totalBytes - headerSize;

// Write header bytes
byte0 = (classVersion << 4) | ((size >> 23) & 0x0F);
byte1 = (size >> 15) & 0xFF;
byte2 = (size >> 7) & 0xFF;
byte3 = ((size << 1) & 0xFE) | (hasSequence ? 1 : 0);

// If sequenced:
byte4 = (sequenceID >> 8) & 0xFF;
byte5 = sequenceID & 0xFF;

// Class ID:
byteN   = (classID >> 8) & 0xFF;
byteN+1 = classID & 0xFF;
```

#### Reading (`ObjectInputStream.readHeader`, lines 245-269)

```java
b0 = readOctet();
b1 = readOctet();
b2 = readOctet();
b3 = readOctet();

version     = (b0 >>> 4) & 0x0F;
bodyLength  = ((b0 & 0x0F) << 23) | (b1 << 15) | (b2 << 7) | ((b3 & 0xFE) >>> 1);
hasSequence = (b3 & 0x01) != 0;

if (hasSequence):
    sequenceID = (readOctet() << 8) | readOctet();

classID = (readOctet() << 8) | readOctet();
```

---

## 4. Storable Interface

Every serializable object implements the `Storable` interface:

```java
public interface Storable {
    void writeToStream(ObjectOutputStream out) throws IOException;
    void readFromStream(ObjectInputStream in, int version) throws IOException;
    int getClassID();
    int getClassVersion();
}
```

- `getClassID()` returns the 16-bit type identifier used in the object header
- `getClassVersion()` returns the version number packed into the header's 4-bit version field
- `readFromStream` receives the version from the header so implementations can handle version differences
- The body format (what `writeToStream`/`readFromStream` read/write) is specific to each class ID

---

## 5. Connection State Machine

The connection manager maintains 10 states:

| Value | Name                         | Description                                            |
| ----- | ---------------------------- | ------------------------------------------------------ |
| 0     | `IDLE`                       | No connection; waiting for login credentials           |
| 1     | `RECONNECTING`               | Attempting TCP connection                              |
| 2     | `REDIRECTING`                | Connected to redirector, awaiting server location      |
| 3     | `SOCKET_OPEN`                | TCP connected to real server, handshake complete       |
| 4     | `CHECKING_IDENTITY`          | Sent IdentityCheck, awaiting result                    |
| 5     | `CREATING_ACCOUNT`           | Sent CreateAccountInfo, awaiting result                |
| 6     | `LOGGING_IN`                 | Sent ServiceLogin, awaiting result                     |
| 7     | `RESYNCING_DROP_BOX`         | Sent DropBoxInfo, awaiting server DropBoxInfo          |
| 8     | `RESYNCING_SERVICE_DROP_BOX` | Waiting for server to finish replaying missed messages |
| 9     | `LOGGED_IN`                  | Fully connected, normal operation                      |

### State Transitions

```
IDLE(0) ──────────────────────────────> RECONNECTING(1)
                                            │
                        ┌───────────────────┤
                        ▼                   ▼
                  REDIRECTING(2)      SOCKET_OPEN(3)
                        │                   │
                        │           ┌───────┼───────────┐
                        │           ▼       ▼           ▼
                        │    CHECKING_   LOGGING_IN  CREATING_
                        │    IDENTITY(4)    (6)     ACCOUNT(5)
                        │           │       │
                        │           ▼       ▼
                        │      SOCKET_  RESYNCING_
                        │      OPEN(3)  DROP_BOX(7)
                        │                   │
                        │                   ▼
                        │           RESYNCING_SERVICE_
                        │           DROP_BOX(8)
                        │                   │
                        │                   ▼
                        └──────────> LOGGED_IN(9)
```

### Transition Details

- **IDLE -> RECONNECTING:** Triggered when credentials become available or reconnect is needed
- **RECONNECTING -> REDIRECTING:** When server/port is unknown (empty), connect to redirector
- **RECONNECTING -> SOCKET_OPEN:** When server/port is known, connect directly
- **REDIRECTING -> SOCKET_OPEN:** After receiving SessionLocationResult, reconnect to real server
- **SOCKET_OPEN -> CHECKING_IDENTITY:** If identity check is needed (`mNeedsIdentityCheck`)
- **SOCKET_OPEN -> LOGGING_IN:** If username is set (normal login flow)
- **CHECKING_IDENTITY -> SOCKET_OPEN:** After receiving IdentityCheckResult
- **LOGGING_IN -> RESYNCING_DROP_BOX:** On successful login (result == 0)
- **RESYNCING_DROP_BOX -> RESYNCING_SERVICE_DROP_BOX or LOGGED_IN:** After DropBoxInfo exchange
- **Any state -> IDLE:** On connection error/disconnect

---

## 6. Authentication Flow

The full login sequence, from cold start:

### Step 1: Redirector Phase

```
Client                          Redirector (port 5001)
  │                                  │
  ├──── TCP connect ────────────────>│
  ├──── short(7) + string(locale) ──>│
  ├──── [SessionLocationCheck] ─────>│
  │<─── [SessionLocationResult] ─────┤
  ├──── TCP close ──────────────────>│
```

### Step 2: Server Connection

```
Client                          Server (IP:port from step 1)
  │                                  │
  ├──── TCP connect ────────────────>│
  ├──── short(7) + string(locale) ──>│
```

### Step 3: Identity Check (optional)

```
  ├──── [IdentityCheck] ───────────>│
  │<─── [IdentityCheckResult] ──────┤
```

### Step 4: Crash Log (if applicable)

```
  ├──── [CrashLog] ────────────────>│   (written directly, not via DropBox)
```

### Step 5: Login

```
  ├──── [ServiceLogin] ────────────>│
  │<─── [ServiceLoginResult] ───────┤
```

### Step 6: System Version Info

```
  ├──── [SystemVersionInfo] ────────>│  (sent on successful login)
```

### Step 7: DropBox Synchronization

```
  ├──── [DropBoxInfo] ──────────────>│  (client's sequence state)
  │<─── [DropBoxInfo] ──────────────┤  (server's sequence state)
  │<─── [replayed objects...] ──────┤  (if any unacknowledged)
  ├──── [resent objects...] ────────>│  (if any unacknowledged)
```

### Step 8: Logged In

Connection transitions to `LOGGED_IN` state. Normal bidirectional message flow begins.

---

## 7. Reliability Layer

### 7.1 Sequence Numbers

- 16-bit unsigned values (stored as Java `short`, treated as unsigned with `& 0xFFFF`)
- Assigned by the client-side `DropBox` when an object is queued for sending
- Wrap around from 65535 to 0
- Sequence comparison uses signed difference: `(short)(a - b)` for wrap-safe ordering

### 7.2 DropBox

The `DropBox` is a client-side persistent queue that provides reliable delivery:

- **`add(object)`** — enqueues an object, returns the assigned sequence number
- **`ack(sequence)`** — removes all objects up to and including the given sequence
- **`resend(lastReceivedByServer)`** — re-enqueues all objects after the given sequence for retransmission
- **`find(sequence)`** — looks up an object by sequence number (used for response matching)

The DropBox is implemented as a doubly-linked list (`DropBoxEntry`) with a sentinel head node. Sequence numbers are tracked with `sFirstSequence` (sequence of the first entry) and `sNumEntries`.

### 7.3 Acknowledgments

- Object: `Ack` (classID 14)
- The client sends an Ack after receiving **10** sequenced messages from the server
- The Ack contains the highest sequence number received
- Acks are **not** sequenced themselves (no DropBox entry)

Source: `Service.sendAckIfNecessary()`:

```java
if ((lastSequenceReceived - mLastAckSequence) >= 10) {
    addObjectToWriteList(new Ack(lastSequenceReceived));
    mLastAckSequence = lastSequenceReceived;
}
```

### 7.4 DropBoxInfo Exchange

At login time, the client and server exchange `DropBoxInfo` (classID 47) to synchronize:

**Client sends:**

- `lastSequenceSent` — last sequence number assigned by client's DropBox
- `lastSequenceReceived` — last sequence number received from server

**Server responds with:**

- `lastSequenceSent` — last sequence the server sent (client uses this to know what to expect)
- `lastSequenceReceived` — last sequence the server received (client resends anything after this)

The client then:

1. Calls `mDropBox.resend(serverLastReceived)` — re-queues unacknowledged objects
2. Sets `mLastAckSequence = serverLastSent`
3. If `clientLastReceived >= serverLastSent` -> state = LOGGED_IN
4. Otherwise -> state = RESYNCING_SERVICE_DROP_BOX (wait for replayed messages)

#### DropBoxInfo Wire Format (version 2)

**Write:**

```
short(hasSequenceInfo ? 1 : 0)
if hasSequenceInfo:
    short(lastSequenceSent)
    short(lastSequenceReceived)
```

**Read:**

```
flags = readShort()
if flags != 0:
    lastSequenceSent = readShort()
    lastSequenceReceived = readShort()
```

A DropBoxInfo with `hasSequenceInfo=false` is used to request the server flush its drop box.

---

## 8. Keepalive

### Ping (classID 15, version 1)

Sent by the server to verify the client is alive.

**Body (version > 1 only):**

```
binary(payload)     — arbitrary payload data
```

When version is 1, the body contains only `binary(payload)` for write but reads nothing.

### Pong (classID 16, version 2)

Sent by the client in response to a Ping.

**Write body:**

```
binary(zeros[payloadSize])    — zero-filled byte array matching Ping's payload size
```

**Read body:**

```
payloadSize = readString().length()    — reads a string and uses its length
```

The client responds to every Ping with a Pong whose payload size matches the Ping's.

Source: `Service.dispatchObject()`:

```java
case 15:  // Ping
    Pong pong = new Pong();
    pong.SetPayloadSize(((Ping) storable).GetPayloadSize());
    sendInternal(pong);
    break;
case 16:  // Pong
    break;  // ignored by client
```

---

## 9. Chunking (Large Object Support)

Objects exceeding a configurable size threshold are split into `ChunkedObject` fragments.

### 9.1 ChunkedObject (classID 125, version 0)

**Wire format:**

```
string(chunkID)         — unique identifier for this chunked transfer
short(classID)          — class ID of the original object
short(partNum)          — 1-based part number
short(totalParts)       — total number of parts
binary(payload)         — fragment of the serialized object data
```

### 9.2 Chunk ID Format

```
{unix_timestamp_seconds}.{sequence_number_unsigned}
```

Example: `1704067200.42`

The sequence number is the DropBox sequence of the original object, masked to unsigned 16-bit (`& 0xFFFF`).

### 9.3 Chunking Process (Sender)

1. Serialize the original object to a byte array (including its full object header)
2. Calculate maximum payload per chunk: `chunkSize - ChunkedObject.getMaxOverheadSize(chunkID)`
   - Overhead = 4 + chunkID.length + 6 + 4 = 14 + chunkID.length
3. Split the serialized bytes into fragments
4. Wrap each fragment in a `ChunkedObject` with incrementing `partNum` (1-based)
5. Send each `ChunkedObject` as a separate sequenced message

### 9.4 Reassembly Process (Receiver)

1. First chunk (partNum == 1) creates a new `ChunkedObjectList` entry
2. Subsequent chunks are appended to the list matching the `chunkID`
3. When all parts are received (`list.size() == totalParts`):
   a. Concatenate all payloads in order
   b. Deserialize the concatenated bytes as a normal object (calls `readObject()`)
   c. Remove the chunk list
4. The reassembled object is then dispatched normally

### 9.5 Message Ordering

Objects arriving between chunks of a multi-part message are **queued** and dispatched after reassembly completes. This is handled by the `MessageOrderTracker`:

- When the first chunk arrives, a `MessageOrderTracker` records its sequence as `highestAllowedSequence`
- Non-chunk objects with sequence > `highestAllowedSequence` are held in a pending list
- After reassembly completes, `highestAllowedSequence` is invalidated and pending objects are dispatched

This ensures the application sees messages in the correct order relative to large objects.

---

## 10. Object Dispatch & Response Matching

### 10.1 Dispatch

When an object is read from the stream, `Service.dispatchObject()` handles it based on classID:

| ClassID                    | Handling                                                                     |
| -------------------------- | ---------------------------------------------------------------------------- |
| 14 (Ack)                   | Calls `mDropBox.ack(sequence)`                                               |
| 15 (Ping)                  | Sends Pong with matching payload                                             |
| 16 (Pong)                  | Ignored                                                                      |
| 18 (Response)              | Matches to request via sequence, dispatches to app                           |
| 37 (SystemUpgrade)         | Starts OTA update transfer                                                   |
| 38 (SystemVersionInfo)     | Sends client's SystemVersionInfo in reply                                    |
| 52 (FunnelTransaction)     | Dispatches to registered app via `NetworkObject.dispatchFunnelTransaction()` |
| 110 (SIMPropertiesRequest) | Sends SIMProperties in reply                                                 |
| 125 (ChunkedObject)        | Passes to `ChunkedObjectHandler` for reassembly                              |
| All others                 | Dispatches to registered application handler                                 |

### 10.2 Response Matching

`Response` (classID 18) contains:

```
short(requestSequence)     — sequence number of the original request
short(responseCount)       — number of response objects
[object]*                  — array of response objects (each with full headers)
```

The server uses `requestSequence` to identify which client request the response is for. The client looks up the original request in the DropBox:

```java
Storable request = mDropBox.find(response.mRequestSequence);
response.mRequest = request;
```

The response is then dispatched to the application that registered the handler for the **request's** classID.

---

## 11. Complete Class ID Registry

### 11.1 Service Objects (with wire format)

#### Ack (classID 14, version 1) — Bidirectional

```
short(highestSequence)     — highest sequence number being acknowledged
```

#### Ping (classID 15, version 1) — S->C

```
// version 1 write: binary(payload_or_null)
// version > 1 read: binary(payload)
```

#### Pong (classID 16, version 2) — C->S

```
// write: binary(zeros[payloadSize])
// read: string(payload)  — uses string length as payload size
```

#### Response (classID 18, version 1) — S->C

```
short(requestSequence)
short(responseCount)
object[responseCount]      — each is a fully-serialized object with header
```

#### ConnectionStatus (classID 20, version 1) — Internal/Broadcast

```
int(status)                — 1 = CONNECTION_UP, 2 = CONNECTION_DOWN
```

#### SystemVersionInfo (classID 38, version 4) — C->S

```
short(0)                   — reserved
short(componentCount)
for each component:
    string(name)
    int(pubFlags)
    int(pubPartner)
    int(pubMajor)
    int(pubMinor)
```

#### ServiceLogin (classID 43, version 5) — C->S

```
short(loginType)           — 0=normal, 1=existing_account, 2=new_sim, 3=credit_card, 4=upgrade
if loginType == 1:
    string(username)
short(simIdType)           — 0=IMSI, 1=ICCID, 2=SIM-less
string(password)
string(simId)              — IMSI, ICCID, or "none"
string(deviceIdType)
string(deviceId)
binary(oldLocationKey)
int(partnerId)
```

#### ServiceLoginResult (classID 44, version 2) — S->C

```
short(result)              — 0=success, -1=bad_password, -2=bad_password_retry,
                             -3=server_error, -4=identity_check_required,
                             -5=need_payment, -6=bad_behavior, -7=user_closed,
                             -8=ccc_closed, -11=insufficient_funds
string(explanation)
if version > 1 AND result == 0:
    binary(newLocationKey)
```

#### IdentityCheck (classID 45, version 4) — C->S

```
short(simIdType)           — 0=IMSI, 1=ICCID, 2=SIM-less
string(simId)              — IMSI, ICCID, or "none"
string(deviceIdType)
string(deviceId)
int(partnerId)
```

#### IdentityCheckResult (classID 46, version 1) — S->C

```
short(action)              — -1=server_error, 0=normal_login, 1=registration,
                             2=optional_registration, 3=credit_card_login, 4=upgrade
if action == 0 or action == 3:
    string(currentUser)
if (action == 2 or action == 4) AND header_version == 2:
    string(currentUser)
```

#### DropBoxInfo (classID 47, version 2) — Bidirectional

```
// Version 2 write:
short(hasSequenceInfo ? 1 : 0)
if hasSequenceInfo:
    short(lastSequenceSent)
    short(lastSequenceReceived)

// Version 2 read:
flags = readShort()
if flags != 0:
    short(lastSequenceSent)
    short(lastSequenceReceived)

// Version 1 (legacy):
short(lastSequenceSent)
short(lastSequenceReceived)
```

#### CreateAccountInfo (classID 48, version 8) — C->S

```
short(version)             — echoes getClassVersion()
string(firstName)
string(lastName)
string(username)
string(password)
short(simIdType)           — 0=IMSI, 1=ICCID
string(simId)
string(deviceId)
string(deviceIdType)
string(secretQuestion)
string(secretAnswer)
int(partnerId)
string(phoneNumber)
string(ipAddress)
int(timeZoneId)
string(locale)
int(hardwareType)
int(deviceOSMajorVersion)
int(deviceOSMinorVersion)
```

#### CreateAccountResult (classID 49, version 2) — S->C

```
short(result)
if result == -2 or result == -6:
    string(suggestedUsernames)
```

#### AccountDetailsInfo (classID 50, version 1) — C->S

```
string(username)
string(password)
string(imAccount)
string(imPassword)
string(imPartner)          — typically "Yahoo"
```

#### AccountDetailsResult (classID 51, version 1) — S->C

```
short(result)
string(explanation)
```

#### FunnelTransaction (classID 52, version 1) — Bidirectional

See Section 13 for detailed sub-protocol.

#### CrashLog (classID 56, version 4) — C->S

```
string(deviceIdType)
string(deviceId)
short(reason)              — 1=system_crash, 100=radio_crash, 101=radio_reset,
                             200=event_log, 201=charge_fault
string(logData)            — written as string(byte[], offset, length)
object(SystemVersionInfo)  — embedded object with full header
```

#### GenericAppResponse (classID 58, version 1) — S->C

Empty body (no fields read or written).

#### SessionLocationCheck (classID 64, version 3) — C->S

```
short(simIdType)           — 0=IMSI, 1=ICCID
string(simId)
string(deviceIdType)
string(deviceId)
int(partnerId)
```

#### SessionLocationResult (classID 65, version 1) — S->C

```
short(ipAddrCount)
for each location record:
    string(ipAddr)
    short(port)            — read as unsigned: readShort() & 0xFFFF
    short(retryCount)
    short(useEncryption)   — 0 = false, non-zero = true
```

#### ServiceTimeSettings (classID 66, version 2) — Bidirectional

```
// Version 2:
int(command)
switch command:
    case -1000 (VERSION_PREF):
        int(protocolVersion)   — currently 2
    case -400 (SETTINGS):
        int(timeZoneId)
        int(timeFormat)        — 1 = 12-hour, 0 = 24-hour

// Version 1 (legacy read only):
int(timeZoneId)
```

#### CellUpdate (classID 68, version 2) — Bidirectional

```
// Write:
string(operator)
string(lac)                — Location Area Code
string(cid)                — Cell ID
short(flags)               — bit 0: location_reveal, bit 1: location_query

// Read (version agnostic):
string(operator)
string(lac)
string(cid)
```

#### CellLocation (classID 69, version 1) — C->S

```
string(operator)
string(lac)
string(cid)
string(location)
```

#### SIMProperties (classID 109, version 1) — C->S

```
short(propertyCount)       — always 3
string("iccid")
string(iccidValue)
string("efgid1")
string(efgid1Value)
string("efgid2")
string(efgid2Value)
```

#### SIMPropertiesRequest (classID 110, version 1) — S->C

Empty body. Triggers client to send SIMProperties.

#### ChunkedObject (classID 125, version 0) — Bidirectional

```
string(chunkId)            — "{unix_timestamp}.{sequence_number}"
short(classId)             — class ID of original object
short(partNum)             — 1-based part number
short(totalParts)
binary(payload)            — fragment of serialized object data
```

### 11.2 Additional Registered Class IDs

These class IDs are registered in `NetworkObject` but their implementations are in application-level code or are placeholders:

| ClassID | Name                       | Notes                        |
| ------- | -------------------------- | ---------------------------- |
| 0       | (none)                     | Reserved null type           |
| 1       | GeneralMailInfo            | Mail notification            |
| 4       | MailTransaction            | Mail sync                    |
| 6       | IM_Yahoo                   | Yahoo IM                     |
| 7       | ContactsTransaction        | Contacts sync                |
| 9       | MailProfileTransaction     | Mail profile sync            |
| 11      | BackingStoreTransaction    | Generic key-value store sync |
| 21      | ContactsRecord             | Contact record               |
| 27      | ServiceTime                | Time synchronization         |
| 29      | DeviceInfo                 | Device information           |
| 30      | CalendarTransaction        | Calendar sync                |
| 31      | CalendarEvent              | Calendar event               |
| 32      | NotesTransaction           | Notes sync                   |
| 33      | NotesRecord                | Notes record                 |
| 34      | TodoTransaction            | Todo sync                    |
| 35      | TodoRecord                 | Todo record                  |
| 36      | UniqueID                   | UID allocation               |
| 37      | SystemUpgrade              | OTA firmware update          |
| 40      | GameLobby                  | Game matchmaking             |
| 41      | RingToneObject             | Ringtone data                |
| 42      | ServiceInfo                | Service metadata             |
| 53      | IM_AIM                     | AIM IM                       |
| 54      | IM_Jabber                  | Jabber/XMPP IM               |
| 55      | PhotoRecordTransaction     | Photo sync                   |
| 57      | GenericServiceEventLog     | Event logging                |
| 60      | BrowserInfo                | Browser data                 |
| 61      | IM_Say2                    | Say2 IM                      |
| 62      | PhotoRecord                | Photo record                 |
| 63      | CustomerCareTools          | Carrier admin tools          |
| 70      | EndDispatcherSession       | Session teardown             |
| 71      | DogzTransaction            | DOGZ game                    |
| 72      | PDMTransaction             | Personal Data Manager        |
| 73      | MonitorDummy               | Monitoring placeholder       |
| 74      | SystemDialogFromNetwork    | Server-pushed dialogs        |
| 81      | PlogTransaction            | PLog sync                    |
| 83      | GeneralAOLMailInfo         | AOL Mail notification        |
| 84      | AOLMailTransaction         | AOL Mail sync                |
| 85      | AOLMailProfileTransaction  | AOL Mail profile             |
| 86      | AOLMailProfile             | AOL Mail profile data        |
| 89      | IM_MSN                     | MSN IM                       |
| 90      | DispatcherSession          | Dispatcher session           |
| 92      | IconRecord                 | App icon data                |
| 93      | IconRecordTransaction      | Icon sync                    |
| 95      | WAPAppInfo                 | WAP application info         |
| 96      | MMSTransaction             | MMS messaging                |
| 97      | IM_ICQ                     | ICQ IM                       |
| 111     | TranscoderRequest          | Media transcoding request    |
| 112     | TranscoderResponse         | Media transcoding response   |
| 113     | ConcConnectionRequest      | Concurrent connection        |
| 114     | ConcAuthenticateRequest    | Concurrent auth request      |
| 115     | ConcAuthenticateResponse   | Concurrent auth response     |
| 116     | BrowserProvisionRequest    | Browser provisioning         |
| 117     | BrowserProvisionMessage    | Browser provision message    |
| 118     | BrowserProvisionSearchInfo | Browser search config        |
| 119     | BrowserProvisionBookmarks  | Browser bookmarks            |
| 120     | BrowserProvisionHint       | Browser hints                |
| 121     | SMSStorableStringArray     | SMS data                     |
| 122     | IM_was_msn                 | Legacy MSN IM                |
| 123     | ThirdPartyUserKeyRequest   | 3rd party key request        |
| 124     | ThirdPartyUserKeyResponse  | 3rd party key response       |
| 126     | SearchTransaction          | Search                       |
| 127     | LogInfo                    | Log information              |

---

## 12. Reconnection Logic

### 12.1 Exponential Backoff

When a connection fails, the client implements exponential backoff:

```
Initial delay:    1,000 ms   (1 second)
Increment:       +8,000 ms   (added per retry while delay < 30s)
Threshold:       30,000 ms   (30 seconds)
After threshold:  delay *= 2  (double each retry)
Maximum:        480,000 ms   (8 minutes)
```

Source: `Service.java`:

```java
if (mReconnectDelay == 0) {
    mReconnectDelay = 1000;
} else if (mReconnectDelay < 30000) {     // CONNECTION_DOWN_TIMEOUT
    mReconnectDelay += 8000;               // CONNECTION_TIMEOUT_INC
} else if (mReconnectDelay < 240000) {
    mReconnectDelay <<= 1;                 // double
} else {
    mReconnectDelay = 480000;              // MAX_RECONNECT_TIMEOUT
}
```

### 12.2 Backoff Progression

| Attempt | Delay                             |
| ------- | --------------------------------- |
| 1       | 1,000 ms                          |
| 2       | 9,000 ms                          |
| 3       | 17,000 ms                         |
| 4       | 25,000 ms                         |
| 5       | 33,000 ms (exceeds 30s threshold) |
| 6       | 66,000 ms                         |
| 7       | 132,000 ms                        |
| 8       | 264,000 ms (exceeds 240s)         |
| 9+      | 480,000 ms (capped)               |

### 12.3 Backoff Reset

The backoff delay is reset to 0 when:

- A data connection becomes ready (`wakeupFromExpBackoff(true)`)

### 12.4 Retry Count

The `retryCount` from `SessionLocationResult` controls how many times the client retries the known server before falling back to the redirector:

- Each reconnection attempt decrements `mRetryCount`
- When `mRetryCount < 0`, the server address is cleared, forcing a new redirector lookup

---

## 13. FunnelTransaction Subprotocol

`FunnelTransaction` (classID 52) is a multiplexed application-level channel that carries HTTP requests, push notifications, and inter-device data.

### 13.1 Common Header

All FunnelTransaction subcommands begin with:

```
int(command)               — identifies the subcommand type
```

### 13.2 Subcommands

#### Version Preference (-800) — Bidirectional

Negotiates the FunnelTransaction protocol version.

```
int(command)               — -800
int(protocolVersion)       — currently 1
```

#### HTTP Request (-801) — C->S

```
int(command)               — -801
int(sequenceId)
string(appId)
string(appClassName)
string(url)
short(flags)
string(headers)
binary(data)
```

#### HTTP Response Data (-802) — S->C

```
int(command)               — -802
int(sequenceId)
string(appId)
string(appClassName)
int(httpResponseCode)      — e.g. 200, 404
if httpResponseCode != 200:
    string(responseMessage)
string(headers)
binary(data)
int(offset)
int(length)
int(total)
```

#### Push Data (-803) — S->C

Server-initiated data push to an application.

```
int(command)               — -803
int(sequenceId)
string(appId)
string(appClassName)
binary(data)
```

#### Device Data (-804) — Bidirectional

Device-to-device data routed through the server.

```
int(command)               — -804
int(sequenceId)
string(appId)
string(appClassName)
string(fromUsername)
string(toUsername)
binary(data)
```

#### Disable App (-805) — S->C

```
int(command)               — -805
string(appId)
```

#### WAP Request (-806) — C->S

Same wire format as HTTP Request (-801).

#### Miscellaneous (-807) — Bidirectional

Generic application data channel.

```
int(command)               — -807
int(sequenceId)
string(appId)
string(appClassName)
binary(data)
```

#### Enable App (-808) — S->C

```
int(command)               — -808
string(appId)
```

### 13.3 Version Negotiation

On receiving a FunnelTransaction with an unsupported version, the client sends a version preference message and throws an `ObjectFormatException`:

```java
if (version != 1) {
    sendVersionPreference();
    throw new ObjectFormatException("FunnelTransaction: can't handle version " + version);
}
```

---

## 14. Writer Thread

The writer thread processes a queue of `WriteObjectEntry` objects:

1. Wait for entries in `mWriteObjectHead` linked list
2. For each entry:
   - If `mHasSequence`: call `mOut.writeSequencedObject(object, sequence, true)`
     - This may produce `ChunkedObject` fragments if the object is too large
     - Each chunk is written individually as a sequenced object
   - If not sequenced: call `mOut.writeObject(object)`
3. Flush the output stream
4. Send an Ack if necessary

Objects sent before `RESYNCING_SERVICE_DROP_BOX` state (e.g., during login) are written without sequence numbers. Objects from the DropBox are written with sequence numbers.

---

## 15. Reader Thread

The reader thread continuously reads objects from the input stream:

1. Call `mIn.readObject(true)` — reads header + body, returns `SequencedStorable` wrapper if sequenced
2. Based on the current state:
   - **REDIRECTING:** Expect `SessionLocationResult` (classID 65)
   - **CHECKING_IDENTITY:** Expect `IdentityCheckResult` (classID 46)
   - **CREATING_ACCOUNT:** Expect `CreateAccountResult` (classID 49)
   - **LOGGING_IN:** Expect `ServiceLoginResult` (classID 44)
   - **RESYNCING_DROP_BOX:** Expect `DropBoxInfo` (classID 47)
   - **RESYNCING_SERVICE_DROP_BOX:** Expect `DropBoxInfo` (classID 47) -> transition to LOGGED_IN
   - **LOGGED_IN:** Dispatch any object type
3. After each object, call `sendAckIfNecessary()` for sequenced messages

---

## 16. Stream Integrity

The connection uses "sanity check" mode (`mUseChunkedInput = true`) where each object read is wrapped in a `ChunkInputStream` that:

- Limits reads to exactly `bodyLength` bytes
- Throws `SizeMismatchException` if the object reads too few or too many bytes
- Recovers by skipping remaining bytes on error (preventing stream desynchronization)

This ensures that a malformed object body doesn't corrupt subsequent objects in the stream.

package proto

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"

	"golang.org/x/crypto/blowfish" // blowfish is deprecated but matches the Danger protocol so we have to use it
)

const (
	blowfishBlockSize  = 8
	maxChunkLen        = 4288
	macLen             = 20
	maxPayloadPerChunk = maxChunkLen - macLen - blowfishBlockSize - 2
)

// SecureConn wraps a net.Conn with the Danger encrypted chunked protocol.
// Uses Blowfish-CBC with persistent state per direction, matching the
// Danger SimCipher/SimCBC implementation.
type SecureConn struct {
	raw    net.Conn
	cipher *blowfish.Cipher

	// CBC state (persists across chunks)
	encIV [blowfishBlockSize]byte // encrypt direction
	decIV [blowfishBlockSize]byte // decrypt direction

	// read buffering
	buf    []byte
	bufOff int
	bufLen int
}

// Handshake performs the Danger secure socket handshake (version exchange + DH + Blowfish init + IV exchange).
func Handshake(conn net.Conn) (*SecureConn, error) {
	// 1. Read 2-byte version from client
	var clientVer [2]byte
	if _, err := io.ReadFull(conn, clientVer[:]); err != nil {
		return nil, fmt.Errorf("read client version: %w", err)
	}
	version := int(clientVer[0])<<8 | int(clientVer[1])
	log.Printf("secure: client secure version=%d", version)
	if version < 2 {
		return nil, fmt.Errorf("unsupported secure protocol version %d", version)
	}

	// 2. Send server version
	serverVer := [2]byte{0, 2}
	if _, err := conn.Write(serverVer[:]); err != nil {
		return nil, fmt.Errorf("write server version: %w", err)
	}

	// 3. DH key exchange (protocol version 2)
	// Generate 255-bit prime p (not 256, because the high bit of the 32-byte
	// representation must be 0 because the client's BigInteger treats
	// bytes as signed two's complement).
	p, err := rand.Prime(rand.Reader, 255)
	if err != nil {
		return nil, fmt.Errorf("generate prime: %w", err)
	}

	// Generator g = 2
	g := big.NewInt(2)

	// Server random: 16 random bytes, prepend 0 to ensure positive
	serverRandomBytes := make([]byte, 17)
	serverRandomBytes[0] = 0
	if _, err := rand.Read(serverRandomBytes[1:]); err != nil {
		return nil, fmt.Errorf("generate server random: %w", err)
	}
	serverRandom := new(big.Int).SetBytes(serverRandomBytes)

	// Compute serverPublic = g^serverRandom mod p
	serverPublic := new(big.Int).Exp(g, serverRandom, p)

	// Send 66 bytes: [32-byte p][0x00][1-byte g][32-byte serverPublic]
	dhParams := make([]byte, 66)
	padBigInt(dhParams[0:32], p)
	dhParams[32] = 0
	dhParams[33] = byte(g.Int64())
	padBigInt(dhParams[34:66], serverPublic)

	if _, err := conn.Write(dhParams); err != nil {
		return nil, fmt.Errorf("write DH params: %w", err)
	}

	// Read 33 bytes from client: [zero-padded 32-byte clientPublic][0x00]
	clientBuf := make([]byte, 33)
	if _, err := io.ReadFull(conn, clientBuf); err != nil {
		return nil, fmt.Errorf("read client public: %w", err)
	}
	clientPublic := new(big.Int).SetBytes(clientBuf[0:32])

	// Compute shared secret = clientPublic^serverRandom mod p
	sharedSecret := new(big.Int).Exp(clientPublic, serverRandom, p)

	// Zero-pad shared secret to 32 bytes
	key := make([]byte, 32)
	padBigInt(key, sharedSecret)

	// Send 33 bytes to client: [32-byte serverPublic][0x00]
	serverPubBuf := make([]byte, 33)
	padBigInt(serverPubBuf[0:32], serverPublic)
	serverPubBuf[32] = 0
	if _, err := conn.Write(serverPubBuf); err != nil {
		return nil, fmt.Errorf("write server public: %w", err)
	}

	// 4. Initialize Blowfish with shared secret
	cipher, err := blowfish.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("init blowfish: %w", err)
	}

	sc := &SecureConn{
		raw:    conn,
		cipher: cipher,
		buf:    make([]byte, maxChunkLen),
		// encIV and decIV are zero-initialized (matching SimCBC.init(null))
	}

	// 5. IV exchange: both sides write 32 zero-bytes then read 32.
	// Must write first (matching client order). TCP buffering prevents deadlock.
	if _, err := sc.Write(make([]byte, 32)); err != nil {
		return nil, fmt.Errorf("write server IV: %w", err)
	}
	ivBuf := make([]byte, 32)
	if _, err := sc.Read(ivBuf); err != nil {
		return nil, fmt.Errorf("read client IV: %w", err)
	}

	log.Printf("secure: handshake complete")
	return sc, nil
}

func (sc *SecureConn) Read(p []byte) (int, error) {
	// If buffer is empty, read a new chunk
	if sc.bufOff >= sc.bufLen {
		if err := sc.readChunk(); err != nil {
			return 0, err
		}
	}
	// Return whatever is available
	n := copy(p, sc.buf[sc.bufOff:sc.bufLen])
	sc.bufOff += n
	return n, nil
}

func (sc *SecureConn) Write(p []byte) (int, error) {
	total := 0
	for total < len(p) {
		remaining := len(p) - total
		chunkSize := min(remaining, maxPayloadPerChunk)
		if err := sc.writeChunk(p[total : total+chunkSize]); err != nil {
			return total, err
		}
		total += chunkSize
	}
	return total, nil
}

func (sc *SecureConn) cbcEncrypt(dst, src []byte) {
	for i := 0; i < len(src); i += blowfishBlockSize {
		// XOR plaintext block with previous ciphertext (or IV)
		for j := range blowfishBlockSize {
			dst[i+j] = src[i+j] ^ sc.encIV[j]
		}
		// Encrypt block
		sc.cipher.Encrypt(dst[i:i+blowfishBlockSize], dst[i:i+blowfishBlockSize])
		// Update IV to this ciphertext block
		copy(sc.encIV[:], dst[i:i+blowfishBlockSize])
	}
}

func (sc *SecureConn) cbcDecrypt(dst, src []byte) {
	var saved [blowfishBlockSize]byte
	for i := 0; i < len(src); i += blowfishBlockSize {
		// Save ciphertext block before decryption
		copy(saved[:], src[i:i+blowfishBlockSize])
		// Decrypt block
		sc.cipher.Decrypt(dst[i:i+blowfishBlockSize], src[i:i+blowfishBlockSize])
		// XOR with previous ciphertext (or IV)
		for j := range blowfishBlockSize {
			dst[i+j] ^= sc.decIV[j]
		}
		// Update IV to this ciphertext block
		copy(sc.decIV[:], saved[:])
	}
}

func (sc *SecureConn) readChunk() error {
	// Read 2-byte length
	var lenBuf [2]byte
	if _, err := io.ReadFull(sc.raw, lenBuf[:]); err != nil {
		return err
	}
	totalLen := int(binary.BigEndian.Uint16(lenBuf[:]))
	chunkLen := totalLen - 2
	if chunkLen < 0 || chunkLen > maxChunkLen {
		return fmt.Errorf("bad chunk size %d", chunkLen)
	}

	// Read encrypted data
	encrypted := make([]byte, chunkLen)
	if _, err := io.ReadFull(sc.raw, encrypted); err != nil {
		return err
	}

	// Decrypt (Blowfish-CBC)
	decrypted := make([]byte, chunkLen)
	sc.cbcDecrypt(decrypted, encrypted)

	// Verify SHA-1 MAC: MAC is over first (chunkLen - 20) bytes
	macStart := chunkLen - macLen
	h := sha1.Sum(decrypted[:macStart])
	for i := range macLen {
		if decrypted[macStart+i] != h[i] {
			return fmt.Errorf("bad MAC")
		}
	}

	// Extract payload: padLen byte is at decrypted[macStart-1]
	padLenByte := int(decrypted[macStart-1])
	payloadLen := macStart - 1 - padLenByte
	if payloadLen < 0 {
		return fmt.Errorf("bad padding length %d", padLenByte)
	}

	copy(sc.buf, decrypted[:payloadLen])
	sc.bufOff = 0
	sc.bufLen = payloadLen
	return nil
}

func (sc *SecureConn) writeChunk(data []byte) error {
	dataLen := len(data)

	// Compute padding to align (data + padLen_byte + MAC) to block size
	padLen := (blowfishBlockSize - ((dataLen + 1 + macLen) % blowfishBlockSize)) % blowfishBlockSize
	plainLen := dataLen + padLen + 1 + macLen
	totalLen := 2 + plainLen

	// Build plaintext: [data][padding(0x55)][padLen_byte][20-byte SHA-1]
	plain := make([]byte, plainLen)
	copy(plain, data)
	for i := range padLen {
		plain[dataLen+i] = 0x55
	}
	plain[dataLen+padLen] = byte(padLen)

	// SHA-1 MAC over (data + padding + padLen_byte)
	h := sha1.Sum(plain[:dataLen+padLen+1])
	copy(plain[dataLen+padLen+1:], h[:])

	// Encrypt (Blowfish-CBC)
	encrypted := make([]byte, plainLen)
	sc.cbcEncrypt(encrypted, plain)

	// Write: [2-byte totalLen][encrypted data]
	wire := make([]byte, totalLen)
	binary.BigEndian.PutUint16(wire[0:2], uint16(totalLen))
	copy(wire[2:], encrypted)

	_, err := sc.raw.Write(wire)
	return err
}

func padBigInt(dst []byte, n *big.Int) {
	b := n.Bytes()
	if len(b) > len(dst) {
		copy(dst, b[len(b)-len(dst):])
	} else {
		copy(dst[len(dst)-len(b):], b)
	}
}

package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"qqtang/internal/accountauth"
	"qqtang/internal/protocol/directory"
	"qqtang/internal/protocol/game"
	"qqtang/internal/protocol/qqtea"
)

type client struct {
	conn net.Conn
	uin  uint32
}

func readFrame(conn net.Conn) ([]byte, error) {
	var prefix [4]byte
	if _, err := io.ReadFull(conn, prefix[:]); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(prefix[:])
	if length < 4 || length > 1<<20 {
		return nil, fmt.Errorf("invalid frame length %d", length)
	}
	frame := make([]byte, length)
	copy(frame, prefix[:])
	if _, err := io.ReadFull(conn, frame[4:]); err != nil {
		return nil, err
	}
	return frame, nil
}

func (c *client) authenticate(password string) error {
	hello, err := accountauth.Marshal(accountauth.Message{Type: accountauth.MessageHello, UIN: c.uin})
	if err != nil {
		return err
	}
	if _, err = c.conn.Write(hello); err != nil {
		return err
	}
	frame, err := readFrame(c.conn)
	if err != nil {
		return err
	}
	challenge, err := accountauth.Unmarshal(frame)
	if err != nil {
		return err
	}
	proof, err := accountauth.ClientProof([]byte(password), challenge.Salt, challenge.Iterations, challenge.Nonce, c.uin)
	if err != nil {
		return err
	}
	message, err := accountauth.Marshal(accountauth.Message{Type: accountauth.MessageProof, UIN: c.uin, Proof: proof})
	if err != nil {
		return err
	}
	if _, err = c.conn.Write(message); err != nil {
		return err
	}
	frame, err = readFrame(c.conn)
	if err != nil {
		return err
	}
	result, err := accountauth.Unmarshal(frame)
	if err != nil {
		return err
	}
	if !result.Accepted {
		return fmt.Errorf("auth rejected: %s", result.Reason)
	}
	return nil
}

// buildGamePacket mirrors the native envelope: 4-byte total length, UIN at
// 12..16, ST slot at 18..50, QQ-TEA payload after 50.
func buildGamePacket(command, route, marker, sectionID uint16, uin uint32, payload []byte) ([]byte, error) {
	plaintext := make([]byte, 14+len(payload))
	binary.BigEndian.PutUint16(plaintext[0:2], command)
	binary.BigEndian.PutUint16(plaintext[6:8], 1)
	binary.BigEndian.PutUint16(plaintext[8:10], route)
	binary.BigEndian.PutUint16(plaintext[10:12], marker)
	binary.BigEndian.PutUint16(plaintext[12:14], sectionID)
	copy(plaintext[14:], payload)
	ciphertext, err := qqtea.EncryptWithReader(plaintext, directory.LocalKey, bytes.NewReader(make([]byte, 32)))
	if err != nil {
		return nil, err
	}
	packet := make([]byte, 50+len(ciphertext))
	binary.BigEndian.PutUint32(packet[0:4], uint32(len(packet)))
	binary.BigEndian.PutUint32(packet[12:16], uin)
	packet[16], packet[17] = 1, byte(len(directory.LocalST))
	copy(packet[18:50], directory.LocalST)
	copy(packet[50:], ciphertext)
	return packet, nil
}

func gbkSlot(text string, size int) []byte {
	slot := make([]byte, size)
	copy(slot, []byte(text))
	return slot
}

func loginPayload(uin uint32) []byte {
	payload := make([]byte, 53)
	binary.BigEndian.PutUint32(payload[0:4], uin)
	binary.BigEndian.PutUint32(payload[4:8], uint32(time.Now().Unix()))
	copy(payload[8:28], gbkSlot("Tester", 20))
	payload[28] = 1
	payload[38] = 1
	payload[29] = 0
	binary.BigEndian.PutUint16(payload[32:34], 1)
	binary.BigEndian.PutUint32(payload[39:43], 0)
	binary.BigEndian.PutUint16(payload[43:45], 0)
	return payload
}

func createRoomPayload(uin uint32) []byte {
	payload := make([]byte, 50)
	binary.BigEndian.PutUint32(payload[0:4], uin)
	binary.BigEndian.PutUint32(payload[4:8], uint32(time.Now().Unix()))
	copy(payload[8:28], gbkSlot("SmokeRoom", 20))
	payload[28] = 0
	payload[45] = 0
	return payload
}

func joinRoomPayload(uin uint32) []byte {
	payload := make([]byte, 9)
	binary.BigEndian.PutUint32(payload[0:4], uin)
	binary.BigEndian.PutUint32(payload[4:8], uint32(time.Now().Unix()))
	payload[8] = 0
	return payload
}

func (c *client) send(command, route, marker, section uint16, payload []byte) (time.Duration, int, error) {
	packet, err := buildGamePacket(command, route, marker, section, c.uin, payload)
	if err != nil {
		return 0, 0, err
	}
	started := time.Now()
	if _, err = c.conn.Write(packet); err != nil {
		return 0, 0, err
	}
	var first time.Duration
	frames := 0
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		frame, readErr := readFrame(c.conn)
		if readErr != nil {
			if frames > 0 {
				return first, frames, nil
			}
			return 0, 0, readErr
		}
		if frames == 0 {
			first = time.Since(started)
		}
		_ = frame
		frames++
	}
}

func registerUDPPresence(conn *net.UDPConn, server *net.UDPAddr, playerID uint16, uin uint32) error {
	local := conn.LocalAddr().(*net.UDPAddr)
	ip := [4]byte{127, 0, 0, 1}
	packet := game.LegacyUDPControlPacket{
		Header:   game.LegacyUDPControlHeader{PlayerID: playerID, UIN: uin, Type: game.LegacyUDPPresenceType},
		Presence: &game.LegacyUDPEndpoint{IPv4: ip, Port: uint16(local.Port)},
	}
	data, err := packet.Encode()
	if err != nil {
		return err
	}
	if _, err = conn.WriteToUDP(data, server); err != nil {
		return err
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buffer := make([]byte, 512)
	if _, _, err = conn.ReadFromUDP(buffer); err != nil {
		return err
	}
	return nil
}

func main() {
	address := flag.String("addr", "127.0.0.1:18000", "game server address")
	password := flag.String("pass", "123456", "account password")
	count := flag.Int("n", 200, "movement packets")
	flag.Parse()

	results := map[string]time.Duration{}
	fail := func(step string, err error) {
		fmt.Fprintf(os.Stderr, "%s failed: %v\n", step, err)
		os.Exit(1)
	}

	a, err := net.DialTimeout("tcp", *address, 5*time.Second)
	if err != nil {
		fail("dial A", err)
	}
	clientA := &client{conn: a, uin: 1000001}
	defer a.Close()

	started := time.Now()
	if err = clientA.authenticate(*password); err != nil {
		fail("auth A", err)
	}
	results["auth"] = time.Since(started)

	if latency, frames, err := clientA.send(0x0064, 1, 0xFFFF, 1, loginPayload(clientA.uin)); err != nil {
		fail("login A", err)
	} else {
		results["login"] = latency
		fmt.Printf("login A ok %dµs frames=%d\n", latency.Microseconds(), frames)
	}

	if latency, frames, err := clientA.send(0x006B, 1, 0xFFFF, 1, createRoomPayload(clientA.uin)); err != nil {
		fail("create room", err)
	} else {
		results["create-room"] = latency
		fmt.Printf("create room ok %dµs frames=%d\n", latency.Microseconds(), frames)
	}

	b, err := net.DialTimeout("tcp", *address, 5*time.Second)
	if err != nil {
		fail("dial B", err)
	}
	clientB := &client{conn: b, uin: 1000002}
	defer b.Close()
	if err = clientB.authenticate(*password); err != nil {
		fail("auth B", err)
	}
	if latency, frames, err := clientB.send(0x0064, 1, 0xFFFF, 1, loginPayload(clientB.uin)); err != nil {
		fail("login B", err)
	} else {
		results["login-B"] = latency
		fmt.Printf("login B ok %dµs frames=%d\n", latency.Microseconds(), frames)
	}
	if latency, frames, err := clientB.send(0x008A, 1, 0xFFFF, 1, joinRoomPayload(clientB.uin)); err != nil {
		fail("join room", err)
	} else {
		results["join-room"] = latency
		fmt.Printf("join room ok %dµs frames=%d\n", latency.Microseconds(), frames)
	}

	// Real-time movement uses QQTPPP over UDP. Register both peers, then send a
	// Type-2 multicast from A and require B to receive the relayed scene body.
	serverUDP := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 18000}
	udpA, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		fail("udp A", err)
	}
	defer udpA.Close()
	udpB, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		fail("udp B", err)
	}
	defer udpB.Close()
	if err = registerUDPPresence(udpA, serverUDP, 1, clientA.uin); err != nil {
		fail("presence A", err)
	}
	if err = registerUDPPresence(udpB, serverUDP, 2, clientB.uin); err != nil {
		fail("presence B", err)
	}

	multicast := game.LegacyUDPControlPacket{
		Header: game.LegacyUDPControlHeader{PlayerID: 1, UIN: clientA.uin, Type: game.LegacyUDPMulticastType, PacketNumber: 2},
		Multicast: &game.LegacyUDPMulticastPayload{
			Targets: []game.LegacyUDPMulticastTarget{{PlayerID: 2, UIN: clientB.uin}},
			Data:    []byte("MOVE-0"),
		},
	}
	started = time.Now()
	for index := 0; index < *count; index++ {
		multicast.Header.PacketNumber = uint32(2 + index)
		multicast.Multicast.Data = []byte(fmt.Sprintf("MOVE-%d", index))
		packet, err := multicast.Encode()
		if err != nil {
			fail("encode multicast", err)
		}
		if _, err = udpA.WriteToUDP(packet, serverUDP); err != nil {
			fail("send multicast", err)
		}
		if index%8 == 7 {
			time.Sleep(200 * time.Microsecond)
		}
	}
	buffer := make([]byte, 1024)
	received := 0
	_ = udpB.SetReadDeadline(time.Now().Add(2 * time.Second))
	for received < *count {
		n, _, readErr := udpB.ReadFromUDP(buffer)
		if readErr != nil {
			break
		}
		relayed, decodeErr := game.DecodeLegacyUDPControlPacket(buffer[:n])
		if decodeErr != nil {
			continue
		}
		if relayed.Multicast == nil || !bytes.HasPrefix(relayed.Multicast.Data, []byte("MOVE-")) {
			fail("relay body", fmt.Errorf("unexpected body"))
		}
		received++
	}
	elapsed := time.Since(started)
	if received == 0 {
		fail("udp relay receive", fmt.Errorf("no packets relayed"))
	}
	results["udp-sync"] = elapsed / time.Duration(received)
	fmt.Printf("udp sync sent=%d recv=%d loss=%d elapsed=%dms avg=%.2fms pps=%.0f\n",
		*count, received, *count-received, elapsed.Milliseconds(), float64(elapsed.Microseconds())/float64(received)/1000, float64(received)/elapsed.Seconds())

	fmt.Println("--- summary ---")
	for _, step := range []string{"auth", "login", "create-room", "login-B", "join-room", "udp-sync"} {
		if value, ok := results[step]; ok {
			fmt.Printf("%-12s %dµs\n", step, value.Microseconds())
		}
	}
}




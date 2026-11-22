package meme

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

const (
	WS_GUID           = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	WS_VERSION        = "13"
	WS_KEY_SIZE       = 16
	WS_MASK_SIZE      = 4
	WS_FIN            = 0x80
	WS_MASK           = 0x80
	WS_RESERVED       = 0x70
	WS_OPCODE         = 0x0f
	WS_LENGTH         = 0x7f
	WS_LENGTH_16      = 126
	WS_LENGTH_64      = 127
	WS_CONTINUATION   = 0
	WS_TEXT           = 1
	WS_CLOSE          = 8
	WS_PING           = 9
	WS_PONG           = 10
	WS_MAX_MESSAGE    = 256 << 10
	WS_CONNECT_LIMIT  = 12 * time.Second
	WS_IDLE_LIMIT     = 90 * time.Second
	WS_CONNECTION     = "Connection"
	WS_UPGRADE        = "Upgrade"
	WS_PROTOCOL       = "websocket"
	WS_HEADER_KEY     = "Sec-WebSocket-Key"
	WS_HEADER_ACCEPT  = "Sec-WebSocket-Accept"
	WS_HEADER_VERSION = "Sec-WebSocket-Version"
)

// websocket is the RFC 6455 text and control frame subset used by the feed.
type websocket struct {
	io.ReadWriteCloser
}

func dialWebsocket(ctx context.Context, client *http.Client, url string) (websocket, error) {
	var nonce [WS_KEY_SIZE]byte
	rand.Read(nonce[:])
	key := base64.StdEncoding.EncodeToString(nonce[:])
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return websocket{}, err
	}
	req.Header.Set(WS_CONNECTION, WS_UPGRADE)
	req.Header.Set(WS_UPGRADE, WS_PROTOCOL)
	req.Header.Set(WS_HEADER_KEY, key)
	req.Header.Set(WS_HEADER_VERSION, WS_VERSION)
	res, err := client.Do(req)
	if err != nil {
		return websocket{}, err
	}
	accept := sha1.Sum([]byte(key + WS_GUID)) // RFC 6455 handshake proof, not authentication.
	connection := strings.Split(strings.ToLower(res.Header.Get(WS_CONNECTION)), ",")
	validConnection := slices.ContainsFunc(connection, func(token string) bool {
		return strings.TrimSpace(token) == strings.ToLower(WS_UPGRADE)
	})
	conn, writable := res.Body.(io.ReadWriteCloser)
	if res.StatusCode != http.StatusSwitchingProtocols || !writable || !validConnection ||
		!strings.EqualFold(res.Header.Get(WS_UPGRADE), WS_PROTOCOL) ||
		res.Header.Get(WS_HEADER_ACCEPT) != base64.StdEncoding.EncodeToString(accept[:]) {
		res.Body.Close()
		return websocket{}, fmt.Errorf("websocket upgrade rejected (HTTP %d)", res.StatusCode)
	}
	return websocket{conn}, nil
}

func (ws websocket) send(opcode byte, data []byte) error {
	if len(data) >= WS_LENGTH_16 {
		return fmt.Errorf("websocket outbound frame too large")
	}
	var mask [WS_MASK_SIZE]byte
	rand.Read(mask[:])
	frame := append([]byte{WS_FIN | opcode, WS_MASK | byte(len(data))}, mask[:]...)
	for i, b := range data {
		frame = append(frame, b^mask[i%len(mask)])
	}
	n, err := ws.Write(frame)
	if err == nil && n != len(frame) {
		return io.ErrShortWrite
	}
	return err
}

func (ws websocket) readFrame(remaining int) (byte, bool, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(ws, header[:]); err != nil {
		return 0, false, nil, err
	}
	fin := header[0]&WS_FIN != 0
	opcode := header[0] & WS_OPCODE
	length := uint64(header[1] & WS_LENGTH)
	if header[0]&WS_RESERVED != 0 || header[1]&WS_MASK != 0 ||
		(opcode >= WS_CLOSE && (!fin || length >= WS_LENGTH_16)) {
		return 0, false, nil, fmt.Errorf("invalid websocket frame header")
	}
	switch length {
	case WS_LENGTH_16:
		var size uint16
		if err := binary.Read(ws, binary.BigEndian, &size); err != nil {
			return 0, false, nil, err
		}
		length = uint64(size)
		if length < WS_LENGTH_16 {
			return 0, false, nil, fmt.Errorf("invalid websocket frame length")
		}
	case WS_LENGTH_64:
		if err := binary.Read(ws, binary.BigEndian, &length); err != nil {
			return 0, false, nil, err
		}
		if length <= 65535 {
			return 0, false, nil, fmt.Errorf("invalid websocket frame length")
		}
	}
	if opcode < WS_CLOSE && length > uint64(remaining) {
		return 0, false, nil, fmt.Errorf("websocket message too large")
	}
	data := make([]byte, int(length))
	_, err := io.ReadFull(ws, data)
	return opcode, fin, data, err
}

func (ws websocket) receive(onFrame func()) ([]byte, error) {
	var message []byte
	fragmented := false
	for {
		opcode, fin, data, err := ws.readFrame(WS_MAX_MESSAGE - len(message))
		if err != nil {
			return nil, err
		}
		onFrame()
		switch opcode {
		case WS_PING:
			if err := ws.send(WS_PONG, data); err != nil {
				return nil, err
			}
		case WS_PONG:
		case WS_CLOSE:
			if err := ws.send(WS_CLOSE, data); err != nil {
				return nil, err
			}
			return nil, io.EOF
		case WS_TEXT, WS_CONTINUATION:
			if (opcode == WS_CONTINUATION) != fragmented {
				return nil, fmt.Errorf("unexpected websocket continuation or text frame")
			}
			message = append(message, data...)
			if fin {
				return message, nil
			}
			fragmented = true
		default:
			return nil, fmt.Errorf("unsupported websocket opcode %d", opcode)
		}
	}
}

package meme

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
	"testing"
)

const WS_FIXTURE_URL = "https://test.invalid/api/data"

type memorySocket struct {
	received *bytes.Reader
	sent     bytes.Buffer
}

func (m *memorySocket) Read(p []byte) (int, error) {
	n, err := m.received.Read(p)
	return n, err
}

func (m *memorySocket) Write(p []byte) (int, error) {
	n, err := m.sent.Write(p)
	return n, err
}

func (m *memorySocket) Close() error {
	m.received.Reset(nil)
	return nil
}

func TestWebSocketUpgradeAndFrames(t *testing.T) {
	launch := fixture(t, FIXTURE_LAUNCH)
	frames := append([]byte{WS_TEXT, WS_LENGTH_16, 1, 0}, launch[:256]...)
	frames = append(frames, WS_FIN|WS_PING, 1, 'x')
	tail := launch[256:]
	frames = append(frames, WS_FIN|WS_CONTINUATION, WS_LENGTH_16, byte(len(tail)>>8), byte(len(tail)))
	frames = append(frames, tail...)
	frames = append(frames, WS_FIN|WS_CLOSE, 0)
	socket := &memorySocket{received: bytes.NewReader(frames)}
	client := &http.Client{Transport: fixtureTransport(func(req *http.Request) (*http.Response, error) {
		key := req.Header.Get(WS_HEADER_KEY)
		decoded, err := base64.StdEncoding.DecodeString(key)
		if err != nil || len(decoded) != WS_KEY_SIZE || req.Header.Get(WS_UPGRADE) != WS_PROTOCOL ||
			req.Header.Get(WS_HEADER_VERSION) != WS_VERSION {
			t.Fatalf("invalid upgrade request: %s", req.Header)
		}
		sum := sha1.Sum([]byte(key + WS_GUID))
		headers := make(http.Header)
		headers.Set(WS_CONNECTION, "keep-alive, Upgrade")
		headers.Set(WS_UPGRADE, WS_PROTOCOL)
		headers.Set(WS_HEADER_ACCEPT, base64.StdEncoding.EncodeToString(sum[:]))
		return &http.Response{StatusCode: http.StatusSwitchingProtocols, Header: headers, Body: socket}, nil
	})}
	ws, err := dialWebsocket(t.Context(), client, WS_FIXTURE_URL)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	frameCount := 0
	countFrame := func() { frameCount++ }
	got, err := ws.receive(countFrame)
	if err != nil || !bytes.Equal(got, launch) || frameCount != 3 {
		t.Fatalf("fragmented frame: %v, received %d bytes across %d frames", err, len(got), frameCount)
	}
	reply := socket.sent.Bytes()
	if len(reply) != 7 || reply[0] != WS_FIN|WS_PONG || reply[1] != WS_MASK|1 || reply[6]^reply[2] != 'x' {
		t.Fatalf("bad masked pong: %x", reply)
	}
	if _, err := ws.receive(countFrame); err != io.EOF {
		t.Fatalf("close frame must end the stream: %v", err)
	}
}

func TestWebSocketRejectsOversizedFrame(t *testing.T) {
	socket := &memorySocket{received: bytes.NewReader([]byte{WS_FIN | WS_TEXT, WS_LENGTH_64, 0, 0, 0, 0, 0, 5, 0, 0})}
	ws := websocket{socket}
	if _, _, _, err := ws.readFrame(WS_MAX_MESSAGE); err == nil {
		t.Fatal("oversized frame was accepted")
	}
}

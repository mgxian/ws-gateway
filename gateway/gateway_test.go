package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

type StubWSConn struct {
	addr   string
	buffer [][]byte
}

func newStubWSConn(addr string) *StubWSConn {
	return &StubWSConn{
		addr:   addr,
		buffer: make([][]byte, 0),
	}
}

func (s *StubWSConn) clear() {
	s.buffer = make([][]byte, 0)
}

func (s *StubWSConn) ReadMessage() (msg []byte, err error) {
	result := s.buffer[0]
	s.buffer = s.buffer[1:]
	return result, nil
}

func (s *StubWSConn) WriteMessage(msg []byte) error {
	s.buffer = append(s.buffer, msg)
	return nil
}

func (s *StubWSConn) RemoteAddr() string {
	return "stub address"
}

type StubWSClientStore struct {
	wsClients                          []Conn
	imClient                           map[int][]Conn
	matchClient                        []Conn
	publicWSClientsForAppWasCalled     bool
	privateWSClientsForMemberWasCalled bool
}

func (s *StubWSClientStore) save(app string, memberID int, ws Conn) error {
	s.wsClients = append(s.wsClients, ws)
	if app == "im" && memberID > 0 {
		s.imClient[memberID] = append(s.imClient[memberID], ws)
	}

	if app == "match" {
		s.matchClient = append(s.matchClient, ws)
	}

	return nil
}

func (s *StubWSClientStore) publicWSClientsForApp(app string) []Conn {
	s.publicWSClientsForAppWasCalled = true
	if app == "match" {
		return s.matchClient
	}
	return nil
}

func (s *StubWSClientStore) privateWSClientsForMember(memberID int) []Conn {
	s.privateWSClientsForMemberWasCalled = true
	return s.imClient[memberID]
}

type FakeAuthServer struct{}

func (s *FakeAuthServer) Auth(memberID int, token string) bool {
	if memberID == 12345 {
		return false
	}
	return true
}

func newWSConnectTo(server *httptest.Server) (*websocket.Conn, *http.Response, error) {
	wsURLPrefix := "ws" + strings.TrimPrefix(server.URL, "http")
	wsURL := wsURLPrefix + "/push"
	return websocket.DefaultDialer.Dial(wsURL, nil)
}

func assertAuth(t *testing.T, ws *websocket.Conn, memberID int, token string, want string) {
	authMsg := fmt.Sprintf(`{"member_id": %d, "token": "%s"}`, memberID, token)
	err := ws.WriteMessage(websocket.TextMessage, []byte(authMsg))
	assert.NoError(t, err)

	ws.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
	_, msg, err := ws.ReadMessage()
	assert.NoError(t, err)
	assert.Equal(t, want, string(msg))
}

func assertSubscribe(t *testing.T, ws *websocket.Conn, apps []string, store wsClientStore) {
	for _, app := range apps {
		subscribeMsg := fmt.Sprintf(`{"app": "%s"}`, app)
		ws.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		err := ws.WriteMessage(websocket.TextMessage, []byte(subscribeMsg))
		assert.NoError(t, err)

		ws.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		_, msg, err := ws.ReadMessage()
		assert.NoError(t, err)
		want := fmt.Sprintf(`{code:200,message:"subscribe %s success"}`, app)
		assert.Equal(t, want, string(msg))
	}
}

func TestWithRegister(t *testing.T) {
	tests := []struct {
		valid       bool
		description string
		memberID    int
		token       string
		authReply   string
		apps        []string
	}{
		{false, "anonymous user connect", -1, "", `{code:200,message:"hello stranger"}`, []string{"match"}},
		{true, "valid member connect", 123456, "654321", `{code:200,message:"hello 123456"}`, []string{"im", "match"}},
		{false, "not valid member connect", 12345, "65432", `{code:401,message:"unauthorized"}`, []string{"im", "match"}},
	}

	aStubWSClientStore := &StubWSClientStore{imClient: make(map[int][]Conn)}
	authServer := &FakeAuthServer{}
	server := httptest.NewServer(NewGatewayServer(aStubWSClientStore, authServer))
	defer server.Close()

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ws, response, err := newWSConnectTo(server)
			if err != nil {
				t.Fatalf("connection failed %v", err)
			}

			defer ws.Close()
			assert.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)

			assertAuth(t, ws, tt.memberID, tt.token, tt.authReply)

			assertSubscribe(t, ws, tt.apps, aStubWSClientStore)

			wantImClientCount := 0
			if tt.valid {
				wantImClientCount = 1
			}
			assert.Equal(t, wantImClientCount, len(aStubWSClientStore.privateWSClientsForMember(tt.memberID)))
		})
	}
	assert.Equal(t, 3, len(aStubWSClientStore.publicWSClientsForApp("match")))
}

func TestWithNoRegister(t *testing.T) {
	aStubWSClientStore := &StubWSClientStore{
		wsClients: make([]Conn, 0),
	}
	authServer := &FakeAuthServer{}
	server := httptest.NewServer(NewGatewayServer(aStubWSClientStore, authServer))
	defer server.Close()
	ws, response, err := newWSConnectTo(server)
	if err != nil {
		t.Fatalf("connection failed %v", err)
	}

	defer ws.Close()
	assert.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)

	err = ws.WriteMessage(websocket.TextMessage, []byte("Hello I'm hacker"))
	assert.NoError(t, err)

	ws.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
	_, msg, err := ws.ReadMessage()
	assert.NoError(t, err)
	assert.Equal(t, `{code:400,message:"missing auth message"}`, string(msg))

	ws.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
	_, _, err = ws.ReadMessage()
	assert.Error(t, err)
}

func TestPushMessage(t *testing.T) {
	imMemberID := 123456
	authServer := &FakeAuthServer{}
	store := &StubWSClientStore{
		imClient: make(map[int][]Conn),
	}
	ws1 := newStubWSConn("1")
	ws2 := newStubWSConn("2")
	store.save("match", -1, ws1)
	store.save("match", -1, ws2)
	store.save("im", imMemberID, ws2)
	server := NewGatewayServer(store, authServer)
	t.Run("broadcast message", func(t *testing.T) {
		ws1.clear()
		ws2.clear()
		msgText := `{"hello":"world"}`
		request := newPushMessagePostRequest("/broadcast", "match", -1, msgText)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		assert.Equal(t, response.Code, http.StatusAccepted)
		assert.Equal(t, true, store.publicWSClientsForAppWasCalled)

		assertBufferLengthEqual(t, len(ws1.buffer), 1)
		assert.Equal(t, []byte(msgText), ws1.buffer[0])

		assertBufferLengthEqual(t, len(ws2.buffer), 1)
		assert.Equal(t, []byte(msgText), ws2.buffer[0])
	})

	t.Run("unicast message", func(t *testing.T) {
		ws1.clear()
		ws2.clear()
		msgText := fmt.Sprintf(`{"hello":"%d"}`, imMemberID)
		request := newPushMessagePostRequest("/unicast", "im", imMemberID, msgText)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		assert.Equal(t, response.Code, http.StatusAccepted)
		assert.Equal(t, true, store.privateWSClientsForMemberWasCalled)

		assertBufferLengthEqual(t, len(ws1.buffer), 0)
		assertBufferLengthEqual(t, len(ws2.buffer), 1)
		assert.Equal(t, []byte(msgText), ws2.buffer[0])
	})
}

func assertBufferLengthEqual(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("websocket buffer length want %d, got %d", want, got)
	}
}

func newPushMessagePostRequest(url string, app string, memberID int, text string) *http.Request {
	msg := PushMessage{
		APP:      app,
		MemberID: memberID,
		Text:     text,
	}

	msgJSON, _ := json.Marshal(msg)
	request := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(msgJSON))
	return request
}

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

type StubWSClientStore struct {
	wsClients   []*websocket.Conn
	imClient    map[int][]*websocket.Conn
	matchClient []*websocket.Conn
}

func (s *StubWSClientStore) Save(app string, memberID int, ws *websocket.Conn) error {
	s.wsClients = append(s.wsClients, ws)
	if app == "im" && memberID > 0 {
		s.imClient[memberID] = append(s.imClient[memberID], ws)
	}

	if app == "match" && memberID > -1 {
		s.matchClient = append(s.matchClient, ws)
	}

	return nil
}

func (s *StubWSClientStore) PublicWSClientsForApp(app string) []*websocket.Conn {
	if app == "match" {
		return s.matchClient
	}
	return nil
}

func (s *StubWSClientStore) PrivateWSClientsForMember(memberID int) []*websocket.Conn {
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

	// aStubWSClientStore := &StubWSClientStore{imClient: make(map[int][]*websocket.Conn)}
	aStubWSClientStore := NewWSClientStore()
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
			assert.Equal(t, wantImClientCount, len(aStubWSClientStore.PrivateWSClientsForMember(tt.memberID)))
		})
	}
	assert.Equal(t, 3, len(aStubWSClientStore.PublicWSClientsForApp("match")))
}

func TestWithNoRegister(t *testing.T) {
	aStubWSClientStore := &StubWSClientStore{
		wsClients: make([]*websocket.Conn, 0),
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

func TestSubscription(t *testing.T) {
}

func TestPushMessage(t *testing.T) {
	aStubWSClientStore := &StubWSClientStore{
		wsClients: make([]*websocket.Conn, 0),
	}
	authServer := &FakeAuthServer{}
	server := NewGatewayServer(aStubWSClientStore, authServer)
	response := httptest.NewRecorder()
	msg := PushMessage{
		MemberID: -1,
		Text:     `{"hello":"world"}`,
	}

	msgJSON, _ := json.Marshal(msg)

	t.Run("broadcast message", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/broadcast", bytes.NewReader(msgJSON))
		server.ServeHTTP(response, request)
		assert.Equal(t, response.Code, http.StatusAccepted)
	})

	t.Run("unicast message", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/unicast", bytes.NewReader(msgJSON))
		server.ServeHTTP(response, request)
		assert.Equal(t, response.Code, http.StatusAccepted)
	})
}

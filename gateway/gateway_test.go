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
	wsClients []*websocket.Conn
}

func (s *StubWSClientStore) Save(memberID int, ws *websocket.Conn) error {
	s.wsClients = append(s.wsClients, ws)
	return nil
}

func newWSConnectTo(server *httptest.Server) (*websocket.Conn, *http.Response, error) {
	wsURLPrefix := "ws" + strings.TrimPrefix(server.URL, "http")
	wsURL := wsURLPrefix + "/push"
	return websocket.DefaultDialer.Dial(wsURL, nil)
}
func TestWithRegister(t *testing.T) {
	tests := []struct {
		description string
		memberID    int
		token       string
		reply       string
	}{
		{"anonymous user connect", -1, "", `{code:200,message:"hello stranger"}`},
		{"member connect", 123456, "654321", `{code:200,message:"hello 123456"}`},
	}

	aStubWSClientStore := &StubWSClientStore{
		wsClients: make([]*websocket.Conn, 0),
	}
	server := httptest.NewServer(NewGatewayServer(aStubWSClientStore))
	defer server.Close()

	nextWantWSClientsCount := 1
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ws, response, err := newWSConnectTo(server)
			if err != nil {
				t.Fatalf("connection failed %v", err)
			}

			defer ws.Close()
			assert.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)

			authMsg := fmt.Sprintf(`{"member_id": %d, "token": "%s"}`, tt.memberID, tt.token)
			err = ws.WriteMessage(websocket.TextMessage, []byte(authMsg))
			assert.NoError(t, err)

			ws.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
			_, msg, err := ws.ReadMessage()
			assert.NoError(t, err)
			assert.Equal(t, tt.reply, string(msg))

			assert.Equal(t, nextWantWSClientsCount, len(aStubWSClientStore.wsClients))
			nextWantWSClientsCount++
		})
	}
}

func TestWithNoRegister(t *testing.T) {
	aStubWSClientStore := &StubWSClientStore{
		wsClients: make([]*websocket.Conn, 0),
	}
	server := httptest.NewServer(NewGatewayServer(aStubWSClientStore))
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
	_, _, err = ws.ReadMessage()
	assert.Error(t, err)
}

func TestPushMessage(t *testing.T) {
	aStubWSClientStore := &StubWSClientStore{
		wsClients: make([]*websocket.Conn, 0),
	}
	server := NewGatewayServer(aStubWSClientStore)
	response := httptest.NewRecorder()
	msg := message{
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

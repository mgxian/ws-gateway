package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

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
			ws, response := mustConnectTo(t, server)
			defer ws.Close()

			assertStatusCode(t, response.StatusCode, http.StatusSwitchingProtocols)
			assertAuth(t, ws, tt.memberID, tt.token, tt.authReply)
			assertSubscribe(t, ws, tt.apps, aStubWSClientStore)

			wantImClientCount := 0
			if tt.valid {
				wantImClientCount = 1
			}
			assertWSclientCount(t, len(aStubWSClientStore.privateWSClientsForMember(tt.memberID)), wantImClientCount)
		})
	}
	assertWSclientCount(t, 3, len(aStubWSClientStore.publicWSClientsForApp("match")))
}

func assertWSclientCount(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("got wrong websocket client count got %d want %d", got, want)
	}
}

func TestWithNoRegister(t *testing.T) {
	store := &StubWSClientStore{
		wsClients: make([]Conn, 0),
	}
	authServer := &FakeAuthServer{}
	server := httptest.NewServer(NewGatewayServer(store, authServer))
	defer server.Close()

	ws, response := mustConnectTo(t, server)
	defer ws.Close()

	assertStatusCode(t, response.StatusCode, http.StatusSwitchingProtocols)

	err := ws.WriteMessage(websocket.TextMessage, []byte("Hello I'm hacker"))
	assertNoError(t, err)

	ws.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
	_, msg, err := ws.ReadMessage()
	assertNoError(t, err)
	assertMessage(t, string(msg), `{code:400,message:"missing auth message"}`)

	ws.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
	_, _, err = ws.ReadMessage()
	assertError(t, err)
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
		assertEqual(t, store.publicWSClientsForAppWasCalled, true)
		assertStatusCode(t, response.Code, http.StatusAccepted)

		assertBufferLengthEqual(t, len(ws1.buffer), 1)
		assertMessage(t, msgText, string(ws1.buffer[0]))

		assertBufferLengthEqual(t, len(ws2.buffer), 1)
		assertMessage(t, msgText, string(ws2.buffer[0]))
	})

	t.Run("unicast message", func(t *testing.T) {
		ws1.clear()
		ws2.clear()
		msgText := fmt.Sprintf(`{"hello":"%d"}`, imMemberID)
		request := newPushMessagePostRequest("/unicast", "im", imMemberID, msgText)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		assertStatusCode(t, response.Code, http.StatusAccepted)
		assertEqual(t, store.privateWSClientsForMemberWasCalled, true)

		assertBufferLengthEqual(t, len(ws1.buffer), 0)
		assertBufferLengthEqual(t, len(ws2.buffer), 1)
		assertMessage(t, msgText, string(ws2.buffer[0]))
	})
}

func mustConnectTo(t *testing.T, server *httptest.Server) (*websocket.Conn, *http.Response) {
	wsURLPrefix := "ws" + strings.TrimPrefix(server.URL, "http")
	wsURL := wsURLPrefix + "/push"
	ws, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("connection failed %v", err)
	}
	return ws, response
}

func assertEqual(t *testing.T, got, want interface{}) {
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func assertMessage(t *testing.T, got, want string) {
	if got != want {
		t.Errorf("got wrong message got %q, want %q", got, want)
	}
}

func assertResponse(t *testing.T, got, want string) {
	if got != want {
		t.Errorf("got wrong response got %q, want %q", got, want)
	}
}

func assertNoError(t *testing.T, err error) {
	if err != nil {
		t.Errorf("did not want error but got an error %v", err)
	}
}

func assertError(t *testing.T, err error) {
	if err == nil {
		t.Errorf("did want error but did not got an error %v", err)
	}
}

func assertStatusCode(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("got wrong status code got %d, want %d", got, want)
	}
}

func assertAuth(t *testing.T, ws *websocket.Conn, memberID int, token string, want string) {
	authMsg := fmt.Sprintf(`{"member_id": %d, "token": "%s"}`, memberID, token)
	err := ws.WriteMessage(websocket.TextMessage, []byte(authMsg))
	assertNoError(t, err)

	ws.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
	_, msg, err := ws.ReadMessage()
	assertNoError(t, err)
	assertMessage(t, string(msg), want)
}

func assertSubscribe(t *testing.T, ws *websocket.Conn, apps []string, store wsClientStore) {
	for _, app := range apps {
		subscribeMsg := fmt.Sprintf(`{"app": "%s"}`, app)
		ws.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		err := ws.WriteMessage(websocket.TextMessage, []byte(subscribeMsg))
		assertNoError(t, err)

		ws.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		_, msg, err := ws.ReadMessage()
		assertNoError(t, err)
		want := fmt.Sprintf(`{code:200,message:"subscribe %s success"}`, app)
		assertMessage(t, string(msg), want)
	}
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

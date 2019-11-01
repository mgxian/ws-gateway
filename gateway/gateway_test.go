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

func TestWithAuthentication(t *testing.T) {
	tests := []struct {
		valid           bool
		description     string
		memberID        int
		token           string
		wantedAuthReply string
		subscribedApps  []string
	}{
		{false, "anonymous user connect", anonymousMemberID, "", helloStrangerMessage(), []string{"match"}},
		{true, "valid member connect", 123456, "654321", helloMessageForMember(123456), []string{imApp, "match"}},
		{false, "not valid member connect", 12345, "65432", unauthorizedMessage(), []string{imApp, "match"}},
	}

	server, store := newServer()
	defer server.Close()

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ws, response := mustConnectTo(t, server)
			defer ws.Close()

			assertStatusCode(t, response.StatusCode, http.StatusSwitchingProtocols)
			mustSendAuthMessage(t, ws, tt.memberID, tt.token)
			msg := mustReadMessageWithTimeout(t, ws, time.Millisecond*10)
			assertMessage(t, msg, tt.wantedAuthReply)

			assertSubscribe(t, ws, tt.subscribedApps, tt.valid)

			wantImClientCount := 0
			if tt.valid {
				wantImClientCount = 1
			}
			assertWSClientCount(t, len(store.privateWSClientsForMember(tt.memberID)), wantImClientCount)
			assertWSClientCount(t, len(store.publicWSClientsForApp("match")), 1)
		})
	}
}

func TestWithNoAuthentication(t *testing.T) {
	server, _ := newServer()
	defer server.Close()

	ws, response := mustConnectTo(t, server)
	defer ws.Close()

	assertStatusCode(t, response.StatusCode, http.StatusSwitchingProtocols)

	mustWriteMessage(t, ws, "Hello I'm hacker")

	msg := mustReadMessageWithTimeout(t, ws, time.Millisecond*10)
	assertMessage(t, msg, missingAuthMessage())

	_, err := readMessageWithTimeout(ws, time.Millisecond*10)
	assertError(t, err)
}

func TestPushMessage(t *testing.T) {
	imMemberID := 123456
	authServer := &FakeAuthServer{}
	store := &StubWSStore{
		imClient: make(map[int][]Conn),
	}
	ws1 := newStubWSConn("1")
	ws2 := newStubWSConn("2")
	store.save("match", anonymousMemberID, ws1)
	store.save("match", anonymousMemberID, ws2)
	store.save(imApp, imMemberID, ws2)
	server := NewGatewayServer(store, authServer)
	t.Run("push public message", func(t *testing.T) {
		ws1.clear()
		ws2.clear()
		msgText := `{"hello":"world"}`
		request := newPushMessagePostRequest("match", anonymousMemberID, msgText)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		assertEqual(t, store.publicWSClientsForAppWasCalled, true)
		assertStatusCode(t, response.Code, http.StatusAccepted)

		assertBufferLengthEqual(t, len(ws1.buffer), 1)
		assertMessage(t, string(ws1.buffer[0]), string(pushMessageJSONFor("match", anonymousMemberID, msgText)))

		assertBufferLengthEqual(t, len(ws2.buffer), 1)
		assertMessage(t, string(ws2.buffer[0]), string(pushMessageJSONFor("match", anonymousMemberID, msgText)))
	})

	t.Run("push im message", func(t *testing.T) {
		ws1.clear()
		ws2.clear()
		msgText := fmt.Sprintf(`{"hello":"%d"}`, imMemberID)
		request := newPushMessagePostRequest(imApp, imMemberID, msgText)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		assertStatusCode(t, response.Code, http.StatusAccepted)
		assertEqual(t, store.privateWSClientsForMemberWasCalled, true)

		assertBufferLengthEqual(t, len(ws1.buffer), 0)
		assertBufferLengthEqual(t, len(ws2.buffer), 1)
		assertMessage(t, string(ws2.buffer[0]), string(pushMessageJSONFor(imApp, imMemberID, msgText)))
	})
}

func TestWSClose(t *testing.T) {
	server, store := newServer()
	defer server.Close()

	ws1, _ := mustConnectTo(t, server)
	mustSendAuthMessage(t, ws1, anonymousMemberID, "")
	mustSendSubscribeMessage(t, ws1, "match")

	ws2, _ := mustConnectTo(t, server)
	mustSendAuthMessage(t, ws2, 123456, "654321")
	mustSendSubscribeMessage(t, ws2, imApp)
	mustSendSubscribeMessage(t, ws2, "match")

	time.Sleep(time.Millisecond * 10)
	assertWSClientCount(t, len(store.publicWSClientsForApp("match")), 2)
	assertWSClientCount(t, len(store.privateWSClientsForMember(123456)), 1)

	ws1.Close()
	time.Sleep(time.Millisecond * 10)
	assertWSClientCount(t, len(store.publicWSClientsForApp("match")), 1)
	assertWSClientCount(t, len(store.privateWSClientsForMember(123456)), 1)

	ws2.Close()
	time.Sleep(time.Millisecond * 10)
	assertWSClientCount(t, len(store.publicWSClientsForApp("match")), 0)
	assertWSClientCount(t, len(store.privateWSClientsForMember(123456)), 0)
}

func TestWSPingPong(t *testing.T) {
	server, _ := newServer()
	defer server.Close()

	ws, _ := mustConnectTo(t, server)
	mustSendAuthMessage(t, ws, anonymousMemberID, "")
	mustReadMessageWithTimeout(t, ws, time.Millisecond*10)

	pongHandlerWasCalled := false
	ws.SetPongHandler(func(data string) error {
		pongHandlerWasCalled = true
		return nil
	})

	err := ws.WriteMessage(websocket.PingMessage, []byte("ping test"))
	assertNoError(t, err)

	readMessageWithTimeout(ws, time.Millisecond*10)
	assertEqual(t, pongHandlerWasCalled, true)
}

func newServer() (*httptest.Server, *StubWSStore) {
	store := &StubWSStore{imClient: make(map[int][]Conn)}
	authServer := &FakeAuthServer{}
	server := httptest.NewServer(NewGatewayServer(store, authServer))
	return server, store
}

func mustConnectTo(t *testing.T, server *httptest.Server) (*websocket.Conn, *http.Response) {
	wsURLPrefix := "ws" + strings.TrimPrefix(server.URL, "http")
	wsURL := wsURLPrefix + websocketURLPath
	ws, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("connection failed %v", err)
	}
	return ws, response
}

func assertWSClientCount(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("got wrong websocket client count got %d want %d", got, want)
	}
}

func assertEqual(t *testing.T, got, want interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func assertMessage(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("got wrong message got %q, want %q", got, want)
	}
}

func assertResponse(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("got wrong response got %q, want %q", got, want)
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("did not want error but got an error %v", err)
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()
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

func mustSendAuthMessage(t *testing.T, ws *websocket.Conn, memberID int, token string) {
	authMsg := fmt.Sprintf(`{"member_id": %d, "token": "%s"}`, memberID, token)
	mustWriteMessage(t, ws, authMsg)
}

func mustWriteMessage(t *testing.T, ws *websocket.Conn, msg string) {
	err := ws.WriteMessage(websocket.TextMessage, []byte(msg))
	if err != nil {
		t.Fatalf("must write message but get an %v", err)
	}
}

func mustReadMessageWithTimeout(t *testing.T, ws *websocket.Conn, timeout time.Duration) string {
	t.Helper()
	msg, err := readMessageWithTimeout(ws, timeout)
	if err != nil {
		t.Fatalf("must get message but did not get one %v", err)
	}
	return string(msg)
}

func readMessageWithTimeout(ws *websocket.Conn, timeout time.Duration) (string, error) {
	ws.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := ws.ReadMessage()
	return string(msg), err
}

func mustSendSubscribeMessage(t *testing.T, ws *websocket.Conn, app string) {
	subscribeMsg := fmt.Sprintf(`{"app": "%s"}`, app)
	mustWriteMessage(t, ws, subscribeMsg)
}

func assertSubscribe(t *testing.T, ws *websocket.Conn, apps []string, isValid bool) {
	for _, app := range apps {
		mustSendSubscribeMessage(t, ws, app)
		msg := mustReadMessageWithTimeout(t, ws, time.Millisecond*10)
		want := subscribeSuccessMessageForApp(app)
		if !isValid && isPrivateApp(app) {
			want = subscribeForbiddenMessageForApp(app)
		}
		assertMessage(t, msg, want)
	}
}

func assertBufferLengthEqual(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("websocket buffer length want %d, got %d", want, got)
	}
}

func newPushMessagePostRequest(app string, memberID int, text string) *http.Request {
	msgJSON := pushMessageJSONFor(app, memberID, text)
	request := httptest.NewRequest(http.MethodPost, pushURLPath, bytes.NewReader(msgJSON))
	return request
}

func pushMessageJSONFor(app string, memberID int, text string) []byte {
	msg := PushMessage{
		App:      app,
		MemberID: memberID,
		Text:     text,
	}

	msgJSON, _ := json.Marshal(msg)
	return msgJSON
}

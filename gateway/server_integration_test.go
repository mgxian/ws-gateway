package gateway

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestConnectToServerAndPushPublicMessage(t *testing.T) {
	store := NewInMemeryWSClientStore()
	authServer := &FakeAuthServer{}
	gateway := NewGatewayServer(store, authServer)
	server := httptest.NewServer(gateway)

	memberID := -1
	app := "match"
	token := ""
	ws1 := mustConnectAndAuthAndSubscribe(t, server, memberID, token, app)
	ws2 := mustConnectAndAuthAndSubscribe(t, server, memberID, token, app)

	pushText := `{"hello":"world"}`
	request := newPushMessagePostRequest("/public", app, memberID, pushText)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	msg1 := mustReadMessageWithTimeout(t, ws1, time.Millisecond*10)
	assertMessage(t, msg1, pushText)

	msg2 := mustReadMessageWithTimeout(t, ws2, time.Millisecond*10)
	assertMessage(t, msg2, pushText)
}

func TestConnectToServerAndPushPrivateMessage(t *testing.T) {
	store := NewInMemeryWSClientStore()
	authServer := &FakeAuthServer{}
	gateway := NewGatewayServer(store, authServer)
	server := httptest.NewServer(gateway)

	memberID1 := 123456
	memberID2 := 12345
	token := "654321"
	app := "im"
	ws1 := mustConnectAndAuthAndSubscribe(t, server, memberID1, token, app)
	ws2 := mustConnectAndAuthAndSubscribe(t, server, memberID2, token, app)

	pushText := fmt.Sprintf(`{"hello":"%d"}`, memberID1)
	request := newPushMessagePostRequest("/im", app, memberID1, pushText)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	msg := mustReadMessageWithTimeout(t, ws1, time.Millisecond*10)
	assertMessage(t, msg, pushText)

	_, err := readMessageWithTimeout(ws2, time.Millisecond*10)
	assertError(t, err)
}

func mustConnectAndAuthAndSubscribe(t *testing.T, server *httptest.Server, memberID int, token string, app string) *websocket.Conn {
	ws, _ := mustConnectTo(t, server)
	mustSendAuthMessage(t, ws, memberID, token)
	mustSendSubscribeMessage(t, ws, app)
	mustReadMessageWithTimeout(t, ws, time.Millisecond*10)
	mustReadMessageWithTimeout(t, ws, time.Millisecond*10)
	return ws
}

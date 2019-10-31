package gateway

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/julienschmidt/httprouter"
)

func TestConnectToServerAndPushPublicMessage(t *testing.T) {
	store := NewInMemeryWSClientStore()
	authServer := &FakeAuthServer{}
	gateway := NewGatewayServer(store, authServer)
	server := httptest.NewServer(gateway)

	memberID := anonymousMemberID
	app := "match"
	token := ""
	ws1 := mustConnectAndAuthAndSubscribe(t, server, memberID, token, app)
	ws2 := mustConnectAndAuthAndSubscribe(t, server, memberID, token, app)

	pushText := `{"hello":"world"}`
	request := newPushMessagePostRequest(app, memberID, pushText)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	msg1 := mustReadMessageWithTimeout(t, ws1, time.Millisecond*10)
	assertMessage(t, msg1, string(pushMessageJSONFor(app, memberID, pushText)))

	msg2 := mustReadMessageWithTimeout(t, ws2, time.Millisecond*10)
	assertMessage(t, msg2, string(pushMessageJSONFor(app, memberID, pushText)))
}

func TestConnectToServerAndPushPrivateMessage(t *testing.T) {
	store := NewInMemeryWSClientStore()
	authServer := &FakeAuthServer{}
	gateway := NewGatewayServer(store, authServer)
	server := httptest.NewServer(gateway)

	memberID1 := 123456
	memberID2 := 12345
	token := "654321"
	ws1 := mustConnectAndAuthAndSubscribe(t, server, memberID1, token, imApp)
	ws2 := mustConnectAndAuthAndSubscribe(t, server, memberID2, token, imApp)

	pushText := fmt.Sprintf(`{"hello":"%d"}`, memberID1)
	request := newPushMessagePostRequest(imApp, memberID1, pushText)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	msg := mustReadMessageWithTimeout(t, ws1, time.Millisecond*10)
	assertMessage(t, msg, string(pushMessageJSONFor(imApp, memberID1, pushText)))

	_, err := readMessageWithTimeout(ws2, time.Millisecond*10)
	assertError(t, err)
}

func TestGatewayStress(t *testing.T) {
	go startServer()
	waitForServerReady()

	app := "match"
	batchCount := 200
	clientCount := 20000
	wsChan := make(chan *websocket.Conn, 100)
	go generateClients(t, app, clientCount, batchCount, wsChan)
	wsClients := collectClients(wsChan)

	msgText := `{"hello":"world"}`
	response, err := pushMessage("http://localhost:5000"+pushURLPath, app, anonymousMemberID, msgText)
	if err != nil {
		t.Fatalf("post push message error: %v", err)
	}
	assertStatusCode(t, response.StatusCode, http.StatusAccepted)

	errorCount := 0
	successCount := 0
	for _, ws := range wsClients {
		msg, err := readMessageWithTimeout(ws, time.Millisecond*10)
		if err != nil {
			errorCount++
		}
		if msg == string(pushMessageJSONFor(app, anonymousMemberID, msgText)) {
			successCount++
		}
	}
	log.Println(len(wsClients), successCount, errorCount)
}

func mustConnectAndAuthAndSubscribe(t *testing.T, server *httptest.Server, memberID int, token string, app string) *websocket.Conn {
	ws, _ := mustConnectTo(t, server)
	mustSendAuthMessage(t, ws, memberID, token)
	mustSendSubscribeMessage(t, ws, app)
	mustReadMessageWithTimeout(t, ws, time.Millisecond*10)
	mustReadMessageWithTimeout(t, ws, time.Millisecond*10)
	return ws
}

func pushMessage(url string, app string, memberID int, text string) (*http.Response, error) {
	msgJSON := pushMessageJSONFor(app, memberID, text)
	return http.Post(url, "application/json", bytes.NewReader(msgJSON))
}

func startServer() {
	store := NewInMemeryWSClientStore()
	authServer := &FakeAuthServer{}
	gateway := NewGatewayServer(store, authServer)
	httpRouter := httprouter.New()
	httpRouter.GET(websocketURLPath, gateway.websocket)
	httpRouter.POST(pushURLPath, gateway.push)
	if err := http.ListenAndServe(":5000", httpRouter); err != nil {
		log.Fatalf("could not listen on port 5000 %v", err)
	}
}

func waitForServerReady() {
	for {
		response, _ := http.Get("http://localhost:5000/health")
		if response != nil {
			break
		}
	}
}

func collectClients(wsChan chan *websocket.Conn) (wsClients []*websocket.Conn) {
	count := 0
	for ws := range wsChan {
		count++
		log.Printf("client received %d ---> %s", count, ws.LocalAddr().String())
		wsClients = append(wsClients, ws)
	}
	return
}

func generateClients(t *testing.T, app string, count int, batchCount int, wsChan chan *websocket.Conn) {
	currentCount := 0
	generateClientsFailed := false
	for {
		lastCount := currentCount
		leftCount := count - currentCount
		batchCount := batchCount
		if leftCount < batchCount {
			batchCount = leftCount
		}
		currentCount += concurrentGetClients(t, batchCount, app, wsChan)
		if currentCount >= count {
			break
		}
		if lastCount == currentCount {
			generateClientsFailed = true
			break
		}
	}
	close(wsChan)
	if generateClientsFailed {
		log.Println("no enough resources")
	}
}

func concurrentGetClients(t *testing.T, batchCount int, app string, wsChan chan *websocket.Conn) int {
	var m sync.Mutex
	var wg sync.WaitGroup
	count := 0
	wg.Add(batchCount)
	for i := 0; i < batchCount; i++ {
		go func() {
			defer wg.Done()
			wsURL := "ws://localhost:5000" + websocketURLPath
			ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				return
			}

			mustSendAuthMessage(t, ws, anonymousMemberID, "")
			mustSendSubscribeMessage(t, ws, app)
			_, err = readMessageWithTimeout(ws, time.Millisecond*100)
			if err != nil {
				ws.Close()
				return
			}
			_, err = readMessageWithTimeout(ws, time.Millisecond*100)
			if err != nil {
				ws.Close()
				return
			}
			m.Lock()
			count++
			m.Unlock()
			wsChan <- ws
		}()
	}
	wg.Wait()
	return count
}

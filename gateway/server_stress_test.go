package gateway

import (
	"bytes"
	"log"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestGatewayStress(t *testing.T) {
	go startServer()
	waitForServerReady()

	app := "match"
	batchCount := 200
	clientCount := 2000
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

func startServer() {
	store := NewInMemeryWSClientStore()
	authServer := &FakeAuthServer{}
	gateway := NewGatewayServer(store, authServer)
	if err := http.ListenAndServe(":5000", gateway); err != nil {
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

func pushMessage(url string, app string, memberID int, text string) (*http.Response, error) {
	msgJSON := pushMessageJSONFor(app, memberID, text)
	return http.Post(url, "application/json", bytes.NewReader(msgJSON))
}

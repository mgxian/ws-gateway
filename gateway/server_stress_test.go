package gateway

import (
	"bytes"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const (
	testWebSocketURL = "ws://localhost:5000"
	testPushHost     = "http://localhost:5000"
)

// const (
// 	testWebSocketURL = "ws://172.16.11.201:5000"
// 	testPushHost     = "http://172.16.11.201:5000"
// )

func TestGatewayStress(t *testing.T) {
	go startServer()
	waitForServerReady()

	app := "match"
	batchCount := 100
	clientCount := 1000
	wsChan := make(chan *websocket.Conn, 100)
	go generateClients(t, app, uint64(clientCount), uint64(batchCount), wsChan)
	wsClients := collectClients(wsChan)

	msgText := `{"hello":"world"}`
	response, err := pushMessage(testPushHost+pushURLPath, app, anonymousMemberID, msgText)
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

func generateClients(t *testing.T, app string, count uint64, batchCount uint64, wsChan chan *websocket.Conn) {
	var currentCount uint64
	generateClientsFailed := false
	for {
		lastCount := currentCount
		leftCount := count - currentCount
		batchCount := batchCount
		if leftCount < batchCount {
			batchCount = leftCount
		}
		successCount, failedCount := concurrentGetClients(t, int(batchCount), app, wsChan)
		log.Printf("websocket connection failed count: %d", failedCount)
		currentCount += successCount
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

func concurrentGetClients(t *testing.T, batchCount int, app string, wsChan chan *websocket.Conn) (uint64, uint64) {
	var wg sync.WaitGroup
	var count uint64
	var failedCount uint64
	wg.Add(batchCount)
	for i := 0; i < batchCount; i++ {
		go func() {
			defer wg.Done()
			wsURL := testWebSocketURL + websocketURLPath
			ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				atomic.AddUint64(&failedCount, 1)
				return
			}

			mustSendAuthMessage(t, ws, anonymousMemberID, "")
			mustSendSubscribeMessage(t, ws, app)
			_, err = readMessageWithTimeout(ws, time.Millisecond*100)
			if err != nil {
				ws.Close()
				atomic.AddUint64(&failedCount, 1)
				return
			}
			_, err = readMessageWithTimeout(ws, time.Millisecond*100)
			if err != nil {
				ws.Close()
				atomic.AddUint64(&failedCount, 1)
				return
			}
			atomic.AddUint64(&count, 1)
			wsChan <- ws
		}()
	}
	wg.Wait()
	return count, failedCount
}

func pushMessage(url string, app string, memberID int, text string) (*http.Response, error) {
	msgJSON := pushMessageJSONFor(app, memberID, text)
	return http.Post(url, "application/json", bytes.NewReader(msgJSON))
}

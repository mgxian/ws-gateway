package gateway

import "github.com/gorilla/websocket"

type wsConn struct {
	conn *websocket.Conn
}

func newWSConn(conn *websocket.Conn) *wsConn {
	return &wsConn{
		conn: conn,
	}
}

func (ws *wsConn) ReadMessage() ([]byte, error) {
	_, msg, err := ws.conn.ReadMessage()
	return msg, err
}

func (ws *wsConn) WriteMessage(msg []byte) error {
	return ws.conn.WriteMessage(websocket.TextMessage, msg)
}

func (ws *wsConn) RemoteAddr() string {
	return ws.conn.RemoteAddr().String()
}

package gateway

// StubWSConn implements Conn for testing purpose
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

// ReadMessage returns message from buffer
func (s *StubWSConn) ReadMessage() (msg []byte, err error) {
	result := s.buffer[0]
	s.buffer = s.buffer[1:]
	return result, nil
}

// WriteMessage writes message to buffer
func (s *StubWSConn) WriteMessage(msg []byte) error {
	s.buffer = append(s.buffer, msg)
	return nil
}

// RemoteAddr returns addr
func (s *StubWSConn) RemoteAddr() string {
	return s.addr
}

// StubWSStore implements wsStore for testing purpose
type StubWSStore struct {
	wsClients                          []Conn
	imClient                           map[int][]Conn
	matchClient                        []Conn
	publicWSClientsForAppWasCalled     bool
	privateWSClientsForMemberWasCalled bool
}

func (s *StubWSStore) save(app string, memberID int, ws Conn) error {
	s.wsClients = append(s.wsClients, ws)
	if app == privateApp && memberID > 0 {
		s.imClient[memberID] = append(s.imClient[memberID], ws)
	}

	if app == "match" {
		s.matchClient = append(s.matchClient, ws)
	}

	return nil
}

func (s *StubWSStore) delete(memberID int, ws Conn) {
	if memberID != -1 {
		delete(s.imClient, memberID)
	}

	foundIndex := -1
	for i, c := range s.matchClient {
		if c.RemoteAddr() == ws.RemoteAddr() {
			foundIndex = i
			break
		}
	}

	if foundIndex != -1 {
		left := make([]Conn, 0)
		left = append(left, s.matchClient[:foundIndex]...)
		left = append(left, s.matchClient[foundIndex+1:]...)
		s.matchClient = left
	}
}

func (s *StubWSStore) publicWSClientsForApp(app string) []Conn {
	s.publicWSClientsForAppWasCalled = true
	if app == "match" {
		return s.matchClient
	}
	return nil
}

func (s *StubWSStore) privateWSClientsForMember(memberID int) []Conn {
	s.privateWSClientsForMemberWasCalled = true
	return s.imClient[memberID]
}

// FakeAuthServer implements AuthServer for testing purpose
type FakeAuthServer struct{}

// Auth check if member authed
func (s *FakeAuthServer) Auth(memberID int, token string) bool {
	if memberID == 12345 {
		return false
	}
	return true
}

package gateway

// StubWSConn implements Conn for testing purposes
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

// StubWSClientStore implements wsClientStore for testing purposes
type StubWSClientStore struct {
	wsClients                          []Conn
	imClient                           map[int][]Conn
	matchClient                        []Conn
	publicWSClientsForAppWasCalled     bool
	privateWSClientsForMemberWasCalled bool
}

func (s *StubWSClientStore) save(app string, memberID int, ws Conn) error {
	s.wsClients = append(s.wsClients, ws)
	if app == "im" && memberID > 0 {
		s.imClient[memberID] = append(s.imClient[memberID], ws)
	}

	if app == "match" {
		s.matchClient = append(s.matchClient, ws)
	}

	return nil
}

func (s *StubWSClientStore) publicWSClientsForApp(app string) []Conn {
	s.publicWSClientsForAppWasCalled = true
	if app == "match" {
		return s.matchClient
	}
	return nil
}

func (s *StubWSClientStore) privateWSClientsForMember(memberID int) []Conn {
	s.privateWSClientsForMemberWasCalled = true
	return s.imClient[memberID]
}

// FakeAuthServer implements AuthServer for testing purposes
type FakeAuthServer struct{}

// Auth check if member authed
func (s *FakeAuthServer) Auth(memberID int, token string) bool {
	if memberID == 12345 {
		return false
	}
	return true
}

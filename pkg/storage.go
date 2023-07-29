package pkg

import (
	"log"
	"sync"
)

type SharedContent struct {
	value string
	cond  *sync.Cond
}

func (s *SharedContent) Get() string {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	if s.value != "" {
		return s.value
	}
	s.cond.Wait()
	return s.value
}
func (s *SharedContent) Set(value string) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	s.value = value
	s.cond.Broadcast()

}
func NewSharedContent() *SharedContent {
	m := sync.Mutex{}
	return &SharedContent{
		cond: sync.NewCond(&m),
	}
}

var clients = make(map[string]*Storage)
var clientsMu = sync.RWMutex{}

type Storage struct {
	ClientID string
	values   map[string]*SharedContent
	mu       *sync.RWMutex
	chalChan chan *DocResponse
}

func (s *Storage) GetOrCreate(path string) *SharedContent {
	s.mu.RLock()
	if val, ok := s.values[path]; ok {
		s.mu.RUnlock()
		return val
	}
	s.mu.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if val, ok := s.values[path]; ok {
		return val
	}
	log.Println("SEND CHAL", path)
	s.chalChan <- &DocResponse{Path: path, Chal: true}
	log.Println("CHAL SENT", path)
	c := NewSharedContent()
	s.values[path] = c
	return c
}

func (s *Storage) Set(path string, content string) {
	s.mu.RLock()
	if curr, ok := s.values[path]; ok {
		s.mu.RUnlock()
		curr.Set(content)
		return
	}
	s.mu.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if curr, ok := s.values[path]; ok {
		curr.Set(content)
		return
	}
	c := NewSharedContent()
	s.values[path] = c
	c.Set(content)
}

func (s *Storage) Close() {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	delete(clients, s.ClientID)
}

func NewStorage(clientID string, chalChan chan *DocResponse) *Storage {
	storage := &Storage{
		chalChan: chalChan,
		ClientID: clientID,
		values:   make(map[string]*SharedContent),
		mu:       &sync.RWMutex{},
	}
	clientsMu.Lock()
	clients[clientID] = storage
	clientsMu.Unlock()
	return storage
}

func GetFromStorage(clientID, reqPath string) *SharedContent {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	client, ok := clients[clientID]
	if !ok {
		return nil
	}
	return client.GetOrCreate(reqPath)
}

package pkg

import (
	"sync"

	"github.com/google/uuid"
)

var clients = make(map[string]*Storage)
var clientsMu = sync.RWMutex{}

type Storage struct {
	ClientID string
	values   map[string]string
	mu       *sync.RWMutex
}

func (s *Storage) Get(hash string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.values[hash]
	return val, ok
}

func (s *Storage) Set(hash string, val string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[hash] = val
}

func (s *Storage) Close() {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	delete(clients, s.ClientID)
}

func NewStorage() *Storage {
	clientID := uuid.New().String()
	storage := &Storage{ClientID: clientID, values: make(map[string]string), mu: &sync.RWMutex{}}
	clientsMu.Lock()
	clients[clientID] = storage
	clientsMu.Unlock()
	return storage
}

func GetFromStorage(clientID, hash string) (string, bool) {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	client, ok := clients[clientID]
	if !ok {
		return "", false
	}
	return client.Get(hash)
}

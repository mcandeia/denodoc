package pkg

import (
	"sync"
)

type CachedContent struct {
	inner  chan string
	cached string
	mu     *sync.RWMutex
}

func (c *CachedContent) Get() string {
	c.mu.RLock()
	if c.cached != "" {
		c.mu.RUnlock()
		return c.cached
	}
	c.mu.RUnlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached != "" {
		return c.cached
	}
	c.cached = <-c.inner
	return c.cached
}
func (c *CachedContent) Set(val string) {
	c.inner <- val
	close(c.inner)
}

func NewCachedContent() *CachedContent {
	return &CachedContent{
		inner: make(chan string, 1),
		mu:    &sync.RWMutex{},
	}

}

var clients = make(map[string]*Storage)
var clientsMu = sync.RWMutex{}

type Storage struct {
	ClientID string
	values   map[string]*CachedContent
	mu       *sync.RWMutex
	chalChan chan *DocResponse
}

func (s *Storage) GetOrCreate(path string) *CachedContent {
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
	s.chalChan <- &DocResponse{Path: path, Chal: true}
	c := NewCachedContent()
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
	c := NewCachedContent()
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
		values:   make(map[string]*CachedContent),
		mu:       &sync.RWMutex{},
	}
	clientsMu.Lock()
	clients[clientID] = storage
	clientsMu.Unlock()
	return storage
}

func GetFromStorage(clientID, reqPath string) *CachedContent {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	client, ok := clients[clientID]
	if !ok {
		return nil
	}
	return client.GetOrCreate(reqPath)
}

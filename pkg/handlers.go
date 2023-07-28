package pkg

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/klauspost/compress/zstd"
)

type BeginDenoDocRequest struct {
	ImportMap string `json:"importMap"`
	CWD       string `json:"cwd"`
}

type DocResponse struct {
	Path     string `json:"path"`
	DocNodes string `json:"docNodes"`
}

type DocRequest struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Hash    string `json:"hash,omitempty"`
}

// Create a writer that caches compressors.
// For this operation type we supply a nil Reader.
var encoder, _ = zstd.NewWriter(nil)

// Compress a buffer.
// If you have a destination buffer, the allocation in the call can also be eliminated.
func Compress(src []byte) []byte {
	return encoder.EncodeAll(src, make([]byte, 0, len(src)))
}

// Create a reader that caches decompressors.
// For this operation type we supply a nil Reader.
var decoder, _ = zstd.NewReader(nil, zstd.WithDecoderConcurrency(0))

// Decompress a buffer. We don't supply a destination buffer,
// so it will be allocated by the decoder.
func Decompress(src []byte) ([]byte, error) {
	return decoder.DecodeAll(src, nil)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func Unmarshal(msg []byte, obj any) error {
	decompressed, err := Decompress(msg)
	if err != nil {
		return err
	}
	return json.Unmarshal(decompressed, obj)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path[:3] != "/ws" {
		if r.URL.RawQuery == "" {
			w.WriteHeader(200)
			w.Write([]byte("[]"))
			return
		}
		serveDenoDoc(w, r)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	abortChan := make(chan error, 1)
	closeHandler := conn.CloseHandler()
	conn.SetCloseHandler(func(code int, text string) error {
		err := closeHandler(code, text)
		abortChan <- err
		return err
	})
	defer conn.Close()

	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Println("read:", err)
		return
	}

	var firstMessage = BeginDenoDocRequest{}
	if err := Unmarshal(message, &firstMessage); err != nil {
		log.Println("read:", err)
		return
	}

	if firstMessage.CWD == "" {
		log.Println("first message is empty")
		return
	}

	importMap, err := ensureCreated(&firstMessage)
	if err != nil {
		log.Println("could not ensure created", err)
		return
	}
	storage := NewStorage()
	defer storage.Close()

	doc := NewDenoDoc(importMap, firstMessage.CWD, storage, abortChan)
	mu := &sync.Mutex{}

	waitAll := &sync.WaitGroup{}
	for {
		mt, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("read err:", err)
			break
		}
		waitAll.Add(1)
		var req = &DocRequest{}
		if err := Unmarshal(message, req); err != nil {
			log.Println("could not unmarshal:", err)
			break
		}
		go func() {
			defer waitAll.Done()

			resp, err := doc.Run(req)
			if err != nil {
				log.Println("denodoc err", err)
				return
			}
			bts, err := json.Marshal(resp)
			if err != nil {
				log.Println("marshal err", err)
				return
			}
			compressed := Compress(bts)
			mu.Lock()
			defer mu.Unlock()
			if err := conn.WriteMessage(mt, compressed); err != nil {
				log.Println("write message err", err)
			}
		}()
	}
	waitAll.Wait()
}
func serveDenoDoc(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	hash, clientId := qs.Get("hash"), qs.Get("client_id")
	if hash == "" || clientId == "" {
		w.WriteHeader(400)
		return
	}
	val, ok := GetFromStorage(clientId, hash)
	if !ok {
		w.WriteHeader(404)
		return
	}
	w.WriteHeader(200)
	w.Write([]byte(val))
}
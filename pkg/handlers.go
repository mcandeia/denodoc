package pkg

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
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
	Chal     bool   `json:"chal"`
}

type DocRequest struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Hash    string `json:"hash,omitempty"`
	Chal    bool   `json:"chal,omitempty"`
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

func Marshal(obj any) ([]byte, error) {
	bts, err := json.Marshal(obj)
	if err != nil {
		log.Println("marshal err", err)
		return nil, err
	}
	return Compress(bts), nil
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path[:3] != "/ws" {
		serveDenoDoc(w, r)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	abortChan := make(chan error, 1)
	defer close(abortChan)
	closeHandler := conn.CloseHandler()
	defer conn.Close()

	conn.SetCloseHandler(func(code int, text string) error {
		err := closeHandler(code, text)
		abortChan <- err
		return err
	})

	mt, message, err := conn.ReadMessage()
	if err != nil {
		log.Println("read:", err)
		return
	}

	var firstMessage = BeginDenoDocRequest{}
	if err := Unmarshal(message, &firstMessage); err != nil || firstMessage.CWD == "" {
		log.Println("first message err or invalid:", err)
		return
	}
	clientID := uuid.New().String()
	importMap, err := ensureCreated(&firstMessage, clientID)
	if err != nil {
		log.Println("could not ensure created", err)
		return
	}
	docRespChan := make(chan *DocResponse)
	defer close(docRespChan)
	storage := NewStorage(clientID, docRespChan)
	defer storage.Close()

	doc := NewDenoDoc(importMap, firstMessage.CWD, storage, abortChan)

	// send loop
	go func() {
		for {
			select {
			case err := <-abortChan:
				if err != nil {
					log.Println(fmt.Sprintf("aborting send loop err %v", err))
				}
				return
			case resp := <-docRespChan:
				bts, err := Marshal(resp)
				if err != nil {
					abortChan <- fmt.Errorf("error when marshalling resp %v", err)
					return
				}

				if err := conn.WriteMessage(mt, bts); err != nil {
					abortChan <- fmt.Errorf("error when writing message %v", err)
					return
				}
			}

		}
	}()
	docReqChan := make(chan *DocRequest)
	defer close(docReqChan)

	waitAll := &sync.WaitGroup{}
	go func() {
		for req := range docReqChan {
			waitAll.Add(1)
			go func(req *DocRequest) {
				defer waitAll.Done()
				resp, err := doc.Run(req)
				if err != nil {
					log.Println("denodoc err", err, req.Path)
					return
				}
				docRespChan <- resp
			}(req)
		}
	}()
	// recv loop
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			abortChan <- fmt.Errorf("read err: %v", err)
			break
		}
		var req = &DocRequest{}
		if err := Unmarshal(message, req); err != nil {
			abortChan <- fmt.Errorf("could not unmarshal: %v", err)
			break
		}
		docReqChan <- req
	}
	waitAll.Wait()
}
func serveDenoDoc(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(r.URL.Path, "/", 3)
	if len(parts) < 3 {
		log.Println("parts is less than 3", parts, r.URL.Path)
		w.WriteHeader(400)
		return
	}

	val := GetFromStorage(parts[1], parts[2])
	if val == nil {
		log.Println("NOT EXISTS", r.URL.Path)
		w.WriteHeader(200)
		w.Write([]byte("[]"))
		return
	}
	content := val.Get()
	w.WriteHeader(200)
	w.Write([]byte(content))
}

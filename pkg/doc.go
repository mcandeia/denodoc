package pkg

import (
	"os/exec"
	"strings"

	"golang.org/x/sync/singleflight"

	lru "github.com/hashicorp/golang-lru/v2"
)

const selfHost = "http://localhost:8080"

var cache, _ = lru.New[string, string](1024)
var flights = singleflight.Group{}

func runDenoDoc(importMap, path string) (string, error) {
	cmd := exec.Command("deno", "doc", "--import-map", importMap, "--json", path)

	// The `Output` method executes the command and
	// collects the output, returning its value
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type DenoDoc struct {
	ImportMap *ImportMapEntry
	CWD       string
	abortChan <-chan error
	Storage   *Storage
}

func (d *DenoDoc) Run(req *DocRequest) (*DocResponse, error) {
	resp, err, _ := flights.Do(req.Path, func() (any, error) {
		if req.Content == "" { // http req
			if cached, ok := cache.Get(req.Path); ok {
				return &DocResponse{Path: req.Path, DocNodes: cached}, nil
			}
			out, err := runDenoDoc(d.ImportMap.Path, req.Path)
			if err != nil {
				return nil, err
			}
			cache.Add(req.Path, out)
			return &DocResponse{Path: req.Path, DocNodes: out}, nil
		}
		if cached, ok := cache.Get(req.Hash); ok {
			return &DocResponse{Path: req.Path, DocNodes: cached}, nil
		}

		d.Storage.Set(req.Hash, req.Content)

		hashedPath := strings.Replace(req.Path, d.CWD, selfHost, 1) + "?hash=" + req.Hash + "&client_id=" + d.Storage.ClientID
		out, err := runDenoDoc(d.ImportMap.Path, hashedPath)
		if err != nil {
			return nil, err
		}

		docNodes := strings.ReplaceAll(out, selfHost, d.CWD)
		cache.Add(req.Hash, docNodes)
		return &DocResponse{Path: req.Path, DocNodes: docNodes}, nil

	})
	if err != nil {
		return nil, err
	}
	return resp.(*DocResponse), nil
}

func NewDenoDoc(importMap *ImportMapEntry, cwd string, st *Storage, abortChan <-chan error) *DenoDoc {
	return &DenoDoc{
		ImportMap: importMap,
		abortChan: abortChan,
		CWD:       cwd,
		Storage:   st,
	}
}

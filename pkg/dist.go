package pkg

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/singleflight"

	lru "github.com/hashicorp/golang-lru/v2"
)

type ImportMapEntry struct {
	Hash string
	Path string
}

var importMaps, _ = lru.New[string, *ImportMapEntry](1024)

var mu = singleflight.Group{}

func md5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

func useLocalHttpServer(importMap, clientID string) string {
	return strings.ReplaceAll(importMap, "\"./\"", fmt.Sprintf("\"%s/%s/\"", selfHost, clientID))
}
func ensureCreated(req *BeginDenoDocRequest, clientID string) (*ImportMapEntry, error) {
	val, err, _ := mu.Do(req.ImportMap, func() (any, error) {
		if val, ok := importMaps.Get(req.ImportMap); ok {
			return val, nil
		}
		importMapMd5 := md5Hash(req.ImportMap)
		importMapDir := filepath.Join(".", "dist", importMapMd5)
		if err := os.MkdirAll(importMapDir, os.ModePerm); err != nil {
			return nil, err
		}

		importMapPath := filepath.Join(importMapDir, "import_map.json")

		if err := os.WriteFile(importMapPath, []byte(useLocalHttpServer(req.ImportMap, clientID)), 0644); err != nil {
			return nil, err
		}

		entry := &ImportMapEntry{
			Hash: importMapMd5,
			Path: importMapPath,
		}
		importMaps.Add(req.ImportMap, entry)

		return entry, nil
	})
	if err != nil {
		return nil, err
	}

	return val.(*ImportMapEntry), nil
}

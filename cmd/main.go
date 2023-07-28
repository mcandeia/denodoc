// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/mcandeia/denodoc/pkg"
)

const addr = "0.0.0.0:8080"

func main() {
	flag.Parse()
	http.HandleFunc("/", pkg.Handler)
	log.Println(fmt.Sprintf("Listening on %s", addr))
	log.Fatal(http.ListenAndServe(addr, nil))
}

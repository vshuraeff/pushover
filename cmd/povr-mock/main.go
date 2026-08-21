package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/vshuraeff/pushover/internal/mockpushover"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "listen address")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server, err := mockpushover.Start(mockpushover.Config{Addr: *addr})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	startup := struct {
		APIURL string `json:"api_url"`
		WSURL  string `json:"ws_url"`
	}{
		APIURL: server.URL(),
		WSURL:  strings.Replace(server.URL(), "http://", "ws://", 1) + "/push",
	}
	data, err := json.Marshal(startup)
	if err != nil {
		_ = server.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(data))

	<-ctx.Done()
	if err := server.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

type helperRequest struct {
	ID         string       `json:"id"`
	Title      string       `json:"title"`
	Subtitle   string       `json:"subtitle,omitempty"`
	Body       string       `json:"body"`
	Icon       string       `json:"icon,omitempty"`
	Attachment string       `json:"attachment,omitempty"`
	Sound      *helperSound `json:"sound,omitempty"`
	Level      string       `json:"level"`
	Thread     string       `json:"thread,omitempty"`
	URL        string       `json:"url,omitempty"`
}

type helperSound struct {
	Name string `json:"name,omitempty"`
	File string `json:"file,omitempty"`
}

type helperEvent struct {
	Event         string `json:"event"`
	ID            string `json:"id,omitempty"`
	Error         string `json:"error,omitempty"`
	URL           string `json:"url,omitempty"`
	Granted       *bool  `json:"granted,omitempty"`
	TimeSensitive *bool  `json:"timeSensitive,omitempty"`
	LevelFallback bool   `json:"levelFallback,omitempty"`
	SoundFallback bool   `json:"soundFallback,omitempty"`
}

type requestResult struct {
	request helperRequest
	err     error
}

func main() {
	once := flag.Bool("once", false, "post one notification and exit")
	flag.Parse()
	os.Exit(run(os.Stdin, os.Stdout, os.Stderr, *once))
}

func readRequestLines(r io.Reader, once bool, results chan<- requestResult) {
	defer close(results)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var request helperRequest
		err := json.Unmarshal(scanner.Bytes(), &request)
		results <- requestResult{request: request, err: err}
		if once {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		results <- requestResult{err: fmt.Errorf("read request: %w", err)}
	}
}

func emitEvent(w io.Writer, event helperEvent) error {
	return json.NewEncoder(w).Encode(event)
}

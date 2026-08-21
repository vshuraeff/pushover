//go:build !(darwin && cgo)

package main

import "io"

func run(stdin io.Reader, stdout, _ io.Writer, once bool) int {
	results := make(chan requestResult)
	go readRequestLines(stdin, once, results)

	for result := range results {
		if result.err != nil {
			if err := emitEvent(stdout, helperEvent{Event: "error", Error: result.err.Error()}); err != nil {
				return 1
			}
			if once {
				return 1
			}
			continue
		}

		event := helperEvent{
			Event: "error",
			ID:    result.request.ID,
			Error: "helper unavailable on this platform/build",
		}
		if err := emitEvent(stdout, event); err != nil {
			return 1
		}
		if once {
			return 1
		}
	}

	if once {
		return 1
	}
	return 0
}

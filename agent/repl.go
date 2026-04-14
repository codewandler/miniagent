package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
)

// RunREPL runs an interactive prompt loop. Conversation history persists
// across turns. input is the source of user prompts (typically os.Stdin).
// Returns nil on clean exit (EOF / "exit" / "quit").
func RunREPL(ctx context.Context, a *Agent, input io.Reader) error {
	scanner := bufio.NewScanner(input)
	out := a.Out()
	turnID := 0

	// Persistent signal handler — routes SIGINT based on agent state.
	var (
		mu         sync.Mutex
		turnCancel context.CancelFunc
	)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	// [REVIEW FIX #3]: close(sigCh) after Stop to terminate the goroutine.
	defer func() {
		signal.Stop(sigCh)
		close(sigCh)
	}()

	go func() {
		for range sigCh {
			mu.Lock()
			cancel := turnCancel
			mu.Unlock()
			if cancel != nil {
				cancel() // mid-run: cancel this turn only
			} else {
				// at prompt: print session totals and exit
				fmt.Fprintln(out)
				PrintSessionUsage(out, a.Tracker().Aggregate())
				os.Exit(130)
			}
		}
	}()

	for {
		fmt.Fprint(out, "miniagent> ")
		if !scanner.Scan() {
			break // EOF (Ctrl+D) or read error
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			break
		}

		turnID++
		turnCtx, cancel := context.WithCancel(ctx)

		mu.Lock()
		turnCancel = cancel
		mu.Unlock()

		err := a.RunTurn(turnCtx, strconv.Itoa(turnID), line)

		mu.Lock()
		turnCancel = nil
		mu.Unlock()
		cancel()

		if err != nil && !errors.Is(err, context.Canceled) {
			printError(out, err)
		}
	}

	fmt.Fprintln(out)
	PrintSessionUsage(out, a.Tracker().Aggregate())
	return nil
}

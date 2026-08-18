package main

import (
	"flag"
	"fmt"
	"log"
	"time"
)

// runBot is the e2e player phase (Issues C/E): two protocol-level players
// join the offline-mode server under test. e2e_player1 sends the one command;
// e2e_player2 sends NOTHING except keepalive answers and exists only as the
// attribution cross-check. Exit 0 only on full success.
func runBot(args []string) error {
	fs := flag.NewFlagSet("bot", flag.ContinueOnError)
	host := fs.String("host", "127.0.0.1", "server host")
	port := fs.Int("port", 25565, "server port")
	command := fs.String("command", "list", "command to send, without the slash")
	timeout := fs.Duration("timeout", 150*time.Second, "global timeout for the whole run")
	settle := fs.Duration("settle", 3*time.Second, "settle time after join and after the command")
	if err := fs.Parse(args); err != nil {
		return err
	}
	log.SetFlags(log.Ltime)

	// Absolute deadline: set on every socket, so no phase can outlive it.
	deadline := time.Now().Add(*timeout)

	name, proto, err := ping(*host, *port, deadline)
	if err != nil {
		return fmt.Errorf("status-ping phase: %w", err)
	}
	r, ok := rowFor(proto)
	if !ok {
		return fmt.Errorf("server %q speaks unsupported protocol %d", name, proto)
	}
	log.Printf("[bots] server %s (protocol %d, table row %s)", name, proto, r.name)

	stop := make(chan struct{})
	errc := make(chan error, 4)
	var first *client
	for _, n := range []string{"e2e_player1", "e2e_player2"} {
		cl, err := join(r, *host, *port, n, deadline)
		if err != nil {
			return fmt.Errorf("%s (protocol %d): login phase, %s: %w", r.name, proto, n, err)
		}
		defer func() { _ = cl.c.Close() }()
		log.Printf("[bots] %s logged in (%s, protocol %d)", n, name, proto)
		go cl.pump(stop, errc)
		if first == nil {
			first = cl
		}
	}

	wait := func(phase string) error {
		select {
		case err := <-errc:
			return fmt.Errorf("%s (protocol %d): %s phase: %w", r.name, proto, phase, err)
		case <-time.After(min(*settle, time.Until(deadline))):
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%s (protocol %d): global timeout after %v", r.name, proto, *timeout)
		}
		return nil
	}
	if err := wait("settle-before-command"); err != nil {
		return err
	}
	if err := first.sendCommand(*command); err != nil {
		return fmt.Errorf("%s (protocol %d): command phase: %w", r.name, proto, err)
	}
	log.Printf("[bots] %s sent /%s", first.name, *command)
	if err := wait("settle-after-command"); err != nil {
		return err
	}
	close(stop) // pumps exit; deferred Closes disconnect both players cleanly
	log.Printf("[bots] done")
	return nil
}

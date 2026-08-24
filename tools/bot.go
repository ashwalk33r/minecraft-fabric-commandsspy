package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"time"
)

// runBot is the e2e player phase: player1 sends /list, player2 joins and
// sends nothing — the attribution cross-check. Exit 0 only on full success.
func runBot(args []string) error {
	fs := flag.NewFlagSet("bot", flag.ContinueOnError)
	host := fs.String("host", "127.0.0.1", "server host")
	port := fs.Int("port", 25565, "server port")
	command := fs.String("command", "list", "command to send, without the slash")
	// Protocol 14 (Beta 1.7.3) answers a modern status ping with 0xFF "Protocol
	// error" — the status handshake postdates it — so that version cannot be
	// negotiated and must be declared. BTA (32769) forks that same pre-Netty
	// framing and is declared for the same reason. 0 keeps the normal
	// negotiate-by-ping path.
	protocol := fs.Int("protocol", 0, "skip the status ping and assume this protocol (14 = Beta 1.7.3, 32769 = BTA)")
	timeout := fs.Duration("timeout", 150*time.Second, "global timeout for the whole run")
	settle := fs.Duration("settle", 3*time.Second, "settle time after join and after the command")
	if err := fs.Parse(args); err != nil {
		return err
	}
	log.SetFlags(log.Ltime)

	// Absolute deadline: set on every socket, so no phase can outlive it.
	deadline := time.Now().Add(*timeout)

	// Protocol 14 (Beta 1.7.3, Babric) is pre-Netty and shares no framing with the
	// table below, so it gets its own client rather than a table row — and it has no
	// status ping to negotiate with either. See beta.go.
	if *protocol == betaProtocolVersion {
		return runBetaBot(*host, *port, *command, deadline, *settle)
	}
	// BTA keeps Beta's framing but changes every layout above it, so it gets its own
	// straight-line client too rather than a parameter on the one above. See bta.go.
	if *protocol == btaProtocolVersion {
		return runBtaBot(*host, *port, *command, deadline, *settle)
	}

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

// runBetaBot is runBot's protocol-14 twin: same phases, same two players, same
// attribution cross-check, over the pre-Netty framing in beta.go. It is separate
// rather than a table row because none of mc.go's conn machinery — VarInt frames,
// compression, the login state machine — exists on this protocol.
func runBetaBot(host string, port int, command string, deadline time.Time, settle time.Duration) error {
	var first net.Conn
	for _, n := range []string{"e2e_player1", "e2e_player2"} {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), time.Until(deadline))
		if err != nil {
			return fmt.Errorf("beta (protocol %d): login phase, %s: %w", betaProtocolVersion, n, err)
		}
		defer func() { _ = c.Close() }()
		if err := c.SetDeadline(deadline); err != nil {
			return err
		}
		if err := betaLogin(c, n); err != nil {
			return fmt.Errorf("beta (protocol %d): login phase, %s: %w", betaProtocolVersion, n, err)
		}
		log.Printf("[bots] %s logged in (Beta 1.7.3, protocol %d)", n, betaProtocolVersion)
		// Drain and discard. Immediately after login the server floods entity and
		// chunk packets this client has no parser for; left unread they fill the
		// socket buffer and stall the server's writer for this connection. Nothing
		// is asserted from the stream — the verdict comes from the server log, as it
		// does for every other loader.
		go func() { _, _ = io.Copy(io.Discard, c) }()
		if first == nil {
			first = c
		}
	}

	// e2e_player2 sends nothing on purpose: it is the attribution cross-check.
	time.Sleep(min(settle, time.Until(deadline)))
	if err := betaSendChat(first, "/"+command); err != nil {
		return fmt.Errorf("beta (protocol %d): command phase: %w", betaProtocolVersion, err)
	}
	log.Printf("[bots] e2e_player1 sent /%s", command)
	time.Sleep(min(settle, time.Until(deadline)))
	if !time.Now().Before(deadline) {
		return fmt.Errorf("beta (protocol %d): global timeout", betaProtocolVersion)
	}
	log.Printf("[bots] done")
	return nil
}

// runBtaBot is runBot's protocol-32769 twin: same phases, same two players, same
// attribution cross-check, over the BTA framing in bta.go. It is separate from
// runBetaBot rather than a parameter on it because the two protocols share only their
// framing — string form, login layout and chat packet all differ — and two straight-line
// clients are cheaper to read than one abstraction over both.
func runBtaBot(host string, port int, command string, deadline time.Time, settle time.Duration) error {
	var first net.Conn
	for _, n := range []string{"e2e_player1", "e2e_player2"} {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), time.Until(deadline))
		if err != nil {
			return fmt.Errorf("bta (protocol %d): login phase, %s: %w", btaProtocolVersion, n, err)
		}
		defer func() { _ = c.Close() }()
		if err := c.SetDeadline(deadline); err != nil {
			return err
		}
		if err := btaLogin(c, n); err != nil {
			return fmt.Errorf("bta (protocol %d): login phase, %s: %w", btaProtocolVersion, n, err)
		}
		log.Printf("[bots] %s logged in (BTA, protocol %d)", n, btaProtocolVersion)
		// Drain and discard, exactly as the protocol-14 bot does: after login the
		// server floods chunk, registry and AES-key packets this client has no parser
		// for, and left unread they fill the socket buffer and stall the server's
		// writer for this connection. Nothing is asserted from the stream — the
		// verdict comes from the server log, as it does for every other loader.
		go func() { _, _ = io.Copy(io.Discard, c) }()
		if first == nil {
			first = c
		}
	}

	// e2e_player2 sends nothing on purpose: it is the attribution cross-check.
	time.Sleep(min(settle, time.Until(deadline)))
	if err := btaSendChat(first, "/"+command); err != nil {
		return fmt.Errorf("bta (protocol %d): command phase: %w", btaProtocolVersion, err)
	}
	log.Printf("[bots] e2e_player1 sent /%s", command)
	time.Sleep(min(settle, time.Until(deadline)))
	if !time.Now().Before(deadline) {
		return fmt.Errorf("bta (protocol %d): global timeout", btaProtocolVersion)
	}
	log.Printf("[bots] done")
	return nil
}

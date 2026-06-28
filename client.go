// Package clipremote is a Go reimplementation of the client (smartphone)
// side of Clip Studio Paint's "companion mode" remote-control protocol — the
// protocol normally used when you scan the in-app QR code to drive CSP on a PC
// from a phone.
//
// The wire protocol, all 24 commands, their JSON detail schemas, the auth
// crypto, and the packet framing were reverse-engineered from the symboled
// Android binary libClipStudioPaint.so. See the pkg/commands package for the
// typed command bodies and pkg/packets for the framing.
package clipremote

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chocolatkey/clipremote/pkg/commands"
	"github.com/chocolatkey/clipremote/pkg/crypto"
	"github.com/chocolatkey/clipremote/pkg/packets"
	"github.com/chocolatkey/clipremote/pkg/protocol"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// DefaultSyncTimeout bounds a synchronous command if no deadline is supplied.
const DefaultSyncTimeout = 10 * time.Second

// ServerPacket is a received protocol packet (an alias of packets.ServerCommand)
// re-exported so callers need not import the packets subpackage.
type ServerPacket = packets.ServerCommand

// ServerCommandHandler handles a host-initiated command. It returns the optional
// reply detail body (raw JSON bytes; nil for an empty ACK) and whether handling
// succeeded (false makes the auto-reply an error packet). Handlers run on the
// receive goroutine and must not block for long.
type ServerCommandHandler func(*packets.ServerCommand) (replyDetail []byte, ok bool)

// Client is a connection to a CSP companion-mode host.
type Client struct {
	atomicSerial atomic.Uint32
	callbacks    cmap.ConcurrentMap[packets.Serial, packets.ClientCommandCallback]

	writeMu sync.Mutex // serializes frame writes and guards conn during reconnect
	conn    net.Conn

	handlersMu sync.RWMutex
	handlers   map[commands.Command]ServerCommandHandler

	password   string
	generation string

	alive            atomic.Bool
	closed           atomic.Bool
	keepaliveStarted atomic.Bool
	reconnecting     atomic.Bool

	// SyncTimeout bounds synchronous calls that are not given their own context.
	SyncTimeout time.Duration
}

// Close terminates the connection and fails any pending callbacks.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	c.alive.Store(false)
	c.resetCallbacks(errors.New("client closed"))
	return c.conn.Close()
}

// Alive reports whether the client is authenticated and the keep-alive is running.
func (c *Client) Alive() bool { return c.alive.Load() }

// RemoteAddr returns the connected host address.
func (c *Client) RemoteAddr() string {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.RemoteAddr().String()
}

// resetCallbacks fails and clears all pending response callbacks.
func (c *Client) resetCallbacks(err error) {
	c.callbacks.IterCb(func(key packets.Serial, v packets.ClientCommandCallback) {
		v(nil, err)
	})
	c.callbacks.Clear()
}

// Reset fails pending callbacks and resets the outgoing serial counter.
func (c *Client) Reset() {
	logrus.Debugln("client reset")
	c.resetCallbacks(errors.New("client reset"))
	c.atomicSerial.Store(0)
}

// ---------------------------------------------------------------------------
// Receive loop + dispatch
// ---------------------------------------------------------------------------

func (c *Client) loop() {
	c.writeMu.Lock()
	conn := c.conn
	c.writeMu.Unlock()
	reader := bufio.NewReaderSize(conn, 1<<16) // created ONCE per connection

	for {
		data, err := reader.ReadBytes(protocol.CommandTerminator)
		if err != nil {
			if c.closed.Load() {
				return
			}
			if err == io.EOF {
				logrus.Infoln("connection closed by host, attempting reconnect")
				c.alive.Store(false)
				c.triggerReconnect()
				return
			}
			logrus.Errorln("read error:", err)
			c.alive.Store(false)
			c.triggerReconnect()
			return
		}

		var pkt packets.ServerCommand
		if err := pkt.Parse(data); err != nil {
			// A single malformed frame should not tear down the connection.
			logrus.Warnln("dropping malformed packet:", err)
			continue
		}
		c.dispatch(&pkt)
	}
}

func (c *Client) dispatch(pkt *packets.ServerCommand) {
	if pkt.Type == packets.TypeClientCommand {
		c.handleIncoming(pkt)
		return
	}

	// Response to one of our commands.
	if cb, ok := c.callbacks.Pop(pkt.Serial); ok {
		cb(pkt, nil)
		return
	}
	logrus.Warnf("unmatched response for serial %d (command %s, %s)", pkt.Serial, pkt.Command, pkt.Type)
}

// handleIncoming runs a registered handler for a host-initiated command and
// always sends the matching ACK (success/error, echoing command + serial),
// mirroring CSP's PWTcpRemoteCommandDispatcher::HandleReceivedPacket.
func (c *Client) handleIncoming(pkt *packets.ServerCommand) {
	logrus.Debugf("incoming command %s serial %d", pkt.Command, pkt.Serial)

	c.handlersMu.RLock()
	h := c.handlers[pkt.Command]
	c.handlersMu.RUnlock()

	var (
		reply []byte
		ok    = true
	)
	if h != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logrus.Errorf("handler for %s panicked: %v", pkt.Command, r)
					ok = false
				}
			}()
			reply, ok = h(pkt)
		}()
	}

	typ := packets.TypeServerResponseSuccess
	if !ok {
		typ = packets.TypeServerResponseError
	}
	if err := c.writeFrame(typ, pkt.Command, pkt.Serial, reply); err != nil {
		logrus.Warnf("failed to ACK incoming %s: %v", pkt.Command, err)
	}
}

// Handle registers a handler for a host-initiated command. Passing nil removes
// the handler. See the typed On* helpers for the common server pushes.
func (c *Client) Handle(command commands.Command, h ServerCommandHandler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	if h == nil {
		delete(c.handlers, command)
		return
	}
	c.handlers[command] = h
}

// ---------------------------------------------------------------------------
// Send
// ---------------------------------------------------------------------------

func (c *Client) writeFrame(typ packets.PacketType, command commands.Command, serial packets.Serial, detail []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return packets.WriteFrame(c.conn, typ, command, serial, detail)
}

// SendCommand sends an outgoing command and registers callback for its response.
func (c *Client) SendCommand(command commands.Command, detail any, callback packets.ClientCommandCallback) {
	if !c.alive.Load() && command != commands.Authenticate {
		callback(nil, errors.New("client is not alive"))
		return
	}

	detailBytes, err := packets.MarshalDetail(detail)
	if err != nil {
		callback(nil, errors.Wrap(err, "failed marshaling command detail"))
		return
	}

	serial := packets.Serial(c.atomicSerial.Add(1) - 1)
	c.callbacks.Set(serial, callback)

	if err := c.writeFrame(packets.TypeClientCommand, command, serial, detailBytes); err != nil {
		c.callbacks.Pop(serial)
		callback(nil, errors.Wrap(err, "failed writing command"))
		return
	}
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		shown := string(detailBytes)
		if command == commands.Authenticate {
			// The Authenticate detail carries the (reversibly obfuscated) connection
			// and rotated reconnect passwords — never log them, even at debug level.
			shown = "[redacted credentials]"
		}
		logrus.Debugf("sent %s serial %d detail %s", command, serial, shown)
	}
}

// SendCommandSync sends a command and blocks for the response, bounded by ctx
// (or SyncTimeout when ctx has no deadline).
func (c *Client) SendCommandSync(ctx context.Context, command commands.Command, detail any) (*packets.ServerCommand, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.syncTimeout())
		defer cancel()
	}

	type result struct {
		scp *packets.ServerCommand
		err error
	}
	ch := make(chan result, 1) // buffered so a late response never blocks
	c.SendCommand(command, detail, func(scp *packets.ServerCommand, err error) {
		ch <- result{scp, err}
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.scp, r.err
	}
}

func (c *Client) syncTimeout() time.Duration {
	if c.SyncTimeout > 0 {
		return c.SyncTimeout
	}
	return DefaultSyncTimeout
}

// call sends a command and waits for the response. CSP's response handlers key
// off the detail body rather than the packet type (the host even replies to a
// successful Authenticate with a 0x06 "error"-typed packet), so an error-typed
// response is surfaced via the returned packet's Type/Detail for inspection
// rather than turned into a Go error here. A nil error means a response arrived.
func (c *Client) call(ctx context.Context, command commands.Command, detail any) (*packets.ServerCommand, error) {
	scp, err := c.SendCommandSync(ctx, command, detail)
	if err != nil {
		return nil, err
	}
	if scp.IsError() && logrus.IsLevelEnabled(logrus.DebugLevel) {
		logrus.Debugf("command %s got an error-typed response: detail=%q", command, scp.RawDetail)
	}
	return scp, nil
}

// ---------------------------------------------------------------------------
// Authentication + keep-alive
// ---------------------------------------------------------------------------

// authHex obfuscates plaintext with the auth key and hex-encodes it (CSP's
// URRemoteControlAuthUtility::EncryptStringWithKey: UTF-8 -> repeating-key XOR
// -> lowercase hex).
func authHex(plaintext []byte) string {
	buf := make([]byte, len(plaintext))
	copy(buf, plaintext)
	crypto.ObfuscateAuthParam(buf)
	return hex.EncodeToString(buf)
}

// Authenticate logs in using the password from the QR config, rotating in a
// fresh password for future reconnects. On success it starts the keep-alive.
func (c *Client) Authenticate(password string) (commands.AuthResult, error) {
	newPassword := crypto.MakePassword()
	detail := []string{
		c.generation,
		authHex([]byte(password)),
		authHex([]byte(newPassword)),
	}

	scp, err := c.SendCommandSync(context.Background(), commands.Authenticate, detail)
	if err != nil {
		return commands.AuthResult{}, errors.Wrap(err, "authenticate")
	}

	res := parseAuthResult(scp)
	if !res.OK {
		c.atomicSerial.Store(0)
		reason := string(res.Reason)
		if reason == "" {
			reason = "unknown reason"
		}
		return res, fmt.Errorf("authentication rejected: %s", reason)
	}

	c.password = newPassword
	c.markAliveAndStartKeepalive()
	return res, nil
}

// Reauthenticate re-establishes a session after a reconnect using the rotated
// password and the reconnection marker.
func (c *Client) Reauthenticate() (commands.AuthResult, error) {
	detail := []string{
		c.generation,
		authHex(protocol.ReconnectionRequest),
		authHex([]byte(c.password)),
	}

	scp, err := c.SendCommandSync(context.Background(), commands.Authenticate, detail)
	if err != nil {
		return commands.AuthResult{}, errors.Wrap(err, "reauthenticate")
	}
	res := parseAuthResult(scp)
	if !res.OK {
		c.atomicSerial.Store(0)
		return res, errors.New("reauthentication rejected")
	}
	c.markAliveAndStartKeepalive()
	return res, nil
}

func parseAuthResult(scp *packets.ServerCommand) commands.AuthResult {
	var res commands.AuthResult
	// A success reply is the bare array [true,false] (won't decode into the
	// struct); a structured reply is an object {AuthErrorReason,...}.
	var obj commands.AuthResponse
	if err := scp.Decode(&obj); err == nil {
		res.Reason = obj.AuthErrorReason
		res.ServerSpecVersion = obj.RemoteCommandSpecVersionOfServer
		res.QuickAccessAvailable = obj.IsQuickAccessAvailable
	}
	// CSP keys auth success on AuthErrorReason in the detail, NOT the packet
	// type: the host sends the Authenticate reply as a 0x06 ("error") packet even
	// on success. Verified empirically and in
	// URRemoteCommandToAuthenticate::HandleResponse, which ignores the packet
	// type and only inspects AuthErrorReason ("Unknown"/"" => success; only
	// VersionMismatch/PasswordMismatch/ServerUnready are failures).
	res.OK = !res.Reason.IsFailure()
	// A bodyless error packet is a genuine failure (a real reply always has a body).
	if len(scp.RawDetail) == 0 && scp.Type == packets.TypeServerResponseError {
		res.OK = false
	}
	return res
}

// markAliveAndStartKeepalive marks the session authenticated, (re)arms the
// idle timer, and starts the single keep-alive goroutine (idempotent across
// reconnects so there is never more than one heartbeat loop).
func (c *Client) markAliveAndStartKeepalive() {
	c.alive.Store(true)
	if c.keepaliveStarted.Swap(true) {
		return
	}
	go c.keepalive()
}

// keepalive sends a TellHeartbeat once per protocol.HeartbeatInterval, matching
// CSP's periodic 1 s client-side heartbeat timer. A single goroutine runs for
// the client's life (across reconnects). On heartbeat failure it triggers a
// reconnect (the receive loop usually detects a dead connection first).
func (c *Client) keepalive() {
	ticker := time.NewTicker(protocol.HeartbeatInterval)
	defer ticker.Stop()
	for range ticker.C {
		if c.closed.Load() {
			return
		}
		if !c.alive.Load() {
			continue
		}
		if err := c.Heartbeat(false); err != nil {
			logrus.Debugln("heartbeat failed:", err)
			c.alive.Store(false)
			c.triggerReconnect()
		}
	}
}

// Heartbeat sends a TellHeartbeat. A bare heartbeat carries no body; set
// idleTimerReset to ask the host to reset its idle timer. Any response (the host
// does not meaningfully error on heartbeats) counts as alive.
func (c *Client) Heartbeat(idleTimerReset bool) error {
	var detail any
	if idleTimerReset {
		detail = commands.HeartbeatRequest{IdleTimerResetRequested: true}
	}
	_, err := c.SendCommandSync(context.Background(), commands.TellHeartbeat, detail)
	return err
}

// ---------------------------------------------------------------------------
// Reconnect
// ---------------------------------------------------------------------------

// triggerReconnect starts a reconnect in the background, ensuring only one runs
// at a time. It is safe to call from both the receive loop (on EOF) and the
// keep-alive (on heartbeat failure).
func (c *Client) triggerReconnect() {
	if c.closed.Load() || c.reconnecting.Swap(true) {
		return
	}
	go func() {
		defer c.reconnecting.Store(false)
		if err := c.reconnect(); err != nil {
			logrus.Errorln("reconnect failed:", err)
			c.Close()
		}
	}()
}

func (c *Client) reconnect() error {
	if c.closed.Load() {
		return errors.New("client closed")
	}
	c.alive.Store(false)
	c.Reset()

	addr := c.RemoteAddr()
	nconn, err := net.Dial("tcp", addr)
	if err != nil {
		return errors.Wrap(err, "failed reconnecting to "+addr)
	}

	c.writeMu.Lock()
	c.conn = nconn
	c.writeMu.Unlock()

	go c.loop()
	if _, err := c.Reauthenticate(); err != nil {
		return errors.Wrap(err, "failed reauthenticating after reconnect")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Connection bootstrap
// ---------------------------------------------------------------------------

// DecodeConfig decodes the QR-code share URL
// (https://companion.clip-studio.com/rc/<lang>?s=XXX) into connection details.
func DecodeConfig(connectionURL string) (ipAddresses []string, port uint16, password string, generation string, err error) {
	curl, err := url.Parse(connectionURL)
	if err != nil {
		err = errors.Wrap(err, "failed to parse connection URL")
		return
	}
	if curl.Host != "companion.clip-studio.com" {
		err = errors.New("connection URL has incorrect host")
		return
	}
	sParam := curl.Query().Get("s")
	if sParam == "" {
		err = errors.New("connection URL has no required 's' parameter")
		return
	}
	sBytes, err := hex.DecodeString(sParam)
	if err != nil {
		err = errors.Wrap(err, "failed to decode 's' parameter hex")
		return
	}
	crypto.ObfuscateRemoteParam(sBytes)
	frags := strings.Split(string(sBytes), "\t")
	if len(frags) != 4 {
		err = errors.New("connection params has incorrect number of items")
		return
	}

	ipAddresses = strings.Split(frags[0], ",")
	rawPort, err := strconv.ParseInt(frags[1], 10, 32)
	if err != nil {
		err = errors.Wrap(err, "failed parsing port "+frags[1])
		return
	}
	port = uint16(rawPort)
	password = frags[2]
	generation = frags[3]
	return
}

// IsShareURL reports whether s is a decodable CSP companion share URL (correct
// host and a well-formed "s" parameter). It is handy as a predicate when
// scanning the screen for the connection QR code.
func IsShareURL(s string) bool {
	_, _, _, _, err := DecodeConfig(s)
	return err == nil
}

// Connect dials the host (trying each candidate address) and starts the receive
// loop. Call Authenticate next.
func Connect(ipAddresses []string, port uint16, generation string) (*Client, error) {
	var conn net.Conn
	var lastErr error
	for _, address := range ipAddresses {
		host := net.JoinHostPort(address, strconv.Itoa(int(port)))
		logrus.Debugln("dialing", host)
		c, err := net.Dial("tcp", host)
		if err != nil {
			lastErr = err
			continue
		}
		conn = c
		break
	}
	if conn == nil {
		return nil, errors.Wrap(lastErr, "failed dialing all candidate addresses")
	}

	client := &Client{
		conn:        conn,
		generation:  generation,
		callbacks:   cmap.NewStringer[packets.Serial, packets.ClientCommandCallback](),
		handlers:    make(map[commands.Command]ServerCommandHandler),
		SyncTimeout: DefaultSyncTimeout,
	}
	logrus.Infoln("connected to", client.RemoteAddr())
	go client.loop()
	return client, nil
}

// ConnectFromURL decodes a QR share URL and connects (without authenticating).
func ConnectFromURL(connectionURL string) (*Client, string, error) {
	ips, port, password, generation, err := DecodeConfig(connectionURL)
	if err != nil {
		return nil, "", err
	}
	client, err := Connect(ips, port, generation)
	if err != nil {
		return nil, "", err
	}
	return client, password, nil
}

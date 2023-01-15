package clipremote

import (
	"bufio"
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

type Client struct {
	atomicSerial atomic.Uint32
	conn         net.Conn
	callbacks    cmap.ConcurrentMap[packets.Serial, packets.ClientCommandCallback]
	password     string
	generation   string
	timeout      *time.Timer
	alive        bool
}

func (c *Client) Close() error {
	c.Reset()
	return c.conn.Close()
}

func (c *Client) Reset() {
	if c.alive {
		c.alive = false
	}
	logrus.Infoln("client-side reset")
	c.callbacks.IterCb(func(key packets.Serial, v packets.ClientCommandCallback) {
		logrus.Debugln("removing callback for client-side reset", key)
		v(nil, errors.New("client-side reset"))
	})
	c.callbacks.Clear()
	c.atomicSerial.Store(0)
}

func (c *Client) reconnect() error {
	c.Reset()
	nconn, err := net.Dial("tcp", c.conn.RemoteAddr().String())
	if err != nil {
		return errors.Wrap(err, "failed reconnecting")
	}
	c.conn = nconn
	c.Reset()
	go c.loop()
	c.Reauthenticate(func(scp *packets.ServerCommand, err error) {
		if err != nil {
			c.Close()
		}
	})
	return nil
}

func (c *Client) Alive() bool {
	return c.alive
}

func (c *Client) RemoteAddr() string {
	return c.conn.RemoteAddr().String()
}

func (c *Client) callbackForSerial(serial packets.Serial, scp *packets.ServerCommand, err error) {
	if callback, ok := c.callbacks.Pop(serial); ok {
		callback(scp, err)
	} else if scp != nil {
		if scp.Type == packets.TypeClientCommand {
			if scp.Serial == 0 {
				// Reset
				logrus.Infof("server-side reset: %v+", scp)
				c.callbacks.IterCb(func(key packets.Serial, v packets.ClientCommandCallback) {
					logrus.Debugln("removing callback for server-side reset", key)
					v(nil, errors.New("server-side reset"))
				})
				c.callbacks.Clear()
			}
			c.atomicSerial.Store(uint32(scp.Serial) + 1)
			c.timeout.Reset(protocol.HeartbeatTimeout)
		} else {
			logrus.Warnf("received response for unknown serial %d: %v+", serial, scp)
		}
	}
}

func (c *Client) loop() error {
	for {
		data, err := bufio.NewReader(c.conn).ReadBytes(protocol.CommandTerminator)
		if err == io.EOF {
			if err := c.reconnect(); err != nil {
				return errors.Wrap(err, "connection was closed, and reconnection failed")
			}
			return nil
		}
		if err != nil {
			c.Close()
			return errors.Wrap(err, "failed reading command response")
		}
		var pkt packets.ServerCommand
		err = pkt.Parse(data)
		if err != nil {
			c.Close()
			return errors.Wrap(err, "failed parsing command response")
		} else {
			c.callbackForSerial(pkt.Serial, &pkt, err)
		}
	}
}

func (c *Client) SendCommand(command commands.Command, detail interface{}, callback packets.ClientCommandCallback) {
	if !c.alive && command != commands.Authenticate {
		callback(nil, errors.New("client is not alive"))
		return
	}
	cmd := packets.ClientCommand{
		Command:  command,
		Serial:   packets.Serial(c.atomicSerial.Load()),
		Detail:   detail,
		Callback: callback,
	}
	c.atomicSerial.Add(1)

	err := cmd.Write(c.conn)
	if err != nil {
		cmd.Callback(nil, errors.Wrap(err, "failed writing command"))
	}

	c.timeout.Reset(protocol.HeartbeatTimeout)
	c.callbacks.Set(cmd.Serial, cmd.Callback)
}

func (c *Client) SendCommandSync(command commands.Command, detail interface{}) (scp *packets.ServerCommand, err error) {
	var wg sync.WaitGroup
	wg.Add(1)
	c.SendCommand(
		command,
		detail,
		func(s *packets.ServerCommand, e error) {
			defer wg.Done()
			scp = s
			err = e
		},
	)
	wg.Wait()
	return
}

func (c *Client) Authenticate(callback packets.ClientCommandCallback, password string) {
	currPass := []byte(password)
	crypto.ObfuscateAuthParam(currPass)

	newPassword := crypto.MakePassword()
	newPass := make([]byte, len(newPassword))
	copy(newPass, newPassword)
	crypto.ObfuscateAuthParam(newPass)

	c.SendCommand(commands.Authenticate, []string{
		c.generation,
		hex.EncodeToString(currPass),
		hex.EncodeToString(newPass),
	}, func(scp *packets.ServerCommand, err error) {
		if err != nil {
			callback(scp, err)
			return
		}
		if scp.Type == packets.TypeServerResponseError {
			callback(scp, errors.New("authentication failed"))
			c.atomicSerial.Store(0)
			return
		}
		c.password = newPassword
		go c.keepalive()
	})
}

func (c *Client) Reauthenticate(callback packets.ClientCommandCallback) {
	currPass := make([]byte, len(c.password))
	copy(currPass, c.password)
	crypto.ObfuscateAuthParam(currPass)

	reconnector := make([]byte, len(protocol.ReconnectionRequest))
	copy(reconnector, protocol.ReconnectionRequest)
	crypto.ObfuscateAuthParam(reconnector)

	c.SendCommand(commands.Authenticate, []string{
		c.generation,
		hex.EncodeToString(reconnector),
		hex.EncodeToString(currPass),
	}, func(scp *packets.ServerCommand, err error) {
		if err != nil {
			callback(scp, err)
			return
		}
		if scp.Type == packets.TypeServerResponseError {
			callback(scp, errors.New("reauthentication failed"))
			c.atomicSerial.Store(0)
			return
		}
		go c.keepalive()
	})
}

func (c *Client) Heartbeat(callback packets.ClientCommandCallback, idleTimerResetRequested bool) {
	c.SendCommand(commands.TellHeartbeat, map[string]bool{
		"IdleTimerResetRequested": true,
	}, func(scp *packets.ServerCommand, err error) {
		if err != nil || scp.Type == packets.TypeServerResponseError {
			if err == nil {
				err = errors.New("heartbeat failed")
			}
			callback(scp, err)
			return
		}
		c.timeout.Reset(protocol.HeartbeatTimeout)
		callback(scp, nil)
	})
}

func (c *Client) keepalive() {
	if c.alive {
		return
	}
	c.alive = true
	for {
		select {
		case <-c.timeout.C:
			if !c.alive {
				return
			}
			c.Heartbeat(func(scp *packets.ServerCommand, err error) {
				if err != nil {
					logrus.Debugln("heartbeat error", err.Error())
					c.alive = false
					c.timeout.Stop()
					c.Reauthenticate(func(scp *packets.ServerCommand, err error) {
						if err != nil {
							c.Close()
						} else {
							c.timeout.Reset(protocol.HeartbeatTimeout)
						}
					})
				}
			}, true)
		}
	}
}

// The connectionURL is what you get from decoding the QR code.
// Should look like: https://companion.clip-studio.com/rc/en-us?s=abc123
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
	rawPort, err := strconv.ParseInt(frags[1], 10, 16)
	if err != nil {
		err = errors.Wrap(err, "failed parsing port "+frags[1])
		return
	}
	port = uint16(rawPort)
	password = frags[2]
	generation = frags[3]

	return
}

// Connect to the CSP server.
func Connect(ipAddresses []string, port uint16, generation string) (*Client, error) {
	var client *Client
	for i, address := range ipAddresses {
		host := fmt.Sprintf("%s:%d", address, port)
		logrus.Debugln("dialing", host)
		conn, err := net.Dial("tcp", host)
		if err != nil {
			if i == len(ipAddresses)-1 {
				return nil, errors.Wrap(err, "failed dialing "+address)
			}
			continue
		}

		client = &Client{
			conn:       conn,
			generation: generation,
			timeout:    time.NewTimer(protocol.HeartbeatTimeout),
			callbacks:  cmap.NewStringer[packets.Serial, packets.ClientCommandCallback](),
		}
		client.Reset()
		break
	}

	logrus.Infoln("connected to " + client.RemoteAddr())
	go client.loop()
	return client, nil
}

// Package mqtt is a thin wrapper around the Eclipse Paho MQTT client.
// It powers two features that share one broker connection:
//
//   - control: a socket with Protocol "mqtt" publishes ON/OFF to the
//     command topic stored in Socket.Code (see internal/sender).
//   - ingest:  a SensorListener subscribes to the topics of sensors with
//     Protocol "mqtt" and records incoming payloads as readings.
//
// Connection config comes from the environment (see FromEnv). A nil
// *Client is safe to hold and pass around — every method is nil-receiver
// safe and Enabled() reports whether a broker was actually configured,
// mirroring matter.Client so deployments without a broker opt out cleanly.
package mqtt

import (
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

const (
	// connectTimeout caps how long startup waits for the first connection
	// attempt before handing off to the background reconnect loop.
	connectTimeout = 10 * time.Second
	// opTimeout caps a single publish/subscribe round-trip.
	opTimeout = 5 * time.Second
	// defaultQoS 1 (at-least-once) — control commands and sensor readings
	// both prefer delivery over the lower latency of QoS 0.
	defaultQoS = 1
)

// Client wraps a paho client configured from the environment.
type Client struct {
	BrokerURL string

	c paho.Client

	mu        sync.Mutex
	onConnect []func()
}

// FromEnv builds a Client from the MQTT_* environment variables. It returns
// nil when MQTT_BROKER_URL is empty or the literal "disabled", so callers
// can keep a nil *Client and rely on Enabled() to gate the MQTT codepaths.
//
// Recognised variables:
//
//	MQTT_BROKER_URL   broker address, e.g. tcp://host:1883 or ssl://host:8883
//	MQTT_CLIENT_ID    client id (default "rf-socket-controller")
//	MQTT_USERNAME     optional username
//	MQTT_PASSWORD     optional password
//	MQTT_TLS_INSECURE "true" to skip TLS cert verification (self-signed brokers)
func FromEnv() *Client {
	raw := strings.TrimSpace(os.Getenv("MQTT_BROKER_URL"))
	if raw == "" || strings.EqualFold(raw, "disabled") {
		return nil
	}

	clientID := strings.TrimSpace(os.Getenv("MQTT_CLIENT_ID"))
	if clientID == "" {
		clientID = "rf-socket-controller"
	}

	opts := paho.NewClientOptions()
	opts.AddBroker(raw)
	opts.SetClientID(clientID)
	opts.SetUsername(strings.TrimSpace(os.Getenv("MQTT_USERNAME")))
	opts.SetPassword(os.Getenv("MQTT_PASSWORD"))
	opts.SetCleanSession(true)
	opts.SetOrderMatters(false)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetConnectTimeout(connectTimeout)
	// Keep trying both the first connect and any later drop in the
	// background so a broker that's down at boot (or restarts later)
	// doesn't take the controller down with it.
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(10 * time.Second)
	opts.SetMaxReconnectInterval(60 * time.Second)

	if insecure, _ := strconv.ParseBool(os.Getenv("MQTT_TLS_INSECURE")); insecure {
		opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	cl := &Client{BrokerURL: raw}
	// The broker forgets our subscriptions across a reconnect (clean
	// session), so re-run every registered hook on each (re)connection.
	opts.SetOnConnectHandler(func(paho.Client) {
		cl.mu.Lock()
		hooks := append([]func(){}, cl.onConnect...)
		cl.mu.Unlock()
		for _, h := range hooks {
			h()
		}
	})
	cl.c = paho.NewClient(opts)
	return cl
}

// Enabled reports whether a broker was configured.
func (c *Client) Enabled() bool { return c != nil && c.BrokerURL != "" }

// Connect starts the connection. With connect-retry enabled the broker is
// reached in the background, so this waits only briefly for a first attempt
// and returns any hard error; a timeout is not fatal.
func (c *Client) Connect() error {
	if !c.Enabled() {
		return fmt.Errorf("mqtt: not configured (set MQTT_BROKER_URL)")
	}
	token := c.c.Connect()
	if token.WaitTimeout(connectTimeout) {
		return token.Error()
	}
	return nil
}

// Connected reports whether a live broker session currently exists.
func (c *Client) Connected() bool {
	return c.Enabled() && c.c.IsConnected()
}

// Publish sends payload to topic at the default QoS, non-retained.
func (c *Client) Publish(topic, payload string) error {
	if !c.Enabled() {
		return fmt.Errorf("mqtt: not configured (set MQTT_BROKER_URL)")
	}
	token := c.c.Publish(topic, defaultQoS, false, payload)
	if !token.WaitTimeout(opTimeout) {
		return fmt.Errorf("mqtt: publish to %q timed out", topic)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt: publish to %q: %w", topic, err)
	}
	return nil
}

// Send publishes ON/OFF to topic. This is the on/off path used by the
// multi-protocol sender; Socket.Code holds the command topic.
func (c *Client) Send(topic string, on bool) error {
	payload := "OFF"
	if on {
		payload = "ON"
	}
	return c.Publish(topic, payload)
}

// Subscribe registers handler for a topic filter (which may contain the
// MQTT '+' and '#' wildcards). handler is called for every matching message.
func (c *Client) Subscribe(filter string, handler func(topic string, payload []byte)) error {
	if !c.Enabled() {
		return fmt.Errorf("mqtt: not configured (set MQTT_BROKER_URL)")
	}
	token := c.c.Subscribe(filter, defaultQoS, func(_ paho.Client, m paho.Message) {
		handler(m.Topic(), m.Payload())
	})
	if !token.WaitTimeout(opTimeout) {
		return fmt.Errorf("mqtt: subscribe to %q timed out", filter)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt: subscribe to %q: %w", filter, err)
	}
	return nil
}

// Unsubscribe removes the given topic filters.
func (c *Client) Unsubscribe(filters ...string) error {
	if !c.Enabled() || len(filters) == 0 {
		return nil
	}
	token := c.c.Unsubscribe(filters...)
	token.WaitTimeout(opTimeout)
	return token.Error()
}

// OnConnect registers a hook run on every successful (re)connection. The
// SensorListener uses it to re-subscribe after a broker restart.
func (c *Client) OnConnect(h func()) {
	if !c.Enabled() {
		return
	}
	c.mu.Lock()
	c.onConnect = append(c.onConnect, h)
	c.mu.Unlock()
}

// Close disconnects from the broker, waiting up to 250ms for a clean exit.
func (c *Client) Close() {
	if c.Enabled() {
		c.c.Disconnect(250)
	}
}

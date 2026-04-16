package openclawgateway

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var ED25519_SPKI_PREFIX = []byte{0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00}

type PendingRequest struct {
	Resolve     func(value interface{})
	Reject      func(err error)
	ExpectFinal bool
	Timer       *time.Timer
}

type GatewayClientOptions struct {
	URL     string
	Headers map[string]string
	OnEvent func(frame GatewayEventFrame)
	OnLog   func(stream, chunk string)
}

type GatewayWsClient struct {
	opts             GatewayClientOptions
	ws               *websocket.Conn
	pending          sync.Map // string -> *PendingRequest
	challengeChan    chan string
	challengeErrChan chan error
	mu               sync.Mutex
	isClosed         bool
}

func NewGatewayWsClient(opts GatewayClientOptions) *GatewayWsClient {
	return &GatewayWsClient{
		opts:             opts,
		challengeChan:    make(chan string, 1),
		challengeErrChan: make(chan error, 1),
	}
}

func (c *GatewayWsClient) Connect(ctx context.Context, buildConnectParams func(nonce string) map[string]interface{}, timeoutMs int) (map[string]interface{}, error) {
	header := http.Header{}
	for k, v := range c.opts.Headers {
		header.Set(k, v)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: time.Duration(timeoutMs) * time.Millisecond,
	}

	conn, _, err := dialer.DialContext(ctx, c.opts.URL, header)
	if err != nil {
		return nil, fmt.Errorf("gateway connect failed: %w", err)
	}
	c.ws = conn

	go c.readLoop()

	select {
	case nonce := <-c.challengeChan:
		signedConnectParams := buildConnectParams(nonce)
		res, err := c.Request("connect", signedConnectParams, timeoutMs, false)
		if err != nil {
			return nil, err
		}
		if m, ok := res.(map[string]interface{}); ok {
			return m, nil
		}
		return nil, nil
	case err := <-c.challengeErrChan:
		return nil, err
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return nil, fmt.Errorf("gateway connect challenge timeout")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *GatewayWsClient) Request(method string, params interface{}, timeoutMs int, expectFinal bool) (interface{}, error) {
	id := uuid.New().String()
	frame := GatewayRequestFrame{
		Type:   "req",
		ID:     id,
		Method: method,
		Params: params,
	}

	payload, err := json.Marshal(frame)
	if err != nil {
		return nil, err
	}

	resChan := make(chan interface{}, 1)
	errChan := make(chan error, 1)

	var timer *time.Timer
	if timeoutMs > 0 {
		timer = time.AfterFunc(time.Duration(timeoutMs)*time.Millisecond, func() {
			c.pending.Delete(id)
			errChan <- fmt.Errorf("gateway request timeout (%s)", method)
		})
	}

	c.pending.Store(id, &PendingRequest{
		Resolve: func(val interface{}) {
			select {
			case resChan <- val:
			default:
			}
		},
		Reject: func(err error) {
			select {
			case errChan <- err:
			default:
			}
		},
		ExpectFinal: expectFinal,
		Timer:       timer,
	})

	c.mu.Lock()
	if c.isClosed || c.ws == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("gateway not connected")
	}
	err = c.ws.WriteMessage(websocket.TextMessage, payload)
	c.mu.Unlock()

	if err != nil {
		c.pending.Delete(id)
		return nil, err
	}

	select {
	case res := <-resChan:
		return res, nil
	case err := <-errChan:
		return nil, err
	}
}

func (c *GatewayWsClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isClosed {
		return
	}
	c.isClosed = true
	if c.ws != nil {
		c.ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "paperclip-complete"))
		c.ws.Close()
	}
}

func (c *GatewayWsClient) failPending(err error) {
	c.pending.Range(func(key, value interface{}) bool {
		req := value.(*PendingRequest)
		if req.Timer != nil {
			req.Timer.Stop()
		}
		req.Reject(err)
		c.pending.Delete(key)
		return true
	})
}

func (c *GatewayWsClient) readLoop() {
	defer func() {
		err := fmt.Errorf("gateway connection closed")
		c.failPending(err)
		select {
		case c.challengeErrChan <- err:
		default:
		}
	}()

	for {
		c.mu.Lock()
		ws := c.ws
		c.mu.Unlock()
		if ws == nil {
			break
		}

		_, message, err := ws.ReadMessage()
		if err != nil {
			if c.opts.OnLog != nil {
				c.opts.OnLog("stderr", fmt.Sprintf("[openclaw-gateway] websocket closed: %v\n", err))
			}
			break
		}
		c.handleMessage(message)
	}
}

func (c *GatewayWsClient) handleMessage(raw []byte) {
	var base map[string]interface{}
	if err := json.Unmarshal(raw, &base); err != nil {
		return
	}

	typ := ""
	if t, ok := base["type"].(string); ok {
		typ = t
	}

	if typ == "event" {
		var event GatewayEventFrame
		if err := json.Unmarshal(raw, &event); err == nil {
			if event.Event == "connect.challenge" {
				if m, ok := event.Payload.(map[string]interface{}); ok {
					if nonce, ok := m["nonce"].(string); ok && nonce != "" {
						select {
						case c.challengeChan <- nonce:
						default:
						}
						return
					}
				}
			}
			if c.opts.OnEvent != nil {
				go c.opts.OnEvent(event)
			}
		}
		return
	}

	if typ == "res" {
		var res GatewayResponseFrame
		if err := json.Unmarshal(raw, &res); err == nil {
			val, ok := c.pending.Load(res.ID)
			if !ok {
				return
			}
			req := val.(*PendingRequest)

			if req.ExpectFinal {
				if m, ok := res.Payload.(map[string]interface{}); ok {
					if s, ok := m["status"].(string); ok && strings.ToLower(s) == "accepted" {
						return // Still waiting
					}
				}
			}

			if req.Timer != nil {
				req.Timer.Stop()
			}
			c.pending.Delete(res.ID)

			if res.Ok {
				req.Resolve(res.Payload)
				return
			}

			msg := "gateway request failed"
			if res.Error.Message != nil {
				if s, ok := res.Error.Message.(string); ok && s != "" {
					msg = s
				}
			} else if res.Error.Code != nil {
				if s, ok := res.Error.Code.(string); ok && s != "" {
					msg = s
				}
			}

			req.Reject(fmt.Errorf("%s", msg))
		}
	}
}

func DerivePublicKeyRaw(publicKeyPem string) []byte {
	block, _ := pem.Decode([]byte(publicKeyPem))
	if block == nil {
		return nil
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil
	}
	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil
	}
	return []byte(edPub)
}

func Base64UrlEncode(buf []byte) string {
	res := base64.URLEncoding.EncodeToString(buf)
	return strings.TrimRight(res, "=")
}

func SignDevicePayload(privateKeyPem string, payload string) string {
	block, _ := pem.Decode([]byte(privateKeyPem))
	if block == nil {
		return ""
	}
	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return ""
	}
	edPriv, ok := priv.(ed25519.PrivateKey)
	if !ok {
		return ""
	}

	sig := ed25519.Sign(edPriv, []byte(payload))
	return Base64UrlEncode(sig)
}

func BuildDeviceAuthPayloadV3(deviceId, clientId, clientMode, role string, scopes []string, signedAtMs int64, token, nonce, platform, deviceFamily string) string {
	scopesStr := strings.Join(scopes, ",")
	return fmt.Sprintf("v3|%s|%s|%s|%s|%s|%d|%s|%s|%s|%s",
		deviceId, clientId, clientMode, role, scopesStr, signedAtMs, token, nonce, platform, deviceFamily)
}

func ResolveDeviceIdentity(configuredPrivateKey string) GatewayDeviceIdentity {
	if configuredPrivateKey != "" {
		raw := DerivePublicKeyRaw(configuredPrivateKey)
		hash := sha256.Sum256(raw)
		return GatewayDeviceIdentity{
			DeviceID:              fmt.Sprintf("%x", hash),
			PublicKeyRawBase64Url: Base64UrlEncode(raw),
			PrivateKeyPem:         configuredPrivateKey,
			Source:                "configured",
		}
	}

	// Generate ephemeral
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
	privPem := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	hash := sha256.Sum256(pub)
	return GatewayDeviceIdentity{
		DeviceID:              fmt.Sprintf("%x", hash),
		PublicKeyRawBase64Url: Base64UrlEncode(pub),
		PrivateKeyPem:         privPem,
		Source:                "ephemeral",
	}
}

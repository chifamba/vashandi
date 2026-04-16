package openclawgateway

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"
)

type ExecutionContext struct {
	RunID     string
	Agent     AgentInfo
	Config    map[string]interface{}
	Context   map[string]interface{}
	Runtime   map[string]interface{}
	AuthToken string

	OnLog   func(stream, chunk string) error
	OnMeta  func(meta map[string]interface{}) error
	OnSpawn func(pid int) error
}

type AgentInfo struct {
	ID        string
	Name      string
	CompanyID string
}

type AdapterRuntimeServiceReport struct {
	ServiceName  string `json:"serviceName"`
	Status       string `json:"status"`
	Lifecycle    string `json:"lifecycle"`
	ScopeType    string `json:"scopeType"`
	URL          string `json:"url,omitempty"`
	ProviderRef  string `json:"providerRef,omitempty"`
	HealthStatus string `json:"healthStatus"`
}

type ExecutionResult struct {
	ExitCode         int
	Signal           string
	TimedOut         bool
	ErrorMessage     string
	Usage            map[string]interface{}
	SessionID        string
	SessionParams    map[string]interface{}
	SessionDisplayID string
	Provider         string
	Biller           string
	Model            string
	BillingType      string
	CostUsd          float64
	ResultJSON       map[string]interface{}
	Summary          string
	ClearSession     bool
	RuntimeServices  []AdapterRuntimeServiceReport
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func asBoolean(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if s, ok := v.(string); ok {
		lower := strings.ToLower(strings.TrimSpace(s))
		return lower == "true" || lower == "1"
	}
	return false
}

func asNumber(v interface{}, fallback int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return fallback
}

func autoApproveDevicePairing(ctx context.Context, url string, headers map[string]string, connectTimeoutMs int, clientId, clientMode, clientVersion, role string, scopes []string, authToken, password, requestId, deviceId string, onLog func(stream, chunk string) error) (bool, string) {
	if authToken == "" && password == "" {
		return false, "shared auth token/password is missing"
	}

	approvalScopes := append(scopes, "operator.pairing")
	client := NewGatewayWsClient(GatewayClientOptions{
		URL:     url,
		Headers: headers,
		OnEvent: func(frame GatewayEventFrame) {},
		OnLog:   func(stream, chunk string) {},
	})

	defer client.Close()

	if onLog != nil {
		onLog("stdout", "[openclaw-gateway] pairing required; attempting automatic pairing approval via gateway methods\n")
	}

	buildParams := func(nonce string) map[string]interface{} {
		authMap := map[string]interface{}{}
		if authToken != "" {
			authMap["token"] = authToken
		}
		if password != "" {
			authMap["password"] = password
		}

		return map[string]interface{}{
			"minProtocol": 3,
			"maxProtocol": 3,
			"client": map[string]interface{}{
				"id":       clientId,
				"version":  clientVersion,
				"platform": runtime.GOOS,
				"mode":     clientMode,
			},
			"role":   role,
			"scopes": approvalScopes,
			"auth":   authMap,
		}
	}

	_, err := client.Connect(ctx, buildParams, connectTimeoutMs)
	if err != nil {
		return false, err.Error()
	}

	if requestId == "" {
		res, err := client.Request("device.pair.list", map[string]interface{}{}, connectTimeoutMs, false)
		if err != nil {
			return false, err.Error()
		}
		if m, ok := res.(map[string]interface{}); ok {
			if pending, ok := m["pending"].([]interface{}); ok && len(pending) > 0 {
				last := pending[len(pending)-1].(map[string]interface{})
				if id, ok := last["requestId"].(string); ok {
					requestId = id
				}
			}
		}
	}

	if requestId == "" {
		return false, "no pending device pairing request found"
	}

	_, err = client.Request("device.pair.approve", map[string]interface{}{"requestId": requestId}, connectTimeoutMs, false)
	if err != nil {
		return false, err.Error()
	}

	return true, requestId
}

func Execute(ctx context.Context, ec ExecutionContext) (ExecutionResult, error) {
	config := ec.Config
	urlValue := asString(config["url"])
	if urlValue == "" {
		return ExecutionResult{ExitCode: 1, ErrorMessage: "OpenClaw gateway adapter missing url"}, nil
	}

	timeoutSec := asNumber(config["timeoutSec"], 120)
	connectTimeoutMs := 15000
	if timeoutSec <= 0 {
		connectTimeoutMs = 10000
	}

	headers := make(map[string]string)
	if h, ok := config["headers"].(map[string]interface{}); ok {
		for k, v := range h {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}

	authToken := asString(config["authToken"])
	if authToken == "" {
		authToken = asString(config["token"])
	}
	if authToken != "" {
		headers["Authorization"] = "Bearer " + authToken
	}

	password := asString(config["password"])

	clientId := asString(config["clientId"])
	if clientId == "" {
		clientId = "gateway-client"
	}
	clientMode := asString(config["clientMode"])
	if clientMode == "" {
		clientMode = "backend"
	}
	clientVersion := "paperclip"
	role := "operator"
	scopes := []string{"operator.admin"}

	autoPairOnFirstConnect := asBoolean(config["autoPairOnFirstConnect"])
	if config["autoPairOnFirstConnect"] == nil {
		autoPairOnFirstConnect = true
	}
	disableDeviceAuth := asBoolean(config["disableDeviceAuth"])

	if ec.OnMeta != nil {
		ec.OnMeta(map[string]interface{}{
			"adapterType": "openclaw_gateway",
			"command":     "gateway",
			"commandArgs": []string{"ws", urlValue, "agent"},
		})
	}

	autoPairAttempted := false

	for {
		var lifecycleError string
		var assistantChunks []string

		client := NewGatewayWsClient(GatewayClientOptions{
			URL:     urlValue,
			Headers: headers,
			OnLog: func(stream, chunk string) {
				if ec.OnLog != nil {
					ec.OnLog(stream, chunk)
				}
			},
			OnEvent: func(frame GatewayEventFrame) {
				if frame.Event == "agent" {
					if m, ok := frame.Payload.(map[string]interface{}); ok {
						if d, ok := m["data"].(map[string]interface{}); ok {
							stream := asString(m["stream"])
							if stream == "assistant" {
								if delta := asString(d["delta"]); delta != "" {
									assistantChunks = append(assistantChunks, delta)
								} else if text := asString(d["text"]); text != "" {
									assistantChunks = append(assistantChunks, text)
								}
							} else if stream == "error" || stream == "lifecycle" {
								if e := asString(d["error"]); e != "" {
									lifecycleError = e
								} else if msg := asString(d["message"]); msg != "" {
									lifecycleError = msg
								}
							}
						}
					}
				}
			},
		})

		var deviceIdentity GatewayDeviceIdentity
		if !disableDeviceAuth {
			deviceIdentity = ResolveDeviceIdentity(asString(config["devicePrivateKeyPem"]))
		}

		buildParams := func(nonce string) map[string]interface{} {
			authMap := map[string]interface{}{}
			if authToken != "" {
				authMap["token"] = authToken
			}
			if password != "" {
				authMap["password"] = password
			}

			connectParams := map[string]interface{}{
				"minProtocol": 3,
				"maxProtocol": 3,
				"client": map[string]interface{}{
					"id":       clientId,
					"version":  clientVersion,
					"platform": runtime.GOOS,
					"mode":     clientMode,
				},
				"role":   role,
				"scopes": scopes,
			}
			if len(authMap) > 0 {
				connectParams["auth"] = authMap
			}

			if !disableDeviceAuth {
				payload := BuildDeviceAuthPayloadV3(
					deviceIdentity.DeviceID, clientId, clientMode, role, scopes,
					time.Now().UnixMilli(), authToken, nonce, runtime.GOOS, "",
				)

				connectParams["device"] = map[string]interface{}{
					"id":        deviceIdentity.DeviceID,
					"publicKey": deviceIdentity.PublicKeyRawBase64Url,
					"signature": SignDevicePayload(deviceIdentity.PrivateKeyPem, payload),
					"signedAt":  time.Now().UnixMilli(),
					"nonce":     nonce,
				}
			}
			return connectParams
		}

		_, err := client.Connect(ctx, buildParams, connectTimeoutMs)
		if err != nil {
			client.Close()

			lower := strings.ToLower(err.Error())
			pairingRequired := strings.Contains(lower, "pairing required")

			if pairingRequired && !disableDeviceAuth && autoPairOnFirstConnect && !autoPairAttempted && (authToken != "" || password != "") {
				autoPairAttempted = true
				ok, reqId := autoApproveDevicePairing(ctx, urlValue, headers, connectTimeoutMs, clientId, clientMode, clientVersion, role, scopes, authToken, password, "", deviceIdentity.DeviceID, ec.OnLog)
				if ok {
					if ec.OnLog != nil {
						ec.OnLog("stdout", fmt.Sprintf("[openclaw-gateway] auto-approved pairing request %s; retrying\n", reqId))
					}
					continue
				}
			}

			return ExecutionResult{ExitCode: 1, ErrorMessage: err.Error()}, nil
		}

		payloadTemplate := make(map[string]interface{})
		if t, ok := config["payloadTemplate"].(map[string]interface{}); ok {
			for k, v := range t {
				payloadTemplate[k] = v
			}
		}
		
		agentParams := payloadTemplate
		agentParams["sessionKey"] = ec.RunID
		agentParams["idempotencyKey"] = ec.RunID
		agentParams["timeout"] = timeoutSec * 1000

		acceptedPayload, err := client.Request("agent", agentParams, connectTimeoutMs, true)
		if err != nil {
			client.Close()
			return ExecutionResult{ExitCode: 1, ErrorMessage: err.Error()}, nil
		}

		var runId string
		if m, ok := acceptedPayload.(map[string]interface{}); ok {
			if r, ok := m["runId"].(string); ok {
				runId = r
			}
			if s, ok := m["status"].(string); ok && strings.ToLower(s) == "error" {
				client.Close()
				errTxt := "OpenClaw gateway agent request failed"
				if sum, ok := m["summary"].(string); ok && sum != "" {
					errTxt = sum
				}
				return ExecutionResult{ExitCode: 1, ErrorMessage: errTxt}, nil
			}
		}

		waitPayload, err := client.Request("agent.wait", map[string]interface{}{"runId": runId, "timeoutMs": timeoutSec * 1000}, (timeoutSec*1000)+connectTimeoutMs, false)
		
		client.Close()
		
		if err != nil {
			msg := err.Error()
			if lifecycleError != "" {
				msg = lifecycleError
			}
			return ExecutionResult{ExitCode: 1, ErrorMessage: msg}, nil
		}

		summary := strings.Join(assistantChunks, "")
		
		return ExecutionResult{
			ExitCode:   0,
			Summary:    summary,
			ResultJSON: waitPayload.(map[string]interface{}),
		}, nil
	}
}

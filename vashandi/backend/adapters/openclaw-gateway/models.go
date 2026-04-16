package openclawgateway

type GatewayDeviceIdentity struct {
	DeviceID              string
	PublicKeyRawBase64Url string
	PrivateKeyPem         string
	Source                string // "configured" or "ephemeral"
}

type GatewayRequestFrame struct {
	Type   string      `json:"type"`
	ID     string      `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

type GatewayErrorDetail struct {
	Code    interface{} `json:"code,omitempty"`
	Message interface{} `json:"message,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

type GatewayResponseFrame struct {
	Type    string             `json:"type"`
	ID      string             `json:"id"`
	Ok      bool               `json:"ok"`
	Payload interface{}        `json:"payload,omitempty"`
	Error   GatewayErrorDetail `json:"error,omitempty"`
}

type GatewayEventFrame struct {
	Type    string      `json:"type"`
	Event   string      `json:"event"`
	Payload interface{} `json:"payload,omitempty"`
	Seq     int         `json:"seq,omitempty"`
}

type WakePayload struct {
	RunId          string
	AgentId        string
	CompanyId      string
	TaskId         *string
	IssueId        *string
	WakeReason     *string
	WakeCommentId  *string
	ApprovalId     *string
	ApprovalStatus *string
	IssueIds       []string
}

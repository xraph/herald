package push

// Headers the sealed driver sets on a gateway request. The gateway reads the
// same names, so both sides import them from here.
const (
	// HeaderKey carries the server's Ed25519 public key, standard base64.
	HeaderKey = "X-Herald-Key"
	// HeaderTimestamp carries the Unix seconds the request was signed at.
	HeaderTimestamp = "X-Herald-Timestamp"
	// HeaderSignature carries the standard base64 Ed25519 signature of SignatureBase.
	HeaderSignature = "X-Herald-Signature"
)

// SendBody is the JSON body of POST /v1/send on a push gateway.
type SendBody struct {
	Handle     string `json:"handle"`
	Sealed     string `json:"sealed,omitempty"`
	KeyID      string `json:"key_id"`
	Urgency    string `json:"urgency"`
	Kind       string `json:"kind"`
	CollapseID string `json:"collapse_id,omitempty"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`
}

// SignatureBase returns the bytes a request signature covers: the timestamp,
// a dot, then the raw body.
func SignatureBase(timestamp string, body []byte) []byte {
	base := append([]byte(timestamp), '.')
	return append(base, body...)
}

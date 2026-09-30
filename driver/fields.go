package driver

// Placement says which provider map a field lives in. Credentials are
// encrypted at rest when a key is configured and never echoed back; settings
// are plain configuration.
type Placement string

// Placements.
const (
	PlacementCredential Placement = "credential"
	PlacementSetting    Placement = "setting"
)

// Field describes one value a driver reads from a provider.
type Field struct {
	Key       string    `json:"key"`
	Label     string    `json:"label"`
	Help      string    `json:"help,omitempty"`
	Required  bool      `json:"required"`
	Secret    bool      `json:"secret"`
	Placement Placement `json:"placement"`
}

// Describer is optional. A driver that implements it gets a form built from
// its fields; one that doesn't gets free-form key/value editing.
type Describer interface {
	Fields() []Field
}

// Describe returns a registered driver's fields. The bool is false for an
// unknown driver and for one that doesn't implement Describer.
func (r *Registry) Describe(name string) ([]Field, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.drivers[name]
	if !ok {
		return nil, false
	}
	desc, ok := d.(Describer)
	if !ok {
		return nil, false
	}
	fields := desc.Fields()
	if fields == nil {
		fields = []Field{}
	}
	return fields, true
}

// SenderFields are the settings Herald reads itself for email: the from
// address and display name, used when no scoped config supplies them.
func SenderFields() []Field {
	return []Field{
		{Key: "from", Label: "From address", Help: "Used when no routing rule sets one.", Placement: PlacementSetting},
		{Key: "from_name", Label: "From name", Placement: PlacementSetting},
	}
}

package config

// Cluster describes optional cluster-level semantics (RAC, PG HA, etc.).
type Cluster struct {
	Kind               string      `yaml:"kind"                           json:"kind"                           validate:"required,eq=Cluster"`
	Type               string      `yaml:"type,omitempty"                 json:"type,omitempty"                 validate:"omitempty,oneof=oracle-rac oracle-single pg-single pg-ha plain"`
	ScanName           string      `yaml:"scan_name,omitempty"            json:"scan_name,omitempty"`
	ScanIPs            []string    `yaml:"scan_ips,omitempty"             json:"scan_ips,omitempty"             validate:"omitempty,dive,ip"`
	InterconnectSubnet string      `yaml:"interconnect_subnet,omitempty"  json:"interconnect_subnet,omitempty"  validate:"omitempty,cidr"`
	HostsEntries       []HostEntry `yaml:"hosts_entries,omitempty"        json:"hosts_entries,omitempty"        validate:"dive"`
}

// HostEntry is a /etc/hosts-style mapping.
type HostEntry struct {
	IP    string   `yaml:"ip"    json:"ip"    validate:"required,ip"`
	Names []string `yaml:"names" json:"names" validate:"required,min=1"`
}

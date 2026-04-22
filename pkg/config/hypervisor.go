package config

// Hypervisor describes the Proxmox-side topology (nodes, VMIDs, disks, NICs,
// ISO/kickstart bits). Owned by proxclt.
type Hypervisor struct {
	Kind      string           `yaml:"kind"                 json:"kind"                 validate:"required,eq=Hypervisor"`
	Nodes     map[string]Node  `yaml:"nodes"                json:"nodes"                validate:"required,min=1,dive"`
	Defaults  *NodeDefaults    `yaml:"defaults,omitempty"   json:"defaults,omitempty"`
	ISO       *ISOConfig       `yaml:"iso,omitempty"        json:"iso,omitempty"`
	Kickstart *KickstartConfig `yaml:"kickstart,omitempty"  json:"kickstart,omitempty"`
}

// Node is one logical VM (mapped onto a Proxmox host + VMID).
type Node struct {
	Proxmox   ProxmoxRef        `yaml:"proxmox"              json:"proxmox"              validate:"required"`
	IPs       map[string]string `yaml:"ips"                  json:"ips"                  validate:"required,dive,ip"`
	Resources *Resources        `yaml:"resources,omitempty"  json:"resources,omitempty"`
	NICs      []NIC             `yaml:"nics,omitempty"       json:"nics,omitempty"       validate:"dive"`
	Disks     []Disk            `yaml:"disks,omitempty"      json:"disks,omitempty"      validate:"dive"`
	Tags      []string          `yaml:"tags,omitempty"       json:"tags,omitempty"`
}

// ProxmoxRef pins a logical node to a physical Proxmox node + VMID.
type ProxmoxRef struct {
	NodeName string `yaml:"node_name" json:"node_name" validate:"required"`
	VMID     int    `yaml:"vm_id"     json:"vm_id"     validate:"required,min=100,max=999999"`
}

// Resources describes VM CPU/memory/BIOS/machine attributes.
type Resources struct {
	Memory  int    `yaml:"memory"            json:"memory"            validate:"min=512"`
	Cores   int    `yaml:"cores"             json:"cores"             validate:"min=1"`
	Sockets int    `yaml:"sockets"           json:"sockets"           validate:"min=1"`
	CPU     string `yaml:"cpu,omitempty"     json:"cpu,omitempty"`
	BIOS    string `yaml:"bios,omitempty"    json:"bios,omitempty"    validate:"omitempty,oneof=seabios ovmf"`
	Machine string `yaml:"machine,omitempty" json:"machine,omitempty"`
}

// NIC is a virtual network interface.
type NIC struct {
	NameField         string      `yaml:"name"                         json:"name"                         validate:"required"`
	Bridge            string      `yaml:"bridge,omitempty"             json:"bridge,omitempty"`
	MAC               string      `yaml:"mac,omitempty"                json:"mac,omitempty"`
	Usage             string      `yaml:"usage"                        json:"usage"                        validate:"required,oneof=public vip scan private management"`
	Bootproto         string      `yaml:"bootproto,omitempty"          json:"bootproto,omitempty"          validate:"omitempty,oneof=static dhcp none link ibft"`
	ControlledBy      string      `yaml:"controlled_by,omitempty"      json:"controlled_by,omitempty"      validate:"omitempty,oneof=NetworkManager crs networkd"`
	Network           string      `yaml:"network,omitempty"            json:"network,omitempty"`
	IPv4              *IPv4Config `yaml:"ipv4,omitempty"               json:"ipv4,omitempty"`
	IPv4Addresses     []string    `yaml:"ipv4_addresses,omitempty"     json:"ipv4_addresses,omitempty"     validate:"omitempty,dive,ip"`
	HostnameAliases   []string    `yaml:"hostname_aliases,omitempty"   json:"hostname_aliases,omitempty"`
	SharedWithCluster bool        `yaml:"shared_with_cluster,omitempty" json:"shared_with_cluster,omitempty"`
}

// IPv4Config is an IPv4 configuration for static NICs.
type IPv4Config struct {
	Address string   `yaml:"address"           json:"address"           validate:"required,cidr"`
	Gateway string   `yaml:"gateway,omitempty" json:"gateway,omitempty" validate:"omitempty,ip"`
	DNS     []string `yaml:"dns,omitempty"     json:"dns,omitempty"     validate:"omitempty,dive,ip"`
}

// Disk describes a single virtual disk attached to a VM.
type Disk struct {
	ID           int    `yaml:"id"                      json:"id"                      validate:"gte=0"`
	Size         string `yaml:"size"                    json:"size"                    validate:"required"`
	StorageClass string `yaml:"storage_class,omitempty" json:"storage_class,omitempty"`
	Storage      string `yaml:"storage,omitempty"       json:"storage,omitempty"`
	Interface    string `yaml:"interface,omitempty"     json:"interface,omitempty"     validate:"omitempty,oneof=scsi0 scsi1 scsi2 scsi3 scsi4 scsi5 scsi6 scsi7 scsi8 scsi9"`
	Shared       bool   `yaml:"shared,omitempty"        json:"shared,omitempty"`
	Tag          string `yaml:"tag,omitempty"           json:"tag,omitempty"`
	Role         string `yaml:"role,omitempty"          json:"role,omitempty"`
}

// ISOConfig describes the installation ISO + kickstart injection bits.
type ISOConfig struct {
	Storage          string `yaml:"storage"                    json:"storage"                    validate:"required"`
	Image            string `yaml:"image"                      json:"image"                      validate:"required"`
	GuestOSType      string `yaml:"guest_os_type,omitempty"    json:"guest_os_type,omitempty"`
	KickstartStorage string `yaml:"kickstart_storage,omitempty" json:"kickstart_storage,omitempty"`
	BootloaderDir    string `yaml:"bootloader_dir,omitempty"   json:"bootloader_dir,omitempty"`
}

// KickstartConfig is the per-env kickstart/preseed inputs proxclt renders into a bootstrap file.
type KickstartConfig struct {
	Distro          string              `yaml:"distro"                     json:"distro"                     validate:"required,oneof=oraclelinux8 oraclelinux9 ubuntu2204 rhel9 rocky9 sles15"`
	Timezone        string              `yaml:"timezone,omitempty"         json:"timezone,omitempty"`
	KeyboardLayout  string              `yaml:"keyboard_layout,omitempty"  json:"keyboard_layout,omitempty"`
	Lang            string              `yaml:"lang,omitempty"             json:"lang,omitempty"`
	Mode            string              `yaml:"mode,omitempty"             json:"mode,omitempty"             validate:"omitempty,oneof=text graphical"`
	IPv6Enabled     bool                `yaml:"ipv6,omitempty"             json:"ipv6,omitempty"`
	ChronyServers   []string            `yaml:"chrony_servers,omitempty"   json:"chrony_servers,omitempty"`
	Sudo            *SudoConfig         `yaml:"sudo,omitempty"             json:"sudo,omitempty"`
	Packages        *PackagesConfig     `yaml:"packages,omitempty"         json:"packages,omitempty"`
	Firewall        *KSFirewall         `yaml:"firewall,omitempty"         json:"firewall,omitempty"`
	UpdateSystem    bool                `yaml:"update_system,omitempty"    json:"update_system,omitempty"`
	SSHKeys         map[string][]string `yaml:"ssh_keys,omitempty"         json:"ssh_keys,omitempty"`
	AdditionalUsers []AdditionalUser    `yaml:"additional_users,omitempty" json:"additional_users,omitempty" validate:"dive"`
}

// SudoConfig captures sudoers-style flags.
type SudoConfig struct {
	WheelNopasswd bool `yaml:"wheel_nopasswd,omitempty" json:"wheel_nopasswd,omitempty"`
}

// PackagesConfig enumerates base and post-install package lists.
type PackagesConfig struct {
	Base []string `yaml:"base,omitempty" json:"base,omitempty"`
	Post []string `yaml:"post,omitempty" json:"post,omitempty"`
}

// KSFirewall gates the install-time firewall.
type KSFirewall struct {
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// AdditionalUser is a local user to create on the freshly-installed host.
type AdditionalUser struct {
	Name   string `yaml:"name"    json:"name"    validate:"required"`
	Wheel  bool   `yaml:"wheel,omitempty" json:"wheel,omitempty"`
	SSHKey string `yaml:"ssh_key" json:"ssh_key" validate:"required"`
}

// NodeDefaults supplies defaults applied to every node entry (unless overridden).
type NodeDefaults struct {
	Resources *Resources `yaml:"resources,omitempty" json:"resources,omitempty"`
	NICs      []NIC      `yaml:"nics,omitempty"      json:"nics,omitempty"`
	Disks     []Disk     `yaml:"disks,omitempty"     json:"disks,omitempty"`
	Tags      []string   `yaml:"tags,omitempty"      json:"tags,omitempty"`
}

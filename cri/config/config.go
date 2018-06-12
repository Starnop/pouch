package config

const (
	// K8sNamespace is the namespace we use to connect containerd when CRI is enabled.
	K8sNamespace = "k8s.io"
)

// Config defines the CRI configuration.
type Config struct {
	// Listen is the listening address which servers CRI.
	Listen string
	// NetworkPluginBinDir is the directory in which the binaries for the plugin is kept.
	NetworkPluginBinDir string
	// NetworkPluginConfDir is the directory in which the admin places a CNI conf.
	NetworkPluginConfDir string
	// SandboxImage is the image used by sandbox container.
	SandboxImage string
	// CriVersion is the cri version
	CriVersion string
}

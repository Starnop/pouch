package config

// Config defines the CRI configuration.
type Config struct {
	// Listen is the listening address which servers CRI.
	Listen string
	// NetworkPluginBinDir is the directory in which the binaries for the plugin is kept.
	NetworkPluginBinDir string
	// NetworkPluginConfDir is the directory in which the admin places a CNI conf.
	NetworkPluginConfDir string
	// NetworkPluginConfTemplate is the file path of golang template used to generate
	// cni config.
	// When it is set, containerd will get cidr from kubelet to replace {{.PodCIDR}} in
	// the template, and write the config into NetworkPluginConfDir.
	// Ideally the cni config should be placed by system admin or cni daemon like calico,
	// weaveworks etc. However, there are still users using kubenet
	// (https://kubernetes.io/docs/concepts/cluster-administration/network-plugins/#kubenet)
	// today, who don't have a cni daemonset in production. NetworkPluginConfTemplate is
	// a temporary backward-compatible solution for them.
	// TODO: Deprecate this option when kubenet is deprecated.
	NetworkPluginConfTemplate string
	// SandboxImage is the image used by sandbox container.
	SandboxImage string
	// CriVersion is the cri version
	CriVersion string
}

package v1alpha1

import (
	"github.com/libopenstorage/openstorage/api"
	"k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Cluster describes a Portworx cluster
type Cluster struct {
	meta.TypeMeta   `json:",inline"`
	meta.ObjectMeta `json:"metadata,omitempty"`
	Spec            ClusterSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterList is a list of Cluster objects in Kubernetes
type ClusterList struct {
	meta.TypeMeta `json:",inline"`
	meta.ListMeta `json:"metadata,omitempty"`

	Items []Cluster `json:"items"`
}

// ClusterSpec defines the specification for a Cluster
type ClusterSpec struct {
	// Kvdb is the key value store configuration
	Kvdb KvdbSpec `json:"kvdb"`
	// PXVersion is the Portworx version/image to use on all nodes of the cluster.
	// +optional
	PXVersion string `json:"pxVersion,omitempty"`
	// Network specifies the networking setting to be used for all nodes. This
	// can be overridden by individual nodes in the NodeSpec
	Network NodeNetwork `json:"network,omitempty"`
	// Storage specifies the storage configuration to be used for all nodes.
	// This can be overridden by individual nodes in the NodeSpec
	Storage StorageSpec `json:"storage,omitempty"`
	// Placement specifies the rules by which PX nodes are selected
	Placement PlacementSpec `json:"placement,omitempty"`
	// Env is the list of environment variables to expose to PX pods
	Env []v1.EnvVar `json:"env,omitempty"`
}

// Nodes are all Portworx nodes participating in this cluster

// KvdbSpec defines the kvdb configuration
type KvdbSpec struct {
	// Endpoints is the list of kvdb endpoints
	Endpoints []string `json:"endpoints"`
	// BasicAuthSecret is the secret contain username and password for basic auth
	BasicAuthSecret string `json:"accessSecret,omitempty"`
	// CertificateSecret is the secret that contains the cert files required for etcd auth
	CertificateSecret string `json:"certificateSecret,omitempty"`
	// ACLTokenSecret is the secret name containing the ACL token for consul auth
	ACLTokenSecret string `json:"aclTokenSecret,omitempty"`
}

// ClusterStatus is the status of the Portworx cluster
type ClusterStatus struct {
	StatusInfo
	Name         string       `json:"name,omitempty"`
	NodeStatuses []NodeStatus `json:"nodeStatuses,omitempty"`
}

// NodeStatus represents status of a cluster node
type NodeStatus struct {
	StatusInfo
	Name string `json:"name,omitempty"`
}

// StatusInfo is used to represent the status of any entity in the cluster
type StatusInfo struct {
	Ready bool       `json:"ready"`
	Code  api.Status `json:"code"`
	// The following follow the same definition as PodStatus
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// NodeNetwork specifies which network interfaces the Node should use for data
// and management transport
type NodeNetwork struct {
	Data string `json:"data"`
	Mgmt string `json:"mgmt"`
}

// StorageSpec specifies the storage configuration for a node
type StorageSpec struct {
	Devices             []string `json:"devices,omitempty"`
	ZeroStorage         bool     `json:"zeroStorage,omitempty"`
	Force               bool     `json:"force,omitempty"`
	UseAll              bool     `json:"useAll,omitempty"`
	UseAllWithParitions bool     `json:"useAllWithParitions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PlacementSpec defines placement rules for various px components
type PlacementSpec struct {
	meta.TypeMeta `json:",inline"`
	PX            Placement `json:"px,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Placement encapsulates the various kubernetes options that control where pods are scheduled and executed.
type Placement struct {
	meta.TypeMeta   `json:",inline"`
	NodeAffinity    *v1.NodeAffinity    `json:"nodeAffinity,omitempty"`
	PodAffinity     *v1.PodAffinity     `json:"podAffinity,omitempty"`
	PodAntiAffinity *v1.PodAntiAffinity `json:"podAntiAffinity,omitempty"`
	Tolerations     []v1.Toleration     `json:"tolerations,omitemtpy"`
}

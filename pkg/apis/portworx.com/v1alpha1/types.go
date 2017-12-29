package v1alpha1

import (
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

	// +optional
	Status ClusterStatus `json:"status,omitempty"`
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
	// Image is the specific image to use on all nodes of the cluster.
	// +optional
	Image string `json:"image,omitempty"`
	// Network specifies the networking setting to be used for all nodes. This
	// can be overridden by individual nodes in the NodeSpec
	Network NodeNetwork `json:"network,omitempty"`
	// Storage specifies the storage configuration to be used for all nodes.
	// This can be overridden by individual nodes in the NodeSpec
	Storage StorageSpec `json:"storage,omitempty"`
	Nodes   []NodeSpec  `json:"nodes,omitempty"`
	Env     []v1.EnvVar `json:"env,omitempty"`
}

// Nodes are all Portworx nodes participating in this cluster

// KvdbSpec defines the kvdb configuration
type KvdbSpec struct {
	Endpoints []string `json:"endpoints"`
}

// ClusterStatus is the status of the Portworx cluster
type ClusterStatus struct {
	StatusInfo
	Conditions   []ClusterCondition `json:"conditions,omitempty"`
	NodeStatuses []NodeStatus       `json:"nodeStatuses,omitempty"`
}

// Node defines a single instance of available storage on a
// node and the appropriate options to apply to it to make it available
// to the cluster.
type Node struct {
	meta.TypeMeta   `json:",inline"`
	meta.ObjectMeta `json:"metadata,omitempty"`
	Spec            NodeSpec `json:"spec,omitempty"`

	// Status represents the current status of the storage node
	// +optional
	Status NodeStatus `json:"status,omitempty"`
}

// NodeList is a list of Node objects in Kubernetes.
type NodeList struct {
	meta.TypeMeta `json:",inline"`
	meta.ListMeta `json:"metadata,omitempty"`
	Items         []Node `json:"items"`
}

// NodeSpec holds specification parameters for a Node.
type NodeSpec struct {
	// +optional
	Name    string      `json:"name,omitempty"`
	Network NodeNetwork `json:"network,omitempty"`
	Storage StorageSpec `json:"storage,omitempty"`
	Env     []v1.EnvVar `json:"env,omitempty"`
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

// StatusCondition is data type for representation status event
type StatusCondition struct {
	Time    meta.Time `json:"time,omitempty"`
	Message string    `json:"message,omitempty"`
	Reason  string    `json:"reason,omitempty"`
}

// StatusInfo represents status of a cluster or a node
type StatusInfo struct {
	Ready bool `json:"ready"`

	// The following follow the same definition as PodStatus
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// ClusterConditionType represents the state of the cluster
type ClusterConditionType string

const (
	// ClusterConditionReady represents ready state of cluster
	ClusterConditionReady ClusterConditionType = "Ready"
	// ClusterConditionOffline represents offline state of cluster
	ClusterConditionOffline ClusterConditionType = "Offline"
)

// ClusterCondition represents condition of a cluster
type ClusterCondition struct {
	// StatusCondition
	StatusCondition
	Type ClusterConditionType `json:"type,omitempty"`
}

// NodeConditionType represents the state of a cluster node
type NodeConditionType string

const (
	// NodeConditionReady represents ready state of a cluster node
	NodeConditionReady NodeConditionType = "Ready"
	// NodeConditionOffline represents offline state of a cluster node
	NodeConditionOffline NodeConditionType = "Offline"
)

// NodeCondition represents the condition of a node
type NodeCondition struct {
	StatusCondition
	Type NodeConditionType `json:"type,omitempty"`
}

// NodeStatus represents status of a cluster node
type NodeStatus struct {
	StatusInfo
	Added      bool            `json:"added,omitempty"`
	Conditions []NodeCondition `json:"conditions,omitempty"`
	PodName    string          `json:"podName,omitempty"`
	Name       string          `json:"name,omitempty"`
}

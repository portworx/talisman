package cluster

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// This package follows the factory pattern.
// Reference: http://matthewbrown.io/2016/01/23/factory-pattern-in-golang/

type clusterInitFunction func(conf interface{}) (Cluster, error)

// Cluster an interface to manage a storage cluster
type Cluster interface {
	// Create creates the given cluster
	Create(obj interface{}) error
	// Upgrade upgrades the given cluster
	Upgrade(new interface{}) error
	// Destory destroys all components of the given cluster
	Destroy(obj interface{}) error
}

var clusterFactories = make(map[string]clusterInitFunction)

// Register is function cluster providers can use to register themselves
func Register(name string, initFunc clusterInitFunction) {
	if initFunc == nil {
		logrus.Fatalf("nil initFunc provided by cluster provider: %s", name)
	}

	_, registered := clusterFactories[name]
	if registered {
		logrus.Errorf("Cluster %s already registered. Ignoring.", name)
		return
	}

	clusterFactories[name] = initFunc
}

// Get is function used to get a registered cluster provider
func Get(name string, conf interface{}) (Cluster, error) {
	provider, ok := clusterFactories[name]
	if !ok {
		return nil, fmt.Errorf("cluster provider %s is not registered", name)
	}

	return provider(conf)
}

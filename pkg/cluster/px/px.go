package px

import (
	apiv1alpha1 "github.com/portworx/talisman/pkg/apis/portworx.com/v1alpha1"
	"github.com/sirupsen/logrus"
)

type pxCluster struct {
}

// Cluster an interface to manage a storage cluster
type Cluster interface {
	// Create creates the given cluster
	Create(c *apiv1alpha1.Cluster) error
	// Status returns the current status of the given cluster
	Status(c *apiv1alpha1.Cluster) (*apiv1alpha1.ClusterStatus, error)
	// Upgrade upgrades the given cluster
	Upgrade(c *apiv1alpha1.Cluster) error
	// Destory destroys all components of the given cluster
	Destroy(c *apiv1alpha1.Cluster) error
}

func (p *pxCluster) Create(c *apiv1alpha1.Cluster) error {
	logrus.Infof("creating a new portworx cluster: %s", c.Name)
	return nil
}

func (p *pxCluster) Status(c *apiv1alpha1.Cluster) (*apiv1alpha1.ClusterStatus, error) {
	return nil, nil
}

func (p *pxCluster) Upgrade(c *apiv1alpha1.Cluster) error {
	logrus.Infof("upgrading px cluster: %s", c.Name)
	return nil
}

func (p *pxCluster) Destroy(c *apiv1alpha1.Cluster) error {
	logrus.Infof("destroying px cluster: %s", c.Name)
	return nil
}

// NewPXClusterProvider creates a new PX cluster
func NewPXClusterProvider(conf interface{}) (Cluster, error) {
	return &pxCluster{}, nil
}

package main

import (
	"flag"

	"github.com/portworx/talisman/pkg/apis/portworx.com/v1alpha1"
	"github.com/portworx/talisman/pkg/cluster/px"
	"github.com/portworx/talisman/pkg/version"
	"github.com/sirupsen/logrus"
)

type pxOperation string

const (
	pxOperationUpgrade pxOperation = "upgrade"
)

// command line arguments
var (
	newPXImage           string
	op                   string
	dockerRegistrySecret string
	kubeconfig           string
)

func main() {
	logrus.Infof("Running talisman: %v", version.Version)
	flag.Parse()

	if len(op) == 0 {
		logrus.Fatalf("error: no operation given for the PX cluster")
	}

	switch pxOperation(op) {
	case pxOperationUpgrade:
		doUpgrade()
	default:
		logrus.Fatalf("error: invalid operation: %s", op)
	}
}

func doUpgrade() {
	if len(newPXImage) == 0 {
		logrus.Fatalf("error: no PX image specified for %s operation", op)
	}

	inst, err := px.NewPXClusterProvider(dockerRegistrySecret, kubeconfig)
	if err != nil {
		logrus.Fatalf("failed to instantiate PX cluster provider. err: %v", err)
	}

	// Create a new spec for the PX cluster. Currently, only changing the PX version is supported.
	newSpec := &v1alpha1.Cluster{
		Spec: v1alpha1.ClusterSpec{
			PXVersion: newPXImage,
		},
	}
	err = inst.Upgrade(newSpec)
	if err != nil {
		logrus.Fatalf("failed to ugprade portworx to version: %v. err: %v", newPXImage, err)
	}
}

func init() {
	flag.StringVar(&op, "operation", "upgrade", "Operation to perform for the Portworx cluster")
	flag.StringVar(&newPXImage, "newimage", "", "New Portworx Image to use for the upgrade")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "(optional) Absolute path of the kubeconfig file")
	flag.StringVar(&dockerRegistrySecret, "dockerregsecret", "", "(optional) Kubernetes Secret to pull docker images from a private registry")
}

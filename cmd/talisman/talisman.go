package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/portworx/talisman/pkg/apis/portworx/v1beta1"
	"github.com/portworx/talisman/pkg/cluster/px"
	"github.com/portworx/talisman/pkg/k8sutils"
	"github.com/portworx/talisman/pkg/version"
	"github.com/sirupsen/logrus"
)

type pxOperation string

const (
	pxOperationUpgrade           pxOperation = "upgrade"
	pxOperationRestoreSharedApps pxOperation = "restoresharedapps"
	pxOperationDelete            pxOperation = "delete"
)

// command line arguments
var (
	newPXImage            string
	newPXTag              string
	newOCIMonImage        string
	newOCIMonTag          string
	wiperImage            string
	wiperTag              string
	op                    string
	dockerRegistrySecret  string
	kubeconfig            string
	sharedAppsScaleDown   string
	wipeCluster           bool
	upgradeTimeoutPerNode int
	logFile               string
	logLevelDebug         bool
	logLevelTrace         bool
)

func main() {
	logrus.Infof("Running talisman: %v", version.Version)
	flag.Parse()

	if len(op) == 0 {
		logrus.Fatalf("error: no operation given for the PX cluster")
	}

	// fix logging
	if logFile != "" {
		err := setLogfile(logFile)
		if err != nil {
			logrus.WithError(err).Warnf("Error setting logging to %s file", logFile)
		}
	}
	if logLevelTrace {
		logrus.SetLevel(logrus.TraceLevel)
	} else if logLevelDebug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	switch pxOperation(op) {
	case pxOperationUpgrade:
		doUpgrade()
	case pxOperationRestoreSharedApps:
		doRestoreSharedApps()
	case pxOperationDelete:
		doDelete()
	default:
		logrus.Fatalf("error: invalid operation: %s", op)
	}
}

func setLogfile(fname string) error {
	f, err := os.OpenFile(fname, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	logrus.Infof("Redirecting all output to %s", fname)
	_, _ = fmt.Fprintln(f, "------------------------------------------------------------------------------")
	os.Stdout, os.Stderr = f, f
	logrus.SetOutput(f)
	logrus.Info("Started logging into ", fname)
	return nil
}

func doUpgrade() {
	logrus.Info("Performing UPGRADE operation")
	if len(newOCIMonTag) == 0 {
		logrus.Fatalf("error: new OCI monitor tag not specified for %s operation", op)
	}

	inst, err := px.NewPXClusterProvider(dockerRegistrySecret, kubeconfig)
	if err != nil {
		logrus.Fatalf("failed to instantiate PX cluster provider. err: %v", err)
	}

	// Create a new spec for the PX cluster. Currently, only changing the PX version is supported.
	newSpec := &v1beta1.Cluster{
		Spec: v1beta1.ClusterSpec{
			OCIMonImage: newOCIMonImage,
			OCIMonTag:   newOCIMonTag,
			PXImage:     newPXImage,
			PXTag:       newPXTag,
		},
	}

	opts := &px.UpgradeOptions{
		SharedAppsScaleDown: px.SharedAppsScaleDownMode(sharedAppsScaleDown),
		TimeoutPerNode:      upgradeTimeoutPerNode,
	}

	err = inst.Upgrade(newSpec, opts)
	if err != nil {
		logrus.Fatalf("failed to upgrade portworx to version: %v. err: %v", newOCIMonTag, err)
	}
}

func doRestoreSharedApps() {
	logrus.Info("Performing RESTORE operation")
	inst, err := k8sutils.New(kubeconfig)
	if err != nil {
		logrus.Fatalf("failed to restore shared apps. err: %v", err)
	}

	err = inst.RestoreScaledAppsReplicas()
	if err != nil {
		logrus.Fatalf("failed to restore shared apps. err: %v", err)
	}
}

func doDelete() {
	logrus.Info("Performing DELETE operation")
	inst, err := px.NewPXClusterProvider(dockerRegistrySecret, kubeconfig)
	if err != nil {
		logrus.Fatalf("failed to instantiate PX cluster provider. err: %v", err)
	}

	opts := &px.DeleteOptions{
		WipeCluster: wipeCluster,
		WiperImage:  wiperImage,
		WiperTag:    wiperTag,
	}
	k8sutils.DebugDumpObjectJS(opts, "DELETE options:")

	err = inst.Delete(nil, opts)
	if err != nil {
		logrus.Fatalf("Failed to delete PX cluster. err: %v", err)
	}
}

func init() {
	flag.StringVar(&op, "operation", "upgrade", fmt.Sprintf("Operation to perform for the Portworx cluster. Supported operations: %s, %s, %s",
		pxOperationUpgrade, pxOperationRestoreSharedApps, pxOperationDelete))
	flag.StringVar(&newOCIMonTag, "ocimontag", "", "New OCI Monitor tag to use for the upgrade")
	flag.StringVar(&newOCIMonImage, "ocimonimage", "portworx/oci-monitor", "(optional) New OCI Monitor Image to use for the upgrade")
	flag.StringVar(&newPXImage, "pximage", "", "(optional) New Portworx Image to use for the upgrade")
	flag.StringVar(&newPXTag, "pxtag", "", "(optional) New Portworx tag to use for the upgrade")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "(optional) Absolute path of the kubeconfig file")
	flag.StringVar(&dockerRegistrySecret, "dockerregsecret", "", "(optional) Kubernetes Secret to pull docker images from a private registry")
	flag.StringVar(&sharedAppsScaleDown, "scaledownsharedapps", string(px.SharedAppsScaleDownAuto),
		fmt.Sprintf("(optional) instructs scale down behavior of Portworx shared apps. Supported values: \n"+
			"\t%s: During the upgrade process, Portworx shared applications will be scaled down to 0 replicas if 1.2 to 1.3 version upgrade is detected.\n"+
			"\t%s: During the upgrade process, Portworx shared applications will be unconditionally scaled down to 0 replicas.\n"+
			"\t%s: Upgrade process will not scale down Portworx shared applications.",
			px.SharedAppsScaleDownAuto, px.SharedAppsScaleDownOn, px.SharedAppsScaleDownOff))
	flag.BoolVar(&wipeCluster, "wipecluster", false, "(optional) If given, all Portworx metadata will be removed from the cluster. "+
		"This means all the data will be wiped off from the cluster and cannot be recovered")
	flag.StringVar(&wiperImage, "wiperimage", "", "Node wiper image to use for the upgrade")
	flag.StringVar(&wiperTag, "wipertag", "", "Node wiper tag to use for the upgrade")
	flag.IntVar(&upgradeTimeoutPerNode, "upgradetimeoutpernode", 600, "Timeout per node in seconds for performing upgrade")
	flag.StringVar(&logFile, "log", "", "Specify log file")
	flag.BoolVar(&logLevelDebug, "debug", false, "(optional) Increase logging verbosity (DEBUG level)")
	flag.BoolVar(&logLevelTrace, "trace", false, "(optional) Increase logging verbosity (TRACE level)")
}

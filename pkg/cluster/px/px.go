package px

import (
	"encoding/csv"
	"fmt"
	"regexp"
	"strings"
	"time"

	osd_api "github.com/libopenstorage/openstorage/api"
	osd_clusterclient "github.com/libopenstorage/openstorage/api/client/cluster"
	osd_volclient "github.com/libopenstorage/openstorage/api/client/volume"
	"github.com/libopenstorage/openstorage/cluster"
	"github.com/libopenstorage/openstorage/volume"
	"github.com/portworx/kvdb"
	"github.com/portworx/kvdb/consul"
	e2 "github.com/portworx/kvdb/etcd/v2"
	e3 "github.com/portworx/kvdb/etcd/v3"
	"github.com/portworx/sched-ops/k8s"
	"github.com/portworx/sched-ops/task"
	apiv1beta1 "github.com/portworx/talisman/pkg/apis/portworx/v1beta1"
	"github.com/portworx/talisman/pkg/k8sutils"
	"github.com/sirupsen/logrus"
	apps_api "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	errUsingInternalEtcd = fmt.Errorf("cluster is using internal etcd")
	configMapNameRegex   = regexp.MustCompile("[^a-zA-Z0-9]+")
	pxListOpts           = metav1.ListOptions{
		LabelSelector: "name=portworx",
	}
	apiLabels = map[string]string{
		"name": "portworx-api",
	}
	errNoPXDaemonset = fmt.Errorf("Portworx daemonset not found on the cluster. Ensure you have " +
		"Portworx specs applied in the cluster before issuing this operation.")
	cores = "/var/cores"
)

const (
	changeCauseAnnotation  = "kubernetes.io/change-cause"
	pxEnableLabelKey       = "px/enabled"
	dsOptPwxVolumeName     = "optpwx"
	dsEtcPwxVolumeName     = "etcpwx"
	dsDbusVolumeName       = "dbus"
	dsSysdVolumeName       = "sysdmount"
	devVolumeName          = "dev"
	multipathVolumeName    = "etc-multipath"
	lvmVolumeName          = "lvm"
	sysVolumeName          = "sys"
	coresVolumeName        = "cores"
	udevVolumeName         = "run-udev-data"
	devMount               = "/dev"
	multipathMount         = "/etc/multipath"
	lvmMount               = "/run/lvm"
	sysMount               = "/sys"
	udevMount              = "/run/udev/data"
	sysdmount              = "/etc/systemd/system"
	dbusPath               = "/var/run/dbus"
	pksPersistentStoreRoot = "/var/vcap/store"
	pxOptPwx               = "/opt/pwx"
	pxEtcdPwx              = "/etc/pwx"
)

type platformType string

const (
	platformTypeDefault platformType = ""
	platformTypePKS     platformType = "pks"
)

// SharedAppsScaleDownMode is type for choosing behavior of scaling down shared apps
type SharedAppsScaleDownMode string

const (
	// SharedAppsScaleDownAuto is mode where shared px apps will be scaled down only when px major version is upgraded
	SharedAppsScaleDownAuto SharedAppsScaleDownMode = "auto"
	// SharedAppsScaleDownOn is mode where shared px apps will be scaled down without any version checks
	SharedAppsScaleDownOn SharedAppsScaleDownMode = "on"
	// SharedAppsScaleDownOff is mode where shared px apps will not be scaled down
	SharedAppsScaleDownOff SharedAppsScaleDownMode = "off"
)

var (
	pxVersionRegex = regexp.MustCompile(`^(\d+\.\d+\.\d+).*`)
)

const (
	pxNodeWiperNamespace            = "kube-system"
	pxSecretsNamespace              = "portworx"
	defaultPXImage                  = "portworx/px-enterprise"
	dockerPullerImage               = "portworx/docker-puller:latest"
	defaultNodeWiperImage           = "portworx/px-node-wiper"
	defaultNodeWiperTag             = "2.1.1"
	pxdRestPort                     = 9001
	pxServiceName                   = "portworx-service"
	pxClusterRoleName               = "node-get-put-list-role"
	pxClusterRoleBindingName        = "node-role-binding"
	pxRoleName                      = "px-role"
	pxRoleBindingName               = "px-role-binding"
	pxServiceAccountName            = "px-account"
	pxDaemonsetContainerName        = "portworx"
	pxAPIDaemonset                  = "portworx-api"
	pxAPIServiceName                = "portworx-api"
	lhRoleName                      = "px-lh-role"
	lhRoleBindingName               = "px-lh-role-binding"
	lhServiceAccountName            = "px-lh-account"
	lhConfigMap                     = "px-lighthouse-config"
	lhServiceName                   = "px-lighthouse"
	lhDeploymentName                = "px-lighthouse"
	csiExtDeploymentName            = "px-csi-ext"
	bootstrapCloudDriveNamespace    = "kube-system"
	internalEtcdConfigMapPrefix     = "px-bootstrap-"
	cloudDriveConfigMapPrefix       = "px-cloud-drive-"
	pvcControllerClusterRole        = "portworx-pvc-controller-role"
	pvcControllerClusterRoleBinding = "portworx-pvc-controller-role-binding"
	pvcControllerServiceAccount     = "portworx-pvc-controller-account"
	pvcControllerName               = "portworx-pvc-controller"
	pxVersionLabel                  = "PX Version"
	talismanServiceAccount          = "talisman-account"
	storkControllerName             = "stork"
	storkServiceName                = "stork-service"
	storkControllerConfigMap        = "stork-config"
	storkControllerClusterRole      = "stork-role"
	storkControllerClusterBinding   = "stork-role-binding"
	storkControllerServiceAccount   = "stork-account"
	storkSchedulerClusterRole       = "stork-scheduler-role"
	storkSchedulerCluserBinding     = "stork-scheduler-role-binding"
	storkSchedulerServiceAccount    = "stork-scheduler-account"
	storkSchedulerName              = "stork-scheduler"
	storkSnapshotStorageClass       = "stork-snapshot-sc"
	csiClusterRoleBinding           = "px-csi-role-binding"
	csiService                      = "px-csi-service"
	csiAccount                      = "px-csi-account"
	csiClusterRole                  = "px-csi-role"
	csiStatefulSet                  = "px-csi-ext"
	osbSecretName                   = "px-osb"
	essentialSecretName             = "px-essential"
	pxNodeWiperDaemonSetName        = "px-node-wiper"
	pxKvdbPrefix                    = "pwx/"
	pxImageEnvKey                   = "PX_IMAGE"
)

type pxClusterOps struct {
	k8sOps               k8s.Ops
	utils                *k8sutils.Instance
	dockerRegistrySecret string
	platform             platformType
	installedNamespaces  []string
}

// timeouts and intervals
const (
	defaultRetryInterval          = 10 * time.Second
	pxNodeWiperTimeout            = 10 * time.Minute
	sharedVolDetachTimeout        = 5 * time.Minute
	daemonsetUpdateTriggerTimeout = 5 * time.Minute
	daemonsetDeleteTimeout        = 5 * time.Minute
)

// UpgradeOptions are options to customize the upgrade process
type UpgradeOptions struct {
	SharedAppsScaleDown SharedAppsScaleDownMode
	TimeoutPerNode      int
}

// DeleteOptions are options to customize the delete process
type DeleteOptions struct {
	// WipeCluster instructs if Portworx cluster metadata needs to be wiped off
	WipeCluster bool
	// WiperImage is the docker tag to use for the node wiper
	WiperImage string
	// WiperTag is the docker tag to use for the node wiper
	WiperTag string
}

// Cluster an interface to manage a storage cluster
type Cluster interface {
	// Create creates the given cluster
	Create(c *apiv1beta1.Cluster) error
	// Status returns the current status of the given cluster
	Status(c *apiv1beta1.Cluster) (*apiv1beta1.ClusterStatus, error)
	// Upgrade upgrades the given cluster
	Upgrade(c *apiv1beta1.Cluster, opts *UpgradeOptions) error
	// Delete removes all components of the given cluster
	Delete(c *apiv1beta1.Cluster, opts *DeleteOptions) error
}

// NewPXClusterProvider creates a new PX cluster
func NewPXClusterProvider(dockerRegistrySecret, kubeconfig string) (Cluster, error) {
	utils, err := k8sutils.New(kubeconfig)
	if err != nil {
		return nil, err
	}
	k8sOps := k8s.Instance()

	// Detect the installedNamespace
	namespaces, err := k8sOps.ListNamespaces(map[string]string{})
	if err != nil {
		return nil, err
	}

	installedNamespaces := make([]string, 0)
	for _, ns := range namespaces.Items {
		dss, err := k8sOps.ListDaemonSets(ns.ObjectMeta.Name, pxListOpts)
		if err != nil {
			return nil, err
		}

		if len(dss) > 0 {
			installedNamespaces = append(installedNamespaces, ns.ObjectMeta.Name)
		}
	}

	if len(installedNamespaces) == 0 {
		return nil, errNoPXDaemonset
	}

	return &pxClusterOps{
		k8sOps:               k8sOps,
		dockerRegistrySecret: dockerRegistrySecret,
		utils:                utils,
		installedNamespaces:  installedNamespaces,
	}, nil
}

func (ops *pxClusterOps) Create(spec *apiv1beta1.Cluster) error {
	logrus.Warnf("creating px cluster is not yet implemented")
	return nil
}

func (ops *pxClusterOps) Status(c *apiv1beta1.Cluster) (*apiv1beta1.ClusterStatus, error) {
	logrus.Warnf("cluster status is not yet implemented")
	return nil, nil
}

func (ops *pxClusterOps) Upgrade(newSpec *apiv1beta1.Cluster, opts *UpgradeOptions) error {
	if newSpec == nil {
		return fmt.Errorf("new cluster spec is required for the upgrade call")
	}

	if len(ops.installedNamespaces) == 0 {
		return errNoPXDaemonset
	}

	if len(ops.installedNamespaces) > 1 {
		return fmt.Errorf("Found Portworx installed in more than one namespace: %s."+
			" Portworx is supported only in one namespace per cluster. "+
			" Delete Portworx specs from all but one namespace and retry.", ops.installedNamespaces)
	}

	ns := ops.installedNamespaces[0]
	svc, err := ops.k8sOps.GetService(pxServiceName, ns)
	if err != nil {
		return err
	}

	ip := svc.Spec.ClusterIP
	if len(ip) == 0 {
		return fmt.Errorf("Portworx service doesn't have a ClusterIP assigned")
	}

	volDriver, clusterManager, err := getPXDriver(ip)
	if err != nil {
		return err
	}

	if err := ops.preFlightChecks(newSpec); err != nil {
		return err
	}

	if len(newSpec.Spec.PXImage) == 0 {
		newSpec.Spec.PXImage = defaultPXImage
	}

	if len(newSpec.Spec.PXTag) == 0 {
		newSpec.Spec.PXTag = newSpec.Spec.OCIMonTag
	}

	isAppDrainNeeded, err := ops.isUpgradeAppDrainRequired(newSpec, clusterManager)
	if err != nil {
		logrus.Infof("failed to check application drain requirement due to: %v", err)
		// This was added for old versions and is no longer a hard requirement.
		// Hence ignoring the error now.
	}

	if isAppDrainNeeded {
		unManaged, err := ops.utils.IsAnyPXAppPodUnmanaged()
		if err != nil {
			return err
		}

		if unManaged {
			return fmt.Errorf("Aborting upgrade as some application pods using Portworx are not being managed by a controller." +
				" Retry after deleting these pods or running them using a controller (e.g Deployment, Statefulset etc).")
		}
	}

	logrus.Infof("Upgrading px cluster to %s. Upgrade opts: %v App drain requirement: %v", newSpec.Spec.OCIMonTag, opts, isAppDrainNeeded)

	logrus.Info("Attempting to parse kvdb info from Portworx daemonset")
	var configParseErr error
	_, _, _, ops.platform, _, configParseErr = ops.parseConfigFromDaemonset()
	if configParseErr != nil && configParseErr != errUsingInternalEtcd {
		err := fmt.Errorf("Failed to parse PX config from Daemonset for upgrading PX due to err: %v", configParseErr)
		return err
	}

	// NOTE: Skip step 1. with ops.runDockerPuller(newOCIMonVer, newPXVer) as it doesn't handle private registries, air gapped installs and containerd

	// 2. (Optional) Scale down px shared applications to 0 replicas if required based on opts
	if ops.isScaleDownOfSharedAppsRequired(isAppDrainNeeded, opts) {
		defer func() { // always restore the replicas
			err = ops.utils.RestoreScaledAppsReplicas()
			if err != nil {
				logrus.Errorf("Failed to restore PX shared applications replicas. Err: %v", err)
			}
		}()

		err = ops.utils.ScaleSharedAppsToZero()
		if err != nil {
			return err
		}

		// Wait till all px shared volumes are detached
		err = ops.waitTillPXSharedVolumesDetached(volDriver)
		if err != nil {
			return err
		}
	} else {
		logrus.Infof("Skipping scale down of shared volume applications")
	}

	// 3. Start rolling upgrade of PX DaemonSet
	err = ops.upgradePX(newSpec.Spec, opts)
	if err != nil {
		return err
	}

	return nil
}

func (ops *pxClusterOps) Delete(c *apiv1beta1.Cluster, opts *DeleteOptions) error {
	var (
		endpoints      []string
		kvdbOpts       map[string]string
		clusterName    string
		configParseErr error
		affinity       *v1.Affinity
	)

	// parse kvdb from daemonset before we delete it
	logrus.Info("Attempting to parse kvdb info from Portworx daemonset")
	endpoints, kvdbOpts, clusterName, ops.platform, affinity, configParseErr = ops.parseConfigFromDaemonset()
	if configParseErr != nil && configParseErr != errUsingInternalEtcd {
		err := fmt.Errorf("Failed to parse PX config from Daemonset for deleting PX due to err: %v", configParseErr)
		return err
	}

	pwxHostPathRoot := "/"
	if ops.platform == platformTypePKS {
		pwxHostPathRoot = pksPersistentStoreRoot
	}

	if err := ops.deleteAllPXComponents(clusterName); err != nil {
		return err // this error is unexpected and should not be ignored
	}

	if opts != nil && opts.WipeCluster {
		// the wipe operation is best effort as it is used when cluster is already in a bad shape. for e.g
		// cluster might have been started with incorrect kvdb information. So we won't be able to wipe that off.

		// Wipe px from each node
		err := ops.runPXNodeWiper(pwxHostPathRoot, opts.WiperImage, opts.WiperTag, affinity)
		if err != nil {
			logrus.Warnf("Failed to wipe Portworx local node state. err: %v", err)
		}

		if configParseErr == errUsingInternalEtcd {
			logrus.Infof("Cluster is using internal etcd. No need to wipe kvdb.")
		} else {
			// Cleanup PX kvdb tree
			err = ops.wipePXKvdb(endpoints, kvdbOpts, clusterName)
			if err != nil {
				logrus.Warnf("Failed to wipe Portworx KVDB tree. err: %v", err)
			}
		}
	}

	return nil
}

// waitTillPXSharedVolumesDetached waits till all shared volumes are detached
func (ops *pxClusterOps) waitTillPXSharedVolumesDetached(volDriver volume.VolumeDriver) error {
	scs, err := ops.utils.GetPXSharedSCs()
	if err != nil {
		return err
	}

	var volsToInspect []string
	for _, sc := range scs {
		pvcs, err := ops.k8sOps.GetPVCsUsingStorageClass(sc)
		if err != nil {
			return err
		}

		for _, pvc := range pvcs {
			pv, err := ops.k8sOps.GetVolumeForPersistentVolumeClaim(&pvc)
			if err != nil {
				return err
			}

			volsToInspect = append(volsToInspect, pv)
		}
	}

	logrus.Infof("Waiting for detachment of PX shared volumes: %s", volsToInspect)

	t := func() (interface{}, bool, error) {

		vols, err := volDriver.Inspect(volsToInspect)
		if err != nil {
			return nil, true, err
		}

		for _, v := range vols {
			if v.AttachedState == osd_api.AttachState_ATTACH_STATE_EXTERNAL {
				return nil, true, fmt.Errorf("volume: %s is still attached", v.Locator.Name)
			}
		}

		logrus.Infof("All shared volumes: %v detached", volsToInspect)
		return nil, false, nil
	}

	if _, err := task.DoRetryWithTimeout(t, sharedVolDetachTimeout, defaultRetryInterval); err != nil {
		return err
	}

	return nil
}

// getPXDriver returns an instance of the PX volume and cluster driver client
func getPXDriver(ip string) (volume.VolumeDriver, cluster.Cluster, error) {
	pxEndpoint := fmt.Sprintf("http://%s:%d", ip, pxdRestPort)

	dClient, err := osd_volclient.NewDriverClient(pxEndpoint, "pxd", "", "pxd")
	if err != nil {
		return nil, nil, err
	}

	cClient, err := osd_clusterclient.NewClusterClient(pxEndpoint, "v1")
	if err != nil {
		return nil, nil, err
	}

	volDriver := osd_volclient.VolumeDriver(dClient)
	clusterManager := osd_clusterclient.ClusterManager(cClient)
	return volDriver, clusterManager, nil
}

// getPXDaemonsets return PX daemonsets in the cluster
func (ops *pxClusterOps) getPXDaemonsets() ([]apps_api.DaemonSet, error) {
	pxDaemonsets := make([]apps_api.DaemonSet, 0)
	for _, ns := range ops.installedNamespaces {
		dss, err := ops.k8sOps.ListDaemonSets(ns, pxListOpts)
		if err != nil {
			return nil, err
		}

		if len(dss) > 0 {
			pxDaemonsets = append(pxDaemonsets, dss...)
		}
	}

	return pxDaemonsets, nil
}

// upgradePX upgrades PX daemonsets and waits till all replicas are ready
func (ops *pxClusterOps) upgradePX(spec apiv1beta1.ClusterSpec, opts *UpgradeOptions) error {
	var err error

	// update PX RBAC cluster role
	pxClusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: pxClusterRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "update", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "delete", "watch", "update"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumeclaims", "persistentvolumes"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"storage.k8s.io"},
				Resources: []string{"storageclasses"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get", "update", "list", "create"},
			},
			{
				APIGroups:     []string{"extensions"},
				Resources:     []string{"podsecuritypolicies"},
				ResourceNames: []string{"privileged"},
				Verbs:         []string{"use"},
			},
			{
				APIGroups: []string{"portworx.io"},
				Resources: []string{"volumeplacementstrategies"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"stork.libopenstorage.org"},
				Resources: []string{"backuplocations"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"core.libopenstorage.org"},
				Resources: []string{"*"},
				Verbs:     []string{"*"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create"},
			},
		},
	}
	_, err = ops.k8sOps.UpdateClusterRole(pxClusterRole)
	if err != nil {
		return err
	}

	// create or update Kubernetes secrets permissions
	if err := ops.updatePxSecretsPermissions(); err != nil {
		return err
	}

	var dss []apps_api.DaemonSet
	expectedGenerations := make(map[types.UID]int64)

	ociImage := fmt.Sprintf("%s:%s", spec.OCIMonImage, spec.OCIMonTag)
	newVersion := ociImage

	t := func() (interface{}, bool, error) {
		dss, err = ops.getPXDaemonsets()
		if err != nil {
			return nil, true, err
		}

		if len(dss) == 0 {
			return nil, true, errNoPXDaemonset
		}

		for _, ds := range dss {
			skip := false
			logrus.Infof("Upgrading PX daemonset: [%s] %s to version: %s", ds.Namespace, ds.Name, newVersion)
			dsCopy := ds.DeepCopy()
			for i := 0; i < len(dsCopy.Spec.Template.Spec.Containers); i++ {
				c := &dsCopy.Spec.Template.Spec.Containers[i]
				if c.Name == pxDaemonsetContainerName {
					oldPxImage := getEnv(c.Env, pxImageEnvKey)

					if len(oldPxImage) > 0 {
						return nil, false, fmt.Errorf("%s=%s env variable is set in the PX Daemonset. "+
							"This upgrade method doesn't support that. Remove the environment variable and retry. ",
							pxImageEnvKey, oldPxImage)
					}

					if c.Image == ociImage {
						logrus.Infof("Skipping upgrade of PX daemonset: [%s] %s as it is already at %s version.",
							ds.Namespace, ds.Name, ociImage)
						expectedGenerations[ds.UID] = ds.Status.ObservedGeneration
						skip = true
					} else {
						expectedGenerations[ds.UID] = ds.Status.ObservedGeneration + 1
						c.Image = ociImage
						dsCopy.Annotations[changeCauseAnnotation] = fmt.Sprintf("update PX to %s", newVersion)
					}
					break
				}
			}

			if skip {
				continue
			}

			updatedDS, err := ops.k8sOps.UpdateDaemonSet(dsCopy)
			if err != nil {
				return nil, true, err
			}

			logrus.Infof("Initiated upgrade of PX daemonset: [%s] %s to version: %s",
				updatedDS.Namespace, updatedDS.Name, newVersion)
		}

		return nil, false, nil
	}

	if _, err = task.DoRetryWithTimeout(t, daemonsetUpdateTriggerTimeout, defaultRetryInterval); err != nil {
		return err
	}

	for _, ds := range dss {
		logrus.Infof("Checking upgrade status of PX daemonset: [%s] %s to version: %s", ds.Namespace, ds.Name, newVersion)

		t = func() (interface{}, bool, error) {
			updatedDS, err := ops.k8sOps.GetDaemonSet(ds.Name, ds.Namespace)
			if err != nil {
				return nil, true, err
			}

			expectedGeneration, _ := expectedGenerations[ds.UID]
			if updatedDS.Status.ObservedGeneration != expectedGeneration {
				return nil, true, fmt.Errorf("PX daemonset: [%s] %s still running previous generation: %d. Expected generation: %d",
					ds.Namespace, ds.Name, updatedDS.Status.ObservedGeneration, expectedGeneration)
			}

			return nil, false, nil
		}

		if _, err = task.DoRetryWithTimeout(t, daemonsetUpdateTriggerTimeout, defaultRetryInterval); err != nil {
			return err
		}

		daemonsetReadyTimeout, err := ops.getDaemonSetReadyTimeout(opts.TimeoutPerNode)
		if err != nil {
			return err
		}

		logrus.Infof("Doing additional validations of PX daemonset: [%s] %s. timeout: %v", ds.Namespace, ds.Name, daemonsetReadyTimeout)
		err = ops.k8sOps.ValidateDaemonSet(ds.Name, ds.Namespace, daemonsetReadyTimeout)
		if err != nil {
			return err
		}

		logrus.Infof("Successfully upgraded PX daemonset: [%s] %s to version: %s", ds.Namespace, ds.Name, newVersion)

		// default target ports for portworx-service
		targetPorts := map[int32]int32{
			9001: 9001,
			9019: 9019,
			9020: 9020,
			9021: 9021,
		}

		// Check portworx-service and override default target ports
		pxService, err := ops.k8sOps.GetService(pxServiceName, ds.Namespace)
		if err != nil {
			return err

		}

		for _, port := range pxService.Spec.Ports {
			targetPorts[port.Port] = port.TargetPort.IntVal
		}

		patch := []byte(fmt.Sprintf(`{"spec":
						   {"ports":[
							  {"name":"px-api","port":9001,"protocol":"TCP","targetPort":%d},
							  {"name":"px-kvdb","port":9019,"protocol":"TCP","targetPort":%d},
							  {"name":"px-sdk","port":9020,"protocol":"TCP","targetPort":%d},
							  {"name":"px-rest-gateway","port":9021,"protocol":"TCP","targetPort":%d}
							]
						}}`, targetPorts[9001], targetPorts[9019], targetPorts[9020], targetPorts[9021]))
		_, err = ops.k8sOps.PatchService(pxServiceName, ds.Namespace, patch)
		if err != nil {
			return err
		}

		logrus.Infof("Successfully patched PX service: [%s] %s", ds.Namespace, pxServiceName)

		// Check portowrx-api
		if err = ops.checkAPIDaemonset(ds.Namespace, ds.Spec.Template.Spec.Affinity); err != nil {
			return err
		}

		if err = ops.checkAPIService(ds.Namespace, targetPorts); err != nil {
			return err
		}
	}

	return nil
}

// setEnv set the envVar on a given key with a given value
func setEnv(envVars []corev1.EnvVar, key, value string) []corev1.EnvVar {
	for i := 0; i < len(envVars); i++ {
		if envVars[i].Name == key {
			envVars[i].Value = value
			return envVars
		}
	}
	return append(envVars, corev1.EnvVar{Name: key, Value: value})
}

// getEnv get the envVar on a given key with a given value
func getEnv(envVars []corev1.EnvVar, key string) string {
	for i := 0; i < len(envVars); i++ {
		if envVars[i].Name == key {
			return envVars[i].Value
		}
	}
	return ""
}

// updatePxSecretsPermissions will update the permissions needed by PX to access secrets
func (ops *pxClusterOps) updatePxSecretsPermissions() error {
	// create px namespace to store secrets
	logrus.Infof("Creating [%s] namespace", pxSecretsNamespace)
	_, err := ops.k8sOps.CreateNamespace(pxSecretsNamespace, nil)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	// update RBAC role
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pxRoleName,
			Namespace: pxSecretsNamespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list", "create", "update", "patch"},
			},
		},
	}

	logrus.Infof("Updating [%s] role in [%s] namespace", pxRoleName, pxSecretsNamespace)
	if err := ops.createOrUpdateRole(role); err != nil {
		return err
	}

	// update RBAC role binding
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pxRoleBindingName,
			Namespace: pxSecretsNamespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      pxServiceAccountName,
				Namespace: ops.installedNamespaces[0],
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     pxRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	logrus.Infof("Updating [%s] rolebinding in [%s] namespace", pxRoleBindingName, pxSecretsNamespace)
	return ops.createOrUpdateRoleBinding(roleBinding)
}

// createOrUpdateRole creates a given role or updates if it already exists
func (ops *pxClusterOps) createOrUpdateRole(role *rbacv1.Role) error {
	_, err := ops.k8sOps.CreateRole(role)
	if errors.IsAlreadyExists(err) {
		if _, err = ops.k8sOps.UpdateRole(role); err != nil {
			return err
		}
	}
	return err
}

// createOrUpdateRoleBinding creates a given role binding or updates if it already exists
func (ops *pxClusterOps) createOrUpdateRoleBinding(binding *rbacv1.RoleBinding) error {
	_, err := ops.k8sOps.CreateRoleBinding(binding)
	if errors.IsAlreadyExists(err) {
		if _, err = ops.k8sOps.UpdateRoleBinding(binding); err != nil {
			return err
		}
	}
	return err
}

// isAnyNodeRunningVersionWithPrefix checks if any node in the cluster has a PX version with given prefix
func (ops *pxClusterOps) isAnyNodeRunningVersionWithPrefix(versionPrefix string, clusterManager cluster.Cluster) (bool, error) {
	cluster, err := clusterManager.Enumerate()
	if err != nil {
		return false, err
	}

	for _, n := range cluster.Nodes {
		ver := cluster.Nodes[0].NodeLabels[pxVersionLabel]
		if len(ver) == 0 {
			return false, fmt.Errorf("no version found in labels for node: %s", cluster.Nodes[0].Id)
		}

		if strings.HasPrefix(ver, versionPrefix) {
			logrus.Infof("Node: %s has version: %s with prefix: %s", n.Id, ver, versionPrefix)
			return true, nil
		}
	}

	return false, nil
}

// isScaleDownOfSharedAppsRequired decides if scaling down of apps using PX is required
func (ops *pxClusterOps) isScaleDownOfSharedAppsRequired(isMajorVerUpgrade bool, opts *UpgradeOptions) bool {
	if opts == nil || len(opts.SharedAppsScaleDown) == 0 || opts.SharedAppsScaleDown == SharedAppsScaleDownAuto {
		return isMajorVerUpgrade
	}

	return opts.SharedAppsScaleDown == SharedAppsScaleDownOn
}

// isUpgradeAppDrainRequired checks if target is 1.3.3/1.3.4 or upgrade is from 1.2 to 1.3/1.4
func (ops *pxClusterOps) isUpgradeAppDrainRequired(spec *apiv1beta1.Cluster, clusterManager cluster.Cluster) (bool, error) {
	currentVersionDublin, err := ops.isAnyNodeRunningVersionWithPrefix("1.2", clusterManager)
	if err != nil {
		return false, err
	}

	newVersion, err := parsePXVersion(spec.Spec.PXTag)
	if err != nil {
		return false, err
	}

	logrus.Infof("Is any node running dublin version: %v. new version: %s", currentVersionDublin, newVersion)

	if strings.HasPrefix(newVersion, "1.3.3") || strings.HasPrefix(newVersion, "1.3.4") {
		// 1.3.3/1.3.4 has a change that requires a reboot even if starting version is not dublin
		return true, nil
	}

	return currentVersionDublin && (strings.HasPrefix(newVersion, "1.3") || strings.HasPrefix(newVersion, "1.4")), nil
}

func (ops *pxClusterOps) preFlightChecks(spec *apiv1beta1.Cluster) error {
	if len(spec.Spec.OCIMonTag) == 0 {
		return fmt.Errorf("pre-flight check failed as new version of Portworx OCI monitor not given to upgrade API")
	}

	// check if PX is not ready in any of the nodes
	dss, err := ops.getPXDaemonsets()
	if err != nil {
		return err
	}

	if len(dss) == 0 {
		return errNoPXDaemonset
	}

	for _, d := range dss {
		if err = ops.k8sOps.ValidateDaemonSet(d.Name, d.Namespace, 1*time.Minute); err != nil {
			return fmt.Errorf("pre-flight check failed as existing Portworx DaemonSet is not ready. err: %v", err)
		}
	}

	return nil
}

func (ops *pxClusterOps) getDaemonSetReadyTimeout(timeoutPerNode int) (time.Duration, error) {
	nodes, err := ops.k8sOps.GetNodes()
	if err != nil {
		return 0, err
	}

	daemonsetReadyTimeout := time.Duration(len(nodes.Items)-1) * time.Duration(timeoutPerNode) * time.Second
	return daemonsetReadyTimeout, nil
}

func (ops *pxClusterOps) wipePXKvdb(endpoints []string, opts map[string]string, clusterName string) error {
	if len(endpoints) == 0 {
		return fmt.Errorf("endpoints not supplied for wiping KVDB")
	}

	if len(clusterName) == 0 {
		return fmt.Errorf("PX cluster name not supplied for wiping KVDB")
	}

	logrus.Infof("Creating kvdb client for: %v", endpoints)
	kvdbInst, err := getKVDBClient(endpoints, opts)
	if err != nil {
		return err
	}

	logrus.Infof("Deleting Portworx kvdb tree: %s/%s for endpoint(s): %v", pxKvdbPrefix, clusterName, endpoints)
	return kvdbInst.DeleteTree(clusterName)
}

func (ops *pxClusterOps) parseConfigFromDaemonset() (
	[]string, map[string]string, string, platformType, *v1.Affinity, error) {
	dss, err := ops.getPXDaemonsets()
	if err != nil {
		return nil, nil, "", platformTypeDefault, nil, err
	}

	if len(dss) == 0 {
		return nil, nil, "", platformTypeDefault, nil, errNoPXDaemonset
	}

	platform := platformTypeDefault
	ds := dss[0]
	var clusterName string
	var usingInternalEtcd bool
	endpoints := make([]string, 0)
	opts := make(map[string]string)

	for _, volume := range ds.Spec.Template.Spec.Volumes {
		if volume.HostPath != nil && volume.Name == dsOptPwxVolumeName {
			if strings.HasPrefix(volume.HostPath.Path, pksPersistentStoreRoot) {
				logrus.Infof("Detected %s install.", platformTypePKS)
				platform = platformTypePKS
				break
			}
		}
	}

	affinity := ds.Spec.Template.Spec.Affinity
	for _, c := range ds.Spec.Template.Spec.Containers {
		if c.Name == pxDaemonsetContainerName {
			for i, arg := range c.Args {
				// Reference : https://docs.portworx.com/scheduler/kubernetes/px-k8s-spec-curl.html
				switch arg {
				case "-b":
					usingInternalEtcd = true
				case "-c":
					clusterName = c.Args[i+1]
				case "-k":
					endpoints, err = splitCSV(c.Args[i+1])
					if err != nil {
						return nil, nil, clusterName, platform, affinity, err
					}
				case "-pwd":
					parts := strings.Split(c.Args[i+1], ":")
					if len(parts) != 0 {
						return nil, nil, clusterName, platform, affinity, fmt.Errorf("failed to parse kvdb username and password: %s", c.Args[i+1])
					}
					opts[kvdb.UsernameKey] = parts[0]
					opts[kvdb.PasswordKey] = parts[1]
				case "-ca":
					opts[kvdb.CAFileKey] = c.Args[i+1]
				case "-cert":
					opts[kvdb.CertFileKey] = c.Args[i+1]
				case "-key":
					opts[kvdb.CertKeyFileKey] = c.Args[i+1]
				case "-acl":
					opts[kvdb.ACLTokenKey] = c.Args[i+1]
				default:
					// pass
				}
			}

			break
		}
	}

	if usingInternalEtcd {
		return endpoints, opts, clusterName, platform, affinity, errUsingInternalEtcd
	}

	if len(endpoints) == 0 {
		return nil, opts, clusterName, platform, affinity, fmt.Errorf("failed to get kvdb endpoint from daemonset containers: %v",
			ds.Spec.Template.Spec.Containers)
	}

	if len(clusterName) == 0 {
		return endpoints, opts, "", platform, affinity, fmt.Errorf("failed to get cluster name from daemonset containers: %v",
			ds.Spec.Template.Spec.Containers)
	}

	return endpoints, opts, clusterName, platform, affinity, nil
}

func (ops *pxClusterOps) deleteAllPXComponents(clusterName string) error {
	strippedClusterName := strings.ToLower(configMapNameRegex.ReplaceAllString(clusterName, ""))

	dss, err := ops.getPXDaemonsets()
	if err != nil {
		return err
	}

	logrus.Infof("Deleting all PX Kubernetes components from the cluster in namespaces: %s",
		ops.installedNamespaces)

	for _, ds := range dss {
		err = ops.k8sOps.DeleteDaemonSet(ds.Name, ds.Namespace)
		if err != nil {
			return err
		}
	}

	clusterRoles := []string{
		pxClusterRoleName,
		storkControllerClusterRole,
		storkSchedulerClusterRole,
		pvcControllerClusterRole,
		csiClusterRole,
	}
	for _, role := range clusterRoles {
		err = ops.k8sOps.DeleteClusterRole(role)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	clusterRoleBindings := []string{
		pxClusterRoleBindingName,
		storkControllerClusterBinding,
		storkSchedulerCluserBinding,
		pvcControllerClusterRoleBinding,
		csiClusterRoleBinding,
	}
	for _, binding := range clusterRoleBindings {
		err = ops.k8sOps.DeleteClusterRoleBinding(binding)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	clusterWideSecret := strings.Replace(clusterName+"_px_secret", "_", "-", -1)
	err = ops.k8sOps.DeleteSecret(clusterWideSecret, pxSecretsNamespace)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	err = ops.k8sOps.DeleteStorageClass(storkSnapshotStorageClass)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	err = ops.k8sOps.DeleteNamespace(pxSecretsNamespace)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	configMaps := []string{
		fmt.Sprintf("%s%s", internalEtcdConfigMapPrefix, strippedClusterName),
		fmt.Sprintf("%s%s", cloudDriveConfigMapPrefix, strippedClusterName),
	}
	for _, cm := range configMaps {
		err = ops.k8sOps.DeleteConfigMap(cm, bootstrapCloudDriveNamespace)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	// Delete other components from all installedNamespaces
	for _, ns := range ops.installedNamespaces {

		// Delete portworx-api daemonset
		err = ops.k8sOps.DeleteDaemonSet(pxAPIDaemonset, ns)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		depNames := []string{
			storkControllerName,
			storkSchedulerName,
			pvcControllerName,
			lhDeploymentName,
			csiExtDeploymentName,
		}
		for _, depName := range depNames {
			err = ops.k8sOps.DeleteDeployment(depName, ns)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		roles := [][]string{
			{pxRoleName, pxSecretsNamespace},
			{lhRoleName, ns},
		}
		for _, role := range roles {
			err = ops.k8sOps.DeleteRole(role[0], role[1])
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		roleBindings := [2][2]string{
			{pxRoleBindingName, pxSecretsNamespace},
			{lhRoleBindingName, ns},
		}
		for _, binding := range roleBindings {
			err = ops.k8sOps.DeleteRoleBinding(binding[0], binding[1])
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		accounts := []string{
			pxServiceAccountName,
			lhServiceAccountName,
			storkControllerServiceAccount,
			storkSchedulerServiceAccount,
			pvcControllerServiceAccount,
			csiAccount,
		}
		for _, acc := range accounts {
			err = ops.k8sOps.DeleteServiceAccount(acc, ns)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		services := []string{
			pxServiceName,
			pxAPIServiceName,
			storkServiceName,
			lhServiceName,
			csiService,
		}
		for _, svc := range services {
			err = ops.k8sOps.DeleteService(svc, ns)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		statefulSets := []string{
			csiStatefulSet,
		}

		for _, ss := range statefulSets {
			err = ops.k8sOps.DeleteStatefulSet(ss, ns)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		secrets := []string{
			osbSecretName,
			essentialSecretName,
		}

		for _, sec := range secrets {
			err = ops.k8sOps.DeleteSecret(sec, ns)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		configMaps = []string{
			lhConfigMap,
			storkControllerConfigMap,
			// below 2 are currently only in the kube-system namespace. keeping them here
			// in case the behavior changes
			fmt.Sprintf("%s%s", internalEtcdConfigMapPrefix, strippedClusterName),
			fmt.Sprintf("%s%s", cloudDriveConfigMapPrefix, strippedClusterName),
		}
		for _, cm := range configMaps {
			err = ops.k8sOps.DeleteConfigMap(cm, ns)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func (ops *pxClusterOps) runPXNodeWiper(pwxHostPathRoot, wiperImage, wiperTag string, affinity *v1.Affinity) error {
	trueVar := true
	typeDirOrCreate := corev1.HostPathDirectoryOrCreate
	labels := map[string]string{
		"name": pxNodeWiperDaemonSetName,
	}

	if len(wiperImage) == 0 {
		wiperImage = defaultNodeWiperImage
	}

	if len(wiperTag) == 0 {
		wiperTag = defaultNodeWiperTag
	}

	if pwxHostPathRoot == pksPersistentStoreRoot {
		cores = "/cores"
	}

	args := []string{"-w", "-r"}
	ds := &apps_api.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pxNodeWiperDaemonSetName,
			Namespace: pxNodeWiperNamespace,
			Labels:    labels,
		},
		Spec: apps_api.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					HostPID:  true,
					Affinity: affinity,
					Containers: []corev1.Container{
						{
							Name:            pxNodeWiperDaemonSetName,
							Image:           fmt.Sprintf("%s:%s", wiperImage, wiperTag),
							ImagePullPolicy: corev1.PullAlways,
							Args:            args,
							SecurityContext: &corev1.SecurityContext{
								Privileged: &trueVar,
							},
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: 30,
								Handler: corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{"cat", "/tmp/px-node-wipe-done"},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      dsEtcPwxVolumeName,
									MountPath: pxEtcdPwx,
								},
								{
									Name:      "hostproc",
									MountPath: "/hostproc",
								},
								{
									Name:      dsOptPwxVolumeName,
									MountPath: pxOptPwx,
								},
								{
									Name:      dsDbusVolumeName,
									MountPath: dbusPath,
								},
								{
									Name:      dsSysdVolumeName,
									MountPath: sysdmount,
								},
								{
									Name:      devVolumeName,
									MountPath: devMount,
								},
								{
									Name:      lvmVolumeName,
									MountPath: lvmMount,
								},
								{
									Name:      multipathVolumeName,
									MountPath: multipathMount,
								},
								{
									Name:      udevVolumeName,
									MountPath: udevMount,
									ReadOnly:  true,
								},
								{
									Name:      sysVolumeName,
									MountPath: sysMount,
								},
								{
									Name:      coresVolumeName,
									MountPath: "/var/cores",
								},
							},
						},
					},
					RestartPolicy:      "Always",
					ServiceAccountName: talismanServiceAccount,
					Volumes: []corev1.Volume{
						{
							Name: dsEtcPwxVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: pwxHostPathRoot + pxEtcdPwx,
								},
							},
						},
						{
							Name: "hostproc",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/proc",
								},
							},
						},
						{
							Name: dsOptPwxVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: pwxHostPathRoot + pxOptPwx,
								},
							},
						},
						{
							Name: dsDbusVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: dbusPath,
								},
							},
						},
						{
							Name: dsSysdVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: sysdmount,
								},
							},
						},
						{
							Name: devVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: devMount,
								},
							},
						},
						{
							Name: multipathVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: multipathMount,
									Type: &typeDirOrCreate,
								},
							},
						},
						{
							Name: lvmVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: lvmMount,
									Type: &typeDirOrCreate,
								},
							},
						},
						{
							Name: udevVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: udevMount,
								},
							},
						},
						{
							Name: sysVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: sysMount,
								},
							},
						},
						{
							Name: coresVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: pwxHostPathRoot + cores,
								},
							},
						},
					},
				},
			},
		},
	}

	return ops.runDaemonSet(ds, pxNodeWiperTimeout)
}

// runDaemonSet runs the given daemonset, attempts to validate if it ran successfully and then
// deletes it
func (ops *pxClusterOps) runDaemonSet(ds *apps_api.DaemonSet, timeout time.Duration) error {
	err := ops.k8sOps.DeleteDaemonSet(ds.Name, ds.Namespace)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	t := func() (interface{}, bool, error) {
		_, err := ops.k8sOps.GetDaemonSet(ds.Name, ds.Namespace)
		if err == nil {
			return nil, true, fmt.Errorf("daemonset: [%s] %s is still present", ds.Namespace, ds.Name)
		}

		return nil, false, nil
	}

	if _, err := task.DoRetryWithTimeout(t, daemonsetDeleteTimeout, defaultRetryInterval); err != nil {
		return err
	}

	ds, err = ops.k8sOps.CreateDaemonSet(ds)
	if err != nil {
		return err
	}

	logrus.Infof("Started daemonSet: [%s] %s", ds.Namespace, ds.Name)

	// Delete the daemonset regardless of status
	defer func(ds *apps_api.DaemonSet) {
		err := ops.k8sOps.DeleteDaemonSet(ds.Name, ds.Namespace)
		if err != nil && !errors.IsNotFound(err) {
			logrus.Warnf("error while deleting daemonset: %v", err)
		}
	}(ds)

	err = ops.k8sOps.ValidateDaemonSet(ds.Name, ds.Namespace, timeout)
	if err != nil {
		return err
	}

	logrus.Infof("Validated successful run of daemonset: [%s] %s", ds.Namespace, ds.Name)

	return nil
}

func (ops *pxClusterOps) checkAPIDaemonset(namespace string, affinity *v1.Affinity) error {
	_, err := ops.k8sOps.GetDaemonSet(pxAPIDaemonset, namespace)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// create it
		apiDS := &apps_api.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pxAPIDaemonset,
				Namespace: namespace,
				Labels:    apiLabels,
			},
			Spec: apps_api.DaemonSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: apiLabels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: apiLabels,
					},
					Spec: corev1.PodSpec{
						Affinity:    affinity,
						HostNetwork: true,
						Containers: []corev1.Container{
							{
								Name:            pxAPIDaemonset,
								Image:           "k8s.gcr.io/pause:3.1",
								ImagePullPolicy: corev1.PullAlways,
								ReadinessProbe: &corev1.Probe{
									PeriodSeconds: 10,
									Handler: v1.Handler{
										HTTPGet: &corev1.HTTPGetAction{
											Host: "127.0.0.1",
											Path: "/status",
											Port: intstr.FromInt(9001),
										},
									},
								},
							},
						},
						RestartPolicy:      "Always",
						ServiceAccountName: pxServiceAccountName,
					},
				},
			},
		}

		_, err = ops.k8sOps.CreateDaemonSet(apiDS)
		if err != nil {
			return err
		}

		logrus.Infof("Created PX API daemonset: [%s] %s", namespace, pxAPIDaemonset)
	}

	return nil
}

func (ops *pxClusterOps) checkAPIService(namespace string, targetPorts map[int32]int32) error {
	_, err := ops.k8sOps.GetService(pxAPIServiceName, namespace)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		// create it
		apiService := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pxAPIServiceName,
				Namespace: namespace,
				Labels:    apiLabels,
			},
			Spec: v1.ServiceSpec{
				Selector: apiLabels,
				Type:     corev1.ServiceTypeClusterIP,
				Ports: []corev1.ServicePort{
					{
						Name:       "px-api",
						Protocol:   corev1.ProtocolTCP,
						Port:       int32(9001),
						TargetPort: intstr.FromInt(int(targetPorts[9001])),
					},
					{
						Name:       "px-sdk",
						Protocol:   corev1.ProtocolTCP,
						Port:       int32(9020),
						TargetPort: intstr.FromInt(int(targetPorts[9020])),
					},
					{
						Name:       "px-rest-gateway",
						Protocol:   corev1.ProtocolTCP,
						Port:       int32(9021),
						TargetPort: intstr.FromInt(int(targetPorts[9021])),
					},
				},
			},
		}

		_, err = ops.k8sOps.CreateService(apiService)
		if err != nil {
			return err
		}
		logrus.Infof("Successfully created PX API service: [%s] %s", namespace, pxAPIServiceName)
	} else {
		// patch it
		patch := []byte(fmt.Sprintf(`{"spec":
						   {"ports":[
							  {"name":"px-api","port":9001,"protocol":"TCP","targetPort":%d},
							  {"name":"px-sdk","port":9020,"protocol":"TCP","targetPort":%d},
							  {"name":"px-rest-gateway","port":9021,"protocol":"TCP","targetPort":%d}
							]
						}}`, targetPorts[9001], targetPorts[9020], targetPorts[9021]))
		_, err = ops.k8sOps.PatchService(pxAPIServiceName, namespace, patch)
		if err != nil {
			return err
		}

		logrus.Infof("Successfully patched PX API service: [%s] %s", namespace, pxAPIServiceName)
	}
	return nil

}

func splitCSV(in string) ([]string, error) {
	r := csv.NewReader(strings.NewReader(in))
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil || len(records) < 1 {
		return []string{}, err
	} else if len(records) > 1 {
		return []string{}, fmt.Errorf("Multiline CSV not supported")
	}
	return records[0], err
}

func getKVDBClient(endpoints []string, opts map[string]string) (kvdb.Kvdb, error) {
	var urlPrefix, kvdbType, kvdbName string
	for i, url := range endpoints {
		urlTokens := strings.Split(url, ":")
		if i == 0 {
			if urlTokens[0] == "etcd" {
				kvdbType = "etcd"
			} else if urlTokens[0] == "consul" {
				kvdbType = "consul"
			} else {
				return nil, fmt.Errorf("unknown discovery endpoint : %v in %v", urlTokens[0], endpoints)
			}
		}

		if urlTokens[1] == "http" {
			urlPrefix = "http"
			urlTokens[1] = ""
		} else if urlTokens[1] == "https" {
			urlPrefix = "https"
			urlTokens[1] = ""
		} else {
			urlPrefix = "http"
		}

		kvdbURL := ""
		for j, v := range urlTokens {
			if j == 0 {
				kvdbURL = urlPrefix
			} else {
				if v != "" {
					kvdbURL = kvdbURL + ":" + v
				}
			}
		}
		endpoints[i] = kvdbURL
	}

	var kvdbVersion string
	var err error
	for i, url := range endpoints {
		kvdbVersion, err = kvdb.Version(kvdbType+"-kv", url, opts)
		if err == nil {
			break
		} else if i == len(endpoints)-1 {
			return nil, err
		}
	}

	switch kvdbVersion {
	case kvdb.ConsulVersion1:
		kvdbName = consul.Name
	case kvdb.EtcdBaseVersion:
		kvdbName = e2.Name
	case kvdb.EtcdVersion3:
		kvdbName = e3.Name
	default:
		return nil, fmt.Errorf("Unknown kvdb endpoint (%v) and version (%v) ", endpoints, kvdbVersion)
	}

	return kvdb.New(kvdbName, pxKvdbPrefix, endpoints, opts, nil)
}

func parsePXVersion(version string) (string, error) {
	matches := pxVersionRegex.FindStringSubmatch(version)
	if len(matches) != 2 {
		return "", fmt.Errorf("failed to get PX version from %s", version)
	}

	return matches[1], nil
}

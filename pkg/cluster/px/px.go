package px

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	osd_api "github.com/libopenstorage/openstorage/api"
	osd_clusterclient "github.com/libopenstorage/openstorage/api/client/cluster"
	osd_volclient "github.com/libopenstorage/openstorage/api/client/volume"
	"github.com/libopenstorage/openstorage/cluster"
	"github.com/libopenstorage/openstorage/volume"
	"github.com/portworx/sched-ops/k8s"
	"github.com/portworx/sched-ops/task"
	apiv1alpha1 "github.com/portworx/talisman/pkg/apis/portworx.com/v1alpha1"
	"github.com/portworx/talisman/pkg/k8sutils"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1 "k8s.io/client-go/pkg/api/v1"
	extensions_api "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	rbacv1 "k8s.io/client-go/pkg/apis/rbac/v1beta1"
)

type pxInstallType string

const (
	pxInstallTypeOCI    pxInstallType = "oci"
	pxInstallTypeDocker pxInstallType = "docker"
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
	defaultPXImage         = "portworx/px-enterprise"
	pxDefaultNamespace     = "kube-system"
	dockerPullerImage      = "portworx/docker-puller:latest"
	pxdRestPort            = 9001
	pxServiceName          = "portworx-service"
	pxClusterRoleName      = "node-get-put-list-role"
	pxVersionLabel         = "PX Version"
	defaultRetryInterval   = 10 * time.Second
	talismanServiceAccount = "talisman-account"
)

type pxClusterOps struct {
	k8sOps               k8s.Ops
	utils                *k8sutils.Instance
	dockerRegistrySecret string
	clusterManager       cluster.Cluster
	volDriver            volume.VolumeDriver
}

// timeouts and intervals
const (
	sharedVolDetachTimeout        = 5 * time.Minute
	daemonsetUpdateTriggerTimeout = 5 * time.Minute
	dockerPullerDeleteTimeout     = 5 * time.Minute
)

// UpgradeOptions are optional to customize the upgrade process
type UpgradeOptions struct {
	SharedAppsScaleDown SharedAppsScaleDownMode
}

// Cluster an interface to manage a storage cluster
type Cluster interface {
	// Create creates the given cluster
	Create(c *apiv1alpha1.Cluster) error
	// Status returns the current status of the given cluster
	Status(c *apiv1alpha1.Cluster) (*apiv1alpha1.ClusterStatus, error)
	// Upgrade upgrades the given cluster
	Upgrade(c *apiv1alpha1.Cluster, opts *UpgradeOptions) error
	// Destory destroys all components of the given cluster
	Destroy(c *apiv1alpha1.Cluster) error
}

// NewPXClusterProvider creates a new PX cluster
func NewPXClusterProvider(dockerRegistrySecret, kubeconfig string) (Cluster, error) {
	utils, err := k8sutils.New(kubeconfig)
	if err != nil {
		return nil, err
	}

	k8sOps := k8s.Instance()

	svc, err := k8sOps.GetService(pxServiceName, pxDefaultNamespace)
	if err != nil {
		return nil, err
	}

	ip := svc.Spec.ClusterIP
	if len(ip) == 0 {
		return nil, fmt.Errorf("PX service doesn't have a clusterIP assigned")
	}

	volDriver, clusterManager, err := getPXDriver(ip)
	if err != nil {
		return nil, err
	}

	return &pxClusterOps{
		volDriver:            volDriver,
		clusterManager:       clusterManager,
		k8sOps:               k8s.Instance(),
		dockerRegistrySecret: dockerRegistrySecret,
		utils:                utils,
	}, nil
}

func (ops *pxClusterOps) Create(spec *apiv1alpha1.Cluster) error {
	logrus.Warnf("creating px cluster is not yet implemented")
	return nil
}

func (ops *pxClusterOps) Status(c *apiv1alpha1.Cluster) (*apiv1alpha1.ClusterStatus, error) {
	logrus.Warnf("cluster status is not yet implemented")
	return nil, nil
}

func (ops *pxClusterOps) Upgrade(new *apiv1alpha1.Cluster, opts *UpgradeOptions) error {
	if new == nil {
		return fmt.Errorf("new cluster spec is required for the upgrade call")
	}

	if err := ops.preFlightChecks(new); err != nil {
		return err
	}

	if len(new.Spec.PXImage) == 0 {
		new.Spec.PXImage = defaultPXImage
	}

	if len(new.Spec.PXTag) == 0 {
		new.Spec.PXTag = new.Spec.OCIMonTag
	}

	newOCIMonVer := fmt.Sprintf("%s:%s", new.Spec.OCIMonImage, new.Spec.OCIMonTag)
	newPXVer := fmt.Sprintf("%s:%s", new.Spec.PXImage, new.Spec.PXTag)

	isAppDrainNeeded, err := ops.doesUpgradeNeedAppDrain(new)
	if err != nil {
		return err
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

	logrus.Infof("Upgrading px cluster to %s. Upgrade opts: %v App drain requirement: %v", newOCIMonVer, opts, isAppDrainNeeded)

	// 1. Start DaemonSet to download the new PX and OCI-mon image and validate it completes
	if err := ops.runDockerPuller(newOCIMonVer); err != nil {
		return err
	}

	if err := ops.runDockerPuller(newPXVer); err != nil {
		return err
	}

	// 2. (Optional) Scale down px shared applications to 0 replicas if required based on opts
	if ops.isScaleDownOfSharedAppsRequired(isAppDrainNeeded, opts) {
		defer func() { // always restore the replicas
			err = ops.utils.RestoreScaledAppsReplicas()
			if err != nil {
				logrus.Errorf("failed to restore PX shared applications replicas. Err: %v", err)
			}
		}()

		err = ops.utils.ScaleSharedAppsToZero()
		if err != nil {
			return err
		}

		// Wait till all px shared volumes are detached
		err = ops.waitTillPXSharedVolumesDetached()
		if err != nil {
			return err
		}
	} else {
		logrus.Infof("skipping scale down of shared volume applications")
	}

	// 3. Start rolling upgrade of PX DaemonSet
	err = ops.upgradePX(newOCIMonVer)
	if err != nil {
		return err
	}

	return nil
}

func (ops *pxClusterOps) Destroy(c *apiv1alpha1.Cluster) error {
	logrus.Warnf("destroying px cluster is implemented")
	return nil
}

// runDockerPuller runs the DaemonSet to start pulling the given image on all nodes
func (ops *pxClusterOps) runDockerPuller(imageToPull string) error {
	stripSpecialRegex, _ := regexp.Compile("[^a-zA-Z0-9]+")
	trueVar := true

	pullerName := fmt.Sprintf("docker-puller-%s", stripSpecialRegex.ReplaceAllString(imageToPull, ""))

	labels := map[string]string{
		"name": pullerName,
	}
	args := []string{"-i", imageToPull, "-w"}

	var env []corev1.EnvVar
	if len(ops.dockerRegistrySecret) != 0 {
		logrus.Infof("using user-provided docker registry credentials from secret: %s", ops.dockerRegistrySecret)
		env = append(env, corev1.EnvVar{
			Name: "REGISTRY_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ops.dockerRegistrySecret,
					},
					Key: "REGISTRY_USER",
				},
			},
		})

		env = append(env, corev1.EnvVar{
			Name: "REGISTRY_PASS",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ops.dockerRegistrySecret,
					},
					Key: "REGISTRY_PASS",
				},
			},
		})
	}

	ds := &extensions_api.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullerName,
			Namespace: pxDefaultNamespace,
			Labels:    labels,
		},
		Spec: extensions_api.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            pullerName,
							Image:           dockerPullerImage,
							ImagePullPolicy: corev1.PullAlways,
							Args:            args,
							Env:             env,
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: 60,
								Handler: corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{"cat", "/tmp/docker-pull-done"},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "varrun",
									MountPath: "/var/run/",
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: &trueVar,
							},
						},
					},
					RestartPolicy:      "Always",
					ServiceAccountName: talismanServiceAccount,
					Volumes: []corev1.Volume{
						{
							Name: "varrun",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/run/",
								},
							},
						},
					},
				},
			},
		},
	}

	// Cleanup any lingering DaemonSet for docker-pull if it exists
	err := ops.k8sOps.DeleteDaemonSet(pullerName, pxDefaultNamespace)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	t := func() (interface{}, bool, error) {
		_, err := ops.k8sOps.GetDaemonSet(pullerName, pxDefaultNamespace)
		if err == nil {
			return nil, true, fmt.Errorf("daemonset: [%s] %s is still present", pxDefaultNamespace, pullerName)
		}

		return nil, false, nil
	}

	if _, err := task.DoRetryWithTimeout(t, dockerPullerDeleteTimeout, defaultRetryInterval); err != nil {
		return err
	}

	ds, err = ops.k8sOps.CreateDaemonSet(ds)
	if err != nil {
		return err
	}

	logrus.Infof("started docker puller daemonSet: %s", ds.Name)

	daemonsetReadyTimeout, err := ops.getDaemonSetReadyTimeout()
	if err != nil {
		return err
	}

	err = ops.k8sOps.ValidateDaemonSet(ds.Name, ds.Namespace, daemonsetReadyTimeout)
	if err != nil {
		logrus.Errorf("failed to run daemonset to pull %s image", imageToPull)
		return err
	}

	logrus.Infof("validated successfull run of docker puller: %s", ds.Name)

	err = ops.k8sOps.DeleteDaemonSet(pullerName, pxDefaultNamespace)
	if err != nil {
		logrus.Warnf("error while deleting docker puller: %v", err)
		return err
	}

	return nil
}

// waitTillPXSharedVolumesDetached waits till all shared volumes are detached
func (ops *pxClusterOps) waitTillPXSharedVolumesDetached() error {
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

	logrus.Infof("waiting for detachment of PX shared volumes: %s", volsToInspect)

	t := func() (interface{}, bool, error) {

		vols, err := ops.volDriver.Inspect(volsToInspect)
		if err != nil {
			return nil, true, err
		}

		for _, v := range vols {
			if v.AttachedState == osd_api.AttachState_ATTACH_STATE_EXTERNAL {
				return nil, true, fmt.Errorf("volume: %s is still attached", v.Locator.Name)
			}
		}

		logrus.Infof("all shared volumes: %v detached", volsToInspect)
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

// getPXDaemonsets return PX daemonsets in the cluster based on given installer type
func (ops *pxClusterOps) getPXDaemonsets(installType pxInstallType) ([]extensions_api.DaemonSet, error) {
	listOpts := metav1.ListOptions{
		LabelSelector: "name=portworx",
	}

	dss, err := ops.k8sOps.ListDaemonSets(pxDefaultNamespace, listOpts)
	if err != nil {
		return nil, err
	}

	var ociList, dockerList []extensions_api.DaemonSet
	for _, ds := range dss {
		for _, c := range ds.Spec.Template.Spec.Containers {
			if c.Name == "portworx" {
				if matched, _ := regexp.MatchString(".+oci-monitor.+", c.Image); matched {
					ociList = append(ociList, ds)
				} else {
					dockerList = append(dockerList, ds)
				}
				break
			}
		}
	}

	if installType == pxInstallTypeDocker {
		return dockerList, nil
	}

	return ociList, nil
}

// upgradePX upgrades PX daemonsets and waits till all replicas are ready
func (ops *pxClusterOps) upgradePX(newVersion string) error {
	var err error

	// update RBAC cluster role
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: pxClusterRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "update", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumeclaims", "persistentvolumes"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	role, err = ops.k8sOps.UpdateClusterRole(role)
	if err != nil {
		return err
	}

	var dss []extensions_api.DaemonSet
	expectedGenerations := make(map[types.UID]int64)

	t := func() (interface{}, bool, error) {
		dss, err = ops.getPXDaemonsets(pxInstallTypeOCI)
		if err != nil {
			return nil, true, err
		}

		if len(dss) == 0 {
			return nil, true, fmt.Errorf("did not find any PX daemonsets for install type: %s", pxInstallTypeOCI)
		}

		for _, ds := range dss {
			skip := false
			logrus.Infof("upgrading PX daemonset: [%s] %s to version: %s", ds.Namespace, ds.Name, newVersion)
			for i := 0; i < len(ds.Spec.Template.Spec.Containers); i++ {
				c := &ds.Spec.Template.Spec.Containers[i]
				if c.Name == "portworx" {
					if c.Image == newVersion {
						logrus.Infof("skipping upgrade of PX daemonset: [%s] %s as it is already at %s version.",
							ds.Namespace, ds.Name, newVersion)
						expectedGenerations[ds.UID] = ds.Status.ObservedGeneration
						skip = true
					} else {
						expectedGenerations[ds.UID] = ds.Status.ObservedGeneration + 1
						c.Image = newVersion
					}
					break
				}
			}

			if skip {
				continue
			}

			updatedDS, err := ops.k8sOps.UpdateDaemonSet(&ds)
			if err != nil {
				return nil, true, err
			}

			logrus.Infof("initiated upgrade of PX daemonset: [%s] %s to version: %s",
				updatedDS.Namespace, updatedDS.Name, newVersion)
		}

		return nil, false, nil
	}

	if _, err = task.DoRetryWithTimeout(t, daemonsetUpdateTriggerTimeout, defaultRetryInterval); err != nil {
		return err
	}

	for _, ds := range dss {
		logrus.Infof("checking upgrade status of PX daemonset: [%s] %s to version: %s", ds.Namespace, ds.Name, newVersion)

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

		daemonsetReadyTimeout, err := ops.getDaemonSetReadyTimeout()
		if err != nil {
			return err
		}

		logrus.Infof("doing additional validations of PX daemonset: [%s] %s. timeout: %v", ds.Namespace, ds.Name, daemonsetReadyTimeout)
		err = ops.k8sOps.ValidateDaemonSet(ds.Name, ds.Namespace, daemonsetReadyTimeout)
		if err != nil {
			return err
		}

		logrus.Infof("successfully upgraded PX daemonset: [%s] %s to version: %s", ds.Namespace, ds.Name, newVersion)
	}

	return nil
}

// isAnyNodeRunningVersionWithPrefix checks if any node in the cluster has a PX version with given prefix
func (ops *pxClusterOps) isAnyNodeRunningVersionWithPrefix(versionPrefix string) (bool, error) {
	cluster, err := ops.clusterManager.Enumerate()
	if err != nil {
		return false, err
	}

	for _, n := range cluster.Nodes {
		ver := cluster.Nodes[0].NodeLabels[pxVersionLabel]
		if len(ver) == 0 {
			return false, fmt.Errorf("no version found in labels for node: %s", cluster.Nodes[0].Id)
		}

		if strings.HasPrefix(ver, versionPrefix) {
			logrus.Infof("node: %s has version: %s with prefix: %s", n.Id, ver, versionPrefix)
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

// doesUpgradeNeedAppDrain checks if target is 1.3.3 or upgrade is from 1.2 to 1.3/1.4
func (ops *pxClusterOps) doesUpgradeNeedAppDrain(spec *apiv1alpha1.Cluster) (bool, error) {
	currentVersionDublin, err := ops.isAnyNodeRunningVersionWithPrefix("1.2")
	if err != nil {
		return false, err
	}

	newVersion, err := parsePXVersion(spec.Spec.OCIMonTag)
	if err != nil {
		return false, err
	}

	logrus.Infof("Is any node running dublin version: %v. new version: %s", currentVersionDublin, newVersion)

	if strings.HasPrefix(newVersion, "1.3.3") {
		// 1.3.3 has a change that requires a reboot even if starting version is dublin
		return true, nil
	}

	return currentVersionDublin && (strings.HasPrefix(newVersion, "1.3") || strings.HasPrefix(newVersion, "1.4")), nil
}

func (ops *pxClusterOps) preFlightChecks(spec *apiv1alpha1.Cluster) error {
	dss, err := ops.getPXDaemonsets(pxInstallTypeDocker)
	if err != nil {
		return err
	}

	if len(dss) > 0 {
		return fmt.Errorf("pre-flight check failed as found: %d PX DaemonSet(s) of type docker. Only upgrading from OCI is supported", len(dss))
	}

	if len(spec.Spec.OCIMonTag) == 0 {
		return fmt.Errorf("pre-flight check failed as new version of Portworx OCI monitor not given to upgrade API")
	}

	// check if PX is not ready in any of the nodes
	dss, err = ops.getPXDaemonsets(pxInstallTypeOCI)
	if err != nil {
		return err
	}

	if len(dss) == 0 {
		return fmt.Errorf("pre-flight check failed as it did not find any PX daemonsets for install type: %s", pxInstallTypeOCI)
	}

	for _, d := range dss {
		if err = ops.k8sOps.ValidateDaemonSet(d.Name, d.Namespace, 1*time.Minute); err != nil {
			return fmt.Errorf("pre-flight check failed as existing Portworx DaemonSet is not ready. err: %v", err)
		}
	}

	return nil
}

func (ops *pxClusterOps) getDaemonSetReadyTimeout() (time.Duration, error) {
	nodes, err := ops.k8sOps.GetNodes()
	if err != nil {
		return 0, err
	}

	daemonsetReadyTimeout := time.Duration(len(nodes.Items)-1) * 10 * time.Minute
	return daemonsetReadyTimeout, nil
}

func parsePXVersion(version string) (string, error) {
	matches := pxVersionRegex.FindStringSubmatch(version)
	if len(matches) != 2 {
		return "", fmt.Errorf("failed to get PX version from %s", version)
	}

	return matches[1], nil
}

func isNotFoundErr(err error) bool {
	matched, _ := regexp.MatchString(".+ not found", err.Error())
	return matched
}

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
	apiv1alpha1 "github.com/portworx/talisman/pkg/apis/portworx.com/v1alpha1"
	"github.com/portworx/talisman/pkg/k8sutils"
	"github.com/sirupsen/logrus"
	apps_api "k8s.io/api/apps/v1beta2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	pxContainerName        = "portworx"
	pxKvdbPrefix           = "pwx/"
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

// UpgradeOptions are options to customize the upgrade process
type UpgradeOptions struct {
	SharedAppsScaleDown SharedAppsScaleDownMode
}

// DeleteOptions are options to customize the delete process
type DeleteOptions struct {
	// WipeCluster instructs if Portworx cluster metadata needs to be wiped off
	WipeCluster bool
}

// Cluster an interface to manage a storage cluster
type Cluster interface {
	// Create creates the given cluster
	Create(c *apiv1alpha1.Cluster) error
	// Status returns the current status of the given cluster
	Status(c *apiv1alpha1.Cluster) (*apiv1alpha1.ClusterStatus, error)
	// Upgrade upgrades the given cluster
	Upgrade(c *apiv1alpha1.Cluster, opts *UpgradeOptions) error
	// Delete removes all components of the given cluster
	Delete(c *apiv1alpha1.Cluster, opts *DeleteOptions) error
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

	logrus.Infof("upgrading px cluster to %s. Upgrade opts: %v", newOCIMonVer, opts)

	// 1. Start DaemonSet to download the new PX and OCI-mon image and validate it completes
	if err := ops.runDockerPuller(newOCIMonVer); err != nil {
		return err
	}

	if err := ops.runDockerPuller(newPXVer); err != nil {
		return err
	}

	// 2. (Optional) Scale down px shared applications to 0 replicas if required based on opts
	scaleDownSharedRequired, err := ops.isScaleDownOfSharedAppsRequired(new, opts)
	if err != nil {
		return err
	}

	if scaleDownSharedRequired {
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

func (ops *pxClusterOps) Delete(c *apiv1alpha1.Cluster, opts *DeleteOptions) error {
	if opts != nil && opts.WipeCluster {
		// the wipe operation is best effort as it is used when cluster is already in a bad shape. for e.g
		// cluster might have been started with incorrect kvdb information. So we won't be able to wipe that off

		// 1. Cleanup PX kvdb tree
		err := ops.wipePXKvdb()
		if err != nil {
			logrus.Warnf("failed to wipe Portworx KVDB tree. err: %v", err)
		}

		// 2. TODO remove PX systemd service on each node
		// 3. TODO peform local node wipes
	}

	// 4. TODO Query all PX k8s components in cluster and delete them

	return nil
}

// runDockerPuller runs the DaemonSet to start pulling the given image on all nodes
func (ops *pxClusterOps) runDockerPuller(imageToPull string) error {
	stripSpecialRegex, _ := regexp.Compile("[^a-zA-Z0-9]+")

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

	ds := &apps_api.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullerName,
			Namespace: pxDefaultNamespace,
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
func (ops *pxClusterOps) getPXDaemonsets(installType pxInstallType) ([]apps_api.DaemonSet, error) {
	listOpts := metav1.ListOptions{
		LabelSelector: "name=portworx",
	}

	dss, err := ops.k8sOps.ListDaemonSets(pxDefaultNamespace, listOpts)
	if err != nil {
		return nil, err
	}

	var ociList, dockerList []apps_api.DaemonSet
	for _, ds := range dss {
		for _, c := range ds.Spec.Template.Spec.Containers {
			if c.Name == pxContainerName {
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
				Resources: []string{"persistentvolumeclaims"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	role, err = ops.k8sOps.UpdateClusterRole(role)
	if err != nil {
		return err
	}

	var dss []apps_api.DaemonSet
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
			dsCopy := ds.DeepCopy()
			for i := 0; i < len(dsCopy.Spec.Template.Spec.Containers); i++ {
				c := &dsCopy.Spec.Template.Spec.Containers[i]
				if c.Name == pxContainerName {
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

			updatedDS, err := ops.k8sOps.UpdateDaemonSet(dsCopy)
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

// isScaleDownOfSharedAppsRequired decides if scaling down of PX shared apps is required
func (ops *pxClusterOps) isScaleDownOfSharedAppsRequired(spec *apiv1alpha1.Cluster, opts *UpgradeOptions) (bool, error) {
	if opts == nil || len(opts.SharedAppsScaleDown) == 0 ||
		opts.SharedAppsScaleDown == SharedAppsScaleDownAuto {
		currentVersionDublin, err := ops.isAnyNodeRunningVersionWithPrefix("1.2")
		if err != nil {
			return false, err
		}

		newMajorMinor, err := parseMajorMinorVersion(spec.Spec.OCIMonTag)
		if err != nil {
			return false, err
		}

		logrus.Infof("is any node running dublin version: %v. new version (major.minor): %s",
			currentVersionDublin, newMajorMinor)

		return currentVersionDublin && newMajorMinor == "1.3", nil
	}

	return opts.SharedAppsScaleDown == SharedAppsScaleDownOn, nil
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

func (ops *pxClusterOps) wipePXKvdb() error {
	logrus.Info("attempting to parse kvdb info from Portworx daemonset")
	endpoints, opts, err := ops.parseKvdbFromDaemonset()
	if err != nil {
		return err
	}

	logrus.Infof("creating kvdb client for: %v", endpoints)
	kvdbInst, err := getKVDBClient(endpoints, opts)
	if err != nil {
		return err
	}

	logrus.Infof("deleting Portworx kvdb tree: %s for endpoint(s): %v", pxKvdbPrefix, endpoints)
	return kvdbInst.DeleteTree("")
}

func (ops *pxClusterOps) parseKvdbFromDaemonset() ([]string, map[string]string, error) {
	dss, err := ops.getPXDaemonsets(pxInstallTypeOCI)
	if err != nil {
		return nil, nil, err
	}

	if len(dss) == 0 {
		return nil, nil, fmt.Errorf("no Portworx daemonset found on the cluster")
	}

	ds := dss[0]
	endpoints := make([]string, 0)
	opts := make(map[string]string)

	for _, c := range ds.Spec.Template.Spec.Containers {
		if c.Name == pxContainerName {
			for i, arg := range c.Args {
				// Reference : https://docs.portworx.com/scheduler/kubernetes/px-k8s-spec-curl.html
				switch arg {
				case "-k":
					endpoints, err = splitCSV(c.Args[i+1])
					if err != nil {
						return nil, nil, err
					}
				case "-pwd":
					parts := strings.Split(c.Args[i+1], ":")
					if len(parts) != 0 {
						return nil, nil, fmt.Errorf("failed to parse kvdb username and password: %s", c.Args[i+1])
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

	if len(endpoints) == 0 {
		return nil, nil, fmt.Errorf("failed to get kvdb endpoint from daemonset containers: %v",
			ds.Spec.Template.Spec.Containers)
	}

	return endpoints, opts, nil
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

func parseMajorMinorVersion(version string) (string, error) {
	matches := regexp.MustCompile(`^(\d+\.\d+).*`).FindStringSubmatch(version)
	if len(matches) != 2 {
		return "", fmt.Errorf("failed to get PX major.minor version from %s", version)
	}

	return matches[1], nil
}

func isNotFoundErr(err error) bool {
	matched, _ := regexp.MatchString(".+ not found", err.Error())
	return matched
}

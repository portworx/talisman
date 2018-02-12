package px

import (
	"fmt"
	"os"
	"regexp"
	"time"

	osd_api "github.com/libopenstorage/openstorage/api"
	osd_volclient "github.com/libopenstorage/openstorage/api/client/volume"
	"github.com/libopenstorage/openstorage/volume"
	"github.com/portworx/sched-ops/k8s"
	"github.com/portworx/sched-ops/task"
	apiv1alpha1 "github.com/portworx/talisman/pkg/apis/portworx.com/v1alpha1"
	"github.com/sirupsen/logrus"
	apps_api "k8s.io/api/apps/v1beta2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type pxInstallType string

const (
	pxInstallTypeOCI    pxInstallType = "oci"
	pxInstallTypeDocker pxInstallType = "docker"
)

const (
	defaultPXImage     = "portworx/px-enterprise"
	pxDefaultNamespace = "kube-system"
	dockerPullerImage  = "portworx/docker-puller:latest"
	replicaMemoryKey   = "px/replicas-before-scale-down"
	pxdRestPort        = 9001
	pxServiceName      = "portworx-service"
	pxClusterRoleName  = "node-get-put-list-role"
)

type pxClusterOps struct {
	kubeClient           kubernetes.Interface
	k8sOps               k8s.Ops
	dockerRegistrySecret string
}

// timeouts and intervals
const (
	deploymentUpdateTimeout       = 5 * time.Minute
	statefulSetUpdateTimeout      = 10 * time.Minute
	defaultRetryInterval          = 10 * time.Second
	sharedVolDetachTimeout        = 5 * time.Minute
	daemonsetUpdateTriggerTimeout = 5 * time.Minute
	dockerPullerDeleteTimeout     = 5 * time.Minute
)

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

// NewPXClusterProvider creates a new PX cluster
func NewPXClusterProvider(dockerRegistrySecret, kubeconfig string) (Cluster, error) {
	var cfg *rest.Config
	var err error

	if len(kubeconfig) == 0 {
		kubeconfig = os.Getenv("KUBECONFIG")
	}

	if len(kubeconfig) > 0 {
		logrus.Debugf("using kubeconfig: %s to create k8s client", kubeconfig)
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		logrus.Debugf("will use in-cluster config to create k8s client", kubeconfig)
		cfg, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil, fmt.Errorf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &pxClusterOps{
		kubeClient:           kubeClient,
		k8sOps:               k8s.Instance(),
		dockerRegistrySecret: dockerRegistrySecret,
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

func (ops *pxClusterOps) Upgrade(new *apiv1alpha1.Cluster) error {
	dss, err := ops.getPXDaemonsets(pxInstallTypeDocker)
	if err != nil {
		return err
	}

	if len(dss) > 0 {
		return fmt.Errorf("found: %d PX DaemonSet(s) of type docker. Only upgrading from OCI is supported", len(dss))
	}

	if len(new.Spec.OCIMonTag) == 0 {
		return fmt.Errorf("new version of Portworx OCI monitor not given to upgrade API")
	}

	if len(new.Spec.PXImage) == 0 {
		new.Spec.PXImage = defaultPXImage
	}

	if len(new.Spec.PXTag) == 0 {
		new.Spec.PXTag = new.Spec.OCIMonTag
	}

	newOCIMonVer := fmt.Sprintf("%s:%s", new.Spec.OCIMonImage, new.Spec.OCIMonTag)
	newPXVer := fmt.Sprintf("%s:%s", new.Spec.PXImage, new.Spec.PXTag)

	logrus.Infof("upgrading px cluster to %s", newOCIMonVer)

	// 1. Start DaemonSet to download the new PX and OCI-mon image and validate it completes
	err = ops.runDockerPuller(newOCIMonVer)
	if err != nil {
		return err
	}

	err = ops.runDockerPuller(newPXVer)
	if err != nil {
		return err
	}

	// 2. Scale down px shared applications to 0 replicas
	defer func() { //    Before scaling down apps, add a defer function to always restore the replicas
		err = ops.restoreScaledAppsReplicas()
		if err != nil {
			logrus.Errorf("failed to restore PX shared applications replicas. Err: %v", err)
		}
	}()

	err = ops.scaleSharedAppsToZero()
	if err != nil {
		return err
	}

	// 3. Wait till all px shared volumes are detached
	err = ops.waitTillPXSharedVolumesDetached()
	if err != nil {
		return err
	}

	// 4. Start rolling upgrade of PX DaemonSet
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
					RestartPolicy: "Always",
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
	policy := metav1.DeletePropagationForeground
	err := ops.kubeClient.Apps().DaemonSets(pxDefaultNamespace).Delete(
		pullerName,
		&metav1.DeleteOptions{PropagationPolicy: &policy})
	if err != nil && !isNotFoundErr(err) {
		return err
	}

	t := func() (interface{}, bool, error) {
		_, err = ops.kubeClient.Apps().DaemonSets(pxDefaultNamespace).Get(pullerName, metav1.GetOptions{})
		if err == nil {
			return nil, true, fmt.Errorf("daemonset: [%s] %s is still present", pxDefaultNamespace, pullerName)
		}

		return nil, false, nil
	}

	if _, err := task.DoRetryWithTimeout(t, dockerPullerDeleteTimeout, defaultRetryInterval); err != nil {
		return err
	}

	ds, err = ops.kubeClient.Apps().DaemonSets(pxDefaultNamespace).Create(ds)
	if err != nil {
		return err
	}

	logrus.Infof("started docker puller daemonSet: %s", ds.Name)

	err = ops.k8sOps.ValidateDaemonSet(ds.Name, ds.Namespace)
	if err != nil {
		logrus.Errorf("failed to run daemonset to pull %s image", imageToPull)
		return err
	}

	logrus.Infof("validated successfull run of docker puller: %s", ds.Name)

	err = ops.kubeClient.Apps().DaemonSets(pxDefaultNamespace).Delete(
		pullerName,
		&metav1.DeleteOptions{PropagationPolicy: &policy})
	if err != nil {
		logrus.Warnf("error while deleting docker puller: %v", err)
		return err
	}

	return nil
}

func (ops *pxClusterOps) scaleSharedAppsToZero() error {
	sharedDeps, sharedSS, err := ops.getPXSharedApps()
	if err != nil {
		return err
	}

	logrus.Infof("found %d deployments and %d statefulsets to scale down", len(sharedDeps), len(sharedSS))

	var valZero int32
	for _, d := range sharedDeps {
		deploymentName := d.Name
		deploymentNamespace := d.Namespace
		logrus.Infof("scaling down deployment: [%s] %s", deploymentNamespace, deploymentName)

		t := func() (interface{}, bool, error) {
			dCopy, err := ops.kubeClient.Apps().Deployments(deploymentNamespace).Get(deploymentName, metav1.GetOptions{})
			if err != nil {
				if isNotFoundErr(err) {
					return nil, false, nil // done as deployment is deleted
				}

				return nil, true, err
			}

			if *dCopy.Spec.Replicas == 0 {
				logrus.Infof("app [%s] %s is already scaled down to 0", dCopy.Namespace, dCopy.Name)
				return nil, false, nil
			}

			dCopy = dCopy.DeepCopy()

			// save current replica count in annotations so it can be used later on to restore the replicas
			dCopy.Annotations[replicaMemoryKey] = fmt.Sprintf("%d", *dCopy.Spec.Replicas)
			dCopy.Spec.Replicas = &valZero

			_, err = ops.kubeClient.Apps().Deployments(dCopy.Namespace).Update(dCopy)
			if err != nil {
				return nil, true, err
			}

			return nil, false, nil
		}

		if _, err := task.DoRetryWithTimeout(t, deploymentUpdateTimeout, defaultRetryInterval); err != nil {
			return err
		}
	}

	for _, s := range sharedSS {
		logrus.Infof("scaling down statefulset: [%s] %s", s.Namespace, s.Name)

		t := func() (interface{}, bool, error) {
			sCopy, err := ops.kubeClient.Apps().StatefulSets(s.Namespace).Get(s.Name, metav1.GetOptions{})
			if err != nil {
				if isNotFoundErr(err) {
					return nil, false, nil // done as statefulset is deleted
				}

				return nil, true, err
			}

			sCopy = sCopy.DeepCopy()
			// save current replica count in annotations so it can be used later on to restore the replicas
			sCopy.Annotations[replicaMemoryKey] = fmt.Sprintf("%d", *sCopy.Spec.Replicas)
			sCopy.Spec.Replicas = &valZero

			_, err = ops.kubeClient.Apps().StatefulSets(sCopy.Namespace).Update(sCopy)
			if err != nil {
				return nil, true, err
			}

			return nil, false, nil
		}

		if _, err := task.DoRetryWithTimeout(t, statefulSetUpdateTimeout, defaultRetryInterval); err != nil {
			return err
		}
	}

	return nil
}

func (ops *pxClusterOps) restoreScaledAppsReplicas() error {
	sharedDeps, sharedSS, err := ops.getPXSharedApps()
	if err != nil {
		return err
	}

	logrus.Infof("found %d deployments and %d statefulsets to restore", len(sharedDeps), len(sharedSS))

	for _, d := range sharedDeps {
		deploymentName := d.Name
		deploymentNamespace := d.Namespace
		logrus.Infof("restoring app: [%s] %s", d.Namespace, d.Name)
		t := func() (interface{}, bool, error) {
			dCopy, err := ops.kubeClient.Apps().Deployments(deploymentNamespace).Get(deploymentName, metav1.GetOptions{})
			if err != nil {
				if isNotFoundErr(err) {
					return nil, false, nil // done as deployment is deleted
				}

				return nil, true, err
			}

			dCopy = dCopy.DeepCopy()
			if dCopy.Annotations == nil {
				return nil, false, nil // done as this is not an app we touched
			}

			val, present := dCopy.Annotations[replicaMemoryKey]
			if !present || len(val) == 0 {
				logrus.Infof("not restoring app: [%s] %s as no annotation found to track replica count", deploymentNamespace, deploymentName)
				return nil, false, nil // done as this is not an app we touched
			}

			parsedVal := intstr.Parse(val)
			if parsedVal.Type != intstr.Int {
				return nil, false /*retry won't help */, fmt.Errorf("failed to parse saved replica count: %v", val)
			}

			delete(dCopy.Annotations, replicaMemoryKey)
			dCopy.Spec.Replicas = &parsedVal.IntVal

			_, err = ops.kubeClient.Apps().Deployments(dCopy.Namespace).Update(dCopy)
			if err != nil {
				return nil, true, err
			}

			return nil, false, nil
		}

		if _, err := task.DoRetryWithTimeout(t, deploymentUpdateTimeout, defaultRetryInterval); err != nil {
			return err
		}
	}

	for _, s := range sharedSS {
		logrus.Infof("restoring app: [%s] %s", s.Namespace, s.Name)

		t := func() (interface{}, bool, error) {
			sCopy, err := ops.kubeClient.Apps().StatefulSets(s.Namespace).Get(s.Name, metav1.GetOptions{})
			if err != nil {
				if isNotFoundErr(err) {
					return nil, false, nil // done as statefulset is deleted
				}

				return nil, true, err
			}

			sCopy = sCopy.DeepCopy()
			if sCopy.Annotations == nil {
				return nil, false, nil // done as this is not an app we touched
			}

			val, present := sCopy.Annotations[replicaMemoryKey]
			if !present || len(val) == 0 {
				return nil, false, nil // done as this is not an app we touched
			}

			parsedVal := intstr.Parse(val)
			if parsedVal.Type != intstr.Int {
				return nil, false, fmt.Errorf("failed to parse saved replica count: %v", val)
			}

			delete(sCopy.Annotations, replicaMemoryKey)
			sCopy.Spec.Replicas = &parsedVal.IntVal

			_, err = ops.kubeClient.Apps().StatefulSets(sCopy.Namespace).Update(sCopy)
			if err != nil {
				return nil, true, err
			}

			return nil, false, nil
		}

		if _, err := task.DoRetryWithTimeout(t, statefulSetUpdateTimeout, defaultRetryInterval); err != nil {
			return err
		}
	}

	return nil
}

// getPXSharedApps returns all deployments and statefulsets using Portworx storage class for shared volumes
func (ops *pxClusterOps) getPXSharedApps() ([]apps_api.Deployment, []apps_api.StatefulSet, error) {
	scs, err := ops.getPXSharedSCs()
	if err != nil {
		return nil, nil, err
	}

	var sharedDeps []apps_api.Deployment
	var sharedSS []apps_api.StatefulSet
	for _, sc := range scs {
		deps, err := ops.k8sOps.GetDeploymentsUsingStorageClass(sc)
		if err != nil {
			return nil, nil, err
		}

		for _, d := range deps {
			sharedDeps = append(sharedDeps, d)
		}

		ss, err := ops.k8sOps.GetStatefulSetsUsingStorageClass(sc)
		if err != nil {
			return nil, nil, err
		}

		for _, s := range ss {
			sharedSS = append(sharedSS, s)
		}
	}

	return sharedDeps, sharedSS, nil
}

// getPXSharedSCs returns all storage classes that have the shared parameter set to true
func (ops *pxClusterOps) getPXSharedSCs() ([]string, error) {
	scs, err := ops.kubeClient.Storage().StorageClasses().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var retList []string
	for _, sc := range scs.Items {
		params, err := ops.k8sOps.GetStorageClassParams(&sc)
		if err != nil {
			return nil, err
		}

		if isShared, present := params["shared"]; present && isShared == "true" {
			retList = append(retList, sc.Name)
		}

	}

	return retList, nil
}

// waitTillPXSharedVolumesDetached waits till all shared volumes are detached
func (ops *pxClusterOps) waitTillPXSharedVolumesDetached() error {
	scs, err := ops.getPXSharedSCs()
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

	pxd, err := ops.getPXDriver()
	if err != nil {
		return err
	}
	logrus.Infof("waiting for detachment of PX shared volumes: %s", volsToInspect)

	t := func() (interface{}, bool, error) {

		vols, err := pxd.Inspect(volsToInspect)
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

// getPXDriver returns an instance of the PX driver client
func (ops *pxClusterOps) getPXDriver() (volume.VolumeDriver, error) {
	svc, err := ops.k8sOps.GetService(pxServiceName, pxDefaultNamespace)
	if err != nil {
		return nil, err
	}

	ip := svc.Spec.ClusterIP
	if len(ip) == 0 {
		return nil, fmt.Errorf("PX service doesn't have a clusterIP assigned")
	}

	pxEndpoint := fmt.Sprintf("http://%s:%d", ip, pxdRestPort)

	dClient, err := osd_volclient.NewDriverClient(pxEndpoint, "pxd", "", "pxd")
	if err != nil {
		return nil, err
	}

	return osd_volclient.VolumeDriver(dClient), nil
}

// getPXDaemonsets return PX daemonsets in the cluster based on given installer type
func (ops *pxClusterOps) getPXDaemonsets(installType pxInstallType) ([]apps_api.DaemonSet, error) {
	listOpts := metav1.ListOptions{
		LabelSelector: "name=portworx",
	}

	dss, err := ops.kubeClient.Apps().DaemonSets(pxDefaultNamespace).List(listOpts)
	if err != nil {
		return nil, err
	}

	var ociList, dockerList []apps_api.DaemonSet
	for _, ds := range dss.Items {
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
				if c.Name == "portworx" {
					if c.Image == newVersion {
						logrus.Infof("skipping PX daemonset: [%s] %s as it is already at %s version.", ds.Namespace, ds.Name, newVersion)
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

			updatedDS, err := ops.kubeClient.Apps().DaemonSets(ds.Namespace).Update(dsCopy)
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
			updatedDS, err := ops.kubeClient.Apps().DaemonSets(ds.Namespace).Get(ds.Name, metav1.GetOptions{})
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

		logrus.Infof("doing additional validations of PX daemonset: [%s] %s", ds.Namespace, ds.Name)
		err = ops.k8sOps.ValidateDaemonSet(ds.Name, ds.Namespace)
		if err != nil {
			return err
		}

		logrus.Infof("successfully upgraded PX daemonset: [%s] %s to version: %s", ds.Namespace, ds.Name, newVersion)
	}

	return nil
}

func isNotFoundErr(err error) bool {
	matched, _ := regexp.MatchString(".+ not found", err.Error())
	return matched
}

package px

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/portworx/sched-ops/k8s"
	"github.com/portworx/sched-ops/task"
	apiv1alpha1 "github.com/portworx/talisman/pkg/apis/portworx.com/v1alpha1"
	"github.com/sirupsen/logrus"
	apps_api "k8s.io/api/apps/v1beta2"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	pxDefaultNamespace = "kube-system"
	dockerPullerImage  = "portworx/docker-puller:latest"
	replicaMemoryKey   = "px/replicas-before-scale-down"
)

type pxClusterOps struct {
	kubeClient           kubernetes.Interface
	k8sOps               k8s.Ops
	dockerRegistrySecret string
}

// timeouts and intervals
const (
	deploymentUpdateTimeout  = 5 * time.Minute
	statefulSetUpdateTimeout = 10 * time.Minute
	defaultRetryInterval     = 10 * time.Second
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
	if len(new.Spec.PXVersion) == 0 {
		return fmt.Errorf("new version of Portworx not given to upgrade API")
	}

	logrus.Infof("upgrading px cluster to %s", new.Spec.PXVersion)

	// 1.Start DaemonSet to download the new PX image and validate it completes
	err := ops.runDockerPuller(new.Spec.PXVersion)
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

	// 3. Wait till all px shared volumes are detached [ Implement in Status(...) ]
	// 4. Start rolling upgrade of PX DaemonSet
	// 5. Wait till all PX pods are in ready state

	return nil
}

func (ops *pxClusterOps) Destroy(c *apiv1alpha1.Cluster) error {
	logrus.Warnf("destroying px cluster is implemented")
	return nil
}

// runDockerPuller runs the DaemonSet to start pulling the given image on all nodes
func (ops *pxClusterOps) runDockerPuller(imageToPull string) error {
	pullerName := "docker-puller"

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

	ds := &extensionsv1beta1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullerName,
			Namespace: pxDefaultNamespace,
			Labels:    labels,
		},
		Spec: extensionsv1beta1.DaemonSetSpec{
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
	err := ops.kubeClient.Extensions().DaemonSets(pxDefaultNamespace).Delete(
		pullerName,
		&metav1.DeleteOptions{PropagationPolicy: &policy})
	if err != nil {
		if matched, _ := regexp.MatchString(".+ not found", err.Error()); !matched {
			return err
		}
	}

	ds, err = ops.kubeClient.Extensions().DaemonSets(pxDefaultNamespace).Create(ds)
	if err != nil {
		return err
	}

	logrus.Infof("started docker puller DaemonSet: %s...", ds.Name)

	err = ops.k8sOps.ValidateDaemonSet(ds.Name, ds.Namespace)
	if err != nil {
		logrus.Errorf("failed to run daemonset to pull %s image", imageToPull)
		return err
	}

	logrus.Infof("validated successfull run of docker puller ")

	err = ops.kubeClient.Extensions().DaemonSets(pxDefaultNamespace).Delete(
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
				if matched, _ := regexp.MatchString(".+ not found", err.Error()); matched {
					return nil, false, nil // done as deployment is deleted
				}

				return nil, true, err
			}

			dCopy = dCopy.DeepCopy()
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
				if matched, _ := regexp.MatchString(".+ not found", err.Error()); matched {
					return nil, false, nil // done as statefulset is deleted
				}

				return nil, true, err
			}

			sCopy = sCopy.DeepCopy()
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
				if matched, _ := regexp.MatchString(".+ not found", err.Error()); matched {
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
				if matched, _ := regexp.MatchString(".+ not found", err.Error()); matched {
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

// getPXSharedApps returns all deployments and statefulsets using Portworx storage class with shared: true
func (ops *pxClusterOps) getPXSharedApps() ([]apps_api.Deployment, []apps_api.StatefulSet, error) {
	scs, err := ops.kubeClient.Storage().StorageClasses().List(metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}

	var sharedDeps []apps_api.Deployment
	var sharedSS []apps_api.StatefulSet
	for _, sc := range scs.Items {

		params, err := ops.k8sOps.GetStorageClassParams(&sc)
		if err != nil {
			return nil, nil, err
		}

		if isShared, present := params["shared"]; present && isShared == "true" {
			deps, err := ops.k8sOps.GetDeploymentsUsingStorageClass(sc.Name)
			if err != nil {
				return nil, nil, err
			}

			for _, d := range deps {
				sharedDeps = append(sharedDeps, d)
			}

			ss, err := ops.k8sOps.GetStatefulSetsUsingStorageClass(sc.Name)
			if err != nil {
				return nil, nil, err
			}

			for _, s := range ss {
				sharedSS = append(sharedSS, s)
			}
		}
	}

	return sharedDeps, sharedSS, nil
}

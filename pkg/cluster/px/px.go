package px

import (
	"fmt"
	"os"
	"regexp"

	"github.com/portworx/sched-ops/k8s"
	apiv1alpha1 "github.com/portworx/talisman/pkg/apis/portworx.com/v1alpha1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	pxDefaultNamespace = "kube-system"
	dockerPullerImage  = "portworx/docker-puller:latest"
)

type pxClusterOps struct {
	kubeClient           kubernetes.Interface
	k8sOps               k8s.Ops
	dockerRegistrySecret string
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

// NewPXClusterProvider creates a new PX cluster
func NewPXClusterProvider(dockerRegistrySecret string) (Cluster, error) {
	var cfg *rest.Config
	var err error

	kubeconfig := os.Getenv("KUBECONFIG")
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

	kubeClient := kubernetes.NewForConfigOrDie(cfg)
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

	// 1.  Start DaemonSet to download the new PX image and validate it completes
	err := ops.runDockerPuller(new.Spec.PXVersion)
	if err != nil {
		return err
	}

	// 2. Find all pods that use PX shared volumes
	// 3. Find controllers for these pods and drain them
	//	For each controller in controllers
	//		Add anti affinity so pods don't run
	// 4. Wait till all px shared volumes are detached [ Implement in Status(...) ]
	//		If timed out waiting, undo 3
	// 5. Start rolling upgrade of PX DaemonSet
	// 6. Wait till all PX pods are in ready state
	// 7. [always] Undo 3

	// DaemonSet
	/*ds, err = ops.kubeClient.Extensions().DaemonSets(ds.Namespace).Update(ds)
	if err != nil {
		logrus.Errorf("failed updating daemonset: %s. Err: %s", ds.Name, err)
		return err
	}*/
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

package px

import (
	"fmt"
	"testing"

	"github.com/portworx/sched-ops/k8s/admissionregistration"
	"github.com/portworx/sched-ops/k8s/apps"
	"github.com/portworx/sched-ops/k8s/core"
	"github.com/portworx/sched-ops/k8s/rbac"
	"github.com/portworx/sched-ops/k8s/storage"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes "k8s.io/client-go/kubernetes/fake"
)

var (
	testsOps        *pxClusterOps
	testNamespace   = "portworx"
	testClusterName = "pxut"
)

/**
At the moment, the UT only covers deletion of new config maps added as part of PWX-24266
*/
func TestDeleteAllPXComponents(t *testing.T) {
	setup()

	// setup fake cluster for delete
	pxDS := &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "portworx",
			Namespace: testNamespace,
			Labels: map[string]string{
				"name": "portworx",
			},
		},
		Spec:   appsv1.DaemonSetSpec{},
		Status: appsv1.DaemonSetStatus{},
	}
	_, err := apps.Instance().CreateDaemonSet(pxDS, metav1.CreateOptions{})
	require.NoError(t, err)

	configMapsToCreate := []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s%s", internalEtcdConfigMapPrefix, testClusterName),
				Namespace: bootstrapCloudDriveNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s%s", cloudDriveConfigMapPrefix, testClusterName),
				Namespace: bootstrapCloudDriveNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pxAttachDrivesetConfigMap,
				Namespace: bootstrapCloudDriveNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%sus-west-2a", pxBringupQueueConfigMapPrefix),
				Namespace: bootstrapCloudDriveNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%sus-west-2b", pxBringupQueueConfigMapPrefix),
				Namespace: bootstrapCloudDriveNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%sus-west-2c", pxBringupQueueConfigMapPrefix),
				Namespace: bootstrapCloudDriveNamespace,
			},
		},
	}

	for _, cm := range configMapsToCreate {
		_, err = core.Instance().CreateConfigMap(cm)
		require.NoError(t, err)
	}

	// Create stork webhook
	webhookConfig := admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("stork-webhooks-cfg"),
		},
	}
	_, err = admissionregistration.Instance().CreateMutatingWebhookConfiguration(&webhookConfig)
	require.NoError(t, err)
	wh, err := admissionregistration.Instance().GetMutatingWebhookConfiguration("stork-webhooks-cfg")
	require.NotNil(t, wh)

	// Test delete
	err = testsOps.deleteAllPXComponents(testClusterName)
	require.NoError(t, err)

	// Check if config maps got deleted
	for _, cm := range configMapsToCreate {
		_, err = core.Instance().GetConfigMap(cm.Name, cm.Namespace)
		// should not be found
		require.Truef(t, errors.IsNotFound(err), "config map: [%s] %s wasn't deleted", cm.Namespace, cm.Name)
	}

	// Webhook should be deleted
	wh, err = admissionregistration.Instance().GetMutatingWebhookConfiguration("stork-webhooks-cfg")
	require.Nil(t, wh)

	// TEST2 Test delete when config maps don't exist
	_, err = apps.Instance().CreateDaemonSet(pxDS, metav1.CreateOptions{})
	require.NoError(t, err)

	err = testsOps.deleteAllPXComponents(testClusterName)
	require.NoError(t, err)
}

func setup() {
	logrus.SetLevel(logrus.TraceLevel)
	fakeKubeClient := kubernetes.NewSimpleClientset()

	core.SetInstance(core.New(fakeKubeClient))
	apps.SetInstance(apps.New(fakeKubeClient.AppsV1(), fakeKubeClient.CoreV1()))
	rbac.SetInstance(rbac.New(fakeKubeClient.RbacV1()))
	storage.SetInstance(storage.New(fakeKubeClient.StorageV1()))
	admissionregistration.SetInstance(admissionregistration.New(fakeKubeClient.AdmissionregistrationV1beta1(), fakeKubeClient.AdmissionregistrationV1()))

	testsOps = &pxClusterOps{
		coreOps:             core.Instance(),
		k8sApps:             apps.Instance(),
		k8sRBAC:             rbac.Instance(),
		k8sStorage:          storage.Instance(),
		k8sAdmissions:       admissionregistration.Instance(),
		installedNamespaces: []string{testNamespace},
	}
}

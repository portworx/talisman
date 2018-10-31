package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/probe"
	clientset "github.com/portworx/talisman/pkg/client/clientset/versioned"
	informers "github.com/portworx/talisman/pkg/client/informers/externalversions"
	"github.com/portworx/talisman/pkg/controller"
	"github.com/sirupsen/logrus"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

var (
	namespace  string
	name       string
	listenAddr string
	chaosLevel int
	createCRD  bool
)

func init() {
	flag.StringVar(&listenAddr, "listen-addr", "0.0.0.0:8080", "The address on which the HTTP server will listen to")
	flag.IntVar(&chaosLevel, "chaos-level", -1, "DO NOT USE IN PRODUCTION - level of chaos injected into the px clusters created by the operator.")
	flag.BoolVar(&createCRD, "create-crd", true, "The operator will not create the Cluster CRD when this flag is set to false.")
	flag.Parse()
}

func newKubeClient() kubernetes.Interface {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		logrus.Fatalf("failed to generate kubeconfig: %v", err)
	}

	return kubernetes.NewForConfigOrDie(cfg)
}

func main() {
	if namespace = os.Getenv("POD_NAMESPACE"); len(namespace) == 0 {
		logrus.Fatalf("must set env POD_NAMESPACE")
	}

	if name = os.Getenv("POD_NAME"); len(name) == 0 {
		logrus.Fatalf("must set env POD_NAME")
	}

	// set up signals so we handle the first shutdown signal gracefully
	c := make(chan os.Signal, 1)
	signal.Notify(c)
	go func() {
		logrus.Infof("received signal: %v", <-c)
		os.Exit(1)
	}()

	// TODO dump version and git sha. Refer to start of etcd-operator
	fmt.Println("Go Version:", runtime.Version())

	id, err := os.Hostname()
	if err != nil {
		logrus.Fatalf("failed to get hostname: %v", err)
	}

	kubecli := newKubeClient()

	// TODO add /metrics endpoint
	http.HandleFunc(probe.HTTPReadyzEndpoint, probe.ReadyzHandler)
	go func() {
		if err := http.ListenAndServe(listenAddr, nil); err != nil {
			logrus.Fatalf("failed to listen of endpoint: %s", listenAddr)
		}
	}()

	rl, err := resourcelock.New(resourcelock.EndpointsResourceLock,
		namespace,
		"talisman",
		kubecli.CoreV1(),
		resourcelock.ResourceLockConfig{
			Identity:      id,
			EventRecorder: controller.CreateRecorder(kubecli, name, namespace),
		})
	if err != nil {
		logrus.Fatalf("error creating lock: %v", err)
	}

	ctx := context.Background()

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				run(nil) // TODO remove stop channels and use context cancel
			},
			OnStoppedLeading: func() {
				logrus.Fatalf("leader election lost")
			},
		},
	})

	panic("unreachable")
}

func run(stopCh <-chan struct{}) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		logrus.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient := kubernetes.NewForConfigOrDie(cfg)
	operatorClient := clientset.NewForConfigOrDie(cfg)
	apiExtClientset := apiextensionsclient.NewForConfigOrDie(cfg)

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	operatorInformerFactory := informers.NewSharedInformerFactory(operatorClient, time.Second*30)

	// TODO: add chaos
	//startChaos(context.Background(), cfg.KubeCli, cfg.Namespace, chaosLevel)

	c := controller.New(kubeClient, operatorClient, apiExtClientset, kubeInformerFactory, operatorInformerFactory)

	go kubeInformerFactory.Start(stopCh)
	go operatorInformerFactory.Start(stopCh)

	err = c.Run(2, stopCh)
	logrus.Fatalf("controller Run() failed: %v", err)
}

/*func startChaos(ctx context.Context, kubecli kubernetes.Interface, ns string, chaosLevel int) {
	m := chaos.NewMonkeys(kubecli)
	ls := labels.SelectorFromSet(map[string]string{"name": "portworx"})

	switch chaosLevel {
	case 1:
		logrus.Info("chaos level = 1: randomly kill one px pod every 30 seconds at 50%")
		c := &chaos.CrashConfig{
			Namespace: ns,
			Selector:  ls,

			KillRate:        rate.Every(30 * time.Second),
			KillProbability: 0.5,
			KillMax:         1,
		}
		go func() {
			time.Sleep(60 * time.Second) // don't start until quorum up
			m.CrushPods(ctx, c)
		}()

	case 2:
		logrus.Info("chaos level = 2: randomly kill at most five px pods every 30 seconds at 50%")
		c := &chaos.CrashConfig{
			Namespace: ns,
			Selector:  ls,

			KillRate:        rate.Every(30 * time.Second),
			KillProbability: 0.5,
			KillMax:         5,
		}

		go m.CrushPods(ctx, c)

	default:
	}
}*/

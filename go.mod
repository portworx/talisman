module github.com/portworx/talisman

go 1.13

require (
	github.com/cenkalti/backoff v2.1.1+incompatible // indirect
	github.com/coreos/bbolt v1.3.3 // indirect
	github.com/coreos/go-systemd v0.0.0-20190719114852-fd7a80b32e1f // indirect
	github.com/kisielk/errcheck v1.5.0
	github.com/libopenstorage/gossip v0.0.0-20190507031959-c26073a01952 // indirect
	github.com/libopenstorage/openstorage v8.0.1-0.20190926212733-daaed777713e+incompatible
	github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709 // indirect
	github.com/portworx/kvdb v0.0.0-20190105022415-cccaa09abfc9
	github.com/portworx/sched-ops v0.0.0-20210301232128-6cd5f08740bf
	github.com/sirupsen/logrus v1.6.0
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b
	k8s.io/api v0.20.4
	k8s.io/apiextensions-apiserver v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v12.0.0+incompatible
)

replace (
	github.com/kubernetes-incubator/external-storage => github.com/libopenstorage/external-storage v5.1.1-0.20211117214128-ec0a73271457+incompatible
	google.golang.org/grpc => google.golang.org/grpc v1.29.1

	// Replacing k8s.io dependencies is required if a dependency or any dependency of a dependency
	// depends on k8s.io/kubernetes. See https://github.com/kubernetes/kubernetes/issues/79384#issuecomment-505725449
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.4
	k8s.io/client-go => k8s.io/client-go v0.20.4
)

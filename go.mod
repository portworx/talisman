module github.com/portworx/talisman

go 1.13

require (
	github.com/cenkalti/backoff v2.1.1+incompatible // indirect
	github.com/coreos/bbolt v1.3.3 // indirect
	github.com/coreos/go-semver v0.2.0 // indirect
	github.com/coreos/go-systemd v0.0.0-20190719114852-fd7a80b32e1f // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/google/uuid v1.1.0 // indirect
	github.com/gorilla/websocket v1.4.1 // indirect
	github.com/jonboulle/clockwork v0.1.0 // indirect
	github.com/kisielk/errcheck v1.2.0
	github.com/libopenstorage/gossip v0.0.0-20190507031959-c26073a01952 // indirect
	github.com/libopenstorage/openstorage v8.0.1-0.20190926212733-daaed777713e+incompatible
	github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709 // indirect
	github.com/portworx/kvdb v0.0.0-20190105022415-cccaa09abfc9
	github.com/portworx/sched-ops v0.0.0-20191014235002-8b089d4b79ad
	github.com/sirupsen/logrus v1.4.2
	github.com/tmc/grpc-websocket-proxy v0.0.0-20190109142713-0ad062ec5ee5 // indirect
	github.com/ugorji/go v1.1.1 // indirect
	github.com/xiang90/probing v0.0.0-20190116061207-43a291ad63a2 // indirect
	go.etcd.io/bbolt v1.3.3 // indirect
	golang.org/x/lint v0.0.0-20190909230951-414d861bb4ac
	k8s.io/api v0.0.0-20190816222004-e3a6b8045b0b
	k8s.io/apiextensions-apiserver v0.0.0-20190918224502-6154570c2037
	k8s.io/apimachinery v0.0.0-20190816221834-a9f1d8a9c101
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
)

replace (
	github.com/kubernetes-incubator/external-storage => github.com/libopenstorage/external-storage v5.1.1-0.20190919185747-9394ee8dd536+incompatible
	k8s.io/client-go v2.0.0-alpha.0.0.20181121191925-a47917edff34+incompatible => k8s.io/client-go v2.0.0+incompatible
)

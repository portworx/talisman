# Builing and Running Portworx Operator

## Build

#### Compile
```bash
make docker-build
```
#### Build and push docker image
```bash
export DOCKER_HUB_REPO=<docker-repo> 
export DOCKER_HUB_OPERATOR_IMAGE=talisman
export DOCKER_HUB_TAG=latest

make deploy
```

## Test

#### Deploy the Portworx CRD
```bash
kubectl create -f examples/px-crd.yaml
```

#### Deploy Portworx cluster
```bash
kubectl create -f examples/px-cluster.yaml
```

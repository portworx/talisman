# Builing and Running Talisman

## Vendoring

For vendoring, we use [dep](https://golang.github.io/dep/).
-  `dep ensure`: install the project's dependencies
-  `dep ensure -update`: update the locked versions of all dependencies
-  `dep ensure -add github.com/pkg/errors`: add a dependency to the project

## Build

#### Compile
```bash
make
```
#### Build and push docker image
```bash
export DOCKER_HUB_REPO=<docker-repo>
export DOCKER_HUB_OPERATOR_IMAGE=talisman
export DOCKER_HUB_TAG=latest

make deploy
```


[![Nightly CI](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-kubevirt-hyperconverged-cluster-operator-main-hco-e2e-deploy-nightly-main-aws)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-kubevirt-hyperconverged-cluster-operator-main-hco-e2e-deploy-nightly-main-aws)
[![Nightly CI images](https://prow.ci.kubevirt.io/badge.svg?jobs=periodic-hco-push-nightly-build-main)](https://prow.ci.kubevirt.io/job-history/gs/kubevirt-prow/logs/periodic-hco-push-nightly-build-main)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubevirt/hyperconverged-cluster-operator)](https://goreportcard.com/report/github.com/kubevirt/hyperconverged-cluster-operator)
[![Coverage Status](https://coveralls.io/repos/github/kubevirt/hyperconverged-cluster-operator/badge.svg?branch=main&service=github)](https://coveralls.io/github/kubevirt/hyperconverged-cluster-operator?branch=main)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=kubevirt_hyperconverged-cluster-operator&metric=alert_status)](https://sonarcloud.io/dashboard?id=kubevirt_hyperconverged-cluster-operator)

# Hyperconverged Cluster Operator

A unified operator deploying and controlling [KubeVirt](https://github.com/kubevirt/kubevirt) and several adjacent operators:

- [Containerized Data Importer](https://github.com/kubevirt/containerized-data-importer)
- [Scheduling, Scale and Performance](https://github.com/kubevirt/ssp-operator)
- [Cluster Network Addons](https://github.com/kubevirt/cluster-network-addons-operator)
- [Node Maintenance](https://github.com/kubevirt/node-maintenance-operator)

This operator is typically installed from the Operator Lifecycle Manager (OLM),
and creates operator CustomResources (CRs) for its underlying operators as can be seen in the diagram below.
Use it to obtain an opinionated deployment of KubeVirt and its helper operators.

![](images/HCO-design.jpg)

In the [HCO components](docs/hco_components.md) doc you can get an up-to-date overview of the involved components.

## Installing HCO using kustomize (Openshift OLM Only)
To install the default community HyperConverged Cluster Operator, along with its underlying components, run:
```bash
$ curl -L https://api.github.com/repos/kubevirt/hyperconverged-cluster-operator/tarball/main | \
tar --strip-components=1 -xvzf - kubevirt-hyperconverged-cluster-operator-*/deploy/kustomize

$ ./deploy/kustomize/deploy_kustomize.sh
```
The deployment is completed when HCO custom resource reports its condition as `Available`.

For more explanation and advanced options for HCO deployment using kustomize, refer to [kustomize deployment documentation](deploy/kustomize/README.md).

## Installing Unreleased Bundle Using A Custom Catalog Source

Hyperconverged Cluster Operator is publishing the latest bundle to [quay.io/kubevirt](https://quay.io/repository/kubevirt)
before publishing tagged, stable releases to [OperatorHub.io](https://operatorhub.io).
The latest bundle is `quay.io/kubevirt/hyperconverged-cluster-bundle:1.16.0-unstable`. It is built and pushed on every merge to
main branch, and contains the most up-to-date manifests, which are pointing to the most recent application images: `hyperconverged-cluster-operator`
and `hyperconverged-cluster-webhook`, which are built together with the bundle from the current code at the main branch.
The unreleased bundle can be consumed on a cluster by creating a CatalogSource pointing to the index image that contains
that bundle: `quay.io/kubevirt/hyperconverged-cluster-index:1.16.0-unstable`.

Make the bundle available in the cluster's packagemanifest by adding the following CatalogSource:
```bash
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: hco-unstable-catalog-source
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: quay.io/kubevirt/hyperconverged-cluster-index:1.16.0-unstable
  displayName: Kubevirt Hyperconverged Cluster Operator
  publisher: Kubevirt Project
EOF
```
Then, create a namespace, subscription and an OperatorGroup to deploy HCO via OLM:
```bash
cat <<EOF | oc apply -f -
apiVersion: v1
kind: Namespace
metadata:
    name: kubevirt-hyperconverged
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
    name: kubevirt-hyperconverged-group
    namespace: kubevirt-hyperconverged
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
    name: hco-operatorhub
    namespace: kubevirt-hyperconverged
spec:
    source: hco-unstable-catalog-source
    sourceNamespace: openshift-marketplace
    name: community-kubevirt-hyperconverged
    channel: "candidate-v1.16"
EOF
```
Then, create the HyperConverged custom resource to complete the installation.
Further information about the HyperConverged CR and its possible configuration options can be found
in the [Cluster Configuration](docs/cluster-configuration.md) doc.
## Using the HCO without OLM or Marketplace

Run the following script to apply the HCO operator:

```bash
$ curl https://raw.githubusercontent.com/kubevirt/hyperconverged-cluster-operator/main/deploy/deploy.sh | bash
```

## Developer Workflow (using [OLM](https://github.com/operator-framework/operator-lifecycle-manager/blob/master/doc/install/install.md#installing-olm))

Build the HCO container using the Makefile recipes `make container-build` and
`make container-push` with vars `IMAGE_REGISTRY`, `REGISTRY_NAMESPACE`, and `IMAGE_TAG`
to direct it's location.

To use the HCO's container, we'll use a registry image to serve metadata to OLM.
Build and push the HCO's registry image.
```bash
# e.g. quay.io, docker.io
export IMAGE_REGISTRY=<image_registry>
export REGISTRY_NAMESPACE=<container_org>
export IMAGE_TAG=example

# build the container images and push them to registry
make container-build container-push

export HCO_OPERATOR_IMAGE=$IMAGE_REGISTRY/$REGISTRY_NAMESPACE/hyperconverged-cluster-operator:$IMAGE_TAG
export HCO_WEBHOOK_IMAGE=$IMAGE_REGISTRY/$REGISTRY_NAMESPACE/hyperconverged-cluster-webhook:$IMAGE_TAG
export ARTIFACTS_SERVER_IMAGE=$IMAGE_REGISTRY/$REGISTRY_NAMESPACE/virt-artifacts-server:$IMAGE_TAG

# Image to be used in CSV manifests
HCO_OPERATOR_IMAGE=$HCO_OPERATOR_IMAGE CSV_VERSION=$CSV_VERSION make build-manifests
sed -i "s|+WEBHOOK_IMAGE_TO_REPLACE+|${HCO_WEBHOOK_IMAGE}|g" deploy/index-image/community-kubevirt-hyperconverged/1.12.0/manifests/kubevirt-hyperconverged-operator.v1.12.0.clusterserviceversion.yaml
sed -i "s|+ARTIFACTS_SERVER_IMAGE_TO_REPLACE+|${ARTIFACTS_SERVER_IMAGE}|g" deploy/index-image/community-kubevirt-hyperconverged/1.12.0/manifests/kubevirt-hyperconverged-operator.v1.12.0.clusterserviceversion.yaml
```

Create the namespace for the HCO.
```bash
$ kubectl create ns kubevirt-hyperconverged
```

For the next set of commands, we will use the
[`operator-sdk`](https://github.com/operator-framework/operator-sdk/releases) CLI tool. Below commands will create a
bundle image, push this image, and finally use that bundle image to install the HyperConverged cluster
Operator on the OLM-enabled cluster.

```shell
operator-sdk generate bundle --input-dir deploy/index-image/ --output-dir _out/bundle
operator-sdk bundle validate _out/bundle  # optional
podman build -f deploy/index-image/bundle.Dockerfile -t $IMAGE_REGISTRY/$REGISTRY_NAMESPACE/hyperconverged-cluster-index:$IMAGE_TAG
podman push $IMAGE_REGISTRY/$REGISTRY_NAMESPACE/hyperconverged-cluster-index:$IMAGE_TAG
operator-sdk bundle validate $IMAGE_REGISTRY/$REGISTRY_NAMESPACE/hyperconverged-cluster-index:$IMAGE_TAG
operator-sdk run bundle -n kubevirt-hyperconverged $IMAGE_REGISTRY/$REGISTRY_NAMESPACE/hyperconverged-cluster-index:$IMAGE_TAG
```

Create an HCO CustomResource, which creates the KubeVirt CR, launching KubeVirt,
CDI (Containerized Data Importer), Network-addons, VM import, TTO (Tekton Tasks Operator) and SSP (Scheduling, Scale
and Performance) Operator.
```bash
$ kubectl create -f deploy/hco.cr.yaml -n kubevirt-hyperconverged
```

## Create a Cluster & Launch the HCO
1. Choose the provider
```bash
#For k8s cluster:
$ export KUBEVIRT_PROVIDER="k8s-1.17"
```
```bash
#For okd cluster:
$ export KUBEVIRT_PROVIDER="okd-4.1"
```
2. Navigate to the project's directory
```bash
$ cd <path>/hyperconverged-cluster-operator
```
3. Remove an old cluster
```bash
$ make cluster-down
```
4. Create a new cluster
```bash
$ make cluster-up
```
5. Clean previous HCO deployment and re-deploy HCO \
   (When making a change, execute only this command - no need to repeat steps 1-3)
```bash
$ make cluster-sync
```
### Command-Line Tool
Use `./cluster/kubectl.sh` as the command-line tool.

For example:
```bash
$ ./cluster/kubectl.sh get pods --all-namespaces
```

## Deploying HCO on top of external provider

In order to use HCO on top of external provider, i.e CRC, use:
```
export KUBEVIRT_PROVIDER=external
export IMAGE_REGISTRY=<container image repository, such as quay.io, default: quay.io>
export REGISTRY_NAMESPACE=<your org under IMAGE_REGISTRY, i.e your_name if you use quay.io/your_name, default: kubevirt>
make cluster-sync
```
`oc` binary should exist, and the cluster should be reachable via `oc` commands.

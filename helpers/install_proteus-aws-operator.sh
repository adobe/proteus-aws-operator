#!/bin/bash

# This script will install the proteus-aws-operator into the Kubernetes cluster pointed at by
# your KUBECONFIG (normally used for local installations).

set -x
set -e

export HELM_EXPERIMENTAL_OCI=1
export ACK_K8S_NAMESPACE=ack-system
export RELEASE_VERSION=v0.0.1
export CHART_EXPORT_PATH=/tmp/chart
export CHART_REPO=artifactory.corp.adobe.com/artifactory/helm-helm-dc-microservices-release
export CHART_REF=$CHART_REPO:proteus-aws-operator-$RELEASE_VERSION

mkdir -p $CHART_EXPORT_PATH

helm chart pull $CHART_REF
helm chart export $CHART_REF --destination $CHART_EXPORT_PATH

helm install --namespace $ACK_K8S_NAMESPACE proteus-aws-operator $CHART_EXPORT_PATH/proteus-aws-operator

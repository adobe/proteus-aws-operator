#!/bin/bash

# This script will install the proteus-aws-operator into the Kubernetes cluster pointed at by
# your KUBECONFIG (normally used for local installations).

set -x
set -e

if [[ -z $ARTIFACTORY_USER ]]; then
	echo "ARTIFACTORY_USER must be set"
fi

if [[ -z $ARTIFACTORY_API_TOKEN ]]; then
	echo "ARTIFACTORY_API_TOKEN must be set"
fi

export HELM_EXPERIMENTAL_OCI=1
export ACK_K8S_NAMESPACE=ack-system
export RELEASE_VERSION=0.0.1
export CHART_EXPORT_PATH=/tmp/chart
export CHART_REPO=helm-helm-dc-microservices-release
export CHART_REF=$CHART_REPO/proteus-aws-operator

mkdir -p $CHART_EXPORT_PATH

helm repo list | grep helm-helm-dc-microservices-release > /dev/null
if [[ $? != 0 ]]; then
	helm repo add $CHART_REPO https://artifactory.corp.adobe.com/artifactory/$CHART_REPO  --username $ARTIFACTORY_USER --password $ARTIFACTORY_API_TOKEN
fi

helm repo update

helm install --namespace $ACK_K8S_NAMESPACE --version $RELEASE_VERSION proteus-aws-operator $CHART_REF

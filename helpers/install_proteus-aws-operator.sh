#!/bin/bash

# This script will install the proteus-aws-operator into the Kubernetes cluster pointed at by
# your KUBECONFIG (normally used for local installations).

set -x
set -e

: ${LOCAL:=false}

if [[ -z $ARTIFACTORY_USER ]]; then
	echo "ARTIFACTORY_USER must be set"
fi

if [[ -z $ARTIFACTORY_API_TOKEN ]]; then
	echo "ARTIFACTORY_API_TOKEN must be set"
fi

export HELM_EXPERIMENTAL_OCI=1
export ACK_K8S_NAMESPACE=ack-system
export RELEASE_VERSION=0.0.3
export CHART_REPO=helm-helm-dc-microservices-release
export CHART_REF=$CHART_REPO/proteus-aws-operator

helm repo list | grep helm-helm-dc-microservices-release > /dev/null
if [[ $? != 0 ]]; then
	helm repo add $CHART_REPO https://artifactory.corp.adobe.com/artifactory/$CHART_REPO  --username $ARTIFACTORY_USER --password $ARTIFACTORY_API_TOKEN
fi

kubectl create namespace $ACK_K8S_NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

helm repo update

if [[ $LOCAL ]]; then
	helm install --namespace $ACK_K8S_NAMESPACE proteus-aws-operator ./helm
else
	helm install --namespace $ACK_K8S_NAMESPACE --version $RELEASE_VERSION proteus-aws-operator $CHART_REF
fi

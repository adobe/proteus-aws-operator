#!/bin/bash

# This script will install the proteus-aws-operator into the Kubernetes cluster pointed at by
# your KUBECONFIG (normally used for local installations).

set -x
set -e

REGION=${REGION:-us-east-1}

if [[ -z "$ARTIFACTORY_USER" ]]; then
	echo "ARTIFACTORY_USER must be set"
	exit 1
fi

if [[ -z "$ARTIFACTORY_API_TOKEN" ]]; then
	echo "ARTIFACTORY_API_TOKEN must be set"
	exit 2
fi

if [[ -z "$AWS_ACCOUNT_ID" ]]; then
	echo "AWS_ACCOUNT_ID must be set"
	exit 3
fi

if [[ -z "$AWS_ACCOUNT_ROLE" ]]; then
	echo "AWS_ACCOUNT_ROLE must be set"
	exit 3
fi

export HELM_EXPERIMENTAL_OCI=1
export ACK_K8S_NAMESPACE=ack-system
export RELEASE_VERSION=0.0.4
export CHART_REPO=helm-helm-dc-microservices-release
export CHART_REF=$CHART_REPO/ack-rds-controller

set +e
helm repo list | grep helm-helm-dc-microservices-release > /dev/null
err=$?
set -e
if [[ $err != 0 ]]; then
	helm repo add $CHART_REPO https://artifactory.corp.adobe.com/artifactory/$CHART_REPO  --username $ARTIFACTORY_USER --password $ARTIFACTORY_API_TOKEN
fi

kubectl create namespace $ACK_K8S_NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

helm repo update

cat <<EOF | helm upgrade --values - --namespace $ACK_K8S_NAMESPACE --version $RELEASE_VERSION --install ack-rds-controller $CHART_REF
deployment:
  annotations:
    iam.amazonaws.com/role: $AWS_ACCOUNT_ROLE
aws:
  region: $REGION
  account_id: $AWS_ACCOUNT_ID
EOF

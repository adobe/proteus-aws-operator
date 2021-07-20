#!/bin/bash

# This script will install the AWS ACK rds-controller into the Kubernetes cluster pointed at
# by your KUBECONFIG (normally used for local installations).

set -x
set -e

export HELM_EXPERIMENTAL_OCI=1
export ACK_K8S_NAMESPACE=ack-system
export SERVICE=rds
export RELEASE_VERSION=v0.0.3
export CHART_EXPORT_PATH=/tmp/chart
export CHART_REPO=public.ecr.aws/aws-controllers-k8s/chart
export CHART_REF=$CHART_REPO:$SERVICE-$RELEASE_VERSION

mkdir -p $CHART_EXPORT_PATH

helm chart pull $CHART_REF
helm chart export $CHART_REF --destination $CHART_EXPORT_PATH

kubectl create namespace $ACK_K8S_NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

helm install --namespace $ACK_K8S_NAMESPACE ack-$SERVICE-controller $CHART_EXPORT_PATH/ack-$SERVICE-controller

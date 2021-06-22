/*
Copyright Adobe Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rds_types "github.com/aws-controllers-k8s/rds-controller/apis/v1alpha1"
)

// DBReplicationGroupSpec defines the desired state of DBReplicationGroup
type DBReplicationGroupSpec struct {
	// NumReplicas is the number of replicas to create
	// +kubebuilder:validation:Required
	NumReplicas *int `json:"numReplicas"`

	// AvailabilityZones is a list of AvailabilityZone names
	//
	// Note: for Aurora, this must match the DBCluster.AvailabilityZones specification
	//
	// +kubebuilder:validation:Required
	AvailabilityZones []*string `json:"availabilityZones"`

	// DBInstance is the base DBInstance specification. All replica instances are cloned from this specification.
	//
	// Note: the DBInstance.AvailabilityZone specification will be overridden by the main AvailabilityZone specification above in each replica.
	// Note: the DBInstance.MultiAZ specification will be explicitly overridden to false
	//
	// For more information see the ACK RDS Controller DBInstance API
	//
	// +kubebuilder:validation:Required
	DBInstance *rds_types.DBInstanceSpec `json:"dbInstance"`
}

// DBReplicationGroupStatus defines the observed state of DBReplicationGroup
type DBReplicationGroupStatus struct {
	DBInstanceBaseIdentifier *string `json:"dbInstanceBaseIdentifier"`

	DBInstances []*rds_types.DBInstance `json:"dbInstances"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DBReplicationGroup is the Schema for the dbreplicationgroups API
type DBReplicationGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DBReplicationGroupSpec   `json:"spec,omitempty"`
	Status DBReplicationGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DBReplicationGroupList contains a list of DBReplicationGroup objects
type DBReplicationGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DBReplicationGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DBReplicationGroup{}, &DBReplicationGroupList{})
}

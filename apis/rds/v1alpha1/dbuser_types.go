/*
Copyright 2021.

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
)

// DBUserSpec defines the desired state of DBUser
type DBUserSpec struct {
	// DBInstanceIdentifier is the identifier of the DBInstance to connect to when creating the DBUser.
	//
	// Note: Either DBClusterIdentifier or DBInstanceIdentifier must be specified, but not both
	//
	// +kubebuilder:validation:Optional
	DBInstanceIdentifier *string `json:"dbInstanceIdentifier,omitempty"`

	// DBClusterIdentifier is the identifier of the Aurora cluster to connect to when creating the DBUser.
	//
	// Note: Either DBClusterIdentifier or DBInstanceIdentifier must be specified, but not both
	//
	// +kubebuilder:validation:Optional
	DBClusterIdentifier *string `json:"dbClusterIdentifier,omitempty"`

	// Username is the role name of the DBUser to create
	// +kubebuilder:validation:Required
	Username *string `json:"username"`

	// Password is the password of the DBUser to create.
	//
	// Default: No user password is created
	//
	// Note: Either Password or UseIAMAuthentication must be specified, but not both
	//
	// +kubebuilder:validation:Optional
	Password *string `json:"password"`

	// UseIAMAuthentication is a boolean value which specifies whether or not to use AWS IAM for Authentication
	// instead of a password.
	//
	// See: https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.IAMDBAuth.DBAccounts.html
	//
	// Note: Either Password or UseIAMAuthentication must be specified, but not both
	//
	// +kubebuilder:validation:Optional
	UseIAMAuthentication *bool `json:"useIAMAuthentication"`

	// GrantStatement is the GRANT statement run after user creation to provide the user specific privileges.
	//
	// Note: The RDS Master User (DBInstance.MasterUsername) does not have super user privileges. Thus, you
	//       cannot use `GRANT ALL PRIVILEGES ON *.* to ?` to grant all privileges to a user.
	//       See: https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.MasterAccounts.html
	//
	// +kubebuilder:validation:Required
	GrantStatement *string `json:"grantStatement"`

	// ApplyGrantWhenExists is a boolean value which specifies whether or not to apply GrantStatement even if
	// the user already exists.
	// +kubebuilder:validation:Optional
	ApplyGrantWhenExists *bool `json:"applyGrantWhenExists,omitempty"`
}

// DBUserStatus defines the observed state of DBUser
type DBUserStatus struct {
	DBUsers *[]DBUser `json:"dbUsers"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DBUser is the Schema for the dbusers API
type DBUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DBUserSpec   `json:"spec,omitempty"`
	Status DBUserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DBUserList contains a list of DBUser
type DBUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DBUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DBUser{}, &DBUserList{})
}

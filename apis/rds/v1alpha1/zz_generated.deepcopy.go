//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2021 Adobe. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License. You may obtain a copy
of the License at http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under
the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR REPRESENTATIONS
OF ANY KIND, either express or implied. See the License for the specific language
governing permissions and limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	apisv1alpha1 "github.com/aws-controllers-k8s/rds-controller/apis/v1alpha1"
	corev1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DBReplicationGroup) DeepCopyInto(out *DBReplicationGroup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DBReplicationGroup.
func (in *DBReplicationGroup) DeepCopy() *DBReplicationGroup {
	if in == nil {
		return nil
	}
	out := new(DBReplicationGroup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DBReplicationGroup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DBReplicationGroupList) DeepCopyInto(out *DBReplicationGroupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DBReplicationGroup, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DBReplicationGroupList.
func (in *DBReplicationGroupList) DeepCopy() *DBReplicationGroupList {
	if in == nil {
		return nil
	}
	out := new(DBReplicationGroupList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DBReplicationGroupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DBReplicationGroupSpec) DeepCopyInto(out *DBReplicationGroupSpec) {
	*out = *in
	if in.NumReplicas != nil {
		in, out := &in.NumReplicas, &out.NumReplicas
		*out = new(int)
		**out = **in
	}
	if in.DBInstance != nil {
		in, out := &in.DBInstance, &out.DBInstance
		*out = new(apisv1alpha1.DBInstanceSpec)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DBReplicationGroupSpec.
func (in *DBReplicationGroupSpec) DeepCopy() *DBReplicationGroupSpec {
	if in == nil {
		return nil
	}
	out := new(DBReplicationGroupSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DBReplicationGroupStatus) DeepCopyInto(out *DBReplicationGroupStatus) {
	*out = *in
	if in.DBInstanceBaseIdentifier != nil {
		in, out := &in.DBInstanceBaseIdentifier, &out.DBInstanceBaseIdentifier
		*out = new(string)
		**out = **in
	}
	if in.DBInstanceIdentifiers != nil {
		in, out := &in.DBInstanceIdentifiers, &out.DBInstanceIdentifiers
		*out = make([]*string, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(string)
				**out = **in
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DBReplicationGroupStatus.
func (in *DBReplicationGroupStatus) DeepCopy() *DBReplicationGroupStatus {
	if in == nil {
		return nil
	}
	out := new(DBReplicationGroupStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DBUser) DeepCopyInto(out *DBUser) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DBUser.
func (in *DBUser) DeepCopy() *DBUser {
	if in == nil {
		return nil
	}
	out := new(DBUser)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DBUser) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DBUserList) DeepCopyInto(out *DBUserList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DBUser, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DBUserList.
func (in *DBUserList) DeepCopy() *DBUserList {
	if in == nil {
		return nil
	}
	out := new(DBUserList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DBUserList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DBUserSpec) DeepCopyInto(out *DBUserSpec) {
	*out = *in
	if in.DBInstanceIdentifier != nil {
		in, out := &in.DBInstanceIdentifier, &out.DBInstanceIdentifier
		*out = new(string)
		**out = **in
	}
	if in.DBClusterIdentifier != nil {
		in, out := &in.DBClusterIdentifier, &out.DBClusterIdentifier
		*out = new(string)
		**out = **in
	}
	if in.Username != nil {
		in, out := &in.Username, &out.Username
		*out = new(string)
		**out = **in
	}
	if in.Password != nil {
		in, out := &in.Password, &out.Password
		*out = new(corev1alpha1.SecretKeyReference)
		**out = **in
	}
	if in.UseIAMAuthentication != nil {
		in, out := &in.UseIAMAuthentication, &out.UseIAMAuthentication
		*out = new(bool)
		**out = **in
	}
	if in.GrantStatement != nil {
		in, out := &in.GrantStatement, &out.GrantStatement
		*out = new(string)
		**out = **in
	}
	if in.ApplyGrantWhenExists != nil {
		in, out := &in.ApplyGrantWhenExists, &out.ApplyGrantWhenExists
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DBUserSpec.
func (in *DBUserSpec) DeepCopy() *DBUserSpec {
	if in == nil {
		return nil
	}
	out := new(DBUserSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DBUserStatus) DeepCopyInto(out *DBUserStatus) {
	*out = *in
	if in.Usernames != nil {
		in, out := &in.Usernames, &out.Usernames
		*out = make([]*string, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(string)
				**out = **in
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DBUserStatus.
func (in *DBUserStatus) DeepCopy() *DBUserStatus {
	if in == nil {
		return nil
	}
	out := new(DBUserStatus)
	in.DeepCopyInto(out)
	return out
}

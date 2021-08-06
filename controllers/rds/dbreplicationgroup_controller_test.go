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

package rds

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"

	rdstypes "github.com/aws-controllers-k8s/rds-controller/apis/v1alpha1"

	"github.com/adobe-platform/proteus-aws-operator/apis/rds/v1alpha1"
)

var _ = Describe("DBReplicationGroup controller", func() {

	const (
		Namespace              = "default"
		DBReplicationGroupName = "test-dbreplicationgroup"

		DBClusterName = "test-dbcluster"

		overallTimeout float64 = 60.0 // seconds
		timeout                = time.Second * 10
		duration               = time.Second * 10
		interval               = time.Millisecond * 250
	)

	Context("When updating DBReplicationGroup Status", func() {
		It("Should update DBInstance's when DBReplicationGroup's are updated", func(done Done) {
			ctx := context.Background()

			c := make(chan bool, 1)

			By("Creating a new DBReplicationGroup")
			dbReplicationGroup := &v1alpha1.DBReplicationGroup{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws.adobe.io/v1alpha1",
					Kind:       "DBReplicationGroup",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBReplicationGroupName,
					Namespace: Namespace,
				},
				Spec: v1alpha1.DBReplicationGroupSpec{
					NumReplicas: intAddr(5),
					AvailabilityZones: []*string{
						strAddr("az1"),
						strAddr("az2"),
					},
					DBInstance: &rdstypes.DBInstanceSpec{
						DBInstanceIdentifier: strAddr("test-dbinstanceid"),
						Engine:               strAddr("mysql"),
						DBInstanceClass:      strAddr("db.m4.large"),
					},
				},
			}
			Expect(k8sClient.Create(ctx, dbReplicationGroup)).Should(Succeed())

			DBReplicationGroupLookupKey := types.NamespacedName{Name: DBReplicationGroupName, Namespace: Namespace}
			createdDBReplicationGroup := &v1alpha1.DBReplicationGroup{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, DBReplicationGroupLookupKey, createdDBReplicationGroup)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			By("Checking that the DBReplicationGroup properly created a set of DBInstances")
			currentInstances := &rdstypes.DBInstanceList{}
			listOpts := []client.ListOption{
				client.InNamespace(dbReplicationGroup.Namespace),
				client.MatchingLabels(labelsForDBReplicationGroup(dbReplicationGroup)),
			}
			expectedNumInstances := (len(dbReplicationGroup.Spec.AvailabilityZones) * *dbReplicationGroup.Spec.NumReplicas)
			Eventually(func() bool {
				err := k8sClient.List(ctx, currentInstances, listOpts...)
				if err != nil {
					return false
				}
				if len(currentInstances.Items) != expectedNumInstances {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for DBInstances to be created")
			Expect(<-c).To(BeTrue())

			// Fake that the rds-controller is actually active in the k8s cluster
			Expect(func() error {
				writeInstance := currentInstances.Items[0]
				for _, instance := range currentInstances.Items[1:len(currentInstances.Items)] {
					instance.Status = rdstypes.DBInstanceStatus{
						Conditions: []*ackv1alpha1.Condition{
							&ackv1alpha1.Condition{
								Type:   ackv1alpha1.ConditionTypeResourceSynced,
								Status: "True",
							},
						},
						ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
							ARN:            (*ackv1alpha1.AWSResourceName)(strAddr("AWS-Resource-Name-12345")),
							OwnerAccountID: (*ackv1alpha1.AWSAccountID)(strAddr("1234567890")),
						},
						ReadReplicaSourceDBInstanceIdentifier: writeInstance.Spec.DBInstanceIdentifier,
					}
					err := k8sClient.Status().Update(context.TODO(), &instance)
					if err != nil {
						return err
					}
				}
				return nil
			}()).Should(Succeed())

			By("Checking that the created DBInstance's have the correct names")
			var dbInstanceID string
			check_map := make(map[string]int)
			Expect(func() bool {
				for _, instance := range currentInstances.Items {
					check_map[*instance.Spec.AvailabilityZone]++

					dbInstanceID = fmt.Sprintf("%s-%s", *dbReplicationGroup.Spec.DBInstance.DBInstanceIdentifier, *instance.Spec.AvailabilityZone)

					if !strings.HasPrefix(*instance.Spec.DBInstanceIdentifier, dbInstanceID) {
						return false
					}
				}
				return true
			}()).Should(BeTrue())

			By("Checking that the correct number of DBInstance's were created for the correct AZs")
			Expect(func() bool {
				for _, az := range dbReplicationGroup.Spec.AvailabilityZones {
					if num_instances, ok := check_map[*az]; ok {
						if num_instances != *dbReplicationGroup.Spec.NumReplicas {
							return false
						}
					} else {
						return false
					}
				}

				return true
			}()).Should(BeTrue())

			By("Patching DBReplicationGroup to have less instances and more AvailabilityZone's")
			patch := client.MergeFrom(dbReplicationGroup.DeepCopy())
			dbReplicationGroup.Spec.NumReplicas = intAddr(2)
			dbReplicationGroup.Spec.AvailabilityZones = []*string{
				strAddr("az1"),
				// az2 is explicitly removed so we can check below
				strAddr("az3"),
				strAddr("az4"),
				strAddr("az5"),
			}
			Expect(k8sClient.Patch(context.TODO(), dbReplicationGroup, patch)).Should(Succeed())

			By("Checking that the DBReplicationGroup properly still has the expected number of DBInstances")
			// currentInstances and listOpts are initialized above
			expectedNumInstances = (len(dbReplicationGroup.Spec.AvailabilityZones) * *dbReplicationGroup.Spec.NumReplicas)
			Eventually(func() bool {
				err := k8sClient.List(ctx, currentInstances, listOpts...)
				if err != nil {
					return false
				}
				instances := make([]*string, 0, len(currentInstances.Items))
				for _, instance := range currentInstances.Items {
					if instance.GetDeletionTimestamp() == nil {
						instances = append(instances, instance.Spec.DBInstanceIdentifier)
					}
				}
				if len(instances) != expectedNumInstances {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for DBInstances to be updated")
			Expect(<-c).To(BeTrue())

			By("Checking that the existing/new DBInstance's have the correct names")
			// check_map and dbInstanceID are declared above
			check_map = make(map[string]int)
			Expect(func() bool {
				for _, instance := range currentInstances.Items {
					if *instance.Spec.AvailabilityZone == "az2" {
						return false
					}
					check_map[*instance.Spec.AvailabilityZone]++

					dbInstanceID = fmt.Sprintf("%s-%s", *dbReplicationGroup.Spec.DBInstance.DBInstanceIdentifier, *instance.Spec.AvailabilityZone)

					if !strings.HasPrefix(*instance.Spec.DBInstanceIdentifier, dbInstanceID) {
						return false
					}
				}
				return true
			}()).Should(BeTrue())

			By("Checking that the correct number of DBInstance's exist for the correct AZs")
			Expect(func() bool {
				for _, az := range dbReplicationGroup.Spec.AvailabilityZones {
					if num_instances, ok := check_map[*az]; ok {
						if num_instances != *dbReplicationGroup.Spec.NumReplicas {
							return false
						}
					} else {
						return false
					}
				}

				return true
			}()).Should(BeTrue())

			close(done)
		}, overallTimeout)
	})
})

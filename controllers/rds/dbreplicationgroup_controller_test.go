/*
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

	rds_types "github.com/aws-controllers-k8s/rds-controller/apis/v1alpha1"

	"github.com/adobe-platform/proteus-aws-operator/apis/rds/v1alpha1"
)

func intAddr(i int) *int {
	return &i
}

func strAddr(s string) *string {
	return &s
}

var _ = Describe("DBReplicationGroup controller", func() {

	const (
		Namespace              = "default"
		DBReplicationGroupName = "test-dbreplicationgroup"

		DBClusterName = "test-dbcluster"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When updating DBReplicationGroup Status", func() {
		It("Should create a DBCluster and related DBInstance's when new DBReplicationGroup's are created", func() {
			By("By creating a new DBReplicationGroup")
			ctx := context.Background()
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
					NumReplicas:       intAddr(2),
					AvailabilityZones: []*string{},
					DBInstance: &rds_types.DBInstanceSpec{
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

			By("By checking that the DBReplicationGroup properly created a set of DBInstances")
			currentInstances := &rds_types.DBInstanceList{}
			listOpts := []client.ListOption{
				client.InNamespace(dbReplicationGroup.Namespace),
				client.MatchingLabels(labelsForDBReplicationGroup(dbReplicationGroup)),
			}
			Eventually(func() bool {
				err := k8sClient.List(ctx, currentInstances, listOpts...)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			By("By checking that the created DBInstance's have the correct names")
			var check_map map[string]int
			var dbInstanceID string
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

			By("By checking that the correct number of DBInstance's were created for the correct AZs")
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
		})
	})
})

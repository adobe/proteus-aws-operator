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

	"github.com/DATA-DOG/go-sqlmock"

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"

	rdstypes "github.com/aws-controllers-k8s/rds-controller/apis/v1alpha1"

	rdsv1alpha1 "github.com/adobe/proteus-aws-operator/apis/rds/v1alpha1"
)

var _ = Describe("DBUser controller", func() {

	const (
		Namespace = "default"

		DBInstanceBaseName = "test-dbinstance"

		DBClusterBaseName = "test-dbcluster"

		DBUserBaseName = "test-dbuser"
		DBUserUsername = "test-username"

		PasswordBaseSecretName = "test-dbuser-passwordsecret"
		PasswordSecretKey      = "test-dbuser-passwordkey"
		PasswordSecretValue    = "test-dbuser-passwordvalue"

		GrantStatement = "GRANT ALL PRIVILEGES ON `%`.* TO ?"

		overallTimeout float64 = 120.0 // seconds
		timeout                = time.Second * 10
		duration               = time.Second * 10
		interval               = time.Millisecond * 250
	)

	ExpectedGrantStatementSQL := strings.Join(strings.Fields(GrantStatement)[0:4], " ")

	// >>>>> DBInstance <<<<<

	Context("CRUD using DBInstanceIdentifier", func() {
		ctx := context.Background()

		// >>>>> MySQL <<<<<

		// >>>>> DBInstance | MySQL | Password <<<<<
		It("Should create/delete a user with a password in a MySQL database", func(done Done) {
			DBInstanceName := fmt.Sprintf("%s-mysql-password", DBInstanceBaseName)
			DBUserName := fmt.Sprintf("%s-mysql-password", DBUserBaseName)
			PasswordSecretName := fmt.Sprintf("%s-mysql-password", PasswordBaseSecretName)

			c := make(chan bool, 2)

			By("Creating a new Password Secret")
			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Immutable: boolRef(true),
				StringData: map[string]string{
					PasswordSecretKey: PasswordSecretValue,
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			secretKeyReference := &ackv1alpha1.SecretKeyReference{
				SecretReference: corev1.SecretReference{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Key: PasswordSecretKey,
			}

			secretLookupKey := types.NamespacedName{Name: PasswordSecretName, Namespace: Namespace}
			createdSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, secretLookupKey, createdSecret)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Creating a new DBInstance")
			dbInstance := &rdstypes.DBInstance{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws/v1alpha1",
					Kind:       "DBInstance",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBInstanceName,
					Namespace: Namespace,
				},
				Spec: rdstypes.DBInstanceSpec{
					DBInstanceIdentifier: strRef(DBInstanceName),
					Engine:               strRef("mysql"),
					DBInstanceClass:      strRef("db.m4.large"),
					MasterUsername:       strRef("root"),
					MasterUserPassword:   secretKeyReference,
				},
				Status: rdstypes.DBInstanceStatus{
					Conditions: []*ackv1alpha1.Condition{
						&ackv1alpha1.Condition{
							Type:   ackv1alpha1.ConditionTypeResourceSynced,
							Status: "True",
						},
					},
					ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
						ARN:            (*ackv1alpha1.AWSResourceName)(strRef("AWS-Resource-Name-12345")),
						OwnerAccountID: (*ackv1alpha1.AWSAccountID)(strRef("1234567890")),
					},
					Endpoint: &rdstypes.Endpoint{
						Address:      strRef("test-hostname"),
						HostedZoneID: strRef("test-hostedzoneid"),
						Port:         int64Ref(3306),
					},
				},
			}
			Expect(k8sClient.Create(ctx, dbInstance)).Should(Succeed())

			// Fake that the rds-controller is actually active in the k8s cluster
			Expect(k8sClient.Status().Update(context.TODO(), dbInstance)).Should(Succeed())

			dbInstanceLookupKey := types.NamespacedName{Name: DBInstanceName, Namespace: Namespace}
			createdDBInstance := &rdstypes.DBInstance{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbInstanceLookupKey, createdDBInstance)
				if err != nil {
					return false
				}
				if createdDBInstance.Status.Endpoint == nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for Password Secret to exist")
			Expect(<-c).To(BeTrue())

			By("Waiting for DBInstance to exist and have an active Endpoint")
			Expect(<-c).To(BeTrue())

			db, mock, err := sqlmock.New()
			Expect(err).Should(Succeed())

			testDB = db

			mock.ExpectExec("CREATE USER.+IDENTIFIED BY").WithArgs(DBUserUsername, PasswordSecretValue).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(ExpectedGrantStatementSQL).WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))

			By("Creating a new DBUser with a password")
			dbUser := &rdsv1alpha1.DBUser{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws.adobe.io/v1alpha1",
					Kind:       "DBUser",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBUserName,
					Namespace: Namespace,
				},
				Spec: rdsv1alpha1.DBUserSpec{
					DBInstanceIdentifier: strRef(DBInstanceName),
					Username:             strRef("test-username"),
					Password:             secretKeyReference,
					GrantStatement:       strRef(GrantStatement),
				},
			}
			Expect(k8sClient.Create(ctx, dbUser)).Should(Succeed())

			dbUserLookupKey := types.NamespacedName{Name: DBUserName, Namespace: Namespace}
			createdDBUser := &rdsv1alpha1.DBUser{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbUserLookupKey, createdDBUser)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for DBUser to be created")
			Expect(<-c).To(BeTrue())

			By("Validating expected Database operations occurred")
			Eventually(func() bool {
				err := mock.ExpectationsWereMet()
				if err != nil {
					return false
				} else {
					c <- true
					return true
				}
			}, timeout, interval).Should(BeTrue())

			By("Waiting for Database operations to execute")
			Expect(<-c).To(BeTrue())

			db, mock, err = sqlmock.New()
			Expect(err).Should(Succeed())

			testDB = db

			mock.ExpectExec("DROP USER").WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))

			By("Deleting DBUser object")
			Expect(k8sClient.Delete(ctx, dbUser)).Should(Succeed())

			dbUserLookupKey = types.NamespacedName{Name: DBUserName, Namespace: Namespace}
			createdDBUser = &rdsv1alpha1.DBUser{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbUserLookupKey, createdDBUser)
				if err != nil {
					c <- false
					return false
				}
				return true
			}, timeout, interval).Should(BeFalse())

			By("Waiting for DBUser to be deleted")
			Expect(<-c).To(BeFalse())

			By("Validating Delete (DROP) Database operation occurred")
			Eventually(func() error {
				return mock.ExpectationsWereMet()
			}, timeout, interval).Should(Succeed())

			close(done)
		}, overallTimeout)

		// >>>>> DBInstance | MySQL | UseIAMAuthentication <<<<<
		It("Should create a user with IAM Authentication in a MySQL database", func(done Done) {
			DBInstanceName := fmt.Sprintf("%s-mysql-iam", DBInstanceBaseName)
			DBUserName := fmt.Sprintf("%s-mysql-iam", DBUserBaseName)
			PasswordSecretName := fmt.Sprintf("%s-mysql-iam", PasswordBaseSecretName)

			c := make(chan bool, 2)

			By("Creating a new Password Secret")
			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Immutable: boolRef(true),
				StringData: map[string]string{
					PasswordSecretKey: PasswordSecretValue,
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			secretKeyReference := &ackv1alpha1.SecretKeyReference{
				SecretReference: corev1.SecretReference{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Key: PasswordSecretKey,
			}

			secretLookupKey := types.NamespacedName{Name: PasswordSecretName, Namespace: Namespace}
			createdSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, secretLookupKey, createdSecret)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Creating a new DBInstance")
			dbInstance := &rdstypes.DBInstance{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws/v1alpha1",
					Kind:       "DBInstance",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBInstanceName,
					Namespace: Namespace,
				},
				Spec: rdstypes.DBInstanceSpec{
					DBInstanceIdentifier: strRef(DBInstanceName),
					Engine:               strRef("mysql"),
					DBInstanceClass:      strRef("db.m4.large"),
					MasterUsername:       strRef("root"),
					MasterUserPassword:   secretKeyReference,
				},
				Status: rdstypes.DBInstanceStatus{
					Conditions: []*ackv1alpha1.Condition{
						&ackv1alpha1.Condition{
							Type:   ackv1alpha1.ConditionTypeResourceSynced,
							Status: "True",
						},
					},
					ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
						ARN:            (*ackv1alpha1.AWSResourceName)(strRef("AWS-Resource-Name-12345")),
						OwnerAccountID: (*ackv1alpha1.AWSAccountID)(strRef("1234567890")),
					},
					Endpoint: &rdstypes.Endpoint{
						Address:      strRef("test-hostname"),
						HostedZoneID: strRef("test-hostedzoneid"),
						Port:         int64Ref(3306),
					},
				},
			}
			Expect(k8sClient.Create(ctx, dbInstance)).Should(Succeed())

			// Fake that the rds-controller is actually active in the k8s cluster
			Expect(k8sClient.Status().Update(context.TODO(), dbInstance)).Should(Succeed())

			dbInstanceLookupKey := types.NamespacedName{Name: DBInstanceName, Namespace: Namespace}
			createdDBInstance := &rdstypes.DBInstance{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbInstanceLookupKey, createdDBInstance)
				if err != nil {
					return false
				}
				if createdDBInstance.Status.Endpoint == nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for Password Secret to exist")
			Expect(<-c).To(BeTrue())

			By("Waiting for DBInstance to exist and have an active Endpoint")
			Expect(<-c).To(BeTrue())

			db, mock, err := sqlmock.New()
			Expect(err).Should(Succeed())

			testDB = db

			mock.ExpectExec("CREATE USER.+IDENTIFIED WITH AWSAuthenticationPlugin.+").WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(ExpectedGrantStatementSQL).WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))

			By("Creating a new DBUser with UseIAMAuthentication")
			dbUser := &rdsv1alpha1.DBUser{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws.adobe.io/v1alpha1",
					Kind:       "DBUser",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBUserName,
					Namespace: Namespace,
				},
				Spec: rdsv1alpha1.DBUserSpec{
					DBInstanceIdentifier: strRef(DBInstanceName),
					Username:             strRef("test-username"),
					UseIAMAuthentication: boolRef(true),
					GrantStatement:       strRef("GRANT ALL PRIVILEGES ON `%`.* TO ?"),
				},
			}
			Expect(k8sClient.Create(ctx, dbUser)).Should(Succeed())

			dbUserLookupKey := types.NamespacedName{Name: DBUserName, Namespace: Namespace}
			createdDBUser := &rdsv1alpha1.DBUser{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbUserLookupKey, createdDBUser)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for DBUser to be created")
			Expect(<-c).To(BeTrue())

			By("Validating expected Database operations occurred")
			Eventually(func() error {
				return mock.ExpectationsWereMet()
			}, timeout, interval).Should(Succeed())

			close(done)
		}, overallTimeout)

		// >>>>> Postgres <<<<<

		// >>>>> DBInstance | Postgres | Password <<<<<
		It("Should create a user with a password in a Postgres database", func(done Done) {
			DBInstanceName := fmt.Sprintf("%s-postgres-password", DBInstanceBaseName)
			DBUserName := fmt.Sprintf("%s-postgres-password", DBUserBaseName)
			PasswordSecretName := fmt.Sprintf("%s-postgres-password", PasswordBaseSecretName)

			c := make(chan bool, 2)

			By("Creating a new Password Secret")
			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Immutable: boolRef(true),
				StringData: map[string]string{
					PasswordSecretKey: PasswordSecretValue,
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			secretKeyReference := &ackv1alpha1.SecretKeyReference{
				SecretReference: corev1.SecretReference{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Key: PasswordSecretKey,
			}

			secretLookupKey := types.NamespacedName{Name: PasswordSecretName, Namespace: Namespace}
			createdSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, secretLookupKey, createdSecret)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Creating a new DBInstance")
			dbInstance := &rdstypes.DBInstance{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws/v1alpha1",
					Kind:       "DBInstance",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBInstanceName,
					Namespace: Namespace,
				},
				Spec: rdstypes.DBInstanceSpec{
					DBInstanceIdentifier: strRef(DBInstanceName),
					Engine:               strRef("postgres"),
					DBInstanceClass:      strRef("db.m4.large"),
					MasterUsername:       strRef("root"),
					MasterUserPassword:   secretKeyReference,
				},
				Status: rdstypes.DBInstanceStatus{
					Conditions: []*ackv1alpha1.Condition{
						&ackv1alpha1.Condition{
							Type:   ackv1alpha1.ConditionTypeResourceSynced,
							Status: "True",
						},
					},
					ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
						ARN:            (*ackv1alpha1.AWSResourceName)(strRef("AWS-Resource-Name-12345")),
						OwnerAccountID: (*ackv1alpha1.AWSAccountID)(strRef("1234567890")),
					},
					Endpoint: &rdstypes.Endpoint{
						Address:      strRef("test-hostname"),
						HostedZoneID: strRef("test-hostedzoneid"),
						Port:         int64Ref(3306),
					},
				},
			}
			Expect(k8sClient.Create(ctx, dbInstance)).Should(Succeed())

			// Fake that the rds-controller is actually active in the k8s cluster
			Expect(k8sClient.Status().Update(context.TODO(), dbInstance)).Should(Succeed())

			dbInstanceLookupKey := types.NamespacedName{Name: DBInstanceName, Namespace: Namespace}
			createdDBInstance := &rdstypes.DBInstance{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbInstanceLookupKey, createdDBInstance)
				if err != nil {
					return false
				}
				if createdDBInstance.Status.Endpoint == nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for Password Secret to exist")
			Expect(<-c).To(BeTrue())

			By("Waiting for DBInstance to exist and have an active Endpoint")
			Expect(<-c).To(BeTrue())

			db, mock, err := sqlmock.New()
			Expect(err).Should(Succeed())

			testDB = db

			mock.ExpectExec("CREATE USER.+WITH PASSWORD").WithArgs(DBUserUsername, PasswordSecretValue).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(ExpectedGrantStatementSQL).WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))

			By("Creating a new DBUser with a password")
			dbUser := &rdsv1alpha1.DBUser{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws.adobe.io/v1alpha1",
					Kind:       "DBUser",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBUserName,
					Namespace: Namespace,
				},
				Spec: rdsv1alpha1.DBUserSpec{
					DBInstanceIdentifier: strRef(DBInstanceName),
					Username:             strRef("test-username"),
					Password:             secretKeyReference,
					GrantStatement:       strRef(GrantStatement),
				},
			}
			Expect(k8sClient.Create(ctx, dbUser)).Should(Succeed())

			dbUserLookupKey := types.NamespacedName{Name: DBUserName, Namespace: Namespace}
			createdDBUser := &rdsv1alpha1.DBUser{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbUserLookupKey, createdDBUser)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for DBUser to be created")
			Expect(<-c).To(BeTrue())

			By("Validating expected Database operations occurred")
			Eventually(func() error {
				return mock.ExpectationsWereMet()
			}, timeout, interval).Should(Succeed())

			close(done)
		}, overallTimeout)

		// >>>>> DBInstance | Postgres | UseIAMAuthentication <<<<<
		It("Should create a user with IAM Authentication in a Postgres database", func(done Done) {
			DBInstanceName := fmt.Sprintf("%s-postgres-iam", DBInstanceBaseName)
			DBUserName := fmt.Sprintf("%s-postgres-iam", DBUserBaseName)
			PasswordSecretName := fmt.Sprintf("%s-postgres-iam", PasswordBaseSecretName)

			c := make(chan bool, 2)

			By("Creating a new Password Secret")
			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Immutable: boolRef(true),
				StringData: map[string]string{
					PasswordSecretKey: PasswordSecretValue,
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			secretKeyReference := &ackv1alpha1.SecretKeyReference{
				SecretReference: corev1.SecretReference{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Key: PasswordSecretKey,
			}

			secretLookupKey := types.NamespacedName{Name: PasswordSecretName, Namespace: Namespace}
			createdSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, secretLookupKey, createdSecret)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Creating a new DBInstance")
			dbInstance := &rdstypes.DBInstance{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws/v1alpha1",
					Kind:       "DBInstance",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBInstanceName,
					Namespace: Namespace,
				},
				Spec: rdstypes.DBInstanceSpec{
					DBInstanceIdentifier: strRef(DBInstanceName),
					Engine:               strRef("postgres"),
					DBInstanceClass:      strRef("db.m4.large"),
					MasterUsername:       strRef("root"),
					MasterUserPassword:   secretKeyReference,
				},
				Status: rdstypes.DBInstanceStatus{
					Conditions: []*ackv1alpha1.Condition{
						&ackv1alpha1.Condition{
							Type:   ackv1alpha1.ConditionTypeResourceSynced,
							Status: "True",
						},
					},
					ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
						ARN:            (*ackv1alpha1.AWSResourceName)(strRef("AWS-Resource-Name-12345")),
						OwnerAccountID: (*ackv1alpha1.AWSAccountID)(strRef("1234567890")),
					},
					Endpoint: &rdstypes.Endpoint{
						Address:      strRef("test-hostname"),
						HostedZoneID: strRef("test-hostedzoneid"),
						Port:         int64Ref(3306),
					},
				},
			}
			Expect(k8sClient.Create(ctx, dbInstance)).Should(Succeed())

			// Fake that the rds-controller is actually active in the k8s cluster
			Expect(k8sClient.Status().Update(context.TODO(), dbInstance)).Should(Succeed())

			dbInstanceLookupKey := types.NamespacedName{Name: DBInstanceName, Namespace: Namespace}
			createdDBInstance := &rdstypes.DBInstance{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbInstanceLookupKey, createdDBInstance)
				if err != nil {
					return false
				}
				if createdDBInstance.Status.Endpoint == nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for Password Secret to exist")
			Expect(<-c).To(BeTrue())

			By("Waiting for DBInstance to exist and have an active Endpoint")
			Expect(<-c).To(BeTrue())

			db, mock, err := sqlmock.New()
			Expect(err).Should(Succeed())

			testDB = db

			mock.ExpectExec("CREATE USER.+WITH LOGIN.+").WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("GRANT rds_iam TO").WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(ExpectedGrantStatementSQL).WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))

			By("Creating a new DBUser with UseIAMAuthentication")
			dbUser := &rdsv1alpha1.DBUser{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws.adobe.io/v1alpha1",
					Kind:       "DBUser",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBUserName,
					Namespace: Namespace,
				},
				Spec: rdsv1alpha1.DBUserSpec{
					DBInstanceIdentifier: strRef(DBInstanceName),
					Username:             strRef("test-username"),
					UseIAMAuthentication: boolRef(true),
					GrantStatement:       strRef("GRANT ALL PRIVILEGES ON `%`.* TO ?"),
				},
			}
			Expect(k8sClient.Create(ctx, dbUser)).Should(Succeed())

			dbUserLookupKey := types.NamespacedName{Name: DBUserName, Namespace: Namespace}
			createdDBUser := &rdsv1alpha1.DBUser{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbUserLookupKey, createdDBUser)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for DBUser to be created")
			Expect(<-c).To(BeTrue())

			By("Validating expected Database operations occurred")
			Eventually(func() error {
				return mock.ExpectationsWereMet()
			}, timeout, interval).Should(Succeed())

			close(done)
		}, overallTimeout)
	})

	// >>>>> DBCluster <<<<<
	Context("CRUD using DBClusterIdentifier", func() {
		ctx := context.Background()

		// >>>>> MySQL <<<<<

		// >>>>> DBCluster | MySQL | Password <<<<<
		It("Should create a user with a password in a MySQL database", func(done Done) {
			DBClusterName := fmt.Sprintf("%s-aurora-mysql-password", DBClusterBaseName)
			DBUserName := fmt.Sprintf("%s-aurora-mysql-password", DBUserBaseName)
			PasswordSecretName := fmt.Sprintf("%s-aurora-mysql-password", PasswordBaseSecretName)

			c := make(chan bool, 2)

			By("Creating a new Password Secret")
			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Immutable: boolRef(true),
				StringData: map[string]string{
					PasswordSecretKey: PasswordSecretValue,
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			secretKeyReference := &ackv1alpha1.SecretKeyReference{
				SecretReference: corev1.SecretReference{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Key: PasswordSecretKey,
			}

			secretLookupKey := types.NamespacedName{Name: PasswordSecretName, Namespace: Namespace}
			createdSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, secretLookupKey, createdSecret)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Creating a new DBCluster")
			dbCluster := &rdstypes.DBCluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws/v1alpha1",
					Kind:       "DBCluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBClusterName,
					Namespace: Namespace,
				},
				Spec: rdstypes.DBClusterSpec{
					DBClusterIdentifier: strRef(DBClusterName),
					Engine:              strRef("aurora-mysql"),
					MasterUsername:      strRef("root"),
					MasterUserPassword:  secretKeyReference,
				},
				Status: rdstypes.DBClusterStatus{
					Conditions: []*ackv1alpha1.Condition{
						&ackv1alpha1.Condition{
							Type:   ackv1alpha1.ConditionTypeResourceSynced,
							Status: "True",
						},
					},
					ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
						ARN:            (*ackv1alpha1.AWSResourceName)(strRef("AWS-Resource-Name-12345")),
						OwnerAccountID: (*ackv1alpha1.AWSAccountID)(strRef("1234567890")),
					},
					Endpoint: strRef("test-hostname:3306"),
				},
			}
			Expect(k8sClient.Create(ctx, dbCluster)).Should(Succeed())

			// Fake that the rds-controller is actually active in the k8s cluster
			Expect(k8sClient.Status().Update(context.TODO(), dbCluster)).Should(Succeed())

			dbClusterLookupKey := types.NamespacedName{Name: DBClusterName, Namespace: Namespace}
			createdDBCluster := &rdstypes.DBCluster{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbClusterLookupKey, createdDBCluster)
				if err != nil {
					return false
				}
				if createdDBCluster.Status.Endpoint == nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for Password Secret to exist")
			Expect(<-c).To(BeTrue())

			By("Waiting for DBCluster to exist and have an active Endpoint")
			Expect(<-c).To(BeTrue())

			db, mock, err := sqlmock.New()
			Expect(err).Should(Succeed())

			testDB = db

			mock.ExpectExec("CREATE USER.+IDENTIFIED BY").WithArgs(DBUserUsername, PasswordSecretValue).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(ExpectedGrantStatementSQL).WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))

			By("Creating a new DBUser with a password")
			dbUser := &rdsv1alpha1.DBUser{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws.adobe.io/v1alpha1",
					Kind:       "DBUser",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBUserName,
					Namespace: Namespace,
				},
				Spec: rdsv1alpha1.DBUserSpec{
					DBClusterIdentifier: strRef(DBClusterName),
					Username:            strRef("test-username"),
					Password:            secretKeyReference,
					GrantStatement:      strRef(GrantStatement),
				},
			}
			Expect(k8sClient.Create(ctx, dbUser)).Should(Succeed())

			dbUserLookupKey := types.NamespacedName{Name: DBUserName, Namespace: Namespace}
			createdDBUser := &rdsv1alpha1.DBUser{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbUserLookupKey, createdDBUser)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for DBUser to be created")
			Expect(<-c).To(BeTrue())

			By("Validating expected Database operations occurred")
			Eventually(func() error {
				return mock.ExpectationsWereMet()
			}, timeout, interval).Should(Succeed())

			close(done)
		}, overallTimeout)

		// >>>>> DBCluster | MySQL | UseIAMAuthentication <<<<<
		It("Should create a user with IAM Authentication in a MySQL database", func(done Done) {
			DBClusterName := fmt.Sprintf("%s-aurora-mysql-iam", DBClusterBaseName)
			DBUserName := fmt.Sprintf("%s-aurora-mysql-iam", DBUserBaseName)
			PasswordSecretName := fmt.Sprintf("%s-aurora-mysql-iam", PasswordBaseSecretName)

			c := make(chan bool, 2)

			By("Creating a new Password Secret")
			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Immutable: boolRef(true),
				StringData: map[string]string{
					PasswordSecretKey: PasswordSecretValue,
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			secretKeyReference := &ackv1alpha1.SecretKeyReference{
				SecretReference: corev1.SecretReference{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Key: PasswordSecretKey,
			}

			secretLookupKey := types.NamespacedName{Name: PasswordSecretName, Namespace: Namespace}
			createdSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, secretLookupKey, createdSecret)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Creating a new DBCluster")
			dbCluster := &rdstypes.DBCluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws/v1alpha1",
					Kind:       "DBCluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBClusterName,
					Namespace: Namespace,
				},
				Spec: rdstypes.DBClusterSpec{
					DBClusterIdentifier: strRef(DBClusterName),
					Engine:              strRef("aurora-mysql"),
					MasterUsername:      strRef("root"),
					MasterUserPassword:  secretKeyReference,
				},
				Status: rdstypes.DBClusterStatus{
					Conditions: []*ackv1alpha1.Condition{
						&ackv1alpha1.Condition{
							Type:   ackv1alpha1.ConditionTypeResourceSynced,
							Status: "True",
						},
					},
					ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
						ARN:            (*ackv1alpha1.AWSResourceName)(strRef("AWS-Resource-Name-12345")),
						OwnerAccountID: (*ackv1alpha1.AWSAccountID)(strRef("1234567890")),
					},
					Endpoint: strRef("test-hostname:3306"),
				},
			}
			Expect(k8sClient.Create(ctx, dbCluster)).Should(Succeed())

			// Fake that the rds-controller is actually active in the k8s cluster
			Expect(k8sClient.Status().Update(context.TODO(), dbCluster)).Should(Succeed())

			dbClusterLookupKey := types.NamespacedName{Name: DBClusterName, Namespace: Namespace}
			createdDBCluster := &rdstypes.DBCluster{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbClusterLookupKey, createdDBCluster)
				if err != nil {
					return false
				}
				if createdDBCluster.Status.Endpoint == nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for Password Secret to exist")
			Expect(<-c).To(BeTrue())

			By("Waiting for DBCluster to exist and have an active Endpoint")
			Expect(<-c).To(BeTrue())

			db, mock, err := sqlmock.New()
			Expect(err).Should(Succeed())

			testDB = db

			mock.ExpectExec("CREATE USER.+IDENTIFIED WITH AWSAuthenticationPlugin.+").WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(ExpectedGrantStatementSQL).WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))

			By("Creating a new DBUser with UseIAMAuthentication")
			dbUser := &rdsv1alpha1.DBUser{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws.adobe.io/v1alpha1",
					Kind:       "DBUser",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBUserName,
					Namespace: Namespace,
				},
				Spec: rdsv1alpha1.DBUserSpec{
					DBClusterIdentifier:  strRef(DBClusterName),
					Username:             strRef("test-username"),
					UseIAMAuthentication: boolRef(true),
					GrantStatement:       strRef("GRANT ALL PRIVILEGES ON `%`.* TO ?"),
				},
			}
			Expect(k8sClient.Create(ctx, dbUser)).Should(Succeed())

			dbUserLookupKey := types.NamespacedName{Name: DBUserName, Namespace: Namespace}
			createdDBUser := &rdsv1alpha1.DBUser{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbUserLookupKey, createdDBUser)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for DBUser to be created")
			Expect(<-c).To(BeTrue())

			By("Validating expected Database operations occurred")
			Eventually(func() error {
				return mock.ExpectationsWereMet()
			}, timeout, interval).Should(Succeed())

			close(done)
		}, overallTimeout)

		// >>>>> Postgres <<<<<

		// >>>>> DBCluster | Postgres | Password <<<<<
		It("Should create a user with a password in a Postgres database", func(done Done) {
			DBClusterName := fmt.Sprintf("%s-aurora-postgresql-password", DBClusterBaseName)
			DBUserName := fmt.Sprintf("%s-aurora-postgresql-password", DBUserBaseName)
			PasswordSecretName := fmt.Sprintf("%s-aurora-postgresql-password", PasswordBaseSecretName)

			c := make(chan bool, 2)

			By("Creating a new Password Secret")
			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Immutable: boolRef(true),
				StringData: map[string]string{
					PasswordSecretKey: PasswordSecretValue,
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			secretKeyReference := &ackv1alpha1.SecretKeyReference{
				SecretReference: corev1.SecretReference{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Key: PasswordSecretKey,
			}

			secretLookupKey := types.NamespacedName{Name: PasswordSecretName, Namespace: Namespace}
			createdSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, secretLookupKey, createdSecret)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Creating a new DBCluster")
			dbCluster := &rdstypes.DBCluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws/v1alpha1",
					Kind:       "DBCluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBClusterName,
					Namespace: Namespace,
				},
				Spec: rdstypes.DBClusterSpec{
					DBClusterIdentifier: strRef(DBClusterName),
					Engine:              strRef("aurora-postgresql"),
					MasterUsername:      strRef("root"),
					MasterUserPassword:  secretKeyReference,
				},
				Status: rdstypes.DBClusterStatus{
					Conditions: []*ackv1alpha1.Condition{
						&ackv1alpha1.Condition{
							Type:   ackv1alpha1.ConditionTypeResourceSynced,
							Status: "True",
						},
					},
					ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
						ARN:            (*ackv1alpha1.AWSResourceName)(strRef("AWS-Resource-Name-12345")),
						OwnerAccountID: (*ackv1alpha1.AWSAccountID)(strRef("1234567890")),
					},
					Endpoint: strRef("test-hostname:3306"),
				},
			}
			Expect(k8sClient.Create(ctx, dbCluster)).Should(Succeed())

			// Fake that the rds-controller is actually active in the k8s cluster
			Expect(k8sClient.Status().Update(context.TODO(), dbCluster)).Should(Succeed())

			dbClusterLookupKey := types.NamespacedName{Name: DBClusterName, Namespace: Namespace}
			createdDBCluster := &rdstypes.DBCluster{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbClusterLookupKey, createdDBCluster)
				if err != nil {
					return false
				}
				if createdDBCluster.Status.Endpoint == nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for Password Secret to exist")
			Expect(<-c).To(BeTrue())

			By("Waiting for DBCluster to exist and have an active Endpoint")
			Expect(<-c).To(BeTrue())

			db, mock, err := sqlmock.New()
			Expect(err).Should(Succeed())

			testDB = db

			mock.ExpectExec("CREATE USER.+WITH PASSWORD").WithArgs(DBUserUsername, PasswordSecretValue).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(ExpectedGrantStatementSQL).WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))

			By("Creating a new DBUser with a password")
			dbUser := &rdsv1alpha1.DBUser{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws.adobe.io/v1alpha1",
					Kind:       "DBUser",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBUserName,
					Namespace: Namespace,
				},
				Spec: rdsv1alpha1.DBUserSpec{
					DBClusterIdentifier: strRef(DBClusterName),
					Username:            strRef("test-username"),
					Password:            secretKeyReference,
					GrantStatement:      strRef(GrantStatement),
				},
			}
			Expect(k8sClient.Create(ctx, dbUser)).Should(Succeed())

			dbUserLookupKey := types.NamespacedName{Name: DBUserName, Namespace: Namespace}
			createdDBUser := &rdsv1alpha1.DBUser{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbUserLookupKey, createdDBUser)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for DBUser to be created")
			Expect(<-c).To(BeTrue())

			By("Validating expected Database operations occurred")
			Eventually(func() error {
				return mock.ExpectationsWereMet()
			}, timeout, interval).Should(Succeed())

			close(done)
		}, overallTimeout)

		// >>>>> DBCluster | Postgres | UseIAMAuthentication <<<<<
		It("Should create a user with IAM Authentication in a Postgres database", func(done Done) {
			DBClusterName := fmt.Sprintf("%s-aurora-postgresql-iam", DBClusterBaseName)
			DBUserName := fmt.Sprintf("%s-aurora-postgresql-iam", DBUserBaseName)
			PasswordSecretName := fmt.Sprintf("%s-aurora-postgresql-iam", PasswordBaseSecretName)

			c := make(chan bool, 2)

			By("Creating a new Password Secret")
			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Immutable: boolRef(true),
				StringData: map[string]string{
					PasswordSecretKey: PasswordSecretValue,
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			secretKeyReference := &ackv1alpha1.SecretKeyReference{
				SecretReference: corev1.SecretReference{
					Name:      PasswordSecretName,
					Namespace: Namespace,
				},
				Key: PasswordSecretKey,
			}

			secretLookupKey := types.NamespacedName{Name: PasswordSecretName, Namespace: Namespace}
			createdSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, secretLookupKey, createdSecret)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Creating a new DBCluster")
			dbCluster := &rdstypes.DBCluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws/v1alpha1",
					Kind:       "DBCluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBClusterName,
					Namespace: Namespace,
				},
				Spec: rdstypes.DBClusterSpec{
					DBClusterIdentifier: strRef(DBClusterName),
					Engine:              strRef("aurora-postgresql"),
					MasterUsername:      strRef("root"),
					MasterUserPassword:  secretKeyReference,
				},
				Status: rdstypes.DBClusterStatus{
					Conditions: []*ackv1alpha1.Condition{
						&ackv1alpha1.Condition{
							Type:   ackv1alpha1.ConditionTypeResourceSynced,
							Status: "True",
						},
					},
					ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
						ARN:            (*ackv1alpha1.AWSResourceName)(strRef("AWS-Resource-Name-12345")),
						OwnerAccountID: (*ackv1alpha1.AWSAccountID)(strRef("1234567890")),
					},
					Endpoint: strRef("test-hostname:3306"),
				},
			}
			Expect(k8sClient.Create(ctx, dbCluster)).Should(Succeed())

			// Fake that the rds-controller is actually active in the k8s cluster
			Expect(k8sClient.Status().Update(context.TODO(), dbCluster)).Should(Succeed())

			dbClusterLookupKey := types.NamespacedName{Name: DBClusterName, Namespace: Namespace}
			createdDBCluster := &rdstypes.DBCluster{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbClusterLookupKey, createdDBCluster)
				if err != nil {
					return false
				}
				if createdDBCluster.Status.Endpoint == nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for Password Secret to exist")
			Expect(<-c).To(BeTrue())

			By("Waiting for DBCluster to exist and have an active Endpoint")
			Expect(<-c).To(BeTrue())

			db, mock, err := sqlmock.New()
			Expect(err).Should(Succeed())

			testDB = db

			mock.ExpectExec("CREATE USER.+WITH LOGIN.+").WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("GRANT rds_iam TO").WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(ExpectedGrantStatementSQL).WithArgs(DBUserUsername).WillReturnResult(sqlmock.NewResult(1, 1))

			By("Creating a new DBUser with UseIAMAuthentication")
			dbUser := &rdsv1alpha1.DBUser{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rds.services.k8s.aws.adobe.io/v1alpha1",
					Kind:       "DBUser",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DBUserName,
					Namespace: Namespace,
				},
				Spec: rdsv1alpha1.DBUserSpec{
					DBClusterIdentifier:  strRef(DBClusterName),
					Username:             strRef("test-username"),
					UseIAMAuthentication: boolRef(true),
					GrantStatement:       strRef("GRANT ALL PRIVILEGES ON `%`.* TO ?"),
				},
			}
			Expect(k8sClient.Create(ctx, dbUser)).Should(Succeed())

			dbUserLookupKey := types.NamespacedName{Name: DBUserName, Namespace: Namespace}
			createdDBUser := &rdsv1alpha1.DBUser{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dbUserLookupKey, createdDBUser)
				if err != nil {
					return false
				}
				c <- true
				return true
			}, timeout, interval).Should(BeTrue())

			By("Waiting for DBUser to be created")
			Expect(<-c).To(BeTrue())

			By("Validating expected Database operations occurred")
			Eventually(func() error {
				return mock.ExpectationsWereMet()
			}, timeout, interval).Should(Succeed())

			close(done)
		}, overallTimeout)
	})
})

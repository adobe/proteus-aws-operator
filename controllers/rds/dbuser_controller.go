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

package rds

import (
	"context"
	"errors"
	"fmt"

	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v4" // postgres

	corev1 "k8s.io/api/core/v1"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/ctrlutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"

	rdstypes "github.com/aws-controllers-k8s/rds-controller/apis/v1alpha1"

	rdsv1alpha1 "github.com/adobe-platform/proteus-aws-operator/apis/rds/v1alpha1"
)

const dbUserFinalizer = "rds.services.k8s.aws.adobe.io/finalizer"

type DB struct {
	DB     *sql.DB
	Engine rdsv1alpha1.Engine
}

// DBUserReconciler reconciles a DBUser object
type DBUserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=rds.services.k8s.aws.adobe.io,resources=dbusers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rds.services.k8s.aws.adobe.io,resources=dbusers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rds.services.k8s.aws.adobe.io,resources=dbusers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *DBUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx).WithValues("dbuser", req.NamespacedName)

	log.Info("Reconciling DBUser")

	// Fetch the DBUser instance
	dbUser := &rdsv1alpha1.DBUser{}
	err := r.Get(ctx, req.NamespacedName, dbUser)
	if err != nil {
		if k8serr.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			log.Info("DBUser resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get DBUser.")
		return ctrl.Result{}, err
	}

	// Get the DB instance
	db, err := r.getDB(log, ctx, dbUser)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer db.DB.Close()

	// Invalid user input, don't keep trying to reconcile
	if db == nil {
		return ctrl.Result{}, nil
	}

	// Should we delete this DBUser?
	if dbUser.GetDeletionTimestamp() != nil {
		if ctrlutil.ContainsFinalizer(dbUser, dbUserFinalizer) {
			err := r.finalizeDBUser(log, ctx, dbUser, db)
			if err != nil {
				return ctrl.Result{}, err
			}

			// Remove dbUserFinalizer.
			ctrlutil.RemoveFinalizer(dbUser, dbUserFinalizer)
			err = r.Update(ctx, dbUser)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Initialize user (create/grant permissions)
	r.initializeDBUser(log, ctx, dbUser, db)

	// Add finalizer for this DBUser
	if !ctrlutil.ContainsFinalizer(dbUser, dbUserFinalizer) {
		ctrlutil.AddFinalizer(dbUser, dbUserFinalizer)
		err = r.Update(ctx, dbUser)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *DBUserReconciler) getDB(log logr.Logger, ctx context.Context, dbUser *rdsv1alpha1.DBUser) (*DB, error) {
	var driver string
	var engine rdsv1alpha1.Engine
	var dsn string
	var password string

	// Driver / Engine
	if *dbUser.Spec.Engine == "mysql" || *dbUser.Spec.Engine == "mariadb" {
		driver = "mysql"
		engine = rdsv1alpha1.MySQL
	} else if *dbUser.Spec.Engine == "postgres" {
		driver = "pgx"
		engine = rdsv1alpha1.Postgres
	} else {
		errMsg := "Invalid database engine type. Must be: mariadb, mysql, postgres"
		err := errors.New(errMsg)
		log.Error(err, errMsg, "engine", *dbUser.Spec.Engine)
		// Return err=nil here so we don't keep trying to reconcile
		return nil, nil
	}

	if dbUser.Spec.DBInstanceIdentifier != nil {
		dbInstance := &rdstypes.DBInstance{}
		err := r.Get(ctx, types.NamespacedName{Name: *dbUser.Spec.DBInstanceIdentifier, Namespace: dbUser.Namespace}, dbInstance)
		if err != nil {
			log.Error(err, "Failed to find DBInstance", "DBInstanceIdentifier", *dbUser.Spec.DBInstanceIdentifier)
			return nil, err
		}

		password, err = r.SecretValueFromReference(ctx, dbInstance.Spec.MasterUserPassword)
		if err != nil {
			log.Error(err, "Failed to retrieve MasterUserPassword secret", "secret", *dbInstance.Spec.MasterUserPassword)
			return nil, err
		}

		dsn = fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/",
			*dbInstance.Spec.MasterUsername,
			password,
			*dbInstance.Status.Endpoint.Address,
			*dbInstance.Status.Endpoint.Port)
	} else if dbUser.Spec.DBClusterIdentifier != nil {
		dbCluster := &rdstypes.DBCluster{}
		err := r.Get(ctx, types.NamespacedName{Name: *dbUser.Spec.DBClusterIdentifier, Namespace: dbUser.Namespace}, dbCluster)
		if err != nil {
			log.Error(err, "Failed to find DBCluster", "DBClusterIdentifier", *dbUser.Spec.DBClusterIdentifier)
			return nil, err
		}

		password, err = r.SecretValueFromReference(ctx, dbCluster.Spec.MasterUserPassword)
		if err != nil {
			log.Error(err, "Failed to retrieve MasterUserPassword secret", "secret", *dbCluster.Spec.MasterUserPassword)
			return nil, err
		}

		dsn = fmt.Sprintf(
			"postgres://%s:%s@%s/",
			*dbCluster.Spec.MasterUsername,
			password,
			*dbCluster.Status.Endpoint)
	} else {
		errMsg := "Must specify DBInstanceIdentifier or DBClusterIdentifier"
		err := errors.New(errMsg)
		log.Error(err, "Must specify DBInstanceIdentifier or DBClusterIdentifier")
		// Return err=nil here so we don't keep trying to reconcile
		return nil, nil
	}

	db, err := sql.Open(driver, dsn)

	if err != nil {
		errMsg := "Failed to create DB instance"
		err := errors.New(errMsg)
		log.Error(err, errMsg)
		// Return err=nil here so we don't keep trying to reconcile
		return nil, err
	}

	return &DB{DB: db, Engine: engine}, nil
}

func (r *DBUserReconciler) initializeDBUser(log logr.Logger, ctx context.Context, dbUser *rdsv1alpha1.DBUser, db *DB) error {
	var err error = nil
	var result sql.Result

	// Create User
	if dbUser.Spec.Password != nil {
		password, err := r.SecretValueFromReference(ctx, dbUser.Spec.Password)
		if err != nil {
			log.Error(err, "Failed to retrieve Password secret", "secret", *dbUser.Spec.Password)
			return err
		}

		if db.Engine == rdsv1alpha1.MySQL {
			result, err = db.DB.ExecContext(ctx, "CREATE USER ? IDENTIFIED BY ?;", *dbUser.Spec.Username, password)
		} else if db.Engine == rdsv1alpha1.Postgres {
			result, err = db.DB.ExecContext(ctx, "CREATE USER ? WITH PASSWORD ?;", *dbUser.Spec.Username, password)
		} else {
			errMsg := "Unknown engine type"
			err := errors.New(errMsg)
			log.Error(err, errMsg, "engine", db.Engine)
			return err
		}
	} else if dbUser.Spec.UseIAMAuthentication != nil {
		if db.Engine == rdsv1alpha1.MySQL {
			result, err = db.DB.ExecContext(ctx, "CREATE USER ? IDENTIFIED WITH AWSAuthenticationPlugin AS 'RDS';", *dbUser.Spec.Username)
		} else if db.Engine == rdsv1alpha1.Postgres {
			result, err = db.DB.ExecContext(ctx, "CREATE USER ? WITH LOGIN;", *dbUser.Spec.Username)
		} else {
			errMsg := "Unknown engine type"
			err := errors.New(errMsg)
			log.Error(err, errMsg, "engine", db.Engine)
			return err
		}
	} else {
		errMsg := "Must specify Password or UseIAMAuthentication=true"
		err := errors.New(errMsg)
		log.Error(err, errMsg, "user", *dbUser.Spec.Username)
		return err
	}

	if err != nil {
		log.Error(err, "Failed to create user", "user", *dbUser.Spec.Username)
		return err
	}

	// Check if the user was created
	rows, err := result.RowsAffected()
	if err != nil {
		log.Error(err, "Failed to determine if the user was created or not", "user", *dbUser.Spec.Username)
		return err
	}

	// IAM support for Postgres
	if rows != 0 && dbUser.Spec.UseIAMAuthentication != nil && db.Engine == rdsv1alpha1.Postgres {
		result, err = db.DB.ExecContext(ctx, "GRANT rds_iam TO ?;", *dbUser.Spec.Username)

		if err != nil {
			log.Error(err, "Failed to grant IAM permissions to user", "user", *dbUser.Spec.Username)
			return err
		}
	}

	// Apply GRANT statement
	if rows != 0 || *dbUser.Spec.ApplyGrantWhenExists {
		result, err = db.DB.ExecContext(ctx, "", *dbUser.Spec.Username)

		if err != nil {
			log.Error(err, "Failed to apply specified GRANT statement", "user", *dbUser.Spec.Username)
			return err
		}

		rows, err := result.RowsAffected()

		if err != nil {
			log.Error(err, "Failed to determine if the user was created or not", "user", *dbUser.Spec.Username)
			return err
		}

		if rows == 0 {
			errMsg := "GRANT statement didn't apply properly"
			err := errors.New(errMsg)
			log.Error(err, errMsg, "user", *dbUser.Spec.Username)
			return err
		}
	}

	return nil
}

func (r *DBUserReconciler) finalizeDBUser(log logr.Logger, ctx context.Context, dbUser *rdsv1alpha1.DBUser, db *DB) error {
	// All database engines use the same DROP USER command
	_, err := db.DB.ExecContext(ctx, "DROP USER ?;", *dbUser.Spec.Username)

	if err != nil {
		log.Error(err, "Failed to delete user", "user", *dbUser.Spec.Username)
		return err
	}

	return nil
}

// Copied from ACK pkg/runtime/reconciler:
// https://github.com/aws-controllers-k8s/runtime/blob/055b089c6a508317ad4bb57ce037dc55061e1829/pkg/runtime/reconciler.go#L88
func (r *DBUserReconciler) SecretValueFromReference(
	ctx context.Context,
	ref *ackv1alpha1.SecretKeyReference,
) (string, error) {

	if ref == nil {
		return "", nil
	}

	namespace := ref.Namespace
	if namespace == "" {
		namespace = "default"
	}

	nsn := client.ObjectKey{
		Namespace: namespace,
		Name:      ref.Name,
	}
	var secret corev1.Secret
	if err := r.Get(ctx, nsn, &secret); err != nil {
		return "", ackerr.SecretNotFound
	}

	// Currently we have only Opaque secrets in scope.
	if secret.Type != corev1.SecretTypeOpaque {
		return "", ackerr.SecretTypeNotSupported
	}

	if value, ok := secret.Data[ref.Key]; ok {
		valuestr := string(value)
		return valuestr, nil
	}

	return "", ackerr.SecretNotFound
}

// SetupWithManager sets up the controller with the Manager.
func (r *DBUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rdsv1alpha1.DBUser{}).
		Complete(r)
}

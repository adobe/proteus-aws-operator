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
	"errors"
	"fmt"
	"strings"

	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v4" // postgres

	corev1 "k8s.io/api/core/v1"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"

	rdstypes "github.com/aws-controllers-k8s/rds-controller/apis/v1alpha1"

	rdsv1alpha1 "github.com/adobe/proteus-aws-operator/apis/rds/v1alpha1"
)

const dbUserFinalizer = "rds.services.k8s.aws.adobe.io/finalizer"

type DB struct {
	DB         *sql.DB
	AllowClose bool
	Engine     rdsv1alpha1.Engine
}

func (db *DB) Close() error {
	if db.AllowClose {
		return db.DB.Close()
	} else {
		return nil
	}
}

var testDB map[string]*sql.DB = make(map[string]*sql.DB)

// DBUserReconciler reconciles a DBUser object
type DBUserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func createLogError(log logr.Logger, errMsg string, keysAndValues ...interface{}) error {
	err := errors.New(errMsg)
	log.Error(err, errMsg, keysAndValues...)
	return err
}

func sqlEscape(param string) string {
	dest := make([]byte, 0, 2*len(param))
	var escape byte
	for i := 0; i < len(param); i++ {
		c := param[i]

		escape = 0

		switch c {
		case 0: /* Must be escaped for 'mysql' */
			escape = '0'
			break
		case '\n': /* Must be escaped for logs */
			escape = 'n'
			break
		case '\r':
			escape = 'r'
			break
		case '\\':
			escape = '\\'
			break
		case '\'':
			escape = '\''
			break
		case '"': /* Better safe than sorry */
			escape = '"'
			break
		case '\032': /* This gives problems on Win32 */
			escape = 'Z'
		}

		if escape != 0 {
			dest = append(dest, '\\', escape)
		} else {
			dest = append(dest, c)
		}
	}

	return string(dest)
}

//+kubebuilder:rbac:groups=rds.services.k8s.aws.adobe.io,resources=dbusers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rds.services.k8s.aws.adobe.io,resources=dbusers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rds.services.k8s.aws.adobe.io,resources=dbusers/finalizers,verbs=update
//+kubebuilder:rbac:groups=rds.services.k8s.aws,resources=dbclusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=rds.services.k8s.aws,resources=dbinstances,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *DBUserReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx).WithValues("dbuser", req.NamespacedName)

	log.Info("Reconciling DBUser")

	// Fetch the DBUser instance
	dbUser := &rdsv1alpha1.DBUser{}
	err := r.Get(ctx, req.NamespacedName, dbUser)
	if err != nil {
		if k8serr.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			log.Info("DBUser resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get DBUser")
		return ctrl.Result{}, err
	}

	// Get the DB instance
	log.V(1).Info("Getting DB object")

	db, err := r.getDB(log, ctx, dbUser)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Invalid user input, don't keep trying to reconcile
	if db == nil {
		return ctrl.Result{}, nil
	}

	defer db.Close()

	// Should we create/delete this DBUser?
	if dbUser.GetDeletionTimestamp() == nil {
		// Initialize user (create/grant permissions)
		log.V(1).Info("Adding DBUser")
		err = r.initializeDBUser(log, ctx, dbUser, db)

		if err != nil {
			log.Error(err, "Failed to add DBUser")
			return ctrl.Result{}, err
		}

		// Add finalizer for this DBUser
		if !ctrlutil.ContainsFinalizer(dbUser, dbUserFinalizer) {
			ctrlutil.AddFinalizer(dbUser, dbUserFinalizer)
			err = r.Update(ctx, dbUser)
			if err != nil {
				log.Error(err, "Failed to add finalizer to DBUser")
				return ctrl.Result{}, err
			}
		}
	} else {
		log.V(1).Info("Deleting DBUser")

		if ctrlutil.ContainsFinalizer(dbUser, dbUserFinalizer) {
			err = r.finalizeDBUser(log, ctx, dbUser, db)
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
	}

	return ctrl.Result{}, nil
}

// getDB will return an opened database/sql database connection given the Kubernetes user specifications (Engine/MasterUsername/MasterUserPassword)
func (r *DBUserReconciler) getDB(
	log logr.Logger,
	ctx context.Context,
	dbUser *rdsv1alpha1.DBUser,
) (*DB, error) {
	var db *sql.DB

	var allowDBClose bool

	var err error

	var driver string

	var dsn string

	var engine string
	var engineType rdsv1alpha1.Engine

	var endpoint string

	var port *int64

	var username string
	var password string

	int64Ref := func(i int64) *int64 {
		return &i
	}

	if dbUser.Spec.DBInstanceIdentifier != nil {
		log.V(1).Info("Retrieving DB information from DBInstance")

		dbInstance := &rdstypes.DBInstance{}
		err = r.Get(ctx, types.NamespacedName{Name: *dbUser.Spec.DBInstanceIdentifier, Namespace: dbUser.Namespace}, dbInstance)
		if err != nil {
			log.Error(err, "Failed to find DBInstance", "DBInstanceIdentifier", *dbUser.Spec.DBInstanceIdentifier)
			return nil, err
		}

		instanceLog := log.WithValues("DBInstance", *dbInstance.Spec.DBInstanceIdentifier)

		instanceLog.V(1).Info("Validating DBInstance")

		if dbInstance.Spec.Engine == nil {
			createLogError(instanceLog, "DBInstance has no engine specification")
			// Return err=nil here so we don't keep trying to reconcile
			return nil, nil
		}

		if dbInstance.Spec.MasterUsername == nil {
			createLogError(instanceLog, "DBInstance has no MasterUsername specification")
			// Return err=nil here so we don't keep trying to reconcile
			return nil, nil
		}

		if dbInstance.Spec.MasterUserPassword == nil {
			createLogError(instanceLog, "DBInstance has no MasterUserPassword specification")
			// Return err=nil here so we don't keep trying to reconcile
			return nil, nil
		}

		if dbInstance.Status.Endpoint == nil || dbInstance.Status.Endpoint.Address == nil || dbInstance.Status.Endpoint.Port == nil {
			return nil, createLogError(instanceLog, "DBInstance has no active Endpoint")
		}

		engine = *dbInstance.Spec.Engine
		username = *dbInstance.Spec.MasterUsername
		endpoint = *dbInstance.Status.Endpoint.Address
		port = dbInstance.Status.Endpoint.Port

		password, err = r.SecretValueFromReference(ctx, dbInstance.Spec.MasterUserPassword)
		if err != nil {
			instanceLog.Error(err, "Failed to retrieve MasterUserPassword secret", "secret", *dbInstance.Spec.MasterUserPassword)
			return nil, err
		}
	} else if dbUser.Spec.DBClusterIdentifier != nil {
		log.V(1).Info("Retrieving DB information from DBCluster")
		dbCluster := &rdstypes.DBCluster{}
		err = r.Get(ctx, types.NamespacedName{Name: *dbUser.Spec.DBClusterIdentifier, Namespace: dbUser.Namespace}, dbCluster)
		if err != nil {
			log.Error(err, "Failed to find DBCluster", "DBClusterIdentifier", *dbUser.Spec.DBClusterIdentifier)
			return nil, err
		}

		clusterLog := log.WithValues("DBCluster", *dbCluster.Spec.DBClusterIdentifier)

		if dbCluster.Spec.Engine == nil {
			createLogError(clusterLog, "DBCluster has no engine specification")
			// Return err=nil here so we don't keep trying to reconcile
			return nil, nil
		}

		if dbCluster.Spec.MasterUsername == nil {
			createLogError(clusterLog, "DBCluster has no MasterUsername specification")
			// Return err=nil here so we don't keep trying to reconcile
			return nil, nil
		}

		if dbCluster.Status.Endpoint == nil {
			return nil, createLogError(clusterLog, "DBCluster has no active Endpoint")
		}

		if dbCluster.Spec.MasterUserPassword == nil {
			createLogError(clusterLog, "DBCluster has no MasterUserPassword specification")
			// Return err=nil here so we don't keep trying to reconcile
			return nil, nil
		}

		engine = *dbCluster.Spec.Engine
		username = *dbCluster.Spec.MasterUsername
		endpoint = *dbCluster.Status.Endpoint
		port = dbCluster.Spec.Port

		password, err = r.SecretValueFromReference(ctx, dbCluster.Spec.MasterUserPassword)
		if err != nil {
			clusterLog.Error(err, "Failed to retrieve MasterUserPassword secret", "secret", *dbCluster.Spec.MasterUserPassword)
			return nil, err
		}
	} else {
		createLogError(log, "Must specify DBInstanceIdentifier or DBClusterIdentifier")
		// Return err=nil here so we don't keep trying to reconcile
		return nil, nil
	}

	if engine == "mysql" || engine == "mariadb" || engine == "aurora" || engine == "aurora-mysql" {
		driver = "mysql"
		engineType = rdsv1alpha1.MySQL

		if port == nil {
			port = int64Ref(3306)
		}

		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/", username, password, endpoint, *port)
	} else if engine == "postgres" || engine == "aurora-postgresql" {
		driver = "pgx"
		engineType = rdsv1alpha1.Postgres

		if port == nil {
			port = int64Ref(5432)
		}

		dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/", username, password, endpoint, *port)
	} else {
		createLogError(log, "Invalid database engine type. Must be: mariadb, mysql, postgres, aurora, aurora-mysql, or aurora-postgresql")
		// Return err=nil here so we don't keep trying to reconcile
		return nil, nil
	}

	log.V(1).Info("Connecting to database", "driver", driver, "dsn", strings.Replace(dsn, password, "*****", -1))
	if currentTestDB, ok := testDB[dbUser.Name]; ok {
		// Database mocking for unit testing
		db = currentTestDB
		err = nil
		allowDBClose = false
	} else {
		db, err = sql.Open(driver, dsn)
		allowDBClose = true
	}

	if err != nil {
		log.Error(err, "Failed to create sql.DB instance", "driver", driver, "dsn", dsn)
		return nil, err
	}

	return &DB{DB: db, Engine: engineType, AllowClose: allowDBClose}, nil
}

// initializeDBUser will create and grant permissions for a Database user to match the Kubernetes user request
func (r *DBUserReconciler) initializeDBUser(
	log logr.Logger,
	ctx context.Context,
	dbUser *rdsv1alpha1.DBUser,
	db *DB,
) error {
	var err error = nil

	var userExists int64 = 0
	var userCreated bool = false

	var query string

	userLog := log.WithValues("user", *dbUser.Spec.Username)

	if db.Engine == rdsv1alpha1.MySQL {
		query = "SELECT 1 FROM mysql.user WHERE user=?;"
	} else if db.Engine == rdsv1alpha1.Postgres {
		query = "SELECT 1 FROM pg_roles WHERE rolname=$1"
	} else {
		return createLogError(userLog, "Unknown engine type", "engine", db.Engine)
	}

	err = db.DB.QueryRowContext(ctx, query, *dbUser.Spec.Username).Scan(&userExists)

	if err != nil && err != sql.ErrNoRows {
		userLog.Error(err, "Failed to determine if user exists")
		return err
	}

	// We have to use string replacement and sql escaping below because parameterized queries cannot use
	// variables in place of identifiers (username or password)

	// Check if the user already exists
	if userExists == 1 {
		userLog.V(1).Info("User already exists")
	} else {
		userLog.V(1).Info("Creating user")

		// Create User
		if dbUser.Spec.Password != nil {
			password, err := r.SecretValueFromReference(ctx, dbUser.Spec.Password)
			if err != nil {
				userLog.Error(err, "Failed to retrieve Password secret", "secret", *dbUser.Spec.Password)
				return err
			}

			if db.Engine == rdsv1alpha1.MySQL {
				query = fmt.Sprintf("CREATE USER %s IDENTIFIED BY '%s';", sqlEscape(*dbUser.Spec.Username), sqlEscape(password))
			} else if db.Engine == rdsv1alpha1.Postgres {
				query = fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s';", sqlEscape(*dbUser.Spec.Username), sqlEscape(password))
			} else {
				return createLogError(userLog, "Unknown engine type", "engine", db.Engine)
			}

			_, err = db.DB.ExecContext(ctx, query, password)
		} else if dbUser.Spec.UseIAMAuthentication != nil && *dbUser.Spec.UseIAMAuthentication {
			if db.Engine == rdsv1alpha1.MySQL {
				query = fmt.Sprintf(
					"CREATE USER %s IDENTIFIED WITH AWSAuthenticationPlugin AS 'RDS';", sqlEscape(*dbUser.Spec.Username))
			} else if db.Engine == rdsv1alpha1.Postgres {
				query = fmt.Sprintf("CREATE USER %s WITH LOGIN;", sqlEscape(*dbUser.Spec.Username))
			} else {
				return createLogError(userLog, "Unknown engine type", "engine", db.Engine)
			}

			_, err = db.DB.ExecContext(ctx, query)
		} else {
			createLogError(userLog, "Must specify Password or UseIAMAuthentication=true")
			return nil
		}

		if err != nil && err != sql.ErrNoRows {
			userLog.Error(err, "Failed to create user")
			return err
		}

		// IAM support for Postgres
		if db.Engine == rdsv1alpha1.Postgres && dbUser.Spec.UseIAMAuthentication != nil && *dbUser.Spec.UseIAMAuthentication {
			query = fmt.Sprintf("GRANT rds_iam TO %s;", sqlEscape(*dbUser.Spec.Username))
			_, err = db.DB.ExecContext(ctx, query)

			if err != nil && err != sql.ErrNoRows {
				userLog.Error(err, "Failed to grant IAM permissions to user")
				return err
			}
		}

		userCreated = true
	}

	userLog.V(1).Info("Applying GRANT statement")

	if userCreated || (dbUser.Spec.ApplyGrantWhenExists != nil && *dbUser.Spec.ApplyGrantWhenExists) {
		// Apply GRANT statement
		grantLog := userLog.WithValues("grantStatement", *dbUser.Spec.GrantStatement)

		query = strings.ReplaceAll(*dbUser.Spec.GrantStatement, "?", sqlEscape(*dbUser.Spec.Username))

		_, err = db.DB.ExecContext(ctx, query)

		if err != nil && err != sql.ErrNoRows {
			grantLog.Error(err, "Failed to apply GRANT statement")
			return err
		}
	}

	return nil
}

// finalizeUser will delete Database users as requested by the Kubernetes user
func (r *DBUserReconciler) finalizeDBUser(
	log logr.Logger,
	ctx context.Context,
	dbUser *rdsv1alpha1.DBUser,
	db *DB,
) error {
	// All database engines use the same DROP USER command
	_, err := db.DB.ExecContext(ctx, fmt.Sprintf("DROP USER %s;", sqlEscape(*dbUser.Spec.Username)))

	if err != nil {
		log.Error(err, "Failed to delete user", "user", *dbUser.Spec.Username)
		return err
	}

	return nil
}

// Copied from ACK pkg/runtime/reconciler:
// https://github.com/aws-controllers-k8s/runtime/blob/055b089c6a508317ad4bb57ce037dc55061e1829/pkg/runtime/reconciler.go#L88
// Note, we could not use/import this directly because the ACK runtime uses a different proprietary reconciler struct
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
func (r *DBUserReconciler) SetupWithManager(
	mgr ctrl.Manager,
) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rdsv1alpha1.DBUser{}).
		Complete(r)
}

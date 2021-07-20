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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/jinzhu/copier"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	rds_types "github.com/aws-controllers-k8s/rds-controller/apis/v1alpha1"

	"github.com/go-logr/logr"

	"github.com/adobe-platform/proteus-aws-operator/api/v1alpha1"
)

const DefaultReplicas int = 1

// ActionType is used to determine which CRUD operation is needed for a DBInstance inside a DBInstanceAction
type ActionType int

const (
	ActionCreate ActionType = iota
	ActionUpdate
	ActionDelete
)

// DBInstanceAction contains a DBInstance and a corresponding action to perform on it
type DBInstanceAction struct {
	Action   ActionType
	Instance *rds_types.DBInstance
}

// DBReplicationGroupReconciler reconciles a DBReplicationGroup object
type DBReplicationGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// labelsForDBReplicationGroup returns the labels for selecting the resources for the given DBReplicationGroup
func labelsForDBReplicationGroup(dbReplicationGroup *v1alpha1.DBReplicationGroup) map[string]string {
	return map[string]string{
		"DBReplicationGroupName":   dbReplicationGroup.ObjectMeta.Name,
		"DBInstanceBaseIdentifier": *dbReplicationGroup.Spec.DBInstance.DBInstanceIdentifier,
	}
}

//+kubebuilder:rbac:groups=dbreplicationgroups.rds.services.k8s.aws.adobe.io,resources=dbreplicationgroups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dbreplicationgroups.rds.services.k8s.aws.adobe.io,resources=dbreplicationgroups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dbreplicationgroups.rds.services.k8s.aws.adobe.io,resources=dbreplicationgroups/finalizers,verbs=update
//+kubebuilder:rbac:groups=rds.services.k8s.aws,resources=dbinstances/finalizers,verbs=update
//+kubebuilder:rbac:groups=rds.services.k8s.aws,resources=dbinstances,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *DBReplicationGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	log.Info("Creating or updating DBReplicationGroup", "req", req)

	// Fetch the DBReplicationGroup instance
	dbReplicationGroup := &v1alpha1.DBReplicationGroup{}
	err := r.Get(ctx, req.NamespacedName, dbReplicationGroup)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("DBReplicationGroup resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get DBReplicationGroup")
		return ctrl.Result{}, err
	}

	r.Log = log.WithValues("DBReplicationGroupName", dbReplicationGroup.ObjectMeta.Name)

	r.Log.V(1).Info("Successfully retrieved dbReplicationGroup")

	r.Log.V(1).Info("Fetching current instance map")
	instanceMap := map[string]rds_types.DBInstance{}
	err = r.currentInstanceMap(ctx, dbReplicationGroup, instanceMap)

	if err != nil {
		// This error was already logged inside currentInstanceMap
		return ctrl.Result{}, err
	}
	r.Log.V(1).Info("Successfully retrieved current instance map", "instanceMap", instanceMap)

	r.Log.V(1).Info("Fetching requested instances and corresponding actions")
	instanceActions := []DBInstanceAction{}
	r.requestedInstanceActions(ctx, dbReplicationGroup, instanceMap, instanceActions)
	r.Log.V(1).Info("Successfully retrieved instances and actions")

	var actionStr string

	r.Log.V(1).Info("Performing requested actions on instances")

	for _, instanceAction := range instanceActions {
		if instanceAction.Action == ActionCreate {
			r.Log.V(2).Info("Creating instance", "instance", *instanceAction.Instance)
			err = r.Create(ctx, instanceAction.Instance)
			actionStr = "create"
		} else if instanceAction.Action == ActionUpdate {
			r.Log.V(2).Info("Updating instance", "instance", *instanceAction.Instance)
			err = r.Update(ctx, instanceAction.Instance)
			actionStr = "update"
		} else if instanceAction.Action == ActionDelete {
			r.Log.V(2).Info("Deleting instance", "instance", *instanceAction.Instance)
			err = r.Delete(ctx, instanceAction.Instance)
			actionStr = "delete"
		} else {
			r.Log.Error(
				errors.New("Invalid action"),
				"Action", instanceAction.Action,
				"DBInstance.Namespace", instanceAction.Instance.ObjectMeta.Namespace,
				"DBInstance.Name", instanceAction.Instance.ObjectMeta.Name,
				"DBInstance.DBReplicationGroup", dbReplicationGroup.ObjectMeta.Name,
			)
		}

		if err != nil {
			r.Log.Error(
				err,
				fmt.Sprintf("Failed to %s DBInstance", actionStr),
				"DBInstance.Namespace", instanceAction.Instance.ObjectMeta.Namespace,
				"DBInstance.Name", instanceAction.Instance.ObjectMeta.Name,
				"DBInstance.DBReplicationGroup", dbReplicationGroup.ObjectMeta.Name,
			)

			return ctrl.Result{}, err
		}

		dbReplicationGroup.Status.DBInstances = append(dbReplicationGroup.Status.DBInstances, instanceAction.Instance)

		r.Log.V(2).Info("Successfully performed action on instance", "action", instanceAction.Action, "instance", *instanceAction.Instance)
	}

	r.Log.V(1).Info("Successfully performed actions on all instances")

	dbReplicationGroup.Status.DBInstanceBaseIdentifier = dbReplicationGroup.Spec.DBInstance.DBInstanceIdentifier

	r.Log.Info("Successfully created or updated DBReplicationGroup", "req", req)

	return ctrl.Result{}, nil
}

// currentInstanceMap returns a map of DBInstanceIdentifier to DBInstance of the current instances in the cluster
func (r *DBReplicationGroupReconciler) currentInstanceMap(ctx context.Context, dbReplicationGroup *v1alpha1.DBReplicationGroup, instanceMap map[string]rds_types.DBInstance) error {
	// Get the current DBInstances in the cluster
	r.Log.V(2).Info("Fetching current instances in cluster")
	currentInstances := &rds_types.DBInstanceList{}
	listOpts := []client.ListOption{
		client.InNamespace(dbReplicationGroup.Namespace),
		client.MatchingLabels(labelsForDBReplicationGroup(dbReplicationGroup)),
	}
	err := r.List(ctx, currentInstances, listOpts...)

	if err != nil {
		// Error getting the list of DBInstances - requeue the request.
		r.Log.Error(err, "Failed to get list of DBInstances")
		return err
	}
	r.Log.V(2).Info("Successfully retrieved current instances in cluster")

	r.Log.V(2).Info("Creating current instance map")
	for _, instance := range currentInstances.Items {
		instanceMap[*instance.Spec.DBInstanceIdentifier] = instance
	}
	r.Log.V(2).Info("Successfully created current instance map", "instanceMap", instanceMap)

	return nil
}

// requestedInstanceActions returns a list of DBInstanceAction instances which contain the Instance+Action(create, update, delete) operation needed
func (r *DBReplicationGroupReconciler) requestedInstanceActions(ctx context.Context, dbReplicationGroup *v1alpha1.DBReplicationGroup, instanceMap map[string]rds_types.DBInstance, instanceActions []DBInstanceAction) {
	if dbReplicationGroup.Spec.DBInstance.MultiAZ != nil && *dbReplicationGroup.Spec.DBInstance.MultiAZ {
		r.Log.Info("Warning: MultiAZ should not be set to true. Explicitly setting MultiAZ to false.")
		dbReplicationGroup.Spec.DBInstance.MultiAZ = func() *bool { b := false; return &b }()
	}

	r.Log.V(2).Info("Creating Instance Actions")

	var instance *rds_types.DBInstance
	var dbInstanceID string
	var az_log logr.Logger

	for _, az := range dbReplicationGroup.Spec.AvailabilityZones {
		az_log = r.Log.WithValues("az", *az)

		az_log.V(2).Info("Creating AZ Instances")
		for i := 0; i < *dbReplicationGroup.Spec.NumReplicas; i++ {
			dbInstanceID = fmt.Sprintf("%s-%s-%d", *dbReplicationGroup.Spec.DBInstance.DBInstanceIdentifier, *az, i)

			instance = r.dbInstance(ctx, dbReplicationGroup, dbInstanceID, az)

			// If the instance is in the map, update it if needed
			if currentInstance, ok := instanceMap[dbInstanceID]; ok {
				if !reflect.DeepEqual(instance.Spec, currentInstance.Spec) {
					az_log.V(2).Info("Instance will be updated", "instance", instance)
					instanceActions = append(instanceActions, DBInstanceAction{Action: ActionUpdate, Instance: instance})
				}
				// else; no change for the instance, so do nothing

				// Remove the instance from the map so we don't delete it below
				delete(instanceMap, dbInstanceID)
			} else {
				az_log.V(2).Info("Instance will be created", "instance", instance)
				instanceActions = append(instanceActions, DBInstanceAction{Action: ActionCreate, Instance: instance})
			}
		}
	}

	// Delete any extraneous read replica instances
	for _, instance := range instanceMap {
		// Only Aurora read replica instances have a valid ReadReplicaSourceDBInstanceIdentifier
		if instance.Status.ReadReplicaSourceDBInstanceIdentifier == nil {
			r.Log.V(2).Info("Instance is write instance. Skipping deletion", "instance", instance)
		} else {
			r.Log.V(2).Info("Instance will be deleted", "instance", instance)
			instanceActions = append(instanceActions, DBInstanceAction{Action: ActionDelete, Instance: &instance})
		}
	}

	r.Log.V(2).Info("Successfully created Instance Actions")
}

// dbInstance returns a DBInstance that matches the request from the User
func (r *DBReplicationGroupReconciler) dbInstance(ctx context.Context, dbReplicationGroup *v1alpha1.DBReplicationGroup, dbInstanceID string, az *string) *rds_types.DBInstance {
	r.Log.V(2).Info("Creating new instance", "DBInstanceIdentifier", dbInstanceID)

	spec := rds_types.DBInstanceSpec{}

	copier.Copy(&spec, &dbReplicationGroup.Spec.DBInstance)

	spec.AvailabilityZone = az
	spec.DBInstanceIdentifier = &dbInstanceID

	instance := &rds_types.DBInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbInstanceID,
			Namespace: dbReplicationGroup.Namespace,
			Labels:    labelsForDBReplicationGroup(dbReplicationGroup),
		},
		Spec: spec,
	}

	r.Log.V(2).Info("Created new instance", "DBInstanceIdentifier", dbInstanceID, "instance", *instance)

	// Set DBReplicationGroup instance as the owner and controller
	r.Log.V(2).Info("Adding dbReplicationGroup as instance owner and controller", "DBInstanceIdentifier", dbInstanceID, "instance", *instance)
	ctrl.SetControllerReference(dbReplicationGroup, instance, r.Scheme)
	r.Log.V(2).Info("Added dbReplicationGroup as instance owner and controller", "DBInstanceIdentifier", dbInstanceID, "instance", *instance)

	return instance
}

// SetupWithManager sets up the controller with the Manager.
func (r *DBReplicationGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DBReplicationGroup{}).
		Owns(&rds_types.DBInstance{}).
		Complete(r)
}

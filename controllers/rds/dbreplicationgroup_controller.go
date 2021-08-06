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
	"reflect"

	"github.com/jinzhu/copier"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	rdstypes "github.com/aws-controllers-k8s/rds-controller/apis/v1alpha1"

	"github.com/go-logr/logr"

	"github.com/adobe-platform/proteus-aws-operator/apis/rds/v1alpha1"
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
	Instance *rdstypes.DBInstance
}

// DBReplicationGroupReconciler reconciles a DBReplicationGroup object
type DBReplicationGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// labelsForDBReplicationGroup returns the labels for selecting the resources for the given DBReplicationGroup
func labelsForDBReplicationGroup(
	dbReplicationGroup *v1alpha1.DBReplicationGroup,
) map[string]string {
	return map[string]string{
		"DBReplicationGroupName":   dbReplicationGroup.ObjectMeta.Name,
		"DBInstanceBaseIdentifier": *dbReplicationGroup.Spec.DBInstance.DBInstanceIdentifier,
	}
}

//+kubebuilder:rbac:groups=rds.services.k8s.aws.adobe.io,resources=dbreplicationgroups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rds.services.k8s.aws.adobe.io,resources=dbreplicationgroups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rds.services.k8s.aws.adobe.io,resources=dbreplicationgroups/finalizers,verbs=update
//+kubebuilder:rbac:groups=rds.services.k8s.aws,resources=dbinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rds.services.k8s.aws,resources=dbinstances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *DBReplicationGroupReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	log.Info("Creating or updating DBReplicationGroup", "req", req)

	// Fetch the DBReplicationGroup instance
	dbReplicationGroup := &v1alpha1.DBReplicationGroup{}
	err := r.Get(ctx, req.NamespacedName, dbReplicationGroup)
	if err != nil {
		if k8serr.IsNotFound(err) {
			log.Info("DBReplicationGroup resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get DBReplicationGroup")
		return ctrl.Result{}, err
	}

	log = log.WithValues("DBReplicationGroupName", dbReplicationGroup.ObjectMeta.Name)

	log.V(1).Info("Successfully retrieved dbReplicationGroup")

	log.V(1).Info("Fetching current instance map")
	instanceMap := map[string]rdstypes.DBInstance{}
	err = r.currentInstanceMap(log, ctx, dbReplicationGroup, instanceMap)

	if err != nil {
		// This error was already logged inside currentInstanceMap
		return ctrl.Result{}, err
	}
	log.V(1).Info("Successfully retrieved current instance map")

	log.V(1).Info("Fetching requested instances and corresponding actions")
	instanceActions, instanceIDs := r.requestedInstanceActions(log, ctx, dbReplicationGroup, instanceMap)
	log.V(1).Info("Successfully retrieved instances and actions")

	log.V(1).Info("Performing requested actions on instances")

	var actionStr string

	for _, instanceAction := range instanceActions {
		if instanceAction.Action == ActionCreate {
			log.V(1).Info("Creating instance", "instance", *instanceAction.Instance)
			err = r.Create(ctx, instanceAction.Instance)
			actionStr = "create"
		} else if instanceAction.Action == ActionUpdate {
			log.V(1).Info("Updating instance", "instance", *instanceAction.Instance)
			err = r.Update(ctx, instanceAction.Instance)
			actionStr = "update"
		} else if instanceAction.Action == ActionDelete {
			log.V(1).Info("Deleting instance", "instance", *instanceAction.Instance)
			err = r.Delete(ctx, instanceAction.Instance)
			actionStr = "delete"

			if k8serr.IsNotFound(err) {
				// The object has already been deleted, so ignore this error
				err = nil
			}
		} else {
			log.Error(
				errors.New("Invalid action"),
				"Action", instanceAction.Action,
				"DBInstance.Namespace", instanceAction.Instance.ObjectMeta.Namespace,
				"DBInstance.Name", instanceAction.Instance.ObjectMeta.Name,
				"DBInstance.DBReplicationGroup", dbReplicationGroup.ObjectMeta.Name,
			)
		}

		if err != nil {
			log.Error(
				err,
				fmt.Sprintf("Failed to %s DBInstance", actionStr),
				"DBInstance.Namespace", instanceAction.Instance.ObjectMeta.Namespace,
				"DBInstance.Name", instanceAction.Instance.ObjectMeta.Name,
				"DBInstance.DBReplicationGroup", dbReplicationGroup.ObjectMeta.Name,
			)

			return ctrl.Result{}, err
		}

		log.V(1).Info("Successfully performed action on instance", "action", instanceAction.Action, "instance", *instanceAction.Instance)
	}

	log.V(1).Info("Successfully performed actions on all instances")

	// Update Status
	updateInstanceIds := !reflect.DeepEqual(instanceIDs, dbReplicationGroup.Status.DBInstanceIdentifiers)
	updateBaseInstanceIdentifier := dbReplicationGroup.Status.DBInstanceBaseIdentifier != dbReplicationGroup.Spec.DBInstance.DBInstanceIdentifier
	if updateInstanceIds || updateBaseInstanceIdentifier {
		dbReplicationGroup.Status.DBInstanceBaseIdentifier = dbReplicationGroup.Spec.DBInstance.DBInstanceIdentifier
		dbReplicationGroup.Status.DBInstanceIdentifiers = instanceIDs

		err := r.Status().Update(context.TODO(), dbReplicationGroup)
		if err != nil {
			log.Error(err, "Failed to update DBReplicationGroup status")
			return ctrl.Result{}, err
		}
	}

	log.Info("Successfully created or updated DBReplicationGroup", "req", req)

	return ctrl.Result{}, nil
}

// currentInstanceMap returns a map of DBInstanceIdentifier to DBInstance of the current instances in the cluster
func (r *DBReplicationGroupReconciler) currentInstanceMap(
	log logr.Logger,
	ctx context.Context,
	dbReplicationGroup *v1alpha1.DBReplicationGroup,
	instanceMap map[string]rdstypes.DBInstance,
) error {
	// Get the current DBInstances in the cluster
	log.V(1).Info("Fetching current instances in cluster")
	currentInstances := &rdstypes.DBInstanceList{}
	listOpts := []client.ListOption{
		client.InNamespace(dbReplicationGroup.Namespace),
		client.MatchingLabels(labelsForDBReplicationGroup(dbReplicationGroup)),
	}
	err := r.List(ctx, currentInstances, listOpts...)

	if err != nil {
		// Error getting the list of DBInstances - requeue the request.
		log.Error(err, "Failed to get list of DBInstances")
		return err
	}
	log.V(1).Info("Successfully retrieved current instances in cluster")

	log.V(1).Info("Creating current instance map")
	for _, instance := range currentInstances.Items {
		// Don't include instances that are in the process of being deleted
		if instance.GetDeletionTimestamp() == nil {
			instanceMap[*instance.Spec.DBInstanceIdentifier] = instance
		}
	}
	log.V(1).Info("Successfully created current instance map", "instanceMap", instanceMap)

	return nil
}

// requestedInstanceActions returns a list of DBInstanceAction instances which contain the Instance+Action(create, update, delete) operation
// needed and a list of will-be active instanceIDs
func (r *DBReplicationGroupReconciler) requestedInstanceActions(
	log logr.Logger,
	ctx context.Context,
	dbReplicationGroup *v1alpha1.DBReplicationGroup,
	instanceMap map[string]rdstypes.DBInstance,
) ([]DBInstanceAction, []*string) {
	if dbReplicationGroup.Spec.DBInstance.MultiAZ != nil && *dbReplicationGroup.Spec.DBInstance.MultiAZ {
		log.Info("Warning: MultiAZ should not be set to true. Explicitly setting MultiAZ to false.")
		dbReplicationGroup.Spec.DBInstance.MultiAZ = func() *bool { b := false; return &b }()
	}

	log.V(1).Info("Creating Instance Actions")

	var instanceActions []DBInstanceAction
	var instanceIDs []*string
	var instance *rdstypes.DBInstance
	var dbInstanceID string
	var az_log logr.Logger

	for _, az := range dbReplicationGroup.Spec.AvailabilityZones {
		az_log = log.WithValues("az", *az)

		az_log.V(1).Info("Creating AZ Instances")
		for i := 0; i < *dbReplicationGroup.Spec.NumReplicas; i++ {
			dbInstanceID = fmt.Sprintf("%s-%s-%d", *dbReplicationGroup.Spec.DBInstance.DBInstanceIdentifier, *az, i)

			instance = r.dbInstance(log, ctx, dbReplicationGroup, dbInstanceID, az)

			// If the instance is in the map, update it if needed
			if currentInstance, ok := instanceMap[dbInstanceID]; ok {
				if !reflect.DeepEqual(instance.Spec, currentInstance.Spec) {
					az_log.V(1).Info("Instance will be updated", "instance", instance)
					instanceActions = append(instanceActions, DBInstanceAction{Action: ActionUpdate, Instance: instance})
				}
				// else; no change for the instance, so do nothing

				instanceIDs = append(instanceIDs, instance.Spec.DBInstanceIdentifier)

				// Remove the instance from the map so we don't delete it below
				delete(instanceMap, dbInstanceID)
			} else {
				az_log.V(1).Info("Instance will be created", "instance", instance)
				instanceActions = append(instanceActions, DBInstanceAction{Action: ActionCreate, Instance: instance})
				instanceIDs = append(instanceIDs, instance.Spec.DBInstanceIdentifier)
			}
		}
	}

	// Delete any extraneous read replica instances
	for _, instance := range instanceMap {
		// Only Aurora read replica instances have a valid ReadReplicaSourceDBInstanceIdentifier
		if instance.Status.ReadReplicaSourceDBInstanceIdentifier == nil {
			log.V(1).Info("Instance is write instance. Skipping deletion", "instance", instance)
		} else {
			log.V(1).Info("Instance will be deleted", "instance", instance)
			instanceActions = append(instanceActions, DBInstanceAction{Action: ActionDelete, Instance: &instance})
		}
	}

	log.V(1).Info("Successfully created Instance Actions", "instanceActions", instanceActions)

	return instanceActions, instanceIDs
}

// dbInstance returns a DBInstance that matches the request from the User
func (r *DBReplicationGroupReconciler) dbInstance(
	log logr.Logger,
	ctx context.Context,
	dbReplicationGroup *v1alpha1.DBReplicationGroup,
	dbInstanceID string,
	az *string,
) *rdstypes.DBInstance {
	log.V(1).Info("Creating new instance", "DBInstanceIdentifier", dbInstanceID)

	spec := rdstypes.DBInstanceSpec{}

	copier.Copy(&spec, &dbReplicationGroup.Spec.DBInstance)

	spec.AvailabilityZone = az
	spec.DBInstanceIdentifier = &dbInstanceID

	instance := &rdstypes.DBInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbInstanceID,
			Namespace: dbReplicationGroup.Namespace,
			Labels:    labelsForDBReplicationGroup(dbReplicationGroup),
		},
		Spec: spec,
	}

	log.V(1).Info("Created new instance", "DBInstanceIdentifier", dbInstanceID, "instance", *instance)

	// Set DBReplicationGroup instance as the owner and controller
	ctrl.SetControllerReference(dbReplicationGroup, instance, r.Scheme)

	return instance
}

// SetupWithManager sets up the controller with the Manager.
func (r *DBReplicationGroupReconciler) SetupWithManager(
	mgr ctrl.Manager,
) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DBReplicationGroup{}).
		Owns(&rdstypes.DBInstance{}).
		Complete(r)
}

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
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	appv1 "github.com/redhat-scholars/visitors-operator/api/v1"
)

var log = ctrllog.Log.WithName("controller_visitorsapp")

// VisitorsAppReconciler ctrls a VisitorsApp object
type VisitorsAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=app.redhat-scholars.github.io,resources=visitorsapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=app.redhat-scholars.github.io,resources=visitorsapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=app.redhat-scholars.github.io,resources=visitorsapps/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VisitorsApp object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/ctrl
func (r *VisitorsAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	log.Info("Reconciling VisitorsApp", "Request.Namespace", req.Namespace, "Request.Name", req.Name)

	// Fetch the VisitorsApp instance
	v := &appv1.VisitorsApp{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, v)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after ctrl req.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the req.
		return ctrl.Result{}, err
	}

	var result *ctrl.Result

	// == MySQL ==========
	result, err = r.ensureSecret(req, v, r.mysqlAuthSecret(v))
	if result != nil {
		return *result, err
	}

	result, err = r.ensureDeployment(req, v, r.mysqlDeployment(v))
	if result != nil {
		return *result, err
	}

	result, err = r.ensureService(req, v, r.mysqlService(v))
	if result != nil {
		return *result, err
	}

	mysqlRunning := r.isMysqlUp(v)

	if !mysqlRunning {
		// If MySQL isn't running yet, requeue the ctrl
		// to run again after a delay
		delay := time.Second * time.Duration(5)

		log.Info(fmt.Sprintf("MySQL isn't running, waiting for %s", delay))
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	// == Visitors Backend  ==========
	result, err = r.ensureDeployment(req, v, r.backendDeployment(v))
	if result != nil {
		return *result, err
	}

	result, err = r.ensureService(req, v, r.backendService(v))
	if result != nil {
		return *result, err
	}

	err = r.updateBackendStatus(v)
	if err != nil {
		// Requeue the req if the status could not be updated
		return ctrl.Result{}, err
	}

	result, err = r.handleBackendChanges(v)
	if result != nil {
		return *result, err
	}

	// == Visitors Frontend ==========
	result, err = r.ensureDeployment(req, v, r.frontendDeployment(v))
	if result != nil {
		return *result, err
	}

	result, err = r.ensureService(req, v, r.frontendService(v))
	if result != nil {
		return *result, err
	}

	err = r.updateFrontendStatus(v)
	if err != nil {
		// Requeue the req
		return ctrl.Result{}, err
	}

	result, err = r.handleFrontendChanges(v)
	if result != nil {
		return *result, err
	}

	// == Finish ==========
	// Everything went fine, don't requeue

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VisitorsAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.VisitorsApp{}).
		Complete(r)
}

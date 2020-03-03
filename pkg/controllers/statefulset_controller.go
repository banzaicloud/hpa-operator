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

package controllers

import (
	"context"
	"github.com/banzaicloud/hpa-operator/pkg/stub"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
)

// StatefulSetReconciler reconciles a StatefulSet object
type StatefulSetReconciler struct {
	client  client.Client
	log     logr.Logger
	scheme  *runtime.Scheme
	handler *stub.HPAHandler
}

func NewStatefulsSetReconciler(client client.Client, log logr.Logger, scheme *runtime.Scheme, handler *stub.HPAHandler) *StatefulSetReconciler {
	return &StatefulSetReconciler{
		client:  client,
		log:     log,
		scheme:  scheme,
		handler: handler,
	}
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch

func (r *StatefulSetReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.log.WithValues("statefulset", req.NamespacedName)

	deployment := &appsv1.StatefulSet{}
	err := r.client.Get(ctx, req.NamespacedName, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	r.handler.HandleReplicaSet(ctx, deployment.UID, deployment.Name, deployment.Namespace,
		deployment.Kind, deployment.APIVersion,
		deployment.Annotations, deployment.Spec.Template.Annotations)

	return ctrl.Result{}, nil
}

func (r *StatefulSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.StatefulSet{}).
		Complete(r)
}

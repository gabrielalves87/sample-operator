/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/gabrielalves87/sample-operator/api/v1alpha1"

	"github.com/google/go-cmp/cmp"
)

// AppReconciler reconciles a App object
type AppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=platform.example.com,resources=apps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.example.com,resources=apps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.example.com,resources=apps/finalizers,verbs=update

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the App object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile

func (r *AppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Starting reconciliation", "app", req.NamespacedName)

	app := &platformv1alpha1.App{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("App resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get App resource")
		return ctrl.Result{}, err
	}

	if err := r.reconcileDeployment(ctx, app); err != nil {
		logger.Error(err, "Failed to reconcile Deployment")
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, app); err != nil {
		logger.Error(err, "Failed to reconcile Service")
		return ctrl.Result{}, err
	}

	if err := r.reconcileIngress(ctx, app); err != nil {
		logger.Error(err, "Failed to reconcile Ingress")
		return ctrl.Result{}, err
	}

	if err := r.updateStatus(ctx, app); err != nil {
		logger.Error(err, "Failed to update App status")
	}

	logger.Info("Reconciliation completed successfully")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.App{}, builder.WithPredicates(predicate.Funcs{
			UpdateFunc: func(e event.TypedUpdateEvent[client.Object]) bool {
				log := ctrl.Log.WithName("Predicate Update")
				appOld, oldok := e.ObjectOld.(*platformv1alpha1.App)
				appNew, newok := e.ObjectNew.(*platformv1alpha1.App)
				if !oldok || !newok {
					return false
				}
				specsChanged := !cmp.Equal(appOld.Spec, appNew.Spec)
				if specsChanged {
					diffSpecs := cmp.Diff(appOld.Spec, appNew.Spec)
					log.Info("platform specs update", "app", appNew.Name, "specs", diffSpecs)
				}
				return specsChanged
			},
		})).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Named("app").
		Complete(r)
}

func (r *AppReconciler) reconcileDeployment(ctx context.Context, app *platformv1alpha1.App) error {
	logger := logf.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      app.Name,
		Namespace: app.Namespace,
	}, deployment)

	if err != nil && apierrors.IsNotFound(err) {
		desiredDeployment, err := r.deploymentForApp(app)
		if err != nil {
			return fmt.Errorf("failed to generate deployment spec: %w", err)
		}
		logger.Info("Creating new Deployment",
			"namespace", desiredDeployment.Namespace,
			"name", desiredDeployment.Name)

		if err := r.Create(ctx, desiredDeployment); err != nil {
			return fmt.Errorf("failed to create deployment: %w", err)
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}
	desiredDeployment, err := r.deploymentForApp(app)
	if err != nil {
		return fmt.Errorf("failed to generate deployment spec: %w", err)
	}
	if !cmp.Equal(deployment.Spec.Template.Spec.Containers[0].Image, desiredDeployment.Spec.Template.Spec.Containers[0].Image) ||
		!cmp.Equal(deployment.Spec.Replicas, desiredDeployment.Spec.Replicas) ||
		!cmp.Equal(deployment.Spec.Template.Spec.Containers[0].Ports, desiredDeployment.Spec.Template.Spec.Containers[0].Ports) {
		logger.Info("Updating existing Deployment",
			"namespace", deployment.Namespace,
			"name", deployment.Name)
		deployment.Spec.Template.Spec.Containers[0].Image = desiredDeployment.Spec.Template.Spec.Containers[0].Image
		deployment.Spec.Template.Spec.Containers[0].Ports = desiredDeployment.Spec.Template.Spec.Containers[0].Ports
		deployment.Spec.Replicas = desiredDeployment.Spec.Replicas
		if err := r.Update(ctx, deployment); err != nil {
			return fmt.Errorf("failed to update deployment: %w", err)
		}
	}
	return nil
}

func (r *AppReconciler) reconcileService(ctx context.Context, app *platformv1alpha1.App) error {
	logger := logf.FromContext(ctx)

	serviceName := fmt.Sprintf("service-%s", app.Name)

	service := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      serviceName,
		Namespace: app.Namespace,
	}, service)

	if err != nil && apierrors.IsNotFound(err) {
		desiredService, err := r.serviceForApp(app)
		if err != nil {
			return fmt.Errorf("failed to generate service spec: %w", err)
		}
		logger.Info("Creating new Service",
			"namespace", desiredService.Namespace,
			"name", desiredService.Name)

		if err := r.Create(ctx, desiredService); err != nil {
			return fmt.Errorf("failed to create service: %w", err)
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	desiredService, err := r.serviceForApp(app)
	if err != nil {
		return fmt.Errorf("failed to generate service spec: %w", err)
	}

	if !cmp.Equal(service.Spec.Ports, desiredService.Spec.Ports) ||
		!cmp.Equal(service.Spec.Selector, desiredService.Spec.Selector) {
		logger.Info("Updating existing Service",
			"namespace", service.Namespace,
			"name", service.Name)

		desiredService.Spec.ClusterIP = service.Spec.ClusterIP
		service.Spec = desiredService.Spec

		if err := r.Update(ctx, service); err != nil {
			return fmt.Errorf("failed to update service: %w", err)
		}
	}

	return nil
}

func (r *AppReconciler) reconcileIngress(ctx context.Context, app *platformv1alpha1.App) error {
	logger := logf.FromContext(ctx)

	ingressName := fmt.Sprintf("ingress-%s", app.Name)

	ingress := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      ingressName,
		Namespace: app.Namespace,
	}, ingress)

	if err != nil && apierrors.IsNotFound(err) {
		desiredIngress, err := r.ingressForApp(app)
		if err != nil {
			return fmt.Errorf("failed to generate ingress spec: %w", err)
		}
		logger.Info("Creating new Ingress",
			"namespace", desiredIngress.Namespace,
			"name", desiredIngress.Name)

		if err := r.Create(ctx, desiredIngress); err != nil {
			return fmt.Errorf("failed to create ingress: %w", err)
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to get ingress: %w", err)
	}

	desiredIngress, err := r.ingressForApp(app)
	if err != nil {
		return fmt.Errorf("failed to generate ingress spec: %w", err)
	}
	if !cmp.Equal(ingress.Spec, desiredIngress.Spec) {
		logger.Info("Updating existing Ingress",
			"namespace", ingress.Namespace,
			"name", ingress.Name)

		ingress.Spec = desiredIngress.Spec
		if err := r.Update(ctx, ingress); err != nil {
			return fmt.Errorf("failed to update ingress: %w", err)
		}
	}

	return nil
}

func (r *AppReconciler) updateStatus(ctx context.Context, app *platformv1alpha1.App) error {
	logger := logf.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      app.Name,
		Namespace: app.Namespace,
	}, deployment); err != nil {
		return fmt.Errorf("failed to get deployment for status update: %w", err)
	}

	url := fmt.Sprintf("http://%s%s", app.Spec.Ingress.Host, app.Spec.Ingress.Path)

	statusChanged := false
	if app.Status.AvailableReplicas != deployment.Status.AvailableReplicas {
		app.Status.AvailableReplicas = deployment.Status.AvailableReplicas
		statusChanged = true
	}

	if app.Status.URL != url {
		app.Status.URL = url
		statusChanged = true
	}

	if app.Status.ObservedGeneration != app.Generation {
		app.Status.ObservedGeneration = app.Generation
		statusChanged = true
	}

	if statusChanged {
		logger.Info("Updating App status",
			"availableReplicas", app.Status.AvailableReplicas,
			"url", app.Status.URL)

		if err := r.Status().Update(ctx, app); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}
	}

	return nil
}

// deplymentForApp returns a App Deployment object
func (r *AppReconciler) deploymentForApp(app *platformv1alpha1.App) (*appsv1.Deployment, error) {

	replicas := int32(1)
	if app.Spec.Deploy.Replicas != nil {
		replicas = *app.Spec.Deploy.Replicas
	}

	port := int32(8080)
	if app.Spec.Deploy.ContainerPort != nil {
		port = *app.Spec.Deploy.ContainerPort
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
			Labels:    getLabelsForApp(app),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: getLabelsForApp(app),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: getLabelsForApp(app),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: app.Spec.Deploy.Image,
						Ports: []corev1.ContainerPort{
							{
								Name:          "http",
								ContainerPort: port,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
					},
				},
			},
		},
	}
	err := ctrl.SetControllerReference(app, dep, r.Scheme)
	if err != nil {
		return nil, err
	}
	return dep, nil
}

// getLabelsForApp returns labels for kubernetes objects
func getLabelsForApp(app *platformv1alpha1.App) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       app.Name,
		"app.kubernetes.io/managed-by": "platform-app-operator",
	}

}

// serviceForApp returns a App service object
func (r *AppReconciler) serviceForApp(app *platformv1alpha1.App) (*corev1.Service, error) {

	serviceName := fmt.Sprintf("service-%s", app.Name)

	port := int32(8080)
	if app.Spec.Service.Port == nil {
		port = *app.Spec.Deploy.ContainerPort
	}
	if app.Spec.Service.Port != nil {
		port = *app.Spec.Service.Port
	}
	targetPort := int32(8080)
	if app.Spec.Deploy.ContainerPort != nil {
		targetPort = *app.Spec.Deploy.ContainerPort
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: app.Namespace,
			Labels:    getLabelsForApp(app),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "app",
					Port:       port,
					TargetPort: intstr.FromInt32(targetPort),
					Protocol:   "TCP",
				},
			},
			Selector: getLabelsForApp(app),
		},
	}
	err := ctrl.SetControllerReference(app, svc, r.Scheme)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

// ingressForApp returns a App ingress object
func (r *AppReconciler) ingressForApp(app *platformv1alpha1.App) (*networkingv1.Ingress, error) {

	ingressName := fmt.Sprintf("ingress-%s", app.Name)

	pathTypeImplementationSpecific := networkingv1.PathTypeImplementationSpecific
	path := app.Spec.Ingress.Path
	if path == "" {
		path = "/"
	}
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: app.Namespace,
			Labels:    getLabelsForApp(app),
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				networkingv1.IngressRule{
					Host: app.Spec.Ingress.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								networkingv1.HTTPIngressPath{
									Path:     path,
									PathType: &pathTypeImplementationSpecific,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "service-" + app.Name,
											Port: networkingv1.ServiceBackendPort{
												Name: "app",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	err := ctrl.SetControllerReference(app, ingress, r.Scheme)
	if err != nil {
		return nil, err
	}
	return ingress, nil
}

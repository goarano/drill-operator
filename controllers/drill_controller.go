/*
Copyright 2022.

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
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apachev1alpha1 "github.com/goarano/drill-operator/api/v1alpha1"
)

// DrillReconciler reconciles a Drill object
type DrillReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=apache.goarano.io,resources=drills,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apache.goarano.io,resources=drills/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apache.goarano.io,resources=drills/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Drill object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *DrillReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Memcached instance
	drill := &apachev1alpha1.Drill{}
	err := r.Get(ctx, req.NamespacedName, drill)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("Drill resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Drill")
		return ctrl.Result{}, err
	}

	res, err := r.reconcileServices(ctx, req, drill)
	if !res.IsZero() || err != nil {
		return res, err
	}

	res, err = r.reconcileStatefulSet(ctx, req, drill)
	if !res.IsZero() || err != nil {
		return res, err
	}

	return ctrl.Result{}, nil
}

func (r *DrillReconciler) reconcileServices(ctx context.Context, req ctrl.Request, drill *apachev1alpha1.Drill) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	hsService := r.serviceHs(drill)
	hsServiceNN := types.NamespacedName{Name: hsService.Name, Namespace: hsService.Namespace}
	found := &corev1.Service{}

	err := r.Get(ctx, hsServiceNN, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new headless Service", "Service.Name", hsService.Name)
		err = r.Create(ctx, hsService)
		if err != nil {
			log.Error(err, "Failed to create new headless Service", "Service.Name", hsService.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	if !equality.Semantic.DeepDerivative(hsService.Spec, found.Spec) {
		found.Spec = hsService.Spec
		err = r.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update headless Service", "Service.Name", found.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	csService := r.serviceCs(drill)
	csServiceNN := types.NamespacedName{Name: csService.Name, Namespace: csService.Namespace}
	found = &corev1.Service{}

	err = r.Get(ctx, csServiceNN, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new client Service", "Service.Name", csService.Name)
		err = r.Create(ctx, csService)
		if err != nil {
			log.Error(err, "Failed to create new client Service", "Service.Name", csService.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	if !equality.Semantic.DeepDerivative(csService.Spec, found.Spec) {
		found.Spec = csService.Spec
		err = r.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update client Service", "Service.Name", found.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *DrillReconciler) reconcileStatefulSet(ctx context.Context, req ctrl.Request, drill *apachev1alpha1.Drill) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	set := r.statefulSet(drill)
	setNN := types.NamespacedName{Name: set.Name, Namespace: set.Namespace}
	found := &appsv1.StatefulSet{}

	err := r.Get(ctx, setNN, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new StatefulSet", "StatefulSet.Name", set.Name)
		err = r.Create(ctx, set)
		if err != nil {
			log.Error(err, "Failed to create new StatefulSet", "StatefulSet.Name", set.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get StatefulSet")
		return ctrl.Result{}, err
	}

	if !equality.Semantic.DeepDerivative(set.Spec.Template, found.Spec.Template) || set.Spec.Replicas != found.Spec.Replicas {
		found.Spec.Template = set.Spec.Template
		err = r.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update StatefulSet", "StatefulSet.Name", found.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return ctrl.Result{}, nil
}

func (r *DrillReconciler) labels(drill *apachev1alpha1.Drill) map[string]string {
	return map[string]string{
		"app":                          "drill",
		"app.kubernetes.io/component":  "drill",
		"app.kubernetes.io/instance":   drill.Name,
		"app.kubernetes.io/managed-by": "drill-operator",
		"app.kubernetes.io/name":       "drill",
		//"app.kubernetes.io/part-of": "TODO",
		"app.kubernetes.io/version": drill.Spec.Version,
	}
}

func (r *DrillReconciler) labelsSelector(drill *apachev1alpha1.Drill) map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by": "drill-operator",
		"app.kubernetes.io/component":  "drill",
		"app.kubernetes.io/instance":   drill.Name,
	}
}

func (r *DrillReconciler) serviceCs(drill *apachev1alpha1.Drill) *corev1.Service {
	nn := types.NamespacedName{Name: strings.Join([]string{drill.Name, "cs"}, "-"), Namespace: drill.Namespace}
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "corev1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nn.Name,
			Namespace: nn.Namespace,
			Labels:    r.labels(drill),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http-rest",
					Port:       8047,
					TargetPort: intstr.FromInt(8047),
				},
				{
					Name:       "jdbc",
					Port:       31010,
					TargetPort: intstr.FromInt(31010),
				},
			},
			Selector: r.labelsSelector(drill),
		},
	}
	ctrl.SetControllerReference(drill, service, r.Scheme)
	return service
}

func (r *DrillReconciler) serviceHs(drill *apachev1alpha1.Drill) *corev1.Service {
	nn := types.NamespacedName{Name: strings.Join([]string{drill.ObjectMeta.Name, "hs"}, "-"), Namespace: drill.Namespace}
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "corev1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nn.Name,
			Namespace: nn.Namespace,
			Labels:    r.labels(drill),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http-rest",
					Port:       8047,
					TargetPort: intstr.FromInt(8047),
				},
				{
					Name:       "jdbc",
					Port:       31010,
					TargetPort: intstr.FromInt(31010),
				},
			},
			Selector:  r.labelsSelector(drill),
			ClusterIP: "None",
		},
	}
	ctrl.SetControllerReference(drill, service, r.Scheme)
	return service
}

func (r *DrillReconciler) statefulSet(drill *apachev1alpha1.Drill) *appsv1.StatefulSet {
	nn := types.NamespacedName{Name: drill.Name, Namespace: drill.Namespace}
	var replicas int32 = drill.Spec.Replicas
	var version string = drill.Spec.Version

	set := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps/appsv1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nn.Name,
			Namespace: nn.Namespace,
			Labels:    r.labels(drill),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: r.labelsSelector(drill)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: r.labels(drill)},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:  "drill-override",
							Image: "ubuntu", // TODO replace
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "override",
									MountPath: "/opt/drill/conf",
								},
							},
							Command: []string{
								"bash",
								"-c",
								strings.Join([]string{
									"echo",
									"'",
									fmt.Sprintf(
										strings.Join([]string{
											"drill.exec: {",
											"	cluster-id: \"mydrillcluster\"",
											"	zk.connect: \"%s-cs:2181\"", //TODO use individual servers?
											"}",
										}, "\n"), drill.Spec.Zookeeper),
									"'",
									">",
									"/opt/drill/conf/drill-override.conf",
								}, " "),
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "drill",
							Image: strings.Join([]string{"apache/drill", version}, ":"),
							Ports: []corev1.ContainerPort{
								{
									Name:          "http-rest",
									ContainerPort: 8047,
								},
								{
									Name:          "jdbc",
									ContainerPort: 31010,
								},
							},
							Command: []string{
								"bash",
								"-c",
								strings.Join([]string{
									"cp -f /opt/drill/conf-override/drill-override.conf /opt/drill/conf/",
									"/opt/drill/bin/drillbit.sh start", // TODO
									"while true; do sleep 5; done",
								}, "; "),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadir",
									MountPath: "/data",
								},
								{
									Name:      "override",
									MountPath: "/opt/drill/conf-override",
								},
							},
							ImagePullPolicy: corev1.PullPolicy("Always"),
						},
					},
					Volumes: []corev1.Volume{{
						Name: "override",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "datadir"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.PersistentVolumeAccessMode("ReadWriteOnce")},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName("storage"): resource.MustParse("100Mi"),
							},
						},
					},
				},
			},
			ServiceName:         r.serviceHs(drill).Name,
			PodManagementPolicy: appsv1.PodManagementPolicyType("OrderedReady"),
			UpdateStrategy:      appsv1.StatefulSetUpdateStrategy{Type: appsv1.StatefulSetUpdateStrategyType("RollingUpdate")},
		},
	}

	multiNodeAffinity := &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
		{
			LabelSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "app.kubernetes.io/managed-by",
					Operator: metav1.LabelSelectorOperator("In"),
					Values:   []string{"drill-operator"},
				},
				{
					Key:      "app.kubernetes.io/component",
					Operator: metav1.LabelSelectorOperator("In"),
					Values:   []string{"drill"},
				},
				{
					Key:      "app.kubernetes.io/instance",
					Operator: metav1.LabelSelectorOperator("In"),
					Values:   []string{drill.Name},
				},
			}},
			TopologyKey: "kubernetes.io/hostname",
		},
	}},
	}

	if !drill.Spec.Debug.SingleNode {
		set.Spec.Template.Spec.Affinity = multiNodeAffinity
	}

	ctrl.SetControllerReference(drill, set, r.Scheme)
	return set
}

// SetupWithManager sets up the controller with the Manager.
func (r *DrillReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apachev1alpha1.Drill{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}

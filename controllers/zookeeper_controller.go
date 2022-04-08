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

// ZookeeperReconciler reconciles a Zookeeper object
type ZookeeperReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=apache.goarano.io,resources=zookeepers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apache.goarano.io,resources=zookeepers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apache.goarano.io,resources=zookeepers/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Zookeeper object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *ZookeeperReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Memcached instance
	zookeeper := &apachev1alpha1.Zookeeper{}
	err := r.Get(ctx, req.NamespacedName, zookeeper)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("Zookeeper resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Zookeeper")
		return ctrl.Result{}, err
	}

	res, err := r.reconcileServices(ctx, req, zookeeper)
	if !res.IsZero() || err != nil {
		return res, err
	}

	res, err = r.reconcileStatefulSet(ctx, req, zookeeper)
	if !res.IsZero() || err != nil {
		return res, err
	}

	return ctrl.Result{}, nil
}

func (r *ZookeeperReconciler) reconcileServices(ctx context.Context, req ctrl.Request, zookeper *apachev1alpha1.Zookeeper) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	hsService := r.serviceHs(zookeper)
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

	csService := r.serviceCs(zookeper)
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

func (r *ZookeeperReconciler) reconcileStatefulSet(ctx context.Context, req ctrl.Request, zookeper *apachev1alpha1.Zookeeper) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	set := r.statefulSet(zookeper)
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

func (r *ZookeeperReconciler) zookeeperLabels(zookeeper *apachev1alpha1.Zookeeper) map[string]string {
	return map[string]string{
		"app":                          "zookeeper",
		"app.kubernetes.io/component":  "zookeeper",
		"app.kubernetes.io/instance":   zookeeper.Name,
		"app.kubernetes.io/managed-by": "drill-operator",
		"app.kubernetes.io/name":       "zookeeper",
		//"app.kubernetes.io/part-of": "TODO",
		"app.kubernetes.io/version": zookeeper.Spec.Version,
	}
}

func (r *ZookeeperReconciler) zookeeperLabelsSelector(zookeeper *apachev1alpha1.Zookeeper) map[string]string {
	return map[string]string{
		"app":                        "zookeeper",
		"app.kubernetes.io/instance": zookeeper.Name,
	}
}

func (r *ZookeeperReconciler) serviceHs(zookeeper *apachev1alpha1.Zookeeper) *corev1.Service {
	nn := types.NamespacedName{Name: strings.Join([]string{zookeeper.ObjectMeta.Name, "hs"}, "-"), Namespace: zookeeper.Namespace}
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "corev1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nn.Name,
			Namespace: nn.Namespace,
			Labels:    r.zookeeperLabels(zookeeper),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "server",
					Port:       2888,
					TargetPort: intstr.FromInt(2888),
				},
				{
					Name:       "leader-election",
					Port:       3888,
					TargetPort: intstr.FromInt(3888),
				},
			},
			Selector:  r.zookeeperLabelsSelector(zookeeper),
			ClusterIP: "None",
		},
	}
	ctrl.SetControllerReference(zookeeper, service, r.Scheme)
	return service
}

func (r *ZookeeperReconciler) serviceCs(zookeeper *apachev1alpha1.Zookeeper) *corev1.Service {
	nn := types.NamespacedName{Name: strings.Join([]string{zookeeper.Name, "cs"}, "-"), Namespace: zookeeper.Namespace}
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "corev1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nn.Name,
			Namespace: nn.Namespace,
			Labels:    r.zookeeperLabels(zookeeper),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "client",
					Port:       2181,
					TargetPort: intstr.FromInt(2181),
				},
			},
			Selector: r.zookeeperLabelsSelector(zookeeper),
		},
	}
	ctrl.SetControllerReference(zookeeper, service, r.Scheme)
	return service
}

func (r *ZookeeperReconciler) statefulSet(zookeeper *apachev1alpha1.Zookeeper) *appsv1.StatefulSet {
	nn := types.NamespacedName{Name: zookeeper.Name, Namespace: zookeeper.Namespace}
	var uid int64 = 1000
	var replicas int32 = zookeeper.Spec.Replicas
	var version string = zookeeper.Spec.Version

	set := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps/appsv1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nn.Name,
			Namespace: nn.Namespace,
			Labels:    r.zookeeperLabels(zookeeper),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: r.zookeeperLabelsSelector(zookeeper)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: r.zookeeperLabels(zookeeper)},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kubernetes-zookeeper",
							Image: strings.Join([]string{"k8s.gcr.io/kubernetes-zookeeper", version}, ":"),
							Command: []string{
								"sh",
								"-c",
								strings.Join([]string{
									"start-zookeeper",
									fmt.Sprintf("--servers=%d", replicas),
									"--data_dir=/var/lib/zookeeper/data",
									"--data_log_dir=/var/lib/zookeeper/data/log",
									"--conf_dir=/opt/zookeeper/conf",
									"--client_port=2181",
									"--election_port=3888",
									"--server_port=2888",
									"--tick_time=2000",
									"--init_limit=10",
									"--sync_limit=5",
									"--heap=512M",
									"--max_client_cnxns=60",
									"--snap_retain_count=3",
									"--purge_interval=12",
									"--max_session_timeout=40000",
									"--min_session_timeout=4000",
									fmt.Sprintf("--log_level=%s", zookeeper.Spec.Debug.LogLevel),
								}, " "),
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "client",
									ContainerPort: 2181,
								},
								{
									Name:          "server",
									ContainerPort: 2888,
								},
								{
									Name:          "leader-election",
									ContainerPort: 3888,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadir",
									MountPath: "/var/lib/zookeeper",
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{
									"sh",
									"-c",
									"zookeeper-ready 2181",
								},
								}},
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{
									"sh",
									"-c",
									"zookeeper-ready 2181",
								},
								}},
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
							},
							ImagePullPolicy: corev1.PullPolicy("Always"),
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser: &uid,
						FSGroup:   &uid,
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "datadir"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.PersistentVolumeAccessMode("ReadWriteOnce")},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName("storage"): resource.MustParse("10Gi"),
							},
						},
					},
				},
			},
			ServiceName:         r.serviceHs(zookeeper).Name,
			PodManagementPolicy: appsv1.PodManagementPolicyType("OrderedReady"),
			UpdateStrategy:      appsv1.StatefulSetUpdateStrategy{Type: appsv1.StatefulSetUpdateStrategyType("RollingUpdate")},
		},
	}

	multiNodeAffinity := &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
		{
			LabelSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "app",
					Operator: metav1.LabelSelectorOperator("In"),
					Values: []string{
						"zk",
					},
				},
			}},
			TopologyKey: "kubernetes.io/hostname",
		},
	}},
	}

	if !zookeeper.Spec.Debug.SingleNode {
		set.Spec.Template.Spec.Affinity = multiNodeAffinity
	}

	ctrl.SetControllerReference(zookeeper, set, r.Scheme)
	return set
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZookeeperReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apachev1alpha1.Zookeeper{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}

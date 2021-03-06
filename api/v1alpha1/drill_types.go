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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DrillSpec defines the desired state of Drill
type DrillSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Number of Replicas to be deployed.
	Replicas int32 `json:"replicas"`
	// Version of Drill.
	// +kubebuilder:default:="1.20.0"
	Version   string `json:"version,omitempty"`
	Zookeeper string `json:"zookeeper"`
	Debug     Debug  `json:"debug,omitempty"`
}

// DrillStatus defines the observed state of Drill
type DrillStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Drill is the Schema for the drills API
type Drill struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DrillSpec   `json:"spec,omitempty"`
	Status DrillStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DrillList contains a list of Drill
type DrillList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Drill `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Drill{}, &DrillList{})
}

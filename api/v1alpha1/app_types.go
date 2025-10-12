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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AppSpec defines the desired state of App.
type AppSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of App. Edit app_types.go to remove/update
	Deploy  AppDeploy  `json:"deploy"`
	Service AppService `json:"service"`
	Ingress AppIngress `json:"ingress"`
}

type AppDeploy struct {
	Image         string `json:"image"`
	Replicas      *int32 `json:"replicas,omitempty"`
	ContainerPort *int32 `json:"containerPort"`
}

type AppService struct {
	TargetPort *int32 `json:"targetPort,omitempty"`
	Port       *int32 `json:"port"`
}

type AppIngress struct {
	Host string `json:"host,omitempty"`
	Path string `json:"path,omitempty"`
}

// AppStatus defines the observed state of App.
type AppStatus struct {
	AvailableReplicas  int32  `json:"availableReplicas,omitempty"`
	URL                string `json:"url,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// App is the Schema for the apps API.
type App struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppSpec   `json:"spec,omitempty"`
	Status AppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AppList contains a list of App.
type AppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []App `json:"items"`
}

func init() {
	SchemeBuilder.Register(&App{}, &AppList{})
}

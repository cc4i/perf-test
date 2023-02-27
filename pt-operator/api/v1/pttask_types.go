/*
Copyright 2023.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PtTaskSpec defines the desired state of PtTask
type PtTaskSpec struct {
	Execution []PtTaskSpecExecution         `json:"execution"`
	Scenarios map[string]PtTaskSpecScenario `json:"scenarios"`
}

type PtTaskSpecExecution struct {
	Executor    string `json:"executor"`
	Concurrency int    `json:"concurrency"`
	HoldFor     string `json:"hold-for"`
	RampUp      string `json:"ramp-up"`
	Iterations  int    `json:"iterations,omitempty"`
	Scenario    string `json:"scenario"`
	Master      string `json:"master,omitempty"`
	Workers     int    `json:"workers,omitempty"`
}
type PtTaskSpecScenario struct {
	DefaultAddress string `json:"default-address"`
	Script         string `json:"script"`
}

// PtTaskStatus defines the observed state of PtTask
type PtTaskStatus struct {

	// Phases include:
	// - "initial",
	// - "privision_master",
	// - "provision_worker",
	// - "testing",
	// - "achievng_logs",
	// - "done"
	Phases map[string]string `json:"phase"`
	Id     string            `json:"id,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PtTask is the Schema for the pttasks API
type PtTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PtTaskSpec   `json:"spec,omitempty"`
	Status PtTaskStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PtTaskList contains a list of PtTask
type PtTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PtTask `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PtTask{}, &PtTaskList{})
}

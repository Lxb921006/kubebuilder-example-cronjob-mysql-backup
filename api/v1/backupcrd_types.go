/*
Copyright 2024.

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
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ConcurrencyPolicy string

const (
	AllowConcurrent ConcurrencyPolicy = "Allow"

	ForbidConcurrent ConcurrencyPolicy = "Forbid"

	ReplaceConcurrent ConcurrencyPolicy = "Replace"
)

// BackupCrdSpec defines the desired state of BackupCrd
type BackupCrdSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Schedule string `json:"schedule"`

	JobTemplate batchv1.JobTemplateSpec `json:"jobTemplate"`

	Replicas *int32 `json:"replicas,omitempty"`

	Suspend *bool `json:"suspend,omitempty"`

	ConcurrencyPolicy ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`

	//+kubebuilder:validation:Minimum=0
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`

	//+kubebuilder:validation:Minimum=0
	FailedJobsHistoryLimit *int32 `json:"failedJobsHistoryLimit,omitempty"`

	//+kubebuilder:validation:Minimum=0
	StartingDeadlineSeconds *int64 `json:"startingDeadlineSeconds,omitempty"`
}

// BackupCrdStatus defines the observed state of BackupCrd
type BackupCrdStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Active []string `json:"active,omitempty"`

	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// BackupCrd is the Schema for the backupcrds API
type BackupCrd struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupCrdSpec   `json:"spec,omitempty"`
	Status BackupCrdStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BackupCrdList contains a list of BackupCrd
type BackupCrdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupCrd `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupCrd{}, &BackupCrdList{})
}

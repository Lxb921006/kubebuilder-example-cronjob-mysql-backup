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
	"github.com/robfig/cron"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var backupcrdlog = logf.Log.WithName("backupcrd-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *BackupCrd) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-mysqlbk-bk-io-v1-backupcrd,mutating=true,failurePolicy=fail,sideEffects=None,groups=mysqlbk.bk.io,resources=backupcrds,verbs=create;update,versions=v1,name=mbackupcrd.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &BackupCrd{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *BackupCrd) Default() {
	backupcrdlog.Info("default", "name", r.Name)
	// TODO(user): fill in your defaulting logic.
	if r.Spec.ConcurrencyPolicy == "" {
		r.Spec.ConcurrencyPolicy = AllowConcurrent
	}
	if r.Spec.Suspend == nil {
		r.Spec.Suspend = new(bool)
	}
	if r.Spec.Replicas == nil {
		r.Spec.Replicas = new(int32)
		*r.Spec.Replicas = 1
	}
	if r.Spec.SuccessfulJobsHistoryLimit == nil {
		r.Spec.SuccessfulJobsHistoryLimit = new(int32)
		*r.Spec.SuccessfulJobsHistoryLimit = 3
	}
	if r.Spec.FailedJobsHistoryLimit == nil {
		r.Spec.FailedJobsHistoryLimit = new(int32)
		*r.Spec.FailedJobsHistoryLimit = 2
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-mysqlbk-bk-io-v1-backupcrd,mutating=false,failurePolicy=fail,sideEffects=None,groups=mysqlbk.bk.io,resources=backupcrds,verbs=create;update,versions=v1,name=vbackupcrd.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &BackupCrd{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *BackupCrd) ValidateCreate() (admission.Warnings, error) {
	backupcrdlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil, r.validateCronJob()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *BackupCrd) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	backupcrdlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil, r.validateCronJob()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *BackupCrd) ValidateDelete() (admission.Warnings, error) {
	backupcrdlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

func (r *BackupCrd) validateCronJob() error {
	var allErrs field.ErrorList

	if err := r.validateCronJobSpec(); err != nil {
		allErrs = append(allErrs, err)
	}
	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "mysqlbk.bk.io", Kind: "BackupCrd"},
		r.Name, allErrs)
}

func (r *BackupCrd) validateCronJobSpec() *field.Error {
	return validateScheduleFormat(
		r.Spec.Schedule,
		field.NewPath("spec").Child("schedule"))
}

func validateScheduleFormat(schedule string, fldPath *field.Path) *field.Error {
	if _, err := cron.ParseStandard(schedule); err != nil {
		return field.Invalid(fldPath, schedule, err.Error())
	}
	return nil
}

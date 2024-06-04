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

package controller

import (
	"context"
	"fmt"
	mysqlbkv1 "github.com/Lxb921006/mysqlBackup/api/v1"
	"github.com/Lxb921006/mysqlBackup/utils"
	"github.com/go-logr/logr"
	"github.com/robfig/cron"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilcache "k8s.io/apimachinery/pkg/util/cache"
	ref "k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sort"
	"time"
)

// BackupCrdReconciler reconciles a BackupCrd object
type BackupCrdReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	utilcache.Clock
	utils.NeedResources
}

//+kubebuilder:rbac:groups=mysqlbk.bk.io,resources=backupcrds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mysqlbk.bk.io,resources=backupcrds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mysqlbk.bk.io,resources=backupcrds/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BackupCrd object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *BackupCrdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logCtr := log.FromContext(ctx)

	// TODO(user): your logic here
	var bk = new(mysqlbkv1.BackupCrd)
	if err := r.Get(ctx, req.NamespacedName, bk); err != nil {
		if errors.IsNotFound(err) {
			logCtr.Error(err, "fail to get BackupCrd batchv1 resource")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, nil
	}

	if err := r.CreateMysqlRefResources(ctx, bk, r.Client, r.Scheme); err != nil {
		logCtr.Error(err, "fail to CreateMysqlRefResources")
		return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
	}

	ifFirstCreateJob, activeJobs, err := r.reconcileJobList(ctx, bk, logCtr)
	if err != nil {
		logCtr.Error(err, "fail to get job list")
		return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
	}

	if bk.Status.LastScheduleTime == nil {
		return ctrl.Result{RequeueAfter: time.Duration(30) * time.Second}, nil
	}

	if ifFirstCreateJob {
		var lastScheduleTime time.Time
		lastScheduleTime, err = time.Parse(time.RFC3339, bk.Status.LastScheduleTime.Time.String())
		if err != nil {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{RequeueAfter: lastScheduleTime.Sub(r.Now())}, nil
	}

	if bk.Spec.Suspend != nil && *bk.Spec.Suspend {
		logCtr.Info("mysql backup cronjob suspended, skipping")
		return ctrl.Result{}, nil
	}

	missionTime, nextTime, _ := r.getNextScheduledTime(bk, r.Now(), logCtr)
	scheduledResult := ctrl.Result{RequeueAfter: nextTime.Sub(r.Now())}

	if missionTime.IsZero() {
		logCtr.Info("unable to obtain the next scheduling time")
		return scheduledResult, nil
	}

	var late bool
	if bk.Spec.StartingDeadlineSeconds != nil {
		late = missionTime.Add(time.Duration(*bk.Spec.StartingDeadlineSeconds) * time.Second).Before(r.Now())
	}

	if late {
		logCtr.Info("missed scheduling time, wait for the next schedule")
		return scheduledResult, nil
	}

	if bk.Spec.ConcurrencyPolicy == mysqlbkv1.ForbidConcurrent && len(activeJobs) > 0 {
		logCtr.Info("forbids concurrent runs, skipping next run if previous run hasn't finished yet", "num active job", len(activeJobs))
		return scheduledResult, nil
	}

	if bk.Spec.ConcurrencyPolicy == mysqlbkv1.ReplaceConcurrent {
		logCtr.Info("cancels currently running job and replaces it with a new one")
		for _, activeJob := range activeJobs {
			if err := r.Delete(ctx, activeJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
				logCtr.Error(err, "unable to delete active job", "job", activeJob)
				return ctrl.Result{}, err
			}
		}
		return scheduledResult, nil
	}

	_, err = r.reconcileJob(ctx, bk, logCtr, missionTime)
	if err != nil {
		return ctrl.Result{}, nil
	}

	return scheduledResult, nil
}

func (r *BackupCrdReconciler) reconcileJobList(ctx context.Context, bk *mysqlbkv1.BackupCrd, logCtr logr.Logger) (bool, []*batchv1.Job, error) {
	var activeJobs []*batchv1.Job
	var successfulJobs []*batchv1.Job
	var failedJobs []*batchv1.Job

	jl := new(batchv1.JobList)
	var lastScheduleTime *time.Time

	if err := r.List(ctx, jl, client.InNamespace(bk.Namespace), client.MatchingFields{jobKey: bk.Name}); err != nil {
		return false, activeJobs, err
	}

	if len(jl.Items) == 0 {
		shed, err := cron.ParseStandard(bk.Spec.Schedule)
		if err != nil {
			return false, activeJobs, err
		}

		nextTime := shed.Next(r.Now())
		for !r.Now().After(nextTime) {
		}

		nextSch := shed.Next(nextTime)
		bk.Status.LastScheduleTime = &metav1.Time{Time: nextSch}
		err = r.Status().Update(ctx, bk)
		if err != nil {
			return false, activeJobs, err
		}

		_, err = r.reconcileJob(ctx, bk, logCtr, nextSch)
		if err != nil {
			return false, activeJobs, err
		}

		return true, activeJobs, nil
	}

	isJobFinished := func(job *batchv1.Job) (bool, batchv1.JobConditionType) {
		for _, c := range job.Status.Conditions {
			if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == corev1.ConditionTrue {
				return true, c.Type
			}
		}

		return false, ""
	}

	getLastScheduleTime := func(job *batchv1.Job) *time.Time {
		jobSchTime := job.Annotations["mysqlbk.bk.io/cronjob-scheduled-timestamp"]
		parse, err := time.Parse(time.RFC3339, jobSchTime)
		if err != nil {
			return &time.Time{}
		}

		return &parse
	}

	sort.Slice(jl.Items, func(i, j int) bool {
		return jl.Items[i].Name < jl.Items[j].Name
	})

	for i, childJob := range jl.Items {
		_, jobStatus := isJobFinished(&childJob)
		switch jobStatus {
		case "":
			activeJobs = append(activeJobs, &jl.Items[i])
		case batchv1.JobFailed:
			failedJobs = append(failedJobs, &jl.Items[i])
		case batchv1.JobComplete:
			successfulJobs = append(successfulJobs, &jl.Items[i])
		}

		lastScheduleTime = getLastScheduleTime(&childJob)
	}

	if lastScheduleTime.IsZero() {
		bk.Status.LastScheduleTime = &metav1.Time{Time: r.Now()}
	} else {
		bk.Status.LastScheduleTime = &metav1.Time{Time: *lastScheduleTime}
	}

	bk.Status.Active = make([]string, 0)
	for _, activeJob := range activeJobs {
		jobRef, err := ref.GetReference(r.Scheme, activeJob)
		if err != nil {
			logCtr.Error(err, "unable to make reference to active job", "job", activeJob)
			continue
		}
		bk.Status.Active = append(bk.Status.Active, jobRef.Name)
	}

	if err := r.Status().Update(ctx, bk); err != nil {
		return false, activeJobs, err
	}

	r.deleteFailedAndSucceedJobs(ctx, bk, failedJobs, successfulJobs, logCtr)

	return false, activeJobs, nil
}

func (r *BackupCrdReconciler) reconcileJob(ctx context.Context, bk *mysqlbkv1.BackupCrd, logCtr logr.Logger, sched time.Time) (*batchv1.Job, error) {
	name := fmt.Sprintf("%s-%d", bk.Name, sched.Unix())
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   bk.Namespace,
			Annotations: make(map[string]string),
		},
		Spec: *bk.Spec.JobTemplate.Spec.DeepCopy(),
	}

	job.Annotations["mysqlbk.bk.io/cronjob-scheduled-timestamp"] = sched.Format(time.RFC3339)

	if err := ctrl.SetControllerReference(bk, job, r.Scheme); err != nil {
		logCtr.Error(err, fmt.Sprintf("fail to set controller reference %s", bk.Name+"-job"))
		return job, err
	}

	if err := r.Create(ctx, job); err != nil {
		logCtr.Error(err, fmt.Sprintf("fail to create job %s", bk.Name+"-job"))
		return job, err
	}

	logCtr.Info("Successfully created job", "job", name)

	return job, nil
}

func (r *BackupCrdReconciler) getNextScheduledTime(bk *mysqlbkv1.BackupCrd, now time.Time, logCtr logr.Logger) (missionTime, nextTime time.Time, err error) {
	var nilTime = time.Time{}
	shed, err := cron.ParseStandard(bk.Spec.Schedule)
	if err != nil {
		return nilTime, nilTime, nil
	}

	var lastScheduleTime time.Time

	if bk.Status.LastScheduleTime != nil {
		lastScheduleTime = bk.Status.LastScheduleTime.Time
	} else {
		lastScheduleTime = bk.CreationTimestamp.Time
	}

	if lastScheduleTime.After(now) {
		return nilTime, shed.Next(now), nil
	}

	for {
		lastScheduleTime = shed.Next(now)
		if lastScheduleTime.After(now) {
			break
		}
	}

	return lastScheduleTime, shed.Next(now), nil
}

func (r *BackupCrdReconciler) deleteFailedAndSucceedJobs(ctx context.Context, bk *mysqlbkv1.BackupCrd, failedJobs, successfulJobs []*batchv1.Job, log logr.Logger) {
	if len(failedJobs) > int(*bk.Spec.FailedJobsHistoryLimit) {
		sort.Slice(failedJobs, func(i, j int) bool {
			if failedJobs[i].Status.StartTime == nil {
				return failedJobs[i].Status.StartTime != nil
			}
			return failedJobs[i].Status.StartTime.Before(failedJobs[j].Status.StartTime)
		})

		for _, job := range failedJobs[:len(failedJobs)-(int(*bk.Spec.FailedJobsHistoryLimit))] {
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
				log.Error(err, "unable to delete old failed job", "job", job.Name)
			} else {
				log.Info("deleted old failed job", "job", job.Name)
			}
		}
	}

	if len(successfulJobs) > int(*bk.Spec.SuccessfulJobsHistoryLimit) {
		sort.Slice(successfulJobs, func(i, j int) bool {
			if successfulJobs[i].Status.StartTime == nil {
				return successfulJobs[i].Status.StartTime != nil
			}
			return successfulJobs[i].Status.StartTime.Before(successfulJobs[j].Status.StartTime)
		})

		for _, job := range successfulJobs[:len(successfulJobs)-(int(*bk.Spec.SuccessfulJobsHistoryLimit))] {
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
				log.Error(err, "unable to delete old successful job", "job", job.Name)
			} else {
				log.Info("deleted old successful job", "job", job.Name)
			}
		}
	}
}

var (
	jobKey   = ".metadata.controller"
	apiGVStr = mysqlbkv1.GroupVersion.String()
)

// SetupWithManager sets up the controller with the Manager.
func (r *BackupCrdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Clock == nil {
		r.Clock = utils.CrdClock{}
	}

	if r.NeedResources == nil {
		r.NeedResources = &utils.CreateResources{}
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, jobKey, func(rawObj client.Object) []string {
		// grab the job object, extract the owner...
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		// ...make sure it's a CronJob...
		if owner.APIVersion != apiGVStr || owner.Kind != "BackupCrd" {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&mysqlbkv1.BackupCrd{}).
		Owns(&batchv1.Job{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

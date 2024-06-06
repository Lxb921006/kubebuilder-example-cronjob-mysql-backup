package utils

import (
	"bytes"
	"context"
	oe "errors"
	"fmt"
	mysqlbkv1 "github.com/Lxb921006/mysqlBackup/api/v1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"os"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"text/template"
	"time"
)

func parseYaml(templateName string, app *mysqlbkv1.BackupCrd) ([]byte, error) {
	tmpDir := "/utils/template"
	dir, err := os.ReadDir(tmpDir)
	if err != nil {
		return []byte{}, err
	}

	var yamlFile string

	for _, v := range dir {
		if v.Name() == templateName+".yaml" {
			yamlFile = filepath.Join(tmpDir, v.Name())
			break
		}
	}

	file, err := os.ReadFile(yamlFile)
	if err != nil {
		return []byte{}, err
	}

	tmpl, err := template.New(templateName).Parse(string(file))
	if err != nil {
		return []byte{}, err
	}

	var buf bytes.Buffer

	if err = tmpl.Execute(&buf, app); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

func MysqlState(app *mysqlbkv1.BackupCrd) (*appsv1.StatefulSet, error) {
	resources := new(appsv1.StatefulSet)

	b, err := parseYaml("mysqlState", app)
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(b, resources); err != nil {
		return nil, err
	}

	return resources, nil

}

func MysqlJob(app *mysqlbkv1.BackupCrd) (*batchv1.Job, error) {
	resources := new(batchv1.Job)
	b, err := parseYaml("mysqlBackupJob", app)
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(b, resources); err != nil {
		return nil, err
	}

	return resources, nil
}

func MysqlService(app *mysqlbkv1.BackupCrd) (*corev1.Service, error) {
	resources := new(corev1.Service)
	b, err := parseYaml("mysqlService", app)
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(b, resources); err != nil {
		return nil, err
	}

	return resources, nil
}

func MysqlSecret(app *mysqlbkv1.BackupCrd) (*corev1.Secret, error) {
	resources := new(corev1.Secret)
	b, err := parseYaml("mysqlSecret", app)
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(b, resources); err != nil {
		return nil, err
	}

	return resources, nil
}

func MysqlPv(app *mysqlbkv1.BackupCrd) (*corev1.PersistentVolume, error) {
	resources := new(corev1.PersistentVolume)
	b, err := parseYaml("mysqlBackupPv", app)
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(b, resources); err != nil {
		return nil, err
	}

	return resources, nil
}

func MysqlPvc(app *mysqlbkv1.BackupCrd) (*corev1.PersistentVolumeClaim, error) {
	resources := new(corev1.PersistentVolumeClaim)
	b, err := parseYaml("mysqlBackupPvc", app)
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(b, resources); err != nil {
		return nil, err
	}

	return resources, nil
}

type controllerConfig struct {
	ctx    context.Context
	bk     *mysqlbkv1.BackupCrd
	r      client.Client
	schema *runtime.Scheme
	log    logr.Logger
}

type CreateResources struct {
}

func (c *CreateResources) CreateMysqlRefResources(ctx context.Context, bk *mysqlbkv1.BackupCrd, r client.Client, schema *runtime.Scheme) error {
	logger := log.FromContext(ctx)
	cr := &controllerConfig{
		ctx:    ctx,
		bk:     bk,
		r:      r,
		schema: schema,
		log:    logger,
	}

	if err := c.createPv(cr); err != nil {
		return err
	}

	if err := c.createPvc(cr); err != nil {
		return err
	}

	if err := c.createSecret(cr); err != nil {
		return err
	}

	if err := c.createSvc(cr); err != nil {
		return err
	}

	if err := c.createState(cr); err != nil {
		return err
	}

	return nil
}

func (c *CreateResources) createSecret(cr *controllerConfig) error {
	resources := new(corev1.Secret)
	if err := cr.r.Get(cr.ctx, types.NamespacedName{Namespace: cr.bk.Namespace, Name: cr.bk.Name + "-secret"}, resources); err != nil {
		if errors.IsNotFound(err) {
			resources, err = MysqlSecret(cr.bk)
			if err != nil {
				return err
			}

			if err := ctrl.SetControllerReference(cr.bk, resources, cr.schema); err != nil {
				return err
			}

			if err := cr.r.Create(cr.ctx, resources); err != nil {
				return err
			}

			return nil
		}

		return err
	}

	return nil
}

func (c *CreateResources) createPv(cr *controllerConfig) error {
	resources := new(corev1.PersistentVolume)
	if err := cr.r.Get(cr.ctx, types.NamespacedName{Name: cr.bk.Name + "-pv"}, resources); err != nil {
		if errors.IsNotFound(err) {
			resources, err = MysqlPv(cr.bk)
			if err != nil {
				return err
			}

			if err = cr.r.Create(cr.ctx, resources); err != nil {
				return err
			}

			return nil
		}

		return err
	}

	return nil
}

func (c *CreateResources) createPvc(cr *controllerConfig) error {
	resources := new(corev1.PersistentVolumeClaim)
	if err := cr.r.Get(cr.ctx, types.NamespacedName{Namespace: cr.bk.Namespace, Name: cr.bk.Name + "-pvc"}, resources); err != nil {
		if errors.IsNotFound(err) {
			resources, err = MysqlPvc(cr.bk)
			if err != nil {
				return err
			}

			if err := cr.r.Create(cr.ctx, resources); err != nil {
				return err
			}

			return nil
		}

		return err
	}

	return nil
}

func (c *CreateResources) createState(cr *controllerConfig) error {
	resources := new(appsv1.StatefulSet)
	if err := cr.r.Get(cr.ctx, types.NamespacedName{Namespace: cr.bk.Namespace, Name: cr.bk.Name + "-state"}, resources); err != nil {
		if errors.IsNotFound(err) {
			resources, err = MysqlState(cr.bk)
			if err != nil {
				return err
			}

			if err := ctrl.SetControllerReference(cr.bk, resources, cr.schema); err != nil {
				return err
			}

			if err := cr.r.Create(cr.ctx, resources); err != nil {
				return err
			}

			if err := c.statesPodCheck(cr, resources); err != nil {
				return err
			}

			return nil
		}

		return err
	}

	if err := c.statesPodCheck(cr, resources); err != nil {
		return err
	}

	return nil
}

func (c *CreateResources) statesPodCheck(cr *controllerConfig, resources *appsv1.StatefulSet) error {
	if *resources.Spec.Replicas != *cr.bk.Spec.Replicas {
		*resources.Spec.Replicas = *cr.bk.Spec.Replicas
		if err := cr.r.Update(cr.ctx, resources); err != nil {
			cr.log.Error(err, "fail to update StatefulSet resource", cr.bk.Name+"-state")
			return err
		}
		cr.log.Info("StatefulSet resource change", "StatefulSet", cr.bk.Name+"-state")
	}

	podList := new(corev1.PodList)
	if err := cr.r.List(cr.ctx, podList, client.InNamespace(cr.bk.Namespace), client.MatchingLabels{"app": cr.bk.Name + "-app"}); err != nil {
		return err
	}

	if len(podList.Items) == 0 {
		return oe.New(fmt.Sprintf("waiting create StatefulSet pod resource %s-state", cr.bk.Name))
	}

	var check = len(podList.Items)

	for _, v := range podList.Items {
		if !v.Status.ContainerStatuses[0].Ready {
			check -= 1
		}
	}

	if check <= 0 {
		return oe.New(fmt.Sprintf("waiting statefulSet %s-state`s pod ready", cr.bk.Name))
	}

	return nil

}

func (c *CreateResources) createSvc(cr *controllerConfig) error {
	resources := new(corev1.Service)
	if err := cr.r.Get(cr.ctx, types.NamespacedName{Namespace: cr.bk.Namespace, Name: cr.bk.Name + "-svc"}, resources); err != nil {
		if errors.IsNotFound(err) {
			resources, err = MysqlService(cr.bk)
			if err != nil {
				return err
			}

			if err := ctrl.SetControllerReference(cr.bk, resources, cr.schema); err != nil {
				return err
			}

			if err := cr.r.Create(cr.ctx, resources); err != nil {
				return err
			}

			return nil
		}

		return err
	}

	return nil
}

type NeedResources interface {
	CreateMysqlRefResources(ctx context.Context, bk *mysqlbkv1.BackupCrd, r client.Client, schema *runtime.Scheme) error
}

type CrdClock struct {
}

func (_ CrdClock) Now() time.Time {
	return time.Now()
}

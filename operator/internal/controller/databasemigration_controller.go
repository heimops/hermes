package controller

import (
	"context"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	hermesio "hermes.io/operator/api/v1alpha1"
)

// DatabaseMigrationReconciler reconciles DatabaseMigration objects.
type DatabaseMigrationReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	DefaultImage string
}

func (r *DatabaseMigrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var dm hermesio.DatabaseMigration
	if err := r.Get(ctx, req.NamespacedName, &dm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reconcileErr := r.sync(ctx, &dm)

	if err := r.updateStatus(ctx, &dm, reconcileErr); err != nil {
		log.FromContext(ctx).Error(err, "Failed to update status")
	}

	return ctrl.Result{}, reconcileErr
}

func (r *DatabaseMigrationReconciler) sync(ctx context.Context, dm *hermesio.DatabaseMigration) error {
	switch dm.Spec.Mode {
	case "schedule":
		return r.syncCronJob(ctx, dm)
	default: // "once"
		return r.syncJob(ctx, dm)
	}
}

func (r *DatabaseMigrationReconciler) syncJob(ctx context.Context, dm *hermesio.DatabaseMigration) error {
	l := log.FromContext(ctx)
	desired := r.buildJob(dm)
	if err := controllerutil.SetControllerReference(dm, desired, r.Scheme); err != nil {
		return err
	}

	var existing batchv1.Job
	err := r.Get(ctx, client.ObjectKey{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			return fmt.Errorf("create Job: %w", err)
		}
		l.Info("Created Job", "name", desired.Name)
	} else if err != nil {
		return err
	} else {
		l.Info("Job already exists", "name", existing.Name)
	}

	dm.Status.JobRef = desired.Name
	return nil
}

func (r *DatabaseMigrationReconciler) syncCronJob(ctx context.Context, dm *hermesio.DatabaseMigration) error {
	l := log.FromContext(ctx)
	desired := r.buildCronJob(dm)
	if err := controllerutil.SetControllerReference(dm, desired, r.Scheme); err != nil {
		return err
	}

	var existing batchv1.CronJob
	err := r.Get(ctx, client.ObjectKey{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			return fmt.Errorf("create CronJob: %w", err)
		}
		l.Info("Created CronJob", "name", desired.Name)
	} else if err != nil {
		return err
	} else {
		existing.Spec = desired.Spec
		if err := r.Update(ctx, &existing); err != nil {
			return fmt.Errorf("update CronJob: %w", err)
		}
		l.Info("Updated CronJob", "name", existing.Name)
	}

	dm.Status.JobRef = desired.Name
	return nil
}

func (r *DatabaseMigrationReconciler) updateStatus(ctx context.Context, dm *hermesio.DatabaseMigration, syncErr error) error {
	cond := metav1.Condition{
		Type:               hermesio.ConditionReady,
		ObservedGeneration: dm.Generation,
	}

	if syncErr != nil {
		cond.Status = metav1.ConditionFalse
		cond.Reason = hermesio.ReasonSyncFailed
		cond.Message = syncErr.Error()
	} else {
		cond.Status = metav1.ConditionTrue
		if dm.Spec.Mode == "schedule" {
			cond.Reason = hermesio.ReasonCronJobSynced
			cond.Message = fmt.Sprintf("CronJob %s synced successfully", dm.Status.JobRef)
		} else {
			cond.Reason = hermesio.ReasonJobSynced
			cond.Message = fmt.Sprintf("Job %s synced successfully", dm.Status.JobRef)
		}
	}

	apimeta.SetStatusCondition(&dm.Status.Conditions, cond)

	if err := r.Status().Update(ctx, dm); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// buildEnv constructs the environment variables for the migrator container.
func (r *DatabaseMigrationReconciler) buildEnv(dm *hermesio.DatabaseMigration) []corev1.EnvVar {
	opts := dm.Spec.Options
	schema := true
	data := true
	batchSize := int32(1000)
	var tables string

	if opts != nil {
		schema = opts.Schema
		data = opts.Data
		if opts.BatchSize > 0 {
			batchSize = opts.BatchSize
		}
		if len(opts.Tables) > 0 {
			tables = strings.Join(opts.Tables, ",")
		}
	}

	src := dm.Spec.Source
	dst := dm.Spec.Destination

	srcPort := src.Port
	if srcPort == 0 {
		srcPort = defaultPort(src.Type)
	}
	dstPort := dst.Port
	if dstPort == 0 {
		dstPort = defaultPort(dst.Type)
	}

	srcUsernameKey := src.SecretRef.UsernameKey
	if srcUsernameKey == "" {
		srcUsernameKey = "username"
	}
	srcPasswordKey := src.SecretRef.PasswordKey
	if srcPasswordKey == "" {
		srcPasswordKey = "password"
	}
	dstUsernameKey := dst.SecretRef.UsernameKey
	if dstUsernameKey == "" {
		dstUsernameKey = "username"
	}
	dstPasswordKey := dst.SecretRef.PasswordKey
	if dstPasswordKey == "" {
		dstPasswordKey = "password"
	}

	env := []corev1.EnvVar{
		{Name: "HERMES_SOURCE_TYPE", Value: src.Type},
		{Name: "HERMES_SOURCE_HOST", Value: src.Host},
		{Name: "HERMES_SOURCE_PORT", Value: fmt.Sprintf("%d", srcPort)},
		{Name: "HERMES_SOURCE_DATABASE", Value: src.Database},
		{Name: "HERMES_SOURCE_SSL_MODE", Value: src.SSLMode},
		{
			Name: "HERMES_SOURCE_USERNAME",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: src.SecretRef.Name},
					Key:                  srcUsernameKey,
				},
			},
		},
		{
			Name: "HERMES_SOURCE_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: src.SecretRef.Name},
					Key:                  srcPasswordKey,
				},
			},
		},
		{Name: "HERMES_DEST_TYPE", Value: dst.Type},
		{Name: "HERMES_DEST_HOST", Value: dst.Host},
		{Name: "HERMES_DEST_PORT", Value: fmt.Sprintf("%d", dstPort)},
		{Name: "HERMES_DEST_DATABASE", Value: dst.Database},
		{Name: "HERMES_DEST_SSL_MODE", Value: dst.SSLMode},
		{
			Name: "HERMES_DEST_USERNAME",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: dst.SecretRef.Name},
					Key:                  dstUsernameKey,
				},
			},
		},
		{
			Name: "HERMES_DEST_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: dst.SecretRef.Name},
					Key:                  dstPasswordKey,
				},
			},
		},
		{Name: "HERMES_MIGRATE_SCHEMA", Value: boolToStr(schema)},
		{Name: "HERMES_MIGRATE_DATA", Value: boolToStr(data)},
		{Name: "HERMES_TABLES", Value: tables},
		{Name: "HERMES_BATCH_SIZE", Value: fmt.Sprintf("%d", batchSize)},
	}

	return env
}

func (r *DatabaseMigrationReconciler) buildJob(dm *hermesio.DatabaseMigration) *batchv1.Job {
	image := dm.Spec.Image
	if image == "" {
		image = r.DefaultImage
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName(dm.Name),
			Namespace: dm.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":        "hermes",
				"hermes/database-migration":     dm.Name,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:  "migrator",
							Image: image,
							Env:   r.buildEnv(dm),
						},
					},
				},
			},
		},
	}
}

func (r *DatabaseMigrationReconciler) buildCronJob(dm *hermesio.DatabaseMigration) *batchv1.CronJob {
	image := dm.Spec.Image
	if image == "" {
		image = r.DefaultImage
	}

	forbid := batchv1.ForbidConcurrent

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName(dm.Name),
			Namespace: dm.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":    "hermes",
				"hermes/database-migration": dm.Name,
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          dm.Spec.Schedule,
			ConcurrencyPolicy: forbid,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:  "migrator",
									Image: image,
									Env:   r.buildEnv(dm),
								},
							},
						},
					},
				},
			},
		},
	}
}

func jobName(crName string) string {
	name := fmt.Sprintf("hermes-%s", crName)
	if len(name) > 63 {
		return name[:63]
	}
	return name
}

func defaultPort(dbType string) int32 {
	switch strings.ToLower(dbType) {
	case "mysql":
		return 3306
	case "postgresql", "postgres":
		return 5432
	default:
		return 0
	}
}

func boolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (r *DatabaseMigrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hermesio.DatabaseMigration{}).
		Owns(&batchv1.Job{}).
		Owns(&batchv1.CronJob{}).
		Complete(r)
}

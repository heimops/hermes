package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition type and reason constants.
const (
	ConditionReady = "Ready"

	ReasonJobSynced    = "JobSynced"
	ReasonCronJobSynced = "CronJobSynced"
	ReasonSyncFailed   = "SyncFailed"
)

// LocalSecretRef references a Secret in the same namespace.
type LocalSecretRef struct {
	// Name of the Secret.
	Name string `json:"name"`

	// UsernameKey is the key in the Secret for the database username.
	// +optional
	// +kubebuilder:default=username
	UsernameKey string `json:"usernameKey,omitempty"`

	// PasswordKey is the key in the Secret for the database password.
	// +optional
	// +kubebuilder:default=password
	PasswordKey string `json:"passwordKey,omitempty"`
}

// DatabaseRef holds the connection parameters for a database.
type DatabaseRef struct {
	// Type is the database engine type.
	// +kubebuilder:validation:Enum=mysql;postgresql
	Type string `json:"type"`

	// Host is the database server hostname or IP.
	Host string `json:"host"`

	// Port is the database server port. Defaults to the engine's standard port.
	// +optional
	Port int32 `json:"port,omitempty"`

	// Database is the name of the database to connect to.
	Database string `json:"database"`

	// SecretRef references a Kubernetes Secret containing the credentials.
	SecretRef LocalSecretRef `json:"secretRef"`

	// SSLMode controls SSL behaviour (e.g. disable, require, verify-full).
	// +optional
	SSLMode string `json:"sslMode,omitempty"`
}

// MigrationOptions controls what is migrated.
type MigrationOptions struct {
	// Schema controls whether DDL (table structure) is migrated.
	// +optional
	// +kubebuilder:default=true
	Schema bool `json:"schema,omitempty"`

	// Data controls whether row data is migrated.
	// +optional
	// +kubebuilder:default=true
	Data bool `json:"data,omitempty"`

	// Tables is a list of specific tables to migrate. Empty means all tables.
	// +optional
	Tables []string `json:"tables,omitempty"`

	// BatchSize is the number of rows read per iteration during data migration.
	// +optional
	// +kubebuilder:default=1000
	// +kubebuilder:validation:Minimum=1
	BatchSize int32 `json:"batchSize,omitempty"`
}

// DatabaseMigrationSpec defines the desired state of a DatabaseMigration.
type DatabaseMigrationSpec struct {
	// Source is the source database configuration.
	Source DatabaseRef `json:"source"`

	// Destination is the destination database configuration.
	Destination DatabaseRef `json:"destination"`

	// Mode controls whether the migration runs once or on a schedule.
	// +kubebuilder:validation:Enum=once;schedule
	// +kubebuilder:default=once
	Mode string `json:"mode"`

	// Schedule is a cron expression (e.g. "0 */6 * * *").
	// Required when mode=schedule.
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Options controls what gets migrated.
	// +optional
	Options *MigrationOptions `json:"options,omitempty"`

	// Image overrides the migrator container image for this migration.
	// +optional
	Image string `json:"image,omitempty"`
}

// DatabaseMigrationStatus holds observed state.
type DatabaseMigrationStatus struct {
	// JobRef is the name of the managed Job or CronJob.
	// +optional
	JobRef string `json:"jobRef,omitempty"`

	// LastMigrationTime is the time the last migration completed.
	// +optional
	LastMigrationTime *metav1.Time `json:"lastMigrationTime,omitempty"`

	// Conditions represent the latest observations of the DatabaseMigration.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dbm
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.source.host`
// +kubebuilder:printcolumn:name="Destination",type=string,JSONPath=`.spec.destination.host`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DatabaseMigration is the Schema for the databasemigrations API.
type DatabaseMigration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseMigrationSpec   `json:"spec,omitempty"`
	Status DatabaseMigrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseMigrationList contains a list of DatabaseMigration.
type DatabaseMigrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseMigration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseMigration{}, &DatabaseMigrationList{})
}

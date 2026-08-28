package storage

import "time"

type ArtifactType string

const (
	ArtifactOCI       ArtifactType = "oci_image"
	ArtifactHelmChart ArtifactType = "helm_chart"
	ArtifactContainer ArtifactType = "container_image"
	ArtifactTerraform ArtifactType = "terraform_module"
	ArtifactGeneric   ArtifactType = "generic"
	ArtifactJAR       ArtifactType = "application/java-archive"
	ArtifactWAR       ArtifactType = "application/java-archive"
	ArtifactBinary    ArtifactType = "application/octet-stream"
)

type StorageBackend string

const (
	BackendOCI   StorageBackend = "oci"
	BackendS3    StorageBackend = "s3"
	BackendLocal StorageBackend = "local"
)

type ArtifactMeta struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        ArtifactType      `json:"type"`
	MediaType   string            `json:"media_type,omitempty"`
	RegistryURL string            `json:"registry_url"`
	Digest      string            `json:"digest"`
	SizeBytes   int64             `json:"size_bytes"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

type UploadResult struct {
	ArtifactID  string `json:"artifact_id"`
	Digest      string `json:"digest"`
	SizeBytes   int64  `json:"size_bytes"`
	RegistryURL string `json:"registry_url"`
}

type StorageConfig struct {
	Backend     StorageBackend `json:"backend"`
	RegistryURL string         `json:"registry_url"` // Harbor 地址
	Username    string         `json:"username,omitempty"`
	Password    string         `json:"password,omitempty"`
	Insecure    bool           `json:"insecure,omitempty"` // HTTP 而非 HTTPS
}

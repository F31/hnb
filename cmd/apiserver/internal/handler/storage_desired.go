package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

type StorageDesiredHandler struct {
	store            storageDesiredStore
	platformURL      string
	client           *http.Client
	delegationSigner *iam.DelegationSigner
}

func NewStorageDesiredHandler(store storageDesiredStore) *StorageDesiredHandler {
	return &StorageDesiredHandler{store: store, client: newInternalHTTPClient(30 * time.Second)}
}

func (h *StorageDesiredHandler) ConfigureOperations(platformURL string, signer *iam.DelegationSigner) {
	h.platformURL, h.delegationSigner = strings.TrimRight(platformURL, "/"), signer
}

type storageBindingImportIntentInput struct {
	TargetID                    string `json:"targetId"`
	TargetVersion               int64  `json:"targetVersion"`
	StorageClassName            string `json:"storageClassName"`
	StorageClassUID             string `json:"storageClassUid"`
	StorageClassResourceVersion string `json:"storageClassResourceVersion"`
}

type storageBindingReconcileIntentInput struct {
	TargetVersion int64 `json:"targetVersion"`
}

type storageDriverIntentInput struct {
	TargetID          string               `json:"targetId"`
	TargetVersion     int64                `json:"targetVersion"`
	PackageID         string               `json:"packageId"`
	PackageVersion    string               `json:"packageVersion"`
	CurrentVersion    string               `json:"currentVersion,omitempty"`
	KubernetesVersion string               `json:"kubernetesVersion"`
	Parameters        map[string]any       `json:"parameters,omitempty"`
	SecretReferences  []bffIntentSecretRef `json:"secretReferences,omitempty"`
}

type retainedVolumeResourceInput struct {
	Namespace, Name, UID, ResourceVersion, Phase, ReclaimPolicy string
	DeletionObserved                                            bool
}

func (v *retainedVolumeResourceInput) UnmarshalJSON(data []byte) error {
	var raw struct {
		Namespace        string `json:"namespace"`
		Name             string `json:"name"`
		UID              string `json:"uid"`
		ResourceVersion  string `json:"resourceVersion"`
		Phase            string `json:"phase"`
		ReclaimPolicy    string `json:"reclaimPolicy"`
		DeletionObserved bool   `json:"deletionObserved"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*v = retainedVolumeResourceInput{raw.Namespace, raw.Name, raw.UID, raw.ResourceVersion, raw.Phase, raw.ReclaimPolicy, raw.DeletionObserved}
	return nil
}

func (v retainedVolumeResourceInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Namespace        string `json:"namespace,omitempty"`
		Name             string `json:"name"`
		UID              string `json:"uid"`
		ResourceVersion  string `json:"resourceVersion"`
		Phase            string `json:"phase,omitempty"`
		ReclaimPolicy    string `json:"reclaimPolicy,omitempty"`
		DeletionObserved bool   `json:"deletionObserved,omitempty"`
	}{v.Namespace, v.Name, v.UID, v.ResourceVersion, v.Phase, v.ReclaimPolicy, v.DeletionObserved})
}

type retainedVolumeDependencyInput struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resourceVersion"`
}
type retainedVolumeIntentInput struct {
	TargetID                string                          `json:"targetId"`
	TargetVersion           int64                           `json:"targetVersion"`
	WorkflowProviderRef     string                          `json:"workflowProviderRef"`
	PersistentVolume        retainedVolumeResourceInput     `json:"persistentVolume"`
	PersistentVolumeClaim   retainedVolumeResourceInput     `json:"persistentVolumeClaim"`
	PodDependencies         []retainedVolumeDependencyInput `json:"podDependencies"`
	StatefulSetDependencies []retainedVolumeDependencyInput `json:"statefulSetDependencies"`
	ApprovalAcknowledged    bool                            `json:"approvalAcknowledged"`
}

func (h *StorageDesiredHandler) ReleaseRetainedVolumeIntent(w http.ResponseWriter, r *http.Request) {
	h.retainedVolumeIntent(w, r, "ReleaseRetainedVolume")
}
func (h *StorageDesiredHandler) SanitizeRetainedVolumeIntent(w http.ResponseWriter, r *http.Request) {
	h.retainedVolumeIntent(w, r, "SanitizeRetainedVolume")
}

func (h *StorageDesiredHandler) retainedVolumeIntent(w http.ResponseWriter, r *http.Request, kind string) {
	tenantID, ok := storageTenant(r)
	volumeID := strings.TrimSpace(r.PathValue("volumeId"))
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	if !boundedStorage(volumeID, 128, true) || !requireStorageIdempotency(w, r) {
		return
	}
	var input retainedVolumeIntentInput
	if !decodeStorageInput(r, &input) || !validRetainedVolumeInput(input) {
		writeStorageProblem(w, r, http.StatusConflict, "RETAINED_VOLUME_DEPENDENCY_CONFLICT", "Fresh Released PV, deleted PVC, Retain policy, no Pod or StatefulSet dependencies, and explicit approval acknowledgement are required.")
		return
	}
	envelope := bffIntentEnvelope{APIVersion: "hnb.io/v1", Kind: kind, Metadata: bffIntentMetadata{IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key"))}, Spec: bffIntentSpec{
		TargetID: input.TargetID, TargetKind: "KubernetesTarget", ExpectedVersion: input.TargetVersion, VolumeID: volumeID, WorkflowProviderRef: input.WorkflowProviderRef,
		PersistentVolume: input.PersistentVolume, PersistentVolumeClaim: input.PersistentVolumeClaim,
		PodDependencies: []any{}, StatefulSetDependencies: []any{},
	}}
	h.submitStorageIntent(w, r, envelope, string(iam.ResourceRetainedVolume), volumeID, iam.ActionExecute)
	_ = tenantID
}

func validRetainedVolumeInput(input retainedVolumeIntentInput) bool {
	if _, err := uuid.Parse(input.TargetID); err != nil || input.TargetVersion < 1 || !boundedStorage(input.WorkflowProviderRef, 256, true) || !input.ApprovalAcknowledged || len(input.PodDependencies) != 0 || len(input.StatefulSetDependencies) != 0 {
		return false
	}
	pv, pvc := input.PersistentVolume, input.PersistentVolumeClaim
	return boundedStorage(pv.Name, 253, true) && boundedStorage(pv.UID, 128, true) && boundedStorage(pv.ResourceVersion, 128, true) && pv.Phase == "Released" && pv.ReclaimPolicy == "Retain" &&
		boundedStorage(pvc.Namespace, 253, true) && boundedStorage(pvc.Name, 253, true) && boundedStorage(pvc.UID, 128, true) && boundedStorage(pvc.ResourceVersion, 128, true) && pvc.DeletionObserved
}

func (h *StorageDesiredHandler) InstallDriverIntent(w http.ResponseWriter, r *http.Request) {
	h.storageDriverIntent(w, r, "InstallStorageDriver", iam.ActionCreate)
}
func (h *StorageDesiredHandler) UpgradeDriverIntent(w http.ResponseWriter, r *http.Request) {
	h.storageDriverIntent(w, r, "UpgradeStorageDriver", iam.ActionUpdate)
}
func (h *StorageDesiredHandler) UninstallDriverIntent(w http.ResponseWriter, r *http.Request) {
	h.storageDriverIntent(w, r, "UninstallStorageDriver", iam.ActionDelete)
}

func (h *StorageDesiredHandler) storageDriverIntent(w http.ResponseWriter, r *http.Request, kind string, action iam.AuthorizationAction) {
	tenantID, installationID, ok := desiredIdentity(r, "installationId")
	if !ok {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_DRIVER_INSTALLATION_NOT_FOUND", "Storage driver installation was not found.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	var input storageDriverIntentInput
	if !decodeStorageInput(r, &input) || !validStorageDriverIntentInput(input, kind, tenantID) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_DRIVER_INTENT", "Storage driver lifecycle input is invalid.")
		return
	}
	expectedInstallationID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("hnb.storage-driver-installation\x00"+tenantID+"\x00"+input.TargetID+"\x00"+input.PackageID)).String()
	if installationID != expectedInstallationID {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_DRIVER_INSTALLATION_NOT_FOUND", "Storage driver installation was not found.")
		return
	}
	envelope := bffIntentEnvelope{APIVersion: "hnb.io/v1", Kind: kind, Metadata: bffIntentMetadata{IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key"))}, Spec: bffIntentSpec{
		InstallationID: installationID, PackageID: input.PackageID, PackageVersion: input.PackageVersion, CurrentVersion: input.CurrentVersion,
		TargetID: input.TargetID, TargetKind: "KubernetesTarget", ExpectedVersion: input.TargetVersion, KubernetesVersion: input.KubernetesVersion,
		Parameters: input.Parameters, SecretReferences: input.SecretReferences,
	}}
	h.submitStorageIntent(w, r, envelope, string(iam.ResourceStorageDriverInstallation), installationID, action)
}

func validStorageDriverIntentInput(input storageDriverIntentInput, kind, tenantID string) bool {
	if _, err := uuid.Parse(input.TargetID); err != nil || input.TargetVersion < 1 || !boundedStorage(input.PackageID, 256, true) ||
		!semverStorage(input.PackageVersion) || !semverStorage(strings.TrimPrefix(input.KubernetesVersion, "v")) || len(input.Parameters) > 32 || len(input.SecretReferences) > 32 {
		return false
	}
	if kind == "UpgradeStorageDriver" {
		if !semverStorage(input.CurrentVersion) {
			return false
		}
	} else if input.CurrentVersion != "" {
		return false
	}
	for _, ref := range input.SecretReferences {
		if !boundedStorage(ref.Provider, 256, true) || ref.Scope != "tenant:"+tenantID || !boundedStorage(ref.Name, 256, true) {
			return false
		}
	}
	for key := range input.Parameters {
		if !boundedStorage(key, 128, true) || forbiddenStorageDriverParameter(key) {
			return false
		}
	}
	return true
}

func semverStorage(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}
func forbiddenStorageDriverParameter(key string) bool {
	switch strings.ToLower(key) {
	case "steps", "command", "commands", "providerid", "providercommand", "credential", "credentials", "fencing", "fencingtoken":
		return true
	}
	return false
}

func (h *StorageDesiredHandler) ImportBindingIntent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := storageTenant(r)
	offeringID := r.PathValue("offeringId")
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	if _, err := uuid.Parse(offeringID); err != nil {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_OFFERING_NOT_FOUND", "Storage offering was not found.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	var input storageBindingImportIntentInput
	if !decodeStorageInput(r, &input) || !validStorageBindingIntentIdentity(input.TargetID, input.TargetVersion, input.StorageClassName, input.StorageClassUID, input.StorageClassResourceVersion) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_BINDING_INTENT", "StorageClass binding import identity is invalid.")
		return
	}
	offering, err := h.store.GetOffering(r.Context(), tenantID, offeringID)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	bindingID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("hnb.storage-binding\x00"+tenantID+"\x00"+offeringID+"\x00"+input.TargetID+"\x00"+input.StorageClassUID)).String()
	envelope := bffIntentEnvelope{APIVersion: "hnb.io/v1", Kind: "ImportStorageClassBinding", Metadata: bffIntentMetadata{IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key"))}, Spec: bffIntentSpec{BindingID: bindingID, OfferingID: offeringID, OfferingVersion: offering.Version, TargetID: input.TargetID, TargetKind: "KubernetesTarget", ExpectedVersion: input.TargetVersion, StorageClassName: input.StorageClassName, StorageClassUID: input.StorageClassUID, StorageClassResourceVersion: input.StorageClassResourceVersion}}
	h.submitStorageBindingIntent(w, r, envelope, offeringID, iam.ActionCreate)
}

func (h *StorageDesiredHandler) ReconcileBindingIntent(w http.ResponseWriter, r *http.Request) {
	tenantID, bindingID, ok := desiredIdentity(r, "bindingId")
	if !ok {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_CLASS_BINDING_NOT_FOUND", "StorageClass binding was not found.")
		return
	}
	expectedBindingVersion, ok := storageIfMatch(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED", "A valid If-Match version is required.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	var input storageBindingReconcileIntentInput
	if !decodeStorageInput(r, &input) || input.TargetVersion < 1 {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_BINDING_INTENT", "StorageClass binding reconcile fence is invalid.")
		return
	}
	binding, err := h.store.GetBinding(r.Context(), tenantID, bindingID)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	if binding.Version != expectedBindingVersion {
		writeStorageProblem(w, r, http.StatusPreconditionFailed, "STORAGE_VERSION_CONFLICT", "The storage record changed; read the latest version and retry.")
		return
	}
	if binding.Freshness != "Fresh" {
		writeStorageProblem(w, r, http.StatusConflict, "STORAGE_CLASS_OBSERVATION_STALE", "A fresh StorageClass observation is required before reconcile.")
		return
	}
	envelope := bffIntentEnvelope{APIVersion: "hnb.io/v1", Kind: "ReconcileStorageClassBinding", Metadata: bffIntentMetadata{IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key"))}, Spec: bffIntentSpec{BindingID: binding.ID, BindingVersion: binding.Version, OfferingID: binding.OfferingID, OfferingVersion: binding.OfferingVersion, TargetID: binding.TargetID, TargetKind: "KubernetesTarget", ExpectedVersion: input.TargetVersion, StorageClassName: binding.StorageClassName, StorageClassUID: binding.StorageClassUID, StorageClassResourceVersion: binding.StorageClassResourceVersion}}
	h.submitStorageBindingIntent(w, r, envelope, bindingID, iam.ActionUpdate)
}

func validStorageBindingIntentIdentity(targetID string, targetVersion int64, name, uid, resourceVersion string) bool {
	_, err := uuid.Parse(targetID)
	return err == nil && targetVersion >= 1 && boundedStorage(name, 253, true) && boundedStorage(uid, 128, true) && boundedStorage(resourceVersion, 128, true)
}

func (h *StorageDesiredHandler) submitStorageBindingIntent(w http.ResponseWriter, r *http.Request, envelope bffIntentEnvelope, resourceID string, action iam.AuthorizationAction) {
	h.submitStorageIntent(w, r, envelope, string(iam.ResourceStorageClassBinding), resourceID, action)
}

func (h *StorageDesiredHandler) submitStorageIntent(w http.ResponseWriter, r *http.Request, envelope bffIntentEnvelope, resourceKind, resourceID string, action iam.AuthorizationAction) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	if h.platformURL == "" || h.delegationSigner == nil {
		writeStorageProblem(w, r, http.StatusServiceUnavailable, "OPERATION_ENGINE_UNAVAILABLE", "Storage intent planning is unavailable.")
		return
	}
	envelope.Metadata.CorrelationID = trusted.CorrelationID
	digest := bffIntentSemanticDigest(envelope)
	token, err := h.delegationSigner.Sign(r.Context(), trusted, iam.DelegationEvidence{Scope: iam.DelegationScope{ResourceKind: resourceKind, ResourceID: resourceID, ProjectID: trusted.ProjectID, EnvironmentID: trusted.EnvironmentID, NamespaceID: trusted.NamespaceID}, Action: action, IntentKind: envelope.Kind, SemanticDigest: digest, CorrelationID: trusted.CorrelationID})
	if err != nil {
		writeStorageProblem(w, r, http.StatusServiceUnavailable, "OPERATION_ENGINE_UNAVAILABLE", "Storage intent planning is unavailable.")
		return
	}
	body, _ := json.Marshal(envelope)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.platformURL+"/v1/intents", strings.NewReader(string(body)))
	if err != nil {
		writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_INTENT_FAILED", "Storage intent could not be submitted.")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Semantic-Digest", digest)
	req.Header.Set("X-Correlation-ID", trusted.CorrelationID)
	req.Header.Set("Idempotency-Key", envelope.Metadata.IdempotencyKey)
	copyHeader(req.Header, r.Header, "X-Trace-Id")
	resp, err := h.client.Do(req)
	if err != nil {
		writeStorageProblem(w, r, http.StatusServiceUnavailable, "OPERATION_ENGINE_UNAVAILABLE", "Storage intent planning is unavailable.")
		return
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		writeStorageProblem(w, r, http.StatusServiceUnavailable, "OPERATION_ENGINE_UNAVAILABLE", "Storage intent planning is unavailable.")
		return
	}
	if resp.StatusCode >= 400 {
		writeMappedUpstreamProblem(w, r, resp.StatusCode, resp.Header.Get("Content-Type"), responseBody)
		return
	}
	var result struct {
		IntentID       string `json:"intentId"`
		OperationID    string `json:"operationId"`
		PlanID         string `json:"planId"`
		Status         string `json:"status"`
		CreatedAt      string `json:"createdAt"`
		SemanticDigest string `json:"semanticDigest"`
		Replayed       bool   `json:"replayed"`
	}
	if json.Unmarshal(responseBody, &result) != nil || result.IntentID == "" || result.OperationID == "" || result.PlanID == "" {
		writeStorageProblem(w, r, http.StatusServiceUnavailable, "OPERATION_ENGINE_INVALID_RESPONSE", "Storage intent planning returned an invalid reference.")
		return
	}
	writeStorageStatusJSON(w, http.StatusAccepted, map[string]any{"intentId": result.IntentID, "executionPlanId": result.PlanID, "operationId": result.OperationID, "status": mapPlatformStatus(result.Status), "semanticDigest": result.SemanticDigest, "createdAt": result.CreatedAt, "correlationId": trusted.CorrelationID, "replayed": result.Replayed})
}

func (h *StorageDesiredHandler) Backends(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := storageTenant(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	if !validStorageListQuery(r, map[string]bool{"providerType": true, "healthState": true, "keyword": true}) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_QUERY", "Pagination or filter parameters are invalid.")
		return
	}
	items, err := h.store.ListBackends(r.Context(), tenantID)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	values := make([]any, len(items))
	for i, item := range items {
		values[i] = backendResponse(item)
	}
	writeStorageStatusJSON(w, http.StatusOK, map[string]any{"schemaVersion": storageSchemaVersion, "items": values, "total": len(values)})
}

func (h *StorageDesiredHandler) ProviderSchemas(w http.ResponseWriter, r *http.Request) {
	if _, ok := storageTenant(r); !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	writeStorageStatusJSON(w, http.StatusOK, map[string]any{"schemaVersion": storageSchemaVersion, "items": listStorageProviderSchemas()})
}

func (h *StorageDesiredHandler) CreateBackend(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := storageTenant(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	var input storageBackendInput
	if !decodeStorageInput(r, &input) || !validBackendInput(input) || !h.validBackendSecret(r, tenantID, input.SecretReference) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_BACKEND", "Storage backend input is invalid; credentials must be a SecretReference.")
		return
	}
	item, err := h.store.CreateBackend(r.Context(), tenantID, uuid.NewString(), input)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	writeVersionedStorage(w, http.StatusCreated, item.Version, backendResponse(item))
}

func (h *StorageDesiredHandler) GetBackend(w http.ResponseWriter, r *http.Request) {
	tenantID, id, ok := desiredIdentity(r, "backendId")
	if !ok {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_BACKEND_NOT_FOUND", "Storage backend was not found.")
		return
	}
	item, err := h.store.GetBackend(r.Context(), tenantID, id)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	writeVersionedStorage(w, http.StatusOK, item.Version, backendResponse(item))
}

func (h *StorageDesiredHandler) UpdateBackend(w http.ResponseWriter, r *http.Request) {
	tenantID, id, ok := desiredIdentity(r, "backendId")
	if !ok {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_BACKEND_NOT_FOUND", "Storage backend was not found.")
		return
	}
	expected, ok := storageIfMatch(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED", "A valid If-Match version is required.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	var input storageBackendInput
	if !decodeStorageInput(r, &input) || !validBackendInput(input) || !h.validBackendSecret(r, tenantID, input.SecretReference) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_BACKEND", "Storage backend input is invalid; credentials must be a SecretReference.")
		return
	}
	item, err := h.store.UpdateBackend(r.Context(), tenantID, id, expected, input)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	writeVersionedStorage(w, http.StatusOK, item.Version, backendResponse(item))
}

func (h *StorageDesiredHandler) DeleteBackend(w http.ResponseWriter, r *http.Request) {
	tenantID, id, ok := desiredIdentity(r, "backendId")
	if !ok {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_BACKEND_NOT_FOUND", "Storage backend was not found.")
		return
	}
	expected, ok := storageIfMatch(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED", "A valid If-Match version is required.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	if err := h.store.DeleteBackend(r.Context(), tenantID, id, expected); err != nil {
		h.storeProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *StorageDesiredHandler) Offerings(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := storageTenant(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	if !validStorageListQuery(r, map[string]bool{"scope": true, "serviceMode": true, "keyword": true}) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_QUERY", "Pagination or filter parameters are invalid.")
		return
	}
	items, err := h.store.ListOfferings(r.Context(), tenantID)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	values := make([]any, len(items))
	for i, item := range items {
		values[i] = offeringResponse(item)
	}
	writeStorageStatusJSON(w, http.StatusOK, map[string]any{"schemaVersion": storageSchemaVersion, "items": values, "total": len(values)})
}

func (h *StorageDesiredHandler) CreateOffering(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := storageTenant(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	var input workloadStorageOfferingInput
	if !decodeStorageInput(r, &input) || !validOfferingInput(input) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_OFFERING", "Storage offering input is invalid.")
		return
	}
	item, err := h.store.CreateOffering(r.Context(), tenantID, uuid.NewString(), input)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	writeVersionedStorage(w, http.StatusCreated, item.Version, offeringResponse(item))
}
func (h *StorageDesiredHandler) GetOffering(w http.ResponseWriter, r *http.Request) {
	tenantID, id, ok := desiredIdentity(r, "offeringId")
	if !ok {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_OFFERING_NOT_FOUND", "Storage offering was not found.")
		return
	}
	item, err := h.store.GetOffering(r.Context(), tenantID, id)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	writeVersionedStorage(w, http.StatusOK, item.Version, offeringResponse(item))
}
func (h *StorageDesiredHandler) UpdateOffering(w http.ResponseWriter, r *http.Request) {
	tenantID, id, ok := desiredIdentity(r, "offeringId")
	if !ok {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_OFFERING_NOT_FOUND", "Storage offering was not found.")
		return
	}
	expected, ok := storageIfMatch(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED", "A valid If-Match version is required.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	var input workloadStorageOfferingInput
	if !decodeStorageInput(r, &input) || !validOfferingInput(input) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_OFFERING", "Storage offering input is invalid.")
		return
	}
	item, err := h.store.UpdateOffering(r.Context(), tenantID, id, expected, input)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	writeVersionedStorage(w, http.StatusOK, item.Version, offeringResponse(item))
}
func (h *StorageDesiredHandler) DeleteOffering(w http.ResponseWriter, r *http.Request) {
	tenantID, id, ok := desiredIdentity(r, "offeringId")
	if !ok {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_OFFERING_NOT_FOUND", "Storage offering was not found.")
		return
	}
	expected, ok := storageIfMatch(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED", "A valid If-Match version is required.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	if err := h.store.DeleteOffering(r.Context(), tenantID, id, expected); err != nil {
		h.storeProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *StorageDesiredHandler) Bindings(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := storageTenant(r)
	offeringID := r.PathValue("offeringId")
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	if _, err := uuid.Parse(offeringID); err != nil {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_OFFERING_NOT_FOUND", "Storage offering was not found.")
		return
	}
	if !validStorageListQuery(r, map[string]bool{"targetId": true, "syncState": true, "keyword": true}) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_QUERY", "Pagination or filter parameters are invalid.")
		return
	}
	if _, err := h.store.GetOffering(r.Context(), tenantID, offeringID); err != nil {
		h.storeProblem(w, r, err)
		return
	}
	items, err := h.store.ListBindings(r.Context(), tenantID, offeringID)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	values := make([]any, len(items))
	for i, item := range items {
		values[i] = bindingResponse(item)
	}
	writeStorageStatusJSON(w, http.StatusOK, map[string]any{"schemaVersion": storageSchemaVersion, "items": values, "total": len(values)})
}
func (h *StorageDesiredHandler) CreateBinding(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := storageTenant(r)
	offeringID := r.PathValue("offeringId")
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	if _, err := uuid.Parse(offeringID); err != nil {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_OFFERING_NOT_FOUND", "Storage offering was not found.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	var input storageClassBindingInput
	if !decodeStorageInput(r, &input) || !validBindingInput(input) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_CLASS_BINDING", "StorageClass binding input is invalid.")
		return
	}
	item, err := h.store.CreateBinding(r.Context(), tenantID, offeringID, uuid.NewString(), input)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	writeVersionedStorage(w, http.StatusCreated, item.Version, bindingResponse(item))
}
func (h *StorageDesiredHandler) GetBinding(w http.ResponseWriter, r *http.Request) {
	tenantID, id, ok := desiredIdentity(r, "bindingId")
	if !ok {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_CLASS_BINDING_NOT_FOUND", "StorageClass binding was not found.")
		return
	}
	item, err := h.store.GetBinding(r.Context(), tenantID, id)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	writeVersionedStorage(w, http.StatusOK, item.Version, bindingResponse(item))
}
func (h *StorageDesiredHandler) UpdateBinding(w http.ResponseWriter, r *http.Request) {
	tenantID, id, ok := desiredIdentity(r, "bindingId")
	if !ok {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_CLASS_BINDING_NOT_FOUND", "StorageClass binding was not found.")
		return
	}
	expected, ok := storageIfMatch(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED", "A valid If-Match version is required.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	var input storageClassBindingInput
	if !decodeStorageInput(r, &input) || !validBindingInput(input) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_CLASS_BINDING", "StorageClass binding input is invalid.")
		return
	}
	item, err := h.store.UpdateBinding(r.Context(), tenantID, id, expected, input)
	if err != nil {
		h.storeProblem(w, r, err)
		return
	}
	writeVersionedStorage(w, http.StatusOK, item.Version, bindingResponse(item))
}
func (h *StorageDesiredHandler) DeleteBinding(w http.ResponseWriter, r *http.Request) {
	tenantID, id, ok := desiredIdentity(r, "bindingId")
	if !ok {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_CLASS_BINDING_NOT_FOUND", "StorageClass binding was not found.")
		return
	}
	expected, ok := storageIfMatch(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED", "A valid If-Match version is required.")
		return
	}
	if !requireStorageIdempotency(w, r) {
		return
	}
	if err := h.store.DeleteBinding(r.Context(), tenantID, id, expected); err != nil {
		h.storeProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func backendResponse(item storageBackendRecord) map[string]any {
	result := map[string]any{"schemaVersion": storageSchemaVersion, "id": item.ID, "tenantId": item.TenantID, "providerType": item.ProviderType, "backendId": item.BackendID, "displayName": item.DisplayName, "healthState": "Unknown", "source": "platform.desired-state", "observedAt": item.UpdatedAt.UTC(), "freshness": "Unknown", "conditions": []any{}, "version": item.Version, "createdAt": item.CreatedAt.UTC(), "updatedAt": item.UpdatedAt.UTC()}
	if item.ProviderSchemaVersion != "" {
		result["providerSchemaVersion"] = item.ProviderSchemaVersion
	}
	if len(item.Attributes) > 0 {
		result["attributes"] = item.Attributes
	}
	if item.Description != "" {
		result["description"] = item.Description
	}
	if item.SecretReference != nil {
		result["secretReference"] = item.SecretReference
	}
	return result
}
func offeringResponse(item workloadStorageOfferingRecord) map[string]any {
	result := map[string]any{"schemaVersion": storageSchemaVersion, "id": item.ID, "scope": "Tenant", "tenantId": item.TenantID, "name": item.Name, "consumptionModel": "KubernetesPersistentVolume", "serviceMode": item.ServiceMode, "accessModes": item.AccessModes, "volumeExpansion": item.VolumeExpansion, "snapshots": item.Snapshots, "clones": item.Clones, "protectionClass": item.ProtectionClass, "conditions": []any{}, "version": item.Version, "createdAt": item.CreatedAt.UTC(), "updatedAt": item.UpdatedAt.UTC()}
	if item.BackendID != "" {
		result["backendId"] = item.BackendID
	}
	if item.Description != "" {
		result["description"] = item.Description
	}
	if len(item.Topology) > 0 {
		result["topology"] = item.Topology
	}
	return result
}
func bindingResponse(item storageClassBindingRecord) map[string]any {
	conditions := item.Conditions
	if conditions == nil {
		conditions = []map[string]any{}
	}
	result := map[string]any{"schemaVersion": storageSchemaVersion, "id": item.ID, "tenantId": item.TenantID, "offeringId": item.OfferingID, "offeringVersion": item.OfferingVersion, "targetId": item.TargetID, "bindingTarget": "KubernetesStorageClass", "storageClassName": item.StorageClassName, "storageClassUid": item.StorageClassUID, "storageClassResourceVersion": item.StorageClassResourceVersion, "syncState": item.SyncState, "isDefault": item.IsDefault, "source": item.Source, "observedAt": item.ObservedAt.UTC(), "freshness": item.Freshness, "conditions": conditions, "version": item.Version, "createdAt": item.CreatedAt.UTC(), "updatedAt": item.UpdatedAt.UTC()}
	if len(item.Topology) > 0 {
		result["topology"] = item.Topology
	}
	return result
}

func decodeStorageInput(r *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, (1<<20)+1))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}
func desiredIdentity(r *http.Request, param string) (string, string, bool) {
	tenantID, ok := storageTenant(r)
	id := r.PathValue(param)
	if !ok {
		return "", "", false
	}
	_, err := uuid.Parse(id)
	return tenantID, id, err == nil
}
func storageIfMatch(r *http.Request) (int64, bool) {
	value := strings.TrimSpace(r.Header.Get("If-Match"))
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, false
	}
	version, err := strconv.ParseInt(value[1:len(value)-1], 10, 64)
	return version, err == nil && version >= 1
}
func requireStorageIdempotency(w http.ResponseWriter, r *http.Request) bool {
	value := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if value == "" || len(value) > 128 || strings.ContainsAny(value, "\x00\r\n") {
		writeStorageProblem(w, r, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "A valid Idempotency-Key is required for storage writes.")
		return false
	}
	return true
}
func writeVersionedStorage(w http.ResponseWriter, status int, version int64, value any) {
	w.Header().Set("ETag", fmt.Sprintf("\"%d\"", version))
	writeStorageStatusJSON(w, status, value)
}
func writeStorageStatusJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (h *StorageDesiredHandler) storeProblem(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errStorageDesiredNotFound):
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_DESIRED_STATE_NOT_FOUND", "Storage desired state was not found.")
	case errors.Is(err, errStorageVersionConflict):
		writeStorageProblem(w, r, http.StatusPreconditionFailed, "STORAGE_VERSION_CONFLICT", "The storage record changed; read the latest version and retry.")
	case errors.Is(err, errStorageAlreadyExists):
		writeStorageProblem(w, r, http.StatusConflict, "STORAGE_DESIRED_STATE_EXISTS", "An equivalent storage record already exists.")
	case errors.Is(err, errStorageInvalidRef):
		writeStorageProblem(w, r, http.StatusUnprocessableEntity, "STORAGE_REFERENCE_INVALID", "A referenced tenant storage resource does not exist.")
	default:
		writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_DESIRED_STATE_FAILED", "Storage desired state could not be persisted.")
	}
}

func validBackendInput(input storageBackendInput) bool {
	return boundedStorage(input.ProviderType, 128, true) && boundedStorage(input.ProviderSchemaVersion, 32, true) && boundedStorage(input.BackendID, 256, true) && boundedStorage(input.DisplayName, 256, true) && boundedStorage(input.Description, 2048, false) && validateStorageProviderAttributes(input.ProviderType, input.ProviderSchemaVersion, input.Attributes) && (input.SecretReference == nil || validSecretReference(*input.SecretReference))
}
func validSecretReference(ref secretReference) bool {
	return boundedStorage(ref.Provider, 256, true) && boundedStorage(ref.Scope, 256, true) && boundedStorage(ref.Name, 256, true) && boundedStorage(ref.Version, 256, false)
}

func (h *StorageDesiredHandler) validBackendSecret(r *http.Request, tenantID string, ref *secretReference) bool {
	if ref == nil {
		return true
	}
	return ref.Scope == "tenant:"+tenantID && h.store.ValidateSecretReference(r.Context(), tenantID, *ref) == nil
}
func validOfferingInput(input workloadStorageOfferingInput) bool {
	if input.BackendID != "" {
		if _, err := uuid.Parse(input.BackendID); err != nil {
			return false
		}
	}
	if !boundedStorage(input.Name, 256, true) || !boundedStorage(input.Description, 2048, false) || input.ConsumptionModel != "KubernetesPersistentVolume" || (input.ServiceMode != "Block" && input.ServiceMode != "File") || len(input.AccessModes) < 1 || len(input.AccessModes) > 4 || !storageEnum(input.VolumeExpansion, "Supported", "Unsupported", "Unknown") || !storageEnum(input.Snapshots, "Supported", "Unsupported", "Unknown") || !storageEnum(input.Clones, "Supported", "Unsupported", "Unknown") || !boundedStorage(input.ProtectionClass, 128, true) || !validTopology(input.Topology) {
		return false
	}
	seen := map[string]bool{}
	for _, mode := range input.AccessModes {
		if !storageEnum(mode, "ReadWriteOnce", "ReadOnlyMany", "ReadWriteMany", "ReadWriteOncePod") || seen[mode] {
			return false
		}
		seen[mode] = true
	}
	return true
}
func validBindingInput(input storageClassBindingInput) bool {
	_, targetErr := uuid.Parse(input.TargetID)
	return input.OfferingVersion >= 1 && targetErr == nil && input.BindingTarget == "KubernetesStorageClass" && boundedStorage(input.StorageClassName, 253, true) && boundedStorage(input.StorageClassUID, 128, true) && boundedStorage(input.StorageClassResourceVersion, 128, true) && storageEnum(input.SyncState, "Discovered", "Imported", "Active", "Drifted", "Rejected", "Retired") && boundedStorage(input.Source, 256, true) && !input.ObservedAt.IsZero() && storageEnum(input.Freshness, "Fresh", "Stale", "Unknown") && validTopology(input.Topology)
}
func validTopology(topology map[string][]string) bool {
	if len(topology) > 32 {
		return false
	}
	for key, values := range topology {
		if !boundedStorage(key, 128, true) || len(values) < 1 || len(values) > 64 {
			return false
		}
		seen := map[string]bool{}
		for _, value := range values {
			if !boundedStorage(value, 256, true) || seen[value] {
				return false
			}
			seen[value] = true
		}
	}
	return true
}
func boundedStorage(value string, max int, required bool) bool {
	value = strings.TrimSpace(value)
	return (!required || value != "") && len(value) <= max && !strings.ContainsAny(value, "\x00\r\n")
}
func storageEnum(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../marketApi'
import { useArtifactUploader } from '../useArtifactUploader'
import ApplicationDrawer from '../components/ApplicationDrawer.vue'
import SecurityPanel from '../pages/SecurityPanel.vue'

type MarketTab = 'market' | 'repositories' | 'security'
type DialogType = 'productDetail' | 'install' | 'product' | 'release' | 'artifact' | 'repository' | 'gc' | 'confirm' | null

type ConfirmOptions = {
  title: string
  message: string
  items?: string[]
  confirmText?: string
  danger?: boolean
}

const confirmOptions = ref<ConfirmOptions | null>(null)
let confirmResolver: ((value: boolean) => void) | null = null

function requestConfirm(options: ConfirmOptions): Promise<boolean> {
  confirmOptions.value = options
  dialog.value = 'confirm'
  return new Promise((resolve) => { confirmResolver = resolve })
}

function resolveConfirm(result: boolean) {
  if (confirmResolver) {
    const resolver = confirmResolver
    confirmResolver = null
    confirmOptions.value = null
    dialog.value = null
    resolver(result)
  }
}
type ProductRecord = { id?: string; key: string; name: string; category: string; version: string; versionCount?: number; status: string; publisher: string; desc: string; publisherId?: string; visibility?: string; project?: string }
type ReleaseRecord = { id?: string; productId?: string; product: string; version: string; channel: string; status: string; digest: string; manifest?: api.MarketReleaseManifest | null; releaseNotes: string }
type ArtifactRecord = { id?: string; name: string; type: string; status: string; lifecycle: string; size: string; digest?: string }
type RepositoryRecord = { id?: string; name: string; type: string; endpoint: string; status: string; sync: string; backend?: string; secretRef?: string }

const { t } = useI18n()
const activeTab = ref<MarketTab>('market')
const query = ref('')
const category = ref('all')
const dialog = ref<DialogType>(null)
const selectedProductName = ref('')
const loading = ref(false)
const apiAvailable = ref(true)
const apiErrorMessage = ref('')
const repositoryError = ref('')
const artifactCenterError = ref('')
const installStep = ref(1)
const installTarget = ref('monolith')
const selectedPublishingProductId = ref('')
const selectedArtifactNode = ref('')
const associationTargetReleaseId = ref('')
const installForm = ref({ name: 'mysql-prod', version: '8.0.36', workspace: 'default', namespace: 'default', configMode: 'default' })
const productForm = ref({ id: '', key: '', name: '', category: 'application', description: '', status: 'draft', visibility: 'tenant', project: 'hnb' })
const harborProjects = ref(['hnb'])

onMounted(async () => {
  try {
    const projects = await api.listHarborProjects()
    if (projects.length) harborProjects.value = projects
  } catch { /* ignore */ }
})
const currentPublisher = ref<{ id: string; name: string; displayName: string } | null>(null)
const releaseForm = ref<{ id: string; productId: string; version: string; releaseNotes: string; manifest?: api.MarketReleaseManifest | null }>({ id: '', productId: '', version: '', releaseNotes: '', manifest: { artifacts: [] } })
const artifactForm = ref({ id: '', fileName: '', artifactType: 'helm_chart', digest: '', status: 'pending', lifecycle: 'active', sizeBytes: 0 })
const repoForm = ref({ id: '', name: '', type: 'OCI Registry', endpoint: '', authMode: 'secret', secretRef: '', syncMode: 'manual', status: 'active' })
const selectedProducts = ref<string[]>([])
const selectedArtifacts = ref<string[]>([])
const selectedRepositories = ref<string[]>([])
const uploader = useArtifactUploader()
const artifactFileError = ref('')
const submitError = ref('')

const loadedProducts = ref<ProductRecord[]>([])
const loadedReleases = ref<ReleaseRecord[]>([])
const loadedArtifacts = ref<ArtifactRecord[]>([])
const unassignedCount = ref(0)
const loadedRepositories = ref<RepositoryRecord[]>([])

const products = computed<ProductRecord[]>(() => loadedProducts.value)
const releases = computed<ReleaseRecord[]>(() => loadedReleases.value)
const artifacts = computed<ArtifactRecord[]>(() => loadedArtifacts.value)
const repositories = computed<RepositoryRecord[]>(() => loadedRepositories.value)

const tabs = computed(() => [
  { key: 'market' as const, label: t('application.marketPage.tabs.market') },
  { key: 'repositories' as const, label: t('application.marketPage.tabs.repositories') },
  { key: 'security' as const, label: t('application.marketPage.tabs.security') },
])

const scope = ref('public')

const scopeTabs = computed(() => [
  { key: 'public', label: t('application.marketPage.store.scopePublic') },
  { key: 'tenant', label: t('application.marketPage.store.scopeTenant') },
])

function toggleScope(key: string) {
  scope.value = scope.value === key ? 'all' : key
}

const categories = computed(() => [
  { key: 'all', label: t('application.marketPage.categories.all') },
  { key: 'application', label: t('application.marketPage.categories.application') },
  { key: 'database', label: t('application.marketPage.categories.database') },
  { key: 'middleware', label: t('application.marketPage.categories.middleware') },
  { key: 'ai', label: t('application.marketPage.categories.ai') },
  { key: 'edge', label: t('application.marketPage.categories.edge') },
  { key: 'tool', label: t('application.marketPage.categories.tool') },
])

const artifactTypes = computed(() => [
  { value: 'oci_image', label: t('application.marketPage.artifactTypes.oci_image') },
  { value: 'helm_chart', label: t('application.marketPage.artifactTypes.helm_chart') },
  { value: 'jar', label: t('application.marketPage.artifactTypes.jar') },
  { value: 'war', label: t('application.marketPage.artifactTypes.war') },
  { value: 'offline_bundle', label: t('application.marketPage.artifactTypes.offline_bundle') },
  { value: 'generic', label: t('application.marketPage.artifactTypes.generic') },
])

const uploadableArtifactSuffixes = ['.tar', '.tar.gz', '.tgz', '.oci', '.jar', '.war', '.zip', '.bundle']
const artifactAccept = computed(() => uploadableArtifactSuffixes.join(','))
const artifactFormatHint = computed(() => uploadableArtifactSuffixes.join(', '))
const artifactTypeLabel = computed(() => artifactTypes.value.find((item) => item.value === artifactForm.value.artifactType)?.label || '-')

const dialogTitle = computed(() => {
  if (!dialog.value) return ''
  if (dialog.value === 'confirm') return t('application.common.confirm')
  if (dialog.value === 'product') return t(productForm.value.id ? 'application.marketPage.dialog.product.editTitle' : 'application.marketPage.dialog.product.title')
  if (dialog.value === 'release') return t(releaseForm.value.id ? 'application.marketPage.dialog.release.editTitle' : 'application.marketPage.dialog.release.title')
  if (dialog.value === 'artifact') return t(artifactForm.value.id ? 'application.marketPage.dialog.artifact.editTitle' : 'application.marketPage.dialog.artifact.title')
  return t(`application.marketPage.dialog.${dialog.value}.title`)
})

const drawerOpen = computed({
  get: () => dialog.value !== null && dialog.value !== 'confirm',
  set: (open: boolean) => { if (!open) closeDialog() },
})

const emptyProduct: ProductRecord = { key: '', name: '', category: 'application', version: '', status: 'draft', publisher: '', desc: '' }

const currentPage = ref(1)
const pageSize = ref(9)
const totalProducts = ref(0)

const totalPages = computed(() => Math.max(1, Math.ceil(totalProducts.value / pageSize.value)))

const visiblePages = computed(() => {
  const pages: number[] = []
  const start = Math.max(1, currentPage.value - 2)
  const end = Math.min(totalPages.value, currentPage.value + 2)
  for (let i = start; i <= end; i++) pages.push(i)
  return pages
})

const filteredProducts = computed(() => {
  const items = products.value
  const term = query.value.trim().toLowerCase()
  return items.filter((p) => {
    const matchCategory = category.value === 'all' || p.category === category.value
    const matchQuery = !term || p.name.toLowerCase().includes(term) || p.desc.toLowerCase().includes(term)
    return matchCategory && matchQuery
  })
})

const selectedProduct = computed(() => products.value.find((product) => product.name === selectedProductName.value) ?? products.value[0] ?? emptyProduct)
const selectedProductReleases = computed(() => releases.value.filter((release) => release.product === selectedProduct.value?.name))
const selectedPublishingProduct = computed(() => products.value.find((product) => product.id === selectedPublishingProductId.value))
const publishingReleases = computed(() => releases.value.filter((release) => release.productId === selectedPublishingProductId.value))
const selectedWorkspaceRelease = computed(() => publishingReleases.value.find((release) => release.id === selectedArtifactNode.value))
const draftReleases = computed(() => publishingReleases.value.filter((release) => release.status === 'draft' && release.id))
const canUploadArtifact = computed(() => selectedWorkspaceRelease.value?.status === 'draft')

function mapRelease(r: api.MarketRelease, product: ProductRecord): ReleaseRecord {
  return {
    id: r.id, productId: r.product_id || product.id, product: product.name,
    version: r.version, channel: 'stable', status: r.status, digest: r.manifest_digest || '', manifest: r.manifest, releaseNotes: r.release_notes || '',
  }
}

function mapArtifact(a: api.MarketArtifact): ArtifactRecord {
  return {
    id: a.id, name: a.name, type: a.artifact_type, status: a.verification_status, digest: a.digest,
    lifecycle: a.lifecycle_state, size: a.size_bytes ? `${(a.size_bytes / 1048576).toFixed(1)} MiB` : '-',
  }
}

async function fetchMarketData() {
  loading.value = true
  repositoryError.value = ''
  apiErrorMessage.value = ''
  try {
    const [productResult, repositoryResult] = await Promise.allSettled([
      api.listProducts({ q: query.value, category: category.value === 'all' ? undefined : category.value, scope: scope.value, page: currentPage.value, pageSize: pageSize.value }),
      api.listRepositories(),
    ])
    const productResponse = productResult.status === 'fulfilled'
      ? { items: productResult.value?.items ?? [], total: productResult.value?.total ?? 0, page: productResult.value?.page ?? 1, pageSize: productResult.value?.pageSize ?? 9 }
      : { items: [], total: 0, page: 1, pageSize: 9 }
    const repoList = repositoryResult.status === 'fulfilled' ? repositoryResult.value : []

    apiAvailable.value = productResult.status === 'fulfilled'
    if (productResult.status === 'rejected') {
      apiErrorMessage.value = productResult.reason instanceof Error ? productResult.reason.message : t('application.marketPage.errors.loadFailed')
    }
    if (repositoryResult.status === 'rejected') {
      repositoryError.value = repositoryResult.reason instanceof Error
        ? repositoryResult.reason.message
        : t('application.marketPage.repositories.loadFailed')
    }

    loadedRepositories.value = repoList.map((r) => ({
      id: r.id, name: r.name, backend: r.backend, type: r.backend || r.type || 'OCI', endpoint: r.endpoint || '-',
      status: r.lifecycle_state === 'active' ? 'healthy' : 'disabled', sync: r.updated_at || '-', secretRef: r.secret_reference,
    }))

    totalProducts.value = productResponse.total
    loadedProducts.value = productResponse.items.map((p) => ({
      id: p.id, key: p.name, name: p.display_name || p.name, category: p.category, version: 'latest', versionCount: p.release_count ?? p.version_count,
      status: p.status, publisher: 'Tenant Workspace', publisherId: p.publisher_id, desc: p.description || '', visibility: p.visibility || 'tenant',
    }))
    if (selectedPublishingProductId.value && !loadedProducts.value.some((product) => product.id === selectedPublishingProductId.value)) leaveProductWorkspace()
  } catch {
    apiAvailable.value = false
  } finally {
    loading.value = false
  }
}

async function loadProductReleases(product: ProductRecord) {
  if (!product.id) return
  const releaseList = await api.listReleases(product.id)
  loadedReleases.value = releaseList.map((release) => mapRelease(release, product))
  product.versionCount = releaseList.length
  if (!draftReleases.value.some((release) => release.id === associationTargetReleaseId.value)) {
    associationTargetReleaseId.value = draftReleases.value[0]?.id || ''
  }
}

async function openProductDetail(product: ProductRecord) {
  selectedProductName.value = product.name
  await loadProductReleases(product).catch(() => { loadedReleases.value = [] })
  dialog.value = 'productDetail'
}

async function openInstall(product: ProductRecord) {
  selectedProductName.value = product.name
  await loadProductReleases(product).catch(() => { loadedReleases.value = [] })
  const firstPublishedRelease = loadedReleases.value.find((release) => release.status === 'published')
  installForm.value.name = `${product.name.toLowerCase().replace(/\s+/g, '-')}-prod`
  installForm.value.version = firstPublishedRelease?.version || product.version
  installStep.value = 1
  dialog.value = 'install'
}

function openDialog(type: Exclude<DialogType, 'productDetail' | 'install' | null>) {
  if (type === 'product') productForm.value = { id: '', key: '', name: '', category: 'application', description: '', status: 'draft', visibility: 'tenant', project: 'hnb' }
  if (type === 'release') releaseForm.value = { id: '', productId: selectedPublishingProductId.value, version: '', releaseNotes: '', manifest: { artifacts: [] } }
  if (type === 'artifact') {
    artifactForm.value = { id: '', fileName: '', artifactType: 'helm_chart', digest: '', status: 'pending', lifecycle: 'active', sizeBytes: 0 }
    artifactFileError.value = ''
    uploader.reset()
  }
  if (type === 'repository') repoForm.value = { id: '', name: '', type: 'OCI Registry', endpoint: '', authMode: 'secret', secretRef: '', syncMode: 'manual', status: 'active' }
  dialog.value = type
}

async function enterProductWorkspace(product: ProductRecord) {
  if (!product.id) return
  selectedPublishingProductId.value = product.id
  selectedArtifactNode.value = ''
  associationTargetReleaseId.value = ''
  selectedArtifacts.value = []
  loadedArtifacts.value = []
  artifactCenterError.value = ''
  try {
    await Promise.all([
      loadProductReleases(product),
      refreshUnassignedCount(),
    ])
  } catch (err) {
    loadedReleases.value = []
    artifactCenterError.value = err instanceof Error ? err.message : t('application.marketPage.artifactCenter.releaseLoadFailed')
  }
}

async function refreshUnassignedCount() {
  try {
    const items = await api.listUnassignedArtifacts()
    unassignedCount.value = items.length
  } catch {
    unassignedCount.value = 0
  }
}

async function manageProductFromDetail() {
  const product = selectedProduct.value
  if (!product?.id) return
  closeDialog()
  await enterProductWorkspace(product)
}

function leaveProductWorkspace() {
  selectedPublishingProductId.value = ''
  selectedArtifactNode.value = ''
  associationTargetReleaseId.value = ''
  selectedArtifacts.value = []
  loadedReleases.value = []
  loadedArtifacts.value = []
  artifactCenterError.value = ''
}

async function selectArtifactNode(nodeId: string) {
  selectedArtifactNode.value = nodeId
  selectedArtifacts.value = []
  artifactCenterError.value = ''
  loadedArtifacts.value = []
  try {
    const artifactList = nodeId === 'unassigned'
      ? await api.listUnassignedArtifacts()
      : await api.listReleaseArtifacts(nodeId)
    loadedArtifacts.value = artifactList.map(mapArtifact)
  } catch (err) {
    artifactCenterError.value = err instanceof Error ? err.message : t('application.marketPage.artifactCenter.artifactLoadFailed')
  }
}

async function refreshWorkspace() {
  const product = selectedPublishingProduct.value
  if (!product) return
  await loadProductReleases(product)
  await refreshUnassignedCount()
  if (selectedArtifactNode.value) await selectArtifactNode(selectedArtifactNode.value)
}

async function retryArtifactCenter() {
  if (selectedArtifactNode.value) await selectArtifactNode(selectedArtifactNode.value)
  else if (selectedPublishingProduct.value) await enterProductWorkspace(selectedPublishingProduct.value)
}

function openEditProduct(product: ProductRecord) {
  productForm.value = { id: product.id || '', key: product.key || product.name, name: product.name, category: product.category, description: product.desc, status: product.status, visibility: product.visibility || 'tenant', project: product.project || 'hnb' }
  dialog.value = 'product'
}

function openEditRelease(release: ReleaseRecord) {
  releaseForm.value = { id: release.id || '', productId: release.productId || '', version: release.version, releaseNotes: release.releaseNotes, manifest: release.manifest }
  dialog.value = 'release'
}

// Artifacts are immutable (digest/size/type/repository are bound to the uploaded
// content). To change anything, detach + GC the old artifact and re-upload. The
// dialog is kept only for the upload path; the "edit" entry point is removed.
// Artifacts are immutable (digest/size/type/repository are bound to the uploaded
// content). To change anything, detach + GC the old artifact and re-upload via
// the upload dialog. The legacy openEditArtifact / updateArtifact entry points
// were removed; keeping this comment as a marker so future edits don't re-add
// the dialog for "editing" a single artifact.

function openUploadDialog() {
  if (!canUploadArtifact.value) return
  openDialog('artifact')
}

function openEditRepository(repo: RepositoryRecord) {
  repoForm.value = { id: repo.id || '', name: repo.name, type: repo.backend === 'oci' ? 'OCI Registry' : repo.type, endpoint: repo.endpoint === '-' ? '' : repo.endpoint, authMode: repo.secretRef ? 'secret' : 'anonymous', secretRef: repo.secretRef || '', syncMode: 'manual', status: repo.status === 'healthy' ? 'active' : 'disabled' }
  dialog.value = 'repository'
}

function closeDialog() {
  if (dialog.value === 'confirm' && confirmResolver) {
    resolveConfirm(false)
    return
  }
  dialog.value = null
  submitError.value = ''
}

function nextInstallStep() {
  if (installStep.value < 3) installStep.value += 1
}

function prevInstallStep() {
  if (installStep.value > 1) installStep.value -= 1
}

async function submitInstall() {
  submitError.value = ''
  try {
    const release = selectedProductReleases.value.find((item) => item.version === installForm.value.version) ?? selectedProductReleases.value[0]
    await api.createApplication({
      name: installForm.value.name,
      product_id: selectedProduct.value?.id,
      release_id: release?.id,
      workspace_id: installForm.value.workspace,
      namespace: installForm.value.namespace,
      config: { version: installForm.value.version, type: installTarget.value },
    })
    closeDialog()
  } catch (err) {
    submitError.value = errorMessage(err, 'application.marketPage.errors.installFailed')
  }
}

async function submitProduct() {
  submitError.value = ''
  const key = productForm.value.key.trim()
  const displayName = productForm.value.name.trim()
  if (!key) { submitError.value = t('application.marketPage.dialog.product.keyRequired'); return }
  if (!/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(key)) { submitError.value = t('application.marketPage.dialog.product.keyInvalid'); return }
  if (!displayName) { submitError.value = t('application.marketPage.dialog.product.displayNameRequired'); return }
  try {
    const payload = { name: key, display_name: displayName, category: productForm.value.category, description: productForm.value.description, status: productForm.value.status, visibility: productForm.value.visibility }
    if (productForm.value.id) await api.updateProduct(productForm.value.id, payload)
    else await api.createProduct(payload)
    productForm.value = { id: '', key: '', name: '', category: 'application', description: '', status: 'draft', visibility: 'tenant', project: 'hnb' }
    closeDialog()
    await fetchMarketData()
  } catch (err) {
    submitError.value = errorMessage(err, 'application.marketPage.errors.productSaveFailed')
  }
}

async function submitRelease() {
  submitError.value = ''
  const version = releaseForm.value.version.trim()
  if (!version) { submitError.value = t('application.marketPage.dialog.release.versionRequired'); return }
  try {
    const productId = releaseForm.value.productId
    const editing = !!releaseForm.value.id
    const payload = { version, release_notes: releaseForm.value.releaseNotes, manifest: releaseForm.value.manifest }
    let createdRelease: api.MarketRelease | undefined
    if (releaseForm.value.id) await api.updateRelease(releaseForm.value.id, payload)
    else if (productId) createdRelease = await api.createRelease(productId, payload)
    closeDialog()
    const product = selectedPublishingProduct.value
    if (!product) return
    const previousNode = selectedArtifactNode.value
    await loadProductReleases(product)
    if (!editing) {
      const createdId = createdRelease?.id || publishingReleases.value.find((release) => release.version === releaseForm.value.version)?.id
      if (createdId) await selectArtifactNode(createdId)
    } else if (previousNode) {
      await selectArtifactNode(previousNode)
    }
  } catch (err) { submitError.value = errorMessage(err, 'application.marketPage.errors.releaseSaveFailed') }
}

function errorMessage(err: unknown, fallbackKey: string): string {
  if (err instanceof Error && err.message) return err.message
  return t(fallbackKey)
}

// runBatched runs `task` for every item of `items` with at most `limit`
// concurrent in-flight requests. It rejects with the first error encountered.
const BATCH_CONCURRENCY = 5
async function runBatched<T>(items: T[], limit: number, task: (item: T) => Promise<unknown>): Promise<void> {
  let index = 0
  const worker = async () => {
    while (index < items.length) {
      const current = index
      index += 1
      await task(items[current])
    }
  }
  await Promise.all(Array.from({ length: Math.min(limit, Math.max(items.length, 1)) }, worker))
}

async function associateSelectedArtifacts() {
  const artifactIds = selectedArtifacts.value.filter(Boolean)
  const releaseId = associationTargetReleaseId.value
  if (!artifactIds.length || !releaseId) return
  artifactCenterError.value = ''
  try {
    await runBatched(artifactIds, BATCH_CONCURRENCY, (artifactId) => api.attachArtifact(releaseId, artifactId))
    selectedArtifacts.value = []
    const product = selectedPublishingProduct.value
    if (product) await loadProductReleases(product)
    await selectArtifactNode('unassigned')
  } catch (err) {
    artifactCenterError.value = err instanceof Error ? err.message : t('application.marketPage.artifactCenter.associateFailed')
  }
}

async function removeSelectedArtifacts() {
  if (selectedArtifactNode.value === 'unassigned') {
    await deleteRecords('artifacts', selectedArtifacts.value)
    return
  }
  const release = selectedWorkspaceRelease.value
  const artifactIds = selectedArtifacts.value.filter(Boolean)
  if (!release?.id || release.status !== 'draft' || !artifactIds.length) return
  const confirmed = await requestConfirm({
    title: t('application.marketPage.confirm.detachSelectedTitle'),
    message: t('application.marketPage.confirm.detachSelectedMessage', { count: artifactIds.length }),
    confirmText: t('application.marketPage.actions.detachSelected'),
  })
  if (!confirmed) return
  artifactCenterError.value = ''
  try {
    await runBatched(artifactIds, BATCH_CONCURRENCY, (artifactId) => api.detachArtifact(release.id!, artifactId))
    selectedArtifacts.value = []
    await refreshWorkspace()
  } catch (err) {
    artifactCenterError.value = err instanceof Error ? err.message : t('application.marketPage.artifactCenter.detachFailed')
  }
}

async function deleteArtifactFromRelease(artifactId: string, releaseId: string) {
  const name = artifactNameById(artifactId) || artifactId
  const confirmed = await requestConfirm({
    title: t('application.marketPage.confirm.deleteArtifactTitle'),
    message: t('application.marketPage.confirm.deleteArtifactMessage', { name }),
    confirmText: t('application.marketPage.actions.delete'),
    danger: true,
  })
  if (!confirmed) return
  artifactCenterError.value = ''
  try {
    await api.detachArtifact(releaseId, artifactId)
    await api.gcArtifact(artifactId, { operation_id: `ui-gc-${Date.now()}`, reason: 'delete from release row' })
    await refreshWorkspace()
  } catch (err) {
    artifactCenterError.value = errorMessage(err, 'application.marketPage.artifactCenter.detachFailed')
  }
}

async function submitArtifact() {
  submitError.value = ''
  try {
    // Artifacts are immutable: only the upload path remains. The edit entry
    // point was removed from the row UI; this guard catches accidental
    // navigation and surfaces a clear error.
    if (artifactForm.value.id) {
      submitError.value = t('application.marketPage.errors.artifactUploadFailed') + ' ' + t('application.marketPage.dialog.artifact.immutableHint')
      return
    }
    if (!uploader.file.value) {
      artifactFileError.value = t('application.marketPage.upload.errors.fileRequired')
      return
    }
    if (!isUploadableArtifactFile(uploader.file.value.name)) {
      artifactFileError.value = t('application.marketPage.upload.errors.unsupportedFormat', { formats: artifactFormatHint.value })
      return
    }
    const releaseId = selectedWorkspaceRelease.value?.id
    if (!releaseId || selectedWorkspaceRelease.value?.status !== 'draft') {
      artifactFileError.value = t('application.marketPage.upload.errors.draftRequired')
      return
    }
    const artifact = await uploader.start(artifactForm.value.artifactType, releaseId)
    if (!artifact) return
    artifactForm.value.digest = artifact.digest
    artifactForm.value.sizeBytes = artifact.size_bytes
    closeDialog()
    await selectArtifactNode(selectedArtifactNode.value)
  } catch (err) { submitError.value = errorMessage(err, 'application.marketPage.errors.artifactUploadFailed') }
}

async function resumeArtifactUpload() {
  const artifact = await uploader.resume()
  if (!artifact) return
  artifactForm.value.digest = artifact.digest
  artifactForm.value.sizeBytes = artifact.size_bytes
  closeDialog()
  await selectArtifactNode(selectedArtifactNode.value)
}

function handleArtifactFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const selectedFile = input.files?.[0] ?? null
  artifactFileError.value = ''
  uploader.selectFile(selectedFile)
  if (!selectedFile) return
  if (!isUploadableArtifactFile(selectedFile.name)) {
    artifactFileError.value = t('application.marketPage.upload.errors.unsupportedFormat', { formats: artifactFormatHint.value })
    input.value = ''
    uploader.reset()
    artifactForm.value.fileName = ''
    artifactForm.value.sizeBytes = 0
    return
  }
  artifactForm.value.fileName = selectedFile.name
  artifactForm.value.sizeBytes = selectedFile.size
  artifactForm.value.artifactType = inferArtifactType(selectedFile.name)
}

function isUploadableArtifactFile(fileName: string) {
  const normalized = fileName.toLowerCase()
  return uploadableArtifactSuffixes.some((suffix) => normalized.endsWith(suffix))
}

function inferArtifactType(fileName: string) {
  const normalized = fileName.toLowerCase()
  if (normalized.endsWith('.jar')) return 'jar'
  if (normalized.endsWith('.war')) return 'war'
  if (normalized.endsWith('.oci') || normalized.endsWith('.tar')) return 'oci_image'
  if (normalized.endsWith('.zip') || normalized.endsWith('.bundle')) return 'offline_bundle'
  if (normalized.endsWith('.tgz') || normalized.endsWith('.tar.gz')) return 'helm_chart'
  return 'generic'
}

function formatBytes(bytes: number) {
  if (!bytes) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KiB`
  if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MiB`
  return `${(bytes / 1073741824).toFixed(2)} GiB`
}

function formatRate(bytesPerSecond: number) {
  if (!bytesPerSecond) return '-'
  return `${formatBytes(bytesPerSecond)}/s`
}

function formatDuration(seconds: number) {
  if (!seconds) return '-'
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  if (minutes < 60) return `${minutes}m ${rest}s`
  const hours = Math.floor(minutes / 60)
  return `${hours}h ${minutes % 60}m`
}

async function submitRepository() {
  submitError.value = ''
  const name = repoForm.value.name.trim()
  const endpoint = repoForm.value.endpoint.trim()
  if (!name) { submitError.value = t('application.marketPage.dialog.repository.nameRequired'); return }
  if (!endpoint) { submitError.value = t('application.marketPage.dialog.repository.endpointRequired'); return }
  try {
    const payload = { name, backend: repoBackend(repoForm.value.type), endpoint, secret_reference: repoForm.value.authMode === 'secret' ? repoForm.value.secretRef : '', lifecycle_state: repoForm.value.status, service_tier: 'minimal', authority_role: 'authoritative', metadata: { sync_mode: repoForm.value.syncMode, display_type: repoForm.value.type } }
    if (repoForm.value.id) await api.updateRepository(repoForm.value.id, payload)
    else await api.createRepository(payload)
    repoForm.value = { id: '', name: '', type: 'OCI Registry', endpoint: '', authMode: 'secret', secretRef: '', syncMode: 'manual', status: 'active' }
    closeDialog()
    await fetchMarketData()
  } catch (err) {
    submitError.value = errorMessage(err, 'application.marketPage.errors.repositorySaveFailed')
  }
}

function repoBackend(type: string) {
  return type === 'S3' ? 's3' : type === 'PVC' ? 'pvc' : type === 'Local' ? 'local' : 'oci'
}

function productNameById(id: string) {
  return loadedProducts.value.find((p) => p.id === id)?.name
}
function repositoryNameById(id: string) {
  return loadedRepositories.value.find((r) => r.id === id)?.name
}
function releaseVersionById(id: string) {
  return loadedReleases.value.find((r) => r.id === id)?.version
}
function artifactNameById(id: string) {
  return loadedArtifacts.value.find((a) => a.id === id)?.name
}

async function deleteRecords(kind: 'products' | 'releases' | 'artifacts' | 'repositories', ids: string[]) {
  const actualIds = ids.filter(Boolean)
  if (!actualIds.length) return
  const kindLabels: Record<typeof kind, string> = {
    products: t('application.marketPage.tabs.market'),
    releases: t('application.marketPage.publishing.version'),
    artifacts: t('application.marketPage.artifacts.title'),
    repositories: t('application.marketPage.tabs.repositories'),
  }
  const items: string[] = []
  if (kind === 'products') items.push(...actualIds.map((id) => productNameById(id) || id))
  else if (kind === 'repositories') items.push(...actualIds.map((id) => repositoryNameById(id) || id))
  else if (kind === 'releases') items.push(...actualIds.map((id) => releaseVersionById(id) || id))
  else if (kind === 'artifacts') items.push(...actualIds.map((id) => artifactNameById(id) || id))
  const confirmed = await requestConfirm({
    title: t('application.marketPage.confirm.deleteTitle', { kind: kindLabels[kind] }),
    message: t('application.marketPage.confirm.deleteSelected', { count: actualIds.length }),
    items,
    confirmText: t('application.marketPage.actions.delete'),
    danger: true,
  })
  if (!confirmed) return
  if (kind === 'products') await runBatched(actualIds, BATCH_CONCURRENCY, (id) => api.deleteProduct(id))
  if (kind === 'releases') await runBatched(actualIds, BATCH_CONCURRENCY, (id) => api.deleteRelease(id))
  if (kind === 'artifacts' && selectedArtifactNode.value === 'unassigned') await runBatched(actualIds, BATCH_CONCURRENCY, (id) => api.gcArtifact(id, { operation_id: `ui-gc-${Date.now()}`, reason: 'delete from console' }))
  if (kind === 'repositories') await runBatched(actualIds, BATCH_CONCURRENCY, (id) => api.deleteRepository(id))
  selectedProducts.value = []
  selectedArtifacts.value = []
  selectedRepositories.value = []
  if (kind === 'releases' && actualIds.includes(selectedArtifactNode.value)) {
    selectedArtifactNode.value = ''
    loadedArtifacts.value = []
  }
  if (kind === 'products' || kind === 'repositories') await fetchMarketData()
  else await refreshWorkspace()
}

async function publishRelease(release: ReleaseRecord) {
  if (!release.id) return
  artifactCenterError.value = ''
  try {
    await api.publishRelease(release.id)
    await refreshWorkspace()
  } catch (err) {
    artifactCenterError.value = errorMessage(err, 'application.marketPage.errors.publishFailed')
  }
}

onMounted(() => {
  fetchMarketData()
})

watch([query, category, scope], () => {
  currentPage.value = 1
  fetchMarketData()
})

async function switchMarketTab(tab: MarketTab) {
  activeTab.value = tab
  await fetchMarketData()
}
</script>

<template>
  <section class="market-page">
    <header class="market-hero">
      <div>
        <p class="market-eyebrow">{{ t('application.marketPage.eyebrow') }}</p>
        <h1>{{ t('application.marketPage.title') }}</h1>
        <p>{{ t('application.marketPage.desc') }}</p>
      </div>
    </header>

    <nav class="market-tabs" aria-label="App market tabs">
      <button v-for="tab in tabs" :key="tab.key" type="button" :class="{ active: activeTab === tab.key }" @click="switchMarketTab(tab.key)">
        {{ tab.label }}
      </button>
    </nav>

    <div v-if="loading" class="market-loading" role="status">
      <span class="spinner" aria-hidden="true"></span>
      <span>{{ t('application.marketPage.loading') }}</span>
    </div>

    <section v-if="activeTab === 'market'" class="market-section">
      <template v-if="!selectedPublishingProduct">
        <div class="store-toolbar">
          <div class="scope-tabs" role="tablist">
            <button v-for="s in scopeTabs" :key="s.key" type="button" :class="{ active: scope === s.key }" @click="toggleScope(s.key)" role="tab">{{ s.label }}</button>
            <div class="scope-tabs-spacer"></div>
            <button class="primary-button" type="button" @click="openDialog('product')">{{ t('application.marketPage.actions.createProduct') }}</button>
          </div>
          <div class="store-toolbar-row">
            <input v-model="query" :placeholder="t('application.marketPage.store.searchPlaceholder')" />
          </div>
          <div class="category-pills">
            <button v-for="item in categories" :key="item.key" type="button" :class="{ active: category === item.key }" @click="category = item.key">
              {{ item.label }}
            </button>
          </div>
        </div>

        <div v-if="totalPages > 1" class="pagination-bar">
          <span class="pagination-info">{{ totalProducts }} {{ t('application.marketPage.store.products') }}</span>
          <div class="pagination-controls">
            <button class="page-button" :disabled="currentPage <= 1" @click="currentPage--; fetchMarketData()">‹</button>
            <span v-for="p in visiblePages" :key="p" :class="{ active: p === currentPage }" class="page-num" @click="currentPage = p; fetchMarketData()">{{ p }}</span>
            <button class="page-button" :disabled="currentPage >= totalPages" @click="currentPage++; fetchMarketData()">›</button>
          </div>
        </div>

        <div class="product-grid">
          <article v-for="product in filteredProducts" :key="product.name" class="product-card">
            <div class="card-actions">
              <button class="icon-action" type="button" :title="t('application.marketPage.actions.edit')" :aria-label="t('application.marketPage.actions.edit')" @click="openEditProduct(product)">
                <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M11.7 1.3a1 1 0 0 1 1.4 0l1.6 1.6a1 1 0 0 1 0 1.4l-8.8 8.8-3.6.6.6-3.6 8.8-8.8ZM5.5 11.9l.4-.1 7.4-7.4-1.7-1.7-7.4 7.4-.1.4-.3 1.7 1.7-.3Z" /></svg>
              </button>
              <button class="icon-action danger" type="button" :title="t('application.marketPage.actions.delete')" :aria-label="t('application.marketPage.actions.delete')" @click="deleteRecords('products', [product.id || ''])">
                <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M6 1h4l.7 1.5H14v1.3H2V2.5h3.3L6 1Zm-2.8 4h9.6l-.6 9.2A1.9 1.9 0 0 1 10.3 16H5.7a1.9 1.9 0 0 1-1.9-1.8L3.2 5Zm2.1 1.3.5 7.8c0 .3.2.5.5.5h3.4c.3 0 .5-.2.5-.5l.5-7.8H5.3Z" /></svg>
              </button>
            </div>
            <div class="product-icon">{{ product.name.slice(0, 1) }}</div>
            <div class="product-main">
              <div class="product-title-row">
                <button class="product-title-button" type="button" @click="openProductDetail(product)">{{ product.name }}</button>
                <span class="status-pill" :class="`status-${product.status}`">{{ t(`application.marketPage.status.${product.status}`) }}</span>
                
              </div>
              <p>{{ product.desc }}</p>
              <div class="product-meta">
                <span>{{ t(`application.marketPage.categories.${product.category}`) }}</span>
                <span>{{ product.version }}</span>
                <span>{{ product.publisher }}</span>
                <span>{{ product.project }}</span>
              </div>
              <footer>
                <button v-if="product.status === 'published'" class="primary-button" type="button" @click="openInstall(product)">{{ t('application.marketPage.actions.install') }}</button>
                <button class="secondary-button" type="button" @click="enterProductWorkspace(product)">{{ t('application.marketPage.actions.manage') }}</button>
              </footer>
            </div>
          </article>
          <div v-if="!apiAvailable && !filteredProducts.length" class="empty-state error-state">
            <span>{{ apiErrorMessage || t('application.marketPage.errors.loadFailed') }}</span>
            <button class="secondary-button" type="button" @click="fetchMarketData">{{ t('application.marketPage.actions.retry') }}</button>
          </div>
          <div v-else-if="!filteredProducts.length" class="empty-state">{{ t('application.marketPage.empty.store') }}</div>
        </div>
      </template>

      <template v-else>
        <div class="section-card workspace-header">
          <button class="back-button" type="button" @click="leaveProductWorkspace">← {{ t('application.marketPage.artifactCenter.back') }}</button>
          <div class="workspace-title">
            <div class="product-icon">{{ selectedPublishingProduct.name.slice(0, 1) }}</div>
            <div>
              <h2>{{ selectedPublishingProduct.name }}</h2>
              <p>{{ t(`application.marketPage.categories.${selectedPublishingProduct.category}`) }} · {{ t(`application.marketPage.status.${selectedPublishingProduct.status}`) }}</p>
            </div>
          </div>
          <div class="action-row">
            <button class="secondary-button" type="button" @click="openDialog('release')">{{ t('application.marketPage.actions.createRelease') }}</button>
            <button class="primary-button" type="button" :disabled="!canUploadArtifact" @click="openUploadDialog">{{ t('application.marketPage.actions.uploadArtifact') }}</button>
          </div>
          <p v-if="!canUploadArtifact" class="workspace-guidance">{{ t('application.marketPage.artifactCenter.selectDraftHint') }}</p>
        </div>

        <div class="artifact-workspace">
          <aside class="section-card version-panel">
            <h3>{{ t('application.marketPage.artifactCenter.versions') }}</h3>
            <button type="button" class="version-node special" :class="{ active: selectedArtifactNode === 'unassigned' }" @click="selectArtifactNode('unassigned')">
              <strong>{{ t('application.marketPage.artifactCenter.unassigned') }}</strong>
              <span v-if="unassignedCount > 0" class="badge">{{ unassignedCount }}</span>
            </button>
            <p class="version-hint">{{ t('application.marketPage.artifactCenter.unassignedHint') }}</p>
            <div v-for="release in publishingReleases" :key="release.id" class="release-node" :class="{ active: selectedArtifactNode === release.id }">
              <button type="button" class="version-node" @click="selectArtifactNode(release.id || '')">
                <strong>{{ release.version }}</strong>
                <span class="status-pill" :class="`status-${release.status}`">{{ t(`application.marketPage.status.${release.status}`) }}</span>
              </button>
              <div class="release-node-actions">
                <button type="button" :disabled="release.status === 'published'" @click="openEditRelease(release)">{{ t('application.marketPage.actions.edit') }}</button>
                <button type="button" :disabled="release.status === 'published'" @click="publishRelease(release)">{{ t('application.marketPage.actions.publish') }}</button>
                <button type="button" :disabled="release.status === 'published'" @click="deleteRecords('releases', [release.id || ''])">{{ t('application.marketPage.actions.delete') }}</button>
              </div>
            </div>
            <div v-if="!publishingReleases.length" class="version-empty">{{ t('application.marketPage.empty.publishing') }}</div>
          </aside>

          <div class="section-card table-card artifact-content">
            <div class="table-toolbar release-toolbar">
              <div>
                <span class="toolbar-label">{{ t('application.marketPage.artifactCenter.currentNode') }}</span>
                <strong>{{ selectedArtifactNode === 'unassigned' ? t('application.marketPage.artifactCenter.unassigned') : selectedWorkspaceRelease?.version || t('application.marketPage.artifactCenter.selectVersion') }}</strong>
              </div>
              <div class="action-row">
                <label v-if="selectedArtifactNode === 'unassigned'" class="association-target">
                  <span>{{ t('application.marketPage.artifactCenter.draftTarget') }}</span>
                  <select v-model="associationTargetReleaseId" :disabled="!draftReleases.length">
                    <option v-if="!draftReleases.length" value="">{{ t('application.marketPage.artifactCenter.noDraft') }}</option>
                    <option v-for="release in draftReleases" :key="release.id" :value="release.id">{{ release.version }}</option>
                  </select>
                </label>
                <button v-if="selectedArtifactNode === 'unassigned'" class="primary-button" type="button" :disabled="!selectedArtifacts.length || !associationTargetReleaseId" @click="associateSelectedArtifacts">{{ t('application.marketPage.actions.associateSelected') }}</button>
                <button v-if="selectedArtifactNode === 'unassigned'" class="secondary-button" type="button" @click="openDialog('gc')">{{ t('application.marketPage.actions.gcPreview') }}</button>
                <button class="secondary-button" type="button" :disabled="!selectedArtifacts.length || (selectedArtifactNode !== 'unassigned' && selectedWorkspaceRelease?.status !== 'draft')" @click="removeSelectedArtifacts">{{ selectedArtifactNode === 'unassigned' ? t('application.marketPage.actions.batchDelete') : t('application.marketPage.actions.detachSelected') }}</button>
              </div>
            </div>
            <div v-if="selectedArtifactNode" class="table-head artifact-grid">
              <span></span>
              <span>{{ t('application.marketPage.artifacts.name') }}</span>
              <span>{{ t('application.marketPage.artifacts.type') }}</span>
              <span>{{ t('application.marketPage.artifacts.verify') }}</span>
              <span>{{ t('application.marketPage.artifacts.lifecycle') }}</span>
              <span>{{ t('application.marketPage.artifacts.size') }}</span>
              <span>{{ t('application.marketPage.common.operations') }}</span>
            </div>
            <div v-for="artifact in artifacts" :key="artifact.id || artifact.name" class="table-row artifact-grid">
              <label class="row-check"><input v-model="selectedArtifacts" type="checkbox" :disabled="selectedArtifactNode !== 'unassigned' && selectedWorkspaceRelease?.status !== 'draft'" :value="artifact.id" /></label>
              <strong>{{ artifact.name }}</strong>
              <span>{{ artifact.type }}</span>
              <span class="status-pill" :class="`status-${artifact.status}`">{{ t(`application.marketPage.status.${artifact.status}`) }}</span>
              <span>{{ artifact.lifecycle }}</span>
              <span>{{ artifact.size }}</span>
              <span class="row-actions">
                <button v-if="selectedArtifactNode !== 'unassigned' && selectedWorkspaceRelease?.status === 'draft' && artifact.id" type="button" class="danger-link" :title="t('application.marketPage.actions.delete')" :aria-label="t('application.marketPage.actions.delete')" @click="deleteArtifactFromRelease(artifact.id, selectedWorkspaceRelease!.id!)">×</button>
                <span v-else-if="selectedWorkspaceRelease?.status === 'published'" class="read-only">{{ t('application.marketPage.artifactCenter.readOnly') }}</span>
              </span>
            </div>
            <div v-if="artifactCenterError" class="empty-state in-table error-state"><span>{{ artifactCenterError }}</span><button class="secondary-button" type="button" @click="retryArtifactCenter">{{ t('application.marketPage.actions.retry') }}</button></div>
            <div v-else-if="!selectedArtifactNode" class="empty-state in-table">{{ t('application.marketPage.artifactCenter.selectVersionHint') }}</div>
            <div v-else-if="!artifacts.length" class="empty-state in-table">{{ t('application.marketPage.empty.artifacts') }}</div>
          </div>
        </div>
      </template>
    </section>

    <section v-else-if="activeTab === 'security'" class="market-section">
      <SecurityPanel />
    </section>

    <section v-else-if="activeTab === 'repositories'" class="market-section split-section">
      <div class="section-card intro-card">
        <h2>{{ t('application.marketPage.repositories.title') }}</h2>
        <p>{{ t('application.marketPage.repositories.desc') }}</p>
        <div class="repo-type-grid">
          <span>Harbor</span>
          <span>OCI Registry</span>
          <span>Helm Repository</span>
          <span>Docker Registry</span>
        </div>
      </div>
      <div class="section-card table-card">
        <div class="table-toolbar"><span></span><button class="primary-button" type="button" @click="openDialog('repository')">{{ t('application.marketPage.actions.createRepository') }}</button><button class="secondary-button" type="button" :disabled="!selectedRepositories.length" @click="deleteRecords('repositories', selectedRepositories)">{{ t('application.marketPage.actions.batchDelete') }}</button></div>
        <div class="table-head repo-grid">
          <span></span>
          <span>{{ t('application.marketPage.repositories.name') }}</span>
          <span>{{ t('application.marketPage.repositories.type') }}</span>
          <span>{{ t('application.marketPage.repositories.endpoint') }}</span>
          <span>{{ t('application.marketPage.repositories.status') }}</span>
          <span>{{ t('application.marketPage.repositories.lastSync') }}</span>
          <span>{{ t('application.marketPage.common.operations') }}</span>
        </div>
        <div v-for="repo in repositories" :key="repo.name" class="table-row repo-grid">
          <label class="row-check"><input v-model="selectedRepositories" type="checkbox" :value="repo.id" /></label>
          <strong>{{ repo.name }}</strong>
          <span>{{ repo.type }}</span>
          <code>{{ repo.endpoint }}</code>
          <span class="status-pill" :class="`status-${repo.status}`">{{ t(`application.marketPage.status.${repo.status}`) }}</span>
          <span>{{ repo.sync }}</span>
          <span class="row-actions"><button type="button" @click="openEditRepository(repo)">{{ t('application.marketPage.actions.edit') }}</button></span>
        </div>
        <div v-if="repositoryError" class="empty-state in-table error-state">
          <span>{{ repositoryError }}</span>
          <button class="secondary-button" type="button" @click="fetchMarketData">{{ t('application.marketPage.actions.retry') }}</button>
        </div>
        <div v-else-if="!repositories.length" class="empty-state in-table">{{ t('application.marketPage.empty.repositories') }}</div>
      </div>
    </section>

    <ApplicationDrawer
      v-model="drawerOpen"
      :title="dialogTitle"
      :width="dialog === 'install' || dialog === 'productDetail' ? 880 : 560"
      :error="submitError"
      :close-label="t('application.common.close')"
      hide-confirm
    >
        <section v-if="dialog === 'productDetail'" class="detail-layout">
          <div class="detail-main">
            <div class="product-icon large">{{ selectedProduct.name.slice(0, 1) }}</div>
            <h3>{{ selectedProduct.name }}</h3>
            <p>{{ selectedProduct.desc }}</p>
            <div class="product-meta">
              <span>{{ t(`application.marketPage.categories.${selectedProduct.category}`) }}</span>
              <span>{{ selectedProduct.version }}</span>
              <span>{{ selectedProduct.publisher }}</span>
            </div>
          </div>
          <div class="detail-side">
            <h3>{{ t('application.marketPage.dialog.productDetail.releases') }}</h3>
            <div v-for="release in selectedProductReleases" :key="release.version" class="release-card">
              <strong>{{ release.version }}</strong>
              <span>{{ release.channel }}</span>
              <code>{{ release.digest }}</code>
            </div>
          </div>
        </section>

        <section v-else-if="dialog === 'install'">
          <ol class="wizard-steps compact-steps">
            <li :class="{ active: installStep === 1, done: installStep > 1 }">{{ t('application.marketPage.install.version') }}</li>
            <li :class="{ active: installStep === 2, done: installStep > 2 }">{{ t('application.marketPage.install.config') }}</li>
            <li :class="{ active: installStep === 3 }">{{ t('application.marketPage.install.confirm') }}</li>
          </ol>
          <div v-if="installStep === 1" class="form-grid two-columns">
            <label><span>{{ t('application.marketPage.install.appType') }}</span><select v-model="installTarget"><option value="monolith">{{ t('application.menu.monolith') }}</option><option value="microservice">{{ t('application.menu.microservice') }}</option></select></label>
            <label><span>{{ t('application.marketPage.publishing.version') }}</span><select v-model="installForm.version"><option v-for="release in selectedProductReleases" :key="release.version">{{ release.version }}</option></select></label>
            <label><span>{{ t('application.marketPage.install.workspace') }}</span><input v-model="installForm.workspace" /></label>
            <label><span>{{ t('application.marketPage.install.namespace') }}</span><input v-model="installForm.namespace" /></label>
          </div>
          <div v-else-if="installStep === 2" class="form-grid two-columns">
            <label><span>{{ t('application.marketPage.install.name') }}</span><input v-model="installForm.name" /></label>
            <label><span>{{ t('application.marketPage.install.configMode') }}</span><select v-model="installForm.configMode"><option value="default">{{ t('application.marketPage.install.defaultConfig') }}</option><option value="custom">{{ t('application.marketPage.install.customConfig') }}</option></select></label>
            <label class="full"><span>{{ t('application.marketPage.install.values') }}</span><textarea rows="6" placeholder="replicaCount: 1&#10;resources: {}"></textarea></label>
          </div>
          <div v-else class="confirm-grid">
            <div><span>{{ t('application.marketPage.publishing.product') }}</span><strong>{{ selectedProduct.name }}</strong></div>
            <div><span>{{ t('application.marketPage.install.appType') }}</span><strong>{{ installTarget === 'monolith' ? t('application.menu.monolith') : t('application.menu.microservice') }}</strong></div>
            <div><span>{{ t('application.marketPage.publishing.version') }}</span><strong>{{ installForm.version }}</strong></div>
            <div><span>{{ t('application.marketPage.install.namespace') }}</span><strong>{{ installForm.namespace }}</strong></div>
          </div>
        </section>

        <section v-else-if="dialog === 'product'" class="form-grid">
          <div class="product-publisher-banner">
            <span class="publisher-label">{{ t('application.marketPage.dialog.product.publisherLabel') }}</span>
            <strong>{{ t('application.marketPage.dialog.product.tenantPublisher') }}</strong>
          </div>
          <label><span>{{ t('application.marketPage.dialog.product.project') }}</span><select v-model="productForm.project"><option v-for="p in harborProjects" :key="p" :value="p">{{ p }}</option></select></label>
          <label><span>{{ t('application.marketPage.dialog.product.key') }}</span><input v-model="productForm.key" :disabled="!!productForm.id" :placeholder="t('application.marketPage.dialog.product.keyPlaceholder')" pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" /></label>
          <label><span>{{ t('application.marketPage.dialog.product.displayName') }}</span><input v-model="productForm.name" :placeholder="t('application.marketPage.dialog.product.displayNamePlaceholder')" /></label>
          <label><span>{{ t('application.marketPage.dialog.product.category') }}</span><select v-model="productForm.category"><option v-for="item in categories.slice(1)" :key="item.key" :value="item.key">{{ item.label }}</option></select></label>
          <label class="full"><span>{{ t('application.marketPage.dialog.product.visibilityLabel') }}</span>
            <div class="radio-group">
              <label class="radio-label"><input v-model="productForm.visibility" type="radio" value="tenant" /> {{ t('application.marketPage.dialog.product.visibilityTenant') }}</label>
              <label class="radio-label"><input v-model="productForm.visibility" type="radio" value="public" /> {{ t('application.marketPage.dialog.product.visibilityPublic') }}</label>
            </div>
          </label>
          <label><span>{{ t('application.marketPage.publishing.status') }}</span><select v-model="productForm.status"><option value="draft">{{ t('application.marketPage.status.draft') }}</option><option value="published">{{ t('application.marketPage.status.published') }}</option><option value="archived">{{ t('application.marketPage.status.archived') }}</option></select></label>
          <label class="full"><span>{{ t('application.marketPage.dialog.product.description') }}</span><textarea v-model="productForm.description" rows="4" /></label>
          <p class="form-hint">{{ t('application.marketPage.dialog.product.keyHint') }}</p>
        </section>

        <section v-else-if="dialog === 'release'" class="form-grid two-columns">
          <label><span>{{ t('application.marketPage.publishing.product') }}</span><select v-model="releaseForm.productId" disabled><option v-for="product in products" :key="product.name" :value="product.id">{{ product.name }}</option></select></label>
          <label><span>{{ t('application.marketPage.publishing.version') }}</span><input v-model="releaseForm.version" placeholder="1.0.0" /></label>
          <label class="full"><span>{{ t('application.marketPage.dialog.release.notes') }}</span><textarea v-model="releaseForm.releaseNotes" rows="3" /></label>
          <p class="form-hint full">{{ t('application.marketPage.dialog.release.draftHint') }}</p>
        </section>

        <section v-else-if="dialog === 'artifact'" class="form-grid two-columns">
          <label v-if="!artifactForm.id"><span>{{ t('application.marketPage.publishing.product') }}</span><input :value="selectedPublishingProduct?.name || '-'" readonly /></label>
          <label v-if="!artifactForm.id"><span>{{ t('application.marketPage.upload.targetRelease') }}</span><input :value="selectedWorkspaceRelease?.version || '-'" readonly /></label>
          <label v-if="!artifactForm.id" class="full"><span>{{ t('application.marketPage.upload.file') }}</span><input type="file" :accept="artifactAccept" @change="handleArtifactFileChange" /></label>
          <label><span>{{ t('application.marketPage.artifacts.name') }}</span><input v-model="artifactForm.fileName" :readonly="!artifactForm.id" :placeholder="t('application.marketPage.upload.autoName')" /></label>
          <label><span>{{ t('application.marketPage.artifacts.type') }}</span><input :value="artifactTypeLabel" readonly /></label>
          <label v-if="artifactForm.id"><span>{{ t('application.marketPage.artifacts.verify') }}</span><select v-model="artifactForm.status"><option value="pending">{{ t('application.marketPage.status.pending') }}</option><option value="verified">{{ t('application.marketPage.status.verified') }}</option><option value="failed">{{ t('application.marketPage.status.failed') }}</option></select></label>
          <label v-if="artifactForm.id"><span>{{ t('application.marketPage.artifacts.lifecycle') }}</span><select v-model="artifactForm.lifecycle"><option value="active">{{ t('application.marketPage.lifecycle.active') }}</option><option value="tombstoned">{{ t('application.marketPage.lifecycle.tombstoned') }}</option><option value="deleting">{{ t('application.marketPage.lifecycle.deleting') }}</option><option value="deleted">{{ t('application.marketPage.lifecycle.deleted') }}</option></select></label>
          <label><span>{{ t('application.marketPage.artifacts.size') }}</span><input :value="formatBytes(artifactForm.sizeBytes)" readonly /></label>
          <label class="full"><span>{{ t('application.marketPage.dialog.artifact.digest') }}</span><input :value="artifactForm.digest || uploader.digest.value || t('application.marketPage.upload.autoDigest')" readonly /></label>
          <p v-if="!artifactForm.id" class="form-hint full">{{ t('application.marketPage.upload.allowedFormats', { formats: artifactFormatHint }) }}</p>
          <p v-if="artifactFileError" class="upload-error full">{{ artifactFileError }}</p>
          <p class="form-hint full">{{ t('application.marketPage.dialog.artifact.hint') }}</p>
          <div v-if="!artifactForm.id" class="upload-panel full">
            <div class="upload-progress-head">
              <span>{{ t(`application.marketPage.upload.status.${uploader.status.value}`) }}</span>
              <strong>{{ uploader.progress.value }}%</strong>
            </div>
            <div class="upload-progress"><span :style="{ width: `${uploader.progress.value}%` }"></span></div>
            <div class="upload-meta">
              <span>{{ t('application.marketPage.upload.uploaded') }}: {{ formatBytes(uploader.uploadedBytes.value) }} / {{ formatBytes(uploader.totalBytes.value) }}</span>
              <span>{{ t('application.marketPage.upload.speed') }}: {{ formatRate(uploader.speedBytesPerSecond.value) }}</span>
              <span>{{ t('application.marketPage.upload.eta') }}: {{ formatDuration(uploader.etaSeconds.value) }}</span>
              <span>{{ t('application.marketPage.upload.concurrency') }}: {{ uploader.effectiveConcurrency.value }}</span>
            </div>
            <p v-if="uploader.error.value" class="upload-error">{{ uploader.error.value }}</p>
          </div>
        </section>

        <section v-else-if="dialog === 'repository'" class="form-grid two-columns">
          <label><span>{{ t('application.marketPage.repositories.name') }}</span><input v-model="repoForm.name" /></label>
          <label><span>{{ t('application.marketPage.repositories.type') }}</span><select v-model="repoForm.type"><option>OCI Registry</option><option>S3</option><option>PVC</option><option>Local</option></select></label>
          <label class="full"><span>{{ t('application.marketPage.repositories.endpoint') }}</span><input v-model="repoForm.endpoint" placeholder="https://harbor.example.com" /></label>
          <label><span>{{ t('application.marketPage.dialog.repository.authMode') }}</span><select v-model="repoForm.authMode"><option value="secret">{{ t('application.marketPage.dialog.repository.secretRef') }}</option><option value="anonymous">{{ t('application.marketPage.dialog.repository.anonymous') }}</option></select></label>
          <label v-if="repoForm.authMode === 'secret'"><span>{{ t('application.marketPage.dialog.repository.secretRef') }}</span><input v-model="repoForm.secretRef" placeholder="secret://..." /></label>
          <label><span>{{ t('application.marketPage.dialog.repository.syncMode') }}</span><select v-model="repoForm.syncMode"><option value="manual">{{ t('application.marketPage.dialog.repository.manual') }}</option><option value="scheduled">{{ t('application.marketPage.dialog.repository.scheduled') }}</option></select></label>
          <label><span>{{ t('application.marketPage.repositories.status') }}</span><select v-model="repoForm.status"><option value="active">{{ t('application.marketPage.status.active') }}</option><option value="disabled">{{ t('application.marketPage.status.disabled') }}</option></select></label>
        </section>

        <section v-else-if="dialog === 'gc'">
          <p class="form-hint">{{ t('application.marketPage.dialog.gc.desc') }}</p>
          <div class="confirm-grid"><div><span>{{ t('application.marketPage.dialog.gc.safe') }}</span><strong>2</strong></div><div><span>{{ t('application.marketPage.dialog.gc.blocked') }}</span><strong>1</strong></div></div>
        </section>

        <template #footer>
          <button class="secondary-button" type="button" @click="closeDialog">{{ t('application.common.cancel') }}</button>
          <button v-if="dialog === 'install' && installStep > 1" class="secondary-button" type="button" @click="prevInstallStep">{{ t('application.common.prev') }}</button>
          <button v-if="dialog === 'product'" class="primary-button" type="button" @click="submitProduct">{{ t('application.common.confirm') }}</button>
          <button v-if="dialog === 'release'" class="primary-button" type="button" @click="submitRelease">{{ t('application.common.confirm') }}</button>
          <button v-if="dialog === 'artifact' && !artifactForm.id && uploader.canPause.value" class="secondary-button" type="button" @click="uploader.pause">{{ t('application.marketPage.upload.pause') }}</button>
          <button v-if="dialog === 'artifact' && !artifactForm.id && uploader.canResume.value" class="secondary-button" type="button" @click="resumeArtifactUpload">{{ t('application.marketPage.upload.resume') }}</button>
          <button v-if="dialog === 'artifact'" class="primary-button" type="button" :disabled="!artifactForm.id && (!uploader.canStart.value || !canUploadArtifact)" @click="submitArtifact">{{ artifactForm.id ? t('application.common.confirm') : t('application.marketPage.upload.start') }}</button>
          <button v-if="dialog === 'repository'" class="primary-button" type="button" @click="submitRepository">{{ t('application.common.confirm') }}</button>
          <button v-if="dialog === 'install' && installStep < 3" class="primary-button" type="button" @click="nextInstallStep">{{ t('application.common.next') }}</button>
          <button v-if="dialog === 'install' && installStep === 3" class="primary-button" type="button" @click="submitInstall">{{ t('application.createWizard.confirm.deploy') }}</button>
          <button v-if="dialog === 'productDetail'" class="secondary-button" type="button" @click="manageProductFromDetail">{{ t('application.marketPage.actions.manage') }}</button>
          <button v-if="dialog === 'gc' || dialog === 'productDetail'" class="primary-button" type="button" @click="closeDialog">{{ t('application.common.confirm') }}</button>
        </template>
    </ApplicationDrawer>

    <div v-if="dialog === 'confirm' && confirmOptions" class="modal-mask" role="dialog" aria-modal="true">
      <div class="modal-card">
        <header class="modal-header">
          <div>
            <p class="market-eyebrow">{{ t('application.marketPage.title') }}</p>
            <h2>{{ dialogTitle }}</h2>
          </div>
          <button class="icon-button" type="button" @click="closeDialog">×</button>
        </header>
        <section class="modal-body">
          <h3 class="confirm-title">{{ confirmOptions.title }}</h3>
          <p class="form-hint">{{ confirmOptions.message }}</p>
          <ul v-if="confirmOptions.items?.length" class="confirm-items">
            <li v-for="item in confirmOptions.items" :key="item">{{ item }}</li>
          </ul>
        </section>
        <footer class="modal-actions">
          <button class="secondary-button" type="button" @click="resolveConfirm(false)">{{ t('application.common.cancel') }}</button>
          <button :class="confirmOptions.danger ? 'danger-button' : 'primary-button'" type="button" @click="resolveConfirm(true)">{{ confirmOptions.confirmText || t('application.common.confirm') }}</button>
        </footer>
      </div>
    </div>
  </section>
</template>

<style scoped>
.market-page { min-height: 100%; padding: 24px; color: var(--hnb-color-text-primary, #edeff5); background: var(--hnb-color-bg-void, #0b0f14); }
.market-hero { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; margin-bottom: 18px; }
.market-eyebrow { margin: 0 0 6px; color: var(--hnb-color-primary, #5b8dff); font-size: 12px; font-weight: 800; letter-spacing: 0.08em; text-transform: uppercase; }
.market-hero h1 { margin: 0; font-size: 28px; }
.market-hero p { margin: 8px 0 0; color: var(--hnb-color-text-secondary, #a9b2c2); max-width: 760px; }
.market-tabs { display: flex; gap: 8px; margin-bottom: 18px; border-bottom: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); }
.market-tabs button { position: relative; padding: 12px 14px; border: 0; background: transparent; color: var(--hnb-color-text-secondary, #a9b2c2); font-weight: 700; cursor: pointer; }
.market-tabs button.active { color: #fff; }
.market-tabs button.active::after { content: ''; position: absolute; left: 12px; right: 12px; bottom: -1px; height: 2px; border-radius: 999px; background: var(--hnb-color-primary, #5b8dff); }
.market-section { display: flex; flex-direction: column; gap: 16px; }
.store-toolbar { display: flex; flex-direction: column; gap: 12px; padding: 16px; border: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); border-radius: 14px; background: var(--hnb-color-bg-void, #0b0f14); }
.store-toolbar-row { display: flex; align-items: center; gap: 12px; }
.store-toolbar-row input { flex: 1; height: 38px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 10px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); padding: 0 12px; }
.category-pills, .action-row, .repo-type-grid { display: flex; flex-wrap: wrap; gap: 8px; }
.category-pills button, .repo-type-grid span { border: 1px solid var(--hnb-color-border, #29344a); border-radius: 999px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-secondary, #a9b2c2); padding: 7px 12px; }
.category-pills button { cursor: pointer; }
.category-pills button.active { color: #fff; border-color: var(--hnb-color-primary, #5b8dff); background: var(--hnb-color-primary, #5b8dff); }
.product-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
.product-card, .section-card { border: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); border-radius: 16px; background: var(--hnb-color-bg-void, #0b0f14); box-shadow: 0 16px 48px rgba(0,0,0,0.18); }
.product-card { position: relative; display: flex; gap: 14px; padding: 16px; min-height: 210px; }
.product-icon { width: 48px; height: 48px; flex: 0 0 auto; display: grid; place-items: center; border-radius: 14px; background: linear-gradient(135deg, var(--hnb-color-primary, #5b8dff), var(--hnb-color-status-success, #12b76a)); color: #fff; font-weight: 900; font-size: 20px; }
.product-main { min-width: 0; display: flex; flex-direction: column; gap: 10px; flex: 1; }
.product-title-row { display: flex; justify-content: space-between; gap: 8px; padding-right: 72px; }
.product-title-button { border: 0; background: transparent; color: var(--hnb-color-text-primary, #edeff5); padding: 0; font-size: 18px; font-weight: 900; text-align: left; cursor: pointer; }
.product-title-button:hover { color: var(--hnb-color-primary, #5b8dff); text-decoration: underline; }
.product-main p { margin: 0; color: var(--hnb-color-text-secondary, #a9b2c2); line-height: 1.55; }
.product-meta { display: flex; flex-wrap: wrap; gap: 8px; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; }
.product-meta span { border: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); border-radius: 999px; padding: 4px 8px; }
.product-card footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: auto; }
.primary-button, .secondary-button { height: 36px; padding: 0 14px; border-radius: 9px; font-weight: 800; cursor: pointer; }
.primary-button:disabled, .secondary-button:disabled { opacity: 0.45; cursor: not-allowed; }
.primary-button { border: 1px solid var(--hnb-color-primary, #5b8dff); background: var(--hnb-color-primary, #5b8dff); color: #fff; }
.secondary-button { border: 1px solid var(--hnb-color-border, #29344a); background: transparent; color: var(--hnb-color-text-primary, #edeff5); }
.secondary-button.danger { border-color: rgba(240,68,56,0.5); color: var(--hnb-color-status-danger, #f04438); }
.split-section { display: grid; grid-template-columns: 320px minmax(0, 1fr); gap: 16px; }
.intro-card { padding: 18px; }
.intro-card h2 { margin: 0; font-size: 19px; }
.intro-card p { color: var(--hnb-color-text-secondary, #a9b2c2); line-height: 1.6; }
.table-card { overflow: hidden; }
.table-head, .table-row { display: grid; gap: 12px; align-items: center; padding: 12px 16px; }
.table-head { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; border-bottom: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); }
.table-row { border-bottom: 1px solid rgba(41,52,65,0.72); }
.table-row:last-child { border-bottom: 0; }
.table-toolbar { display: flex; justify-content: flex-end; gap: 8px; padding: 14px 16px; border-bottom: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); }
.table-toolbar.release-toolbar { justify-content: space-between; align-items: center; }
.toolbar-label { display: block; margin-bottom: 4px; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; }
.release-grid { grid-template-columns: 32px 1.2fr 0.7fr 0.7fr 0.8fr 1.1fr 1.1fr; }
.artifact-grid { grid-template-columns: 32px 1.3fr 0.8fr 0.8fr 0.8fr 0.7fr 0.9fr; }
.repo-grid { grid-template-columns: 32px 0.9fr 0.7fr 1.4fr 0.7fr 0.9fr 0.9fr; }
.row-check { display: inline-flex; align-items: center; justify-content: center; }
.card-actions { position: absolute; top: 12px; right: 12px; display: flex; gap: 6px; }
.icon-action { width: 30px; height: 30px; display: inline-grid; place-items: center; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; background: rgba(15,21,29,0.86); color: var(--hnb-color-text-primary, #edeff5); cursor: pointer; }
.icon-action:hover { border-color: var(--hnb-color-primary, #5b8dff); color: #fff; background: var(--hnb-color-primary, #5b8dff); }
.icon-action.danger:hover { border-color: rgba(240,68,56,0.72); color: var(--hnb-color-status-danger, #f04438); background: rgba(240,68,56,0.12); }
.icon-action svg { width: 15px; height: 15px; fill: currentColor; }
.row-actions { display: flex; flex-wrap: wrap; gap: 6px; }
.row-actions button { border: 1px solid var(--hnb-color-border, #29344a); border-radius: 7px; background: transparent; color: var(--hnb-color-text-primary, #edeff5); padding: 5px 8px; cursor: pointer; }
.row-actions button:disabled { opacity: 0.45; cursor: not-allowed; }
.workspace-header { position: relative; display: grid; grid-template-columns: 1fr auto; gap: 12px 20px; padding: 18px; }
.back-button { grid-column: 1 / -1; width: fit-content; border: 0; background: transparent; color: var(--hnb-color-primary, #5b8dff); padding: 0; cursor: pointer; }
.workspace-title { display: flex; align-items: center; gap: 12px; }
.workspace-title h2, .version-panel h3 { margin: 0; }
.workspace-title p { margin: 5px 0 0; color: var(--hnb-color-text-secondary, #a9b2c2); }
.workspace-header > .action-row { align-self: center; justify-content: flex-end; }
.workspace-guidance { grid-column: 1 / -1; margin: 0; color: var(--hnb-color-status-warning, #f79009); font-size: 12px; text-align: right; }
.artifact-workspace { display: grid; grid-template-columns: 260px minmax(0, 1fr); gap: 16px; align-items: start; }
.version-panel { display: grid; gap: 10px; padding: 14px; }
.version-panel h3 { padding: 4px 2px 8px; font-size: 15px; }
.release-node { overflow: hidden; border: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); border-radius: 10px; background: var(--hnb-color-bg-surface, #101425); }
.release-node.active { border-color: var(--hnb-color-primary, #5b8dff); background: var(--hnb-color-bg-elevated, #171d31); }
.version-node { width: 100%; display: flex; align-items: center; justify-content: space-between; gap: 8px; border: 0; background: transparent; color: var(--hnb-color-text-primary, #edeff5); padding: 11px; text-align: left; cursor: pointer; }
.version-node.special { border: 1px dashed var(--hnb-color-border, #29344a); border-radius: 10px; background: var(--hnb-color-bg-surface, #101425); }
.version-node.special.active { border-color: var(--hnb-color-primary, #5b8dff); background: var(--hnb-color-primary, #5b8dff); }
.release-node-actions { display: flex; gap: 5px; padding: 0 8px 8px; }
.release-node-actions button { border: 0; background: transparent; color: var(--hnb-color-text-secondary, #a9b2c2); padding: 3px; font-size: 11px; cursor: pointer; }
.release-node-actions button:disabled { opacity: 0.4; cursor: not-allowed; }
.version-empty { padding: 14px 4px; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; text-align: center; }
.artifact-content { min-width: 0; }
.association-target { display: flex; align-items: center; gap: 7px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 12px; }
.association-target select { height: 36px; max-width: 180px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 9px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); padding: 0 10px; }
.empty-state { grid-column: 1 / -1; padding: 28px 16px; border: 1px dashed var(--hnb-color-border, #29344a); border-radius: 14px; color: var(--hnb-color-text-secondary, #a9b2c2); text-align: center; background: rgba(15,21,29,0.62); }
.empty-state.in-table { border: 0; border-top: 1px solid rgba(41,52,65,0.72); border-radius: 0; background: transparent; }
.error-state { display: flex; align-items: center; justify-content: center; gap: 12px; color: var(--hnb-color-status-danger, #f04438); }
.status-pill { display: inline-flex; align-items: center; width: fit-content; border-radius: 999px; padding: 4px 9px; font-size: 12px; font-weight: 800; border: 1px solid var(--hnb-color-border, #29344a); color: var(--hnb-color-text-secondary, #a9b2c2); }
.status-published, .status-verified, .status-healthy { color: var(--hnb-color-status-success, #12b76a); border-color: rgba(18,183,106,0.38); background: rgba(18,183,106,0.1); }
.status-draft, .status-pending, .status-syncing { color: var(--hnb-color-status-warning, #f79009); border-color: rgba(253,176,34,0.38); background: rgba(253,176,34,0.1); }
.status-disabled { color: var(--hnb-color-text-secondary, #a9b2c2); }
code { color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 12px; word-break: break-all; }
.modal-mask { position: fixed; inset: 0; z-index: 1000; display: flex; justify-content: flex-end; background: rgba(0,0,0,0.58); }
.modal-card { width: min(560px, 100%); height: 100%; display: flex; flex-direction: column; background: var(--hnb-color-bg-void, #0b0f14); border-left: 1px solid var(--hnb-color-border, #29344a); box-shadow: -24px 0 80px rgba(0,0,0,0.35); }
.modal-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; padding: 20px; border-bottom: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); }
.modal-header h2 { margin: 0; font-size: 21px; }
.icon-button { width: 32px; height: 32px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; background: transparent; color: var(--hnb-color-text-primary, #edeff5); cursor: pointer; font-size: 20px; }
.modal-body { flex: 1; overflow: auto; padding: 20px; }
.modal-actions { display: flex; justify-content: flex-end; gap: 10px; padding: 16px 20px; border-top: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); }
.detail-layout { display: grid; grid-template-columns: 1.2fr 1fr; gap: 16px; }
.detail-main, .detail-side, .release-card { border: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); border-radius: 14px; background: var(--hnb-color-bg-surface, #101425); padding: 16px; }
.product-icon.large { width: 64px; height: 64px; font-size: 26px; margin-bottom: 12px; }
.detail-main h3, .detail-side h3 { margin: 0 0 10px; }
.release-card { display: grid; gap: 6px; margin-bottom: 10px; }
.wizard-steps { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; padding: 0; margin: 0 0 18px; list-style: none; }
.wizard-steps li { padding: 10px; border-radius: 10px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-tertiary, #6b7a8a); text-align: center; font-weight: 800; }
.wizard-steps li.active { color: #fff; background: var(--hnb-color-primary, #5b8dff); }
.wizard-steps li.done { color: var(--hnb-color-status-success, #12b76a); }
.form-grid { display: grid; gap: 14px; }
.form-grid.two-columns { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.form-grid .full { grid-column: 1 / -1; }
.form-grid label { display: flex; flex-direction: column; gap: 7px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; }
.form-grid input, .form-grid select, .form-grid textarea { width: 100%; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 9px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); padding: 10px 12px; }
.form-grid input[disabled] { background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-tertiary, #6b7a8a); cursor: not-allowed; }
.form-hint { margin: 0; color: var(--hnb-color-text-secondary, #a9b2c2); line-height: 1.6; }
.product-publisher-banner { display: flex; align-items: center; justify-content: space-between; padding: 10px 14px; border: 1px dashed var(--hnb-color-border, #29344a); border-radius: 10px; background: var(--hnb-color-bg-void, #0b0f14); grid-column: 1 / -1; }
.product-publisher-banner .publisher-label { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; }
.product-publisher-banner strong { color: var(--hnb-color-text-primary, #edeff5); font-size: 14px; font-weight: 600; }
.market-loading { display: flex; align-items: center; gap: 10px; padding: 12px 18px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; }
.spinner { width: 14px; height: 14px; border: 2px solid var(--hnb-color-border, #29344a); border-top-color: var(--hnb-color-status-info, #5bb8f5); border-radius: 50%; animation: market-spin 0.8s linear infinite; }
@keyframes market-spin { to { transform: rotate(360deg); } }
.confirm-title { margin: 0 0 6px; color: var(--hnb-color-text-primary, #edeff5); font-size: 16px; font-weight: 600; }
.confirm-items { list-style: none; padding: 12px 14px; margin: 0; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 9px; background: var(--hnb-color-bg-surface, #101425); max-height: 180px; overflow-y: auto; color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; line-height: 1.6; }
.confirm-items li + li { border-top: 1px solid var(--hnb-color-bg-elevated, var(--hnb-color-bg-elevated, #171d31)); }
.danger-button { background: var(--hnb-color-status-danger, #f04438); color: var(--hnb-color-status-danger, #f04438); border: 1px solid var(--hnb-color-status-danger, #f04438); }
.danger-button:hover { background: var(--hnb-color-status-danger, #f04438); }
.danger-link { background: transparent; color: var(--hnb-color-status-danger, #f04438); border: none; font-size: 18px; line-height: 1; padding: 0 6px; cursor: pointer; }
.danger-link:hover { color: var(--hnb-color-status-danger, #f04438); }
.read-only { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; font-style: italic; }
.version-hint { margin: 6px 0 12px; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; line-height: 1.5; }
.badge { display: inline-block; min-width: 22px; padding: 2px 8px; margin-left: 8px; border-radius: 999px; background: var(--hnb-color-bg-elevated, #171d31); color: var(--hnb-color-text-primary, #edeff5); font-size: 12px; font-weight: 600; text-align: center; }
.badge-public { background: var(--hnb-color-primary, #5b8dff); color: #fff; }
.scope-tabs { display: flex; align-items: center; gap: 6px; margin-bottom: 14px; }
.scope-tabs button { background: transparent; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 14px; font-weight: 500; padding: 7px 18px; cursor: pointer; }
.scope-tabs button.active { background: var(--hnb-color-primary, #5b8dff); border-color: var(--hnb-color-primary, #5b8dff); color: #fff; }
.scope-tabs button:hover:not(.active) { background: var(--hnb-color-bg-elevated, #171d31); }
.scope-tabs-spacer { flex: 1; }
.radio-group { display: flex; flex-direction: column; gap: 8px; }
.radio-label { display: flex; align-items: center; gap: 8px; color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; cursor: pointer; }
.pagination-bar { display: flex; align-items: center; justify-content: space-between; padding: 12px 0; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; }
.pagination-info { color: var(--hnb-color-text-tertiary, #6b7a8a); }
.pagination-controls { display: flex; align-items: center; gap: 4px; }
.page-button { background: transparent; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; color: var(--hnb-color-text-primary, #edeff5); font-size: 14px; padding: 6px 12px; cursor: pointer; }
.page-button:disabled { opacity: 0.4; cursor: default; }
.page-button:hover:not(:disabled) { background: var(--hnb-color-bg-elevated, #171d31); }
.page-num { display: inline-flex; align-items: center; justify-content: center; min-width: 32px; height: 32px; border-radius: 6px; color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; cursor: pointer; }
.page-num:hover { background: var(--hnb-color-bg-elevated, #171d31); }
.page-num.active { background: var(--hnb-color-primary, #5b8dff); color: #fff; }
.upload-panel { display: grid; gap: 10px; padding: 14px; border: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); border-radius: 12px; background: var(--hnb-color-bg-surface, #101425); }
.upload-progress-head, .upload-meta { display: flex; justify-content: space-between; gap: 12px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; }
.upload-progress { height: 8px; overflow: hidden; border-radius: 999px; background: var(--hnb-color-border, var(--hnb-color-border, #29344a)); }
.upload-progress span { display: block; height: 100%; border-radius: inherit; background: linear-gradient(90deg, var(--hnb-color-primary, #5b8dff), var(--hnb-color-status-success, #12b76a)); transition: width 0.18s ease; }
.upload-meta { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; }
.upload-error { margin: 0; color: var(--hnb-color-status-danger, #f04438); font-size: 12px; }
.confirm-grid { display: grid; gap: 10px; }
.confirm-grid div { display: flex; justify-content: space-between; gap: 14px; padding: 12px; border: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); border-radius: 10px; background: var(--hnb-color-bg-surface, #101425); }
.confirm-grid span { color: var(--hnb-color-text-secondary, #a9b2c2); }
@media (max-width: 1100px) { .product-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .split-section, .artifact-workspace { grid-template-columns: 1fr; } }
@media (max-width: 720px) { .market-page { padding: 16px; } .market-hero { flex-direction: column; } .product-grid { grid-template-columns: 1fr; } .table-head { display: none; } .table-row { grid-template-columns: 1fr; } .detail-layout, .form-grid.two-columns { grid-template-columns: 1fr; } .modal-card { width: 100%; } .workspace-header { grid-template-columns: 1fr; } .workspace-header > .action-row { justify-content: flex-start; } .workspace-guidance { text-align: left; } .table-toolbar.release-toolbar { align-items: flex-start; flex-direction: column; } }
</style>

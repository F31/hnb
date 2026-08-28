import { computed, ref } from 'vue'
import * as api from './marketApi'

type UploadStatus = 'idle' | 'hashing' | 'uploading' | 'paused' | 'confirming' | 'completed' | 'failed'

interface ResumeState {
  sessionId: string
  fileFingerprint: string
  endpoint: string
  offset: number
  expiresAt: string
}

function fileFingerprint(file: File): string {
  return `${file.name}:${file.size}:${file.lastModified}`
}

export function useArtifactUploader() {
  const file = ref<File | null>(null)
  const status = ref<UploadStatus>('idle')
  const uploadedBytes = ref(0)
  const totalBytes = ref(0)
  const digest = ref('')
  const error = ref('')
  const session = ref<api.UploadSessionResponse | null>(null)
  const effectiveConcurrency = ref(1)
  const controller = ref<AbortController | null>(null)
  const transferEndpoint = ref('')
  const startedAt = ref(0)
  const speedBytesPerSecond = ref(0)

  const progress = computed(() => totalBytes.value > 0 ? Math.round((uploadedBytes.value / totalBytes.value) * 100) : 0)
  const etaSeconds = computed(() => {
    if (!speedBytesPerSecond.value || uploadedBytes.value >= totalBytes.value) return 0
    return Math.ceil((totalBytes.value - uploadedBytes.value) / speedBytesPerSecond.value)
  })
  const canPause = computed(() => status.value === 'uploading')
  const canResume = computed(() => status.value === 'paused' && !!file.value && !!session.value && !!transferEndpoint.value)
  const canStart = computed(() => !!file.value && !['hashing', 'uploading', 'confirming'].includes(status.value))

  function reset() {
    controller.value?.abort()
    file.value = null
    status.value = 'idle'
    uploadedBytes.value = 0
    totalBytes.value = 0
    digest.value = ''
    error.value = ''
    session.value = null
    transferEndpoint.value = ''
    effectiveConcurrency.value = 1
    startedAt.value = 0
    speedBytesPerSecond.value = 0
  }

  function selectFile(nextFile: File | null) {
    reset()
    file.value = nextFile
    totalBytes.value = nextFile?.size ?? 0
  }

  function persistResumeState() {
    if (!file.value || !session.value || !transferEndpoint.value) return
    const key = session.value.transfer?.resume_storage_key || session.value.upload_policy?.resume_storage_key
    if (!key) return
    const state: ResumeState = {
      sessionId: session.value.session_id,
      fileFingerprint: fileFingerprint(file.value),
      endpoint: transferEndpoint.value,
      offset: uploadedBytes.value,
      expiresAt: session.value.expires_at,
    }
    localStorage.setItem(key, JSON.stringify(state))
  }

  function clearResumeState() {
    const key = session.value?.transfer?.resume_storage_key || session.value?.upload_policy?.resume_storage_key
    if (key) localStorage.removeItem(key)
  }

  async function retry<T>(operation: () => Promise<T>, maxRetries: number): Promise<T> {
    let lastError: unknown
    for (let attempt = 0; attempt <= maxRetries; attempt += 1) {
      try {
        return await operation()
      } catch (err) {
        lastError = err
        if (attempt === maxRetries) break
        await new Promise((resolve) => setTimeout(resolve, Math.min(500 * 2 ** attempt, 4000)))
      }
    }
    throw lastError
  }

  async function uploadParts(signal: AbortSignal) {
    if (!file.value || !session.value || !transferEndpoint.value) return
    const chunkSize = session.value.transfer?.chunk_size || session.value.upload_policy?.chunk_size || 8 * 1024 * 1024
    const maxRetries = session.value.transfer?.max_retries ?? session.value.upload_policy?.max_retries ?? 3
    const maxConcurrency = Math.max(1, Math.min(session.value.transfer?.max_concurrency || session.value.upload_policy?.max_concurrency || 1, 6))
    effectiveConcurrency.value = maxConcurrency
    const partCount = Math.ceil(file.value.size / chunkSize)
    const remoteStatus = await api.getTransferStatus(transferEndpoint.value).catch(() => null)
    const completed = new Set(remoteStatus?.completed_parts ?? [])
    uploadedBytes.value = remoteStatus?.uploaded_bytes ?? 0
    if (!startedAt.value) startedAt.value = Date.now()
    const completedBytesByWorker = new Map<number, number>()
    function updateProgress(delta: number) {
      uploadedBytes.value = Math.min(totalBytes.value, uploadedBytes.value + delta)
      const elapsedSeconds = Math.max((Date.now() - startedAt.value) / 1000, 0.001)
      speedBytesPerSecond.value = uploadedBytes.value / elapsedSeconds
    }
    let nextPart = 1
    const worker = async (workerIndex: number) => {
      while (nextPart <= partCount) {
        if (signal.aborted) throw new DOMException('upload aborted', 'AbortError')
        const partNumber = nextPart
        nextPart += 1
        if (completed.has(partNumber)) continue
        const start = (partNumber - 1) * chunkSize
        const end = Math.min(start + chunkSize, file.value!.size)
        const chunk = file.value!.slice(start, end)
        await retry(() => api.uploadTransferPart(transferEndpoint.value, partNumber, chunk), maxRetries)
        completedBytesByWorker.set(workerIndex, (completedBytesByWorker.get(workerIndex) || 0) + chunk.size)
        updateProgress(chunk.size)
        persistResumeState()
      }
    }
    await Promise.all(Array.from({ length: maxConcurrency }, (_, index) => worker(index)))
  }

  async function start(artifactType: string, releaseId?: string) {
    if (!file.value) return null
    error.value = ''
    controller.value = new AbortController()
    try {
      status.value = 'hashing'
      session.value = await api.createUploadSession({
        filename: file.value.name,
        artifact_type: artifactType,
        size_bytes: file.value.size,
        ...(releaseId ? { release_id: releaseId } : {}),
      })
      totalBytes.value = file.value.size
      transferEndpoint.value = session.value.transfer?.endpoint || session.value.transfer_endpoint || ''
      if (!transferEndpoint.value) throw new Error('transfer endpoint is not available')
      persistResumeState()
      status.value = 'uploading'
      startedAt.value = Date.now()
      await uploadParts(controller.value.signal)
      status.value = 'confirming'
      const artifact = await api.completeTransfer(transferEndpoint.value)
      digest.value = artifact.digest
      clearResumeState()
      status.value = 'completed'
      return artifact
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        status.value = 'paused'
        persistResumeState()
        return null
      }
      error.value = err instanceof Error ? err.message : String(err)
      status.value = 'failed'
      return null
    }
  }

  async function resume() {
    if (!file.value || !session.value || !transferEndpoint.value) return null
    error.value = ''
    controller.value = new AbortController()
    try {
      status.value = 'uploading'
      if (!startedAt.value) startedAt.value = Date.now()
      await uploadParts(controller.value.signal)
      status.value = 'confirming'
      const artifact = await api.completeTransfer(transferEndpoint.value)
      digest.value = artifact.digest
      clearResumeState()
      status.value = 'completed'
      return artifact
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        status.value = 'paused'
        persistResumeState()
        return null
      }
      error.value = err instanceof Error ? err.message : String(err)
      status.value = 'failed'
      return null
    }
  }

  function pause() {
    controller.value?.abort()
  }

  return {
    file,
    status,
    uploadedBytes,
    totalBytes,
    digest,
    error,
    session,
    progress,
    effectiveConcurrency,
    speedBytesPerSecond,
    etaSeconds,
    canPause,
    canResume,
    canStart,
    selectFile,
    start,
    pause,
    resume,
    reset,
  }
}

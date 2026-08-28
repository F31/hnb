<script setup lang="ts">
import { reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBButton, HNBFormField, HNBSelectInput } from '@hnb/ui-kit'
import type { ProviderBackendSchema, StorageBackendInput } from '@hnb/contracts/storage'

const props = withDefaults(defineProps<{
  schema: ProviderBackendSchema
  submitting?: boolean
}>(), { submitting: false })

const emit = defineEmits<{ submit: [input: StorageBackendInput] }>()
const { t } = useI18n()

const common = reactive({
  backendId: '',
  displayName: '',
  description: '',
  secretProvider: '',
  secretScope: '',
  secretName: '',
  secretVersion: '',
})
const attributes = reactive<Record<string, string | number | boolean>>({})

function updateAttribute(name: string, type: string, event: Event): void {
  const target = event.target as HTMLInputElement
  if (type === 'boolean') attributes[name] = target.checked
  else if (type === 'number') attributes[name] = target.value === '' ? 0 : Number(target.value)
  else attributes[name] = target.value
}

function fieldLabel(name: string, fallback: string): string {
  const key = {
    provisioner: 'provisioner',
    volumeBindingMode: 'volumeBindingMode',
    allowExpansion: 'allowExpansion',
    server: 'server',
    exportPath: 'exportPath',
    readOnly: 'readOnly',
  }[name]
  return key ? t(`resource.storage.backendForm.attribute.${key}`) : fallback
}

function optionLabel(option: string): string {
  const key = {
    Immediate: 'Immediate',
    WaitForFirstConsumer: 'WaitForFirstConsumer',
  }[option]
  return key ? t(`resource.storage.backendForm.option.${key}`) : option
}

function submit(): void {
  const secretReference = common.secretProvider && common.secretScope && common.secretName
    ? {
        provider: common.secretProvider,
        scope: common.secretScope,
        name: common.secretName,
        ...(common.secretVersion ? { version: common.secretVersion } : {}),
      }
    : undefined
  emit('submit', {
    providerType: props.schema.providerType,
    providerSchemaVersion: props.schema.providerSchemaVersion,
    backendId: common.backendId,
    displayName: common.displayName,
    ...(common.description ? { description: common.description } : {}),
    ...(secretReference ? { secretReference } : {}),
    attributes: { ...attributes },
  })
}
</script>

<template>
  <form class="backend-form" :aria-label="$t('resource.storage.backendForm.ariaLabel')" @submit.prevent="submit">
    <div class="form-grid">
      <HNBFormField :label="$t('resource.storage.backendForm.backendId')" input-id="storage-backend-id" required>
        <input id="storage-backend-id" v-model="common.backendId" required maxlength="256" />
      </HNBFormField>
      <HNBFormField :label="$t('resource.storage.backendForm.displayName')" input-id="storage-backend-name" required>
        <input id="storage-backend-name" v-model="common.displayName" required maxlength="256" />
      </HNBFormField>
      <HNBFormField :label="$t('resource.storage.backendForm.description')" input-id="storage-backend-description">
        <input id="storage-backend-description" v-model="common.description" maxlength="2048" />
      </HNBFormField>
      <HNBFormField :label="$t('resource.storage.backendForm.secretProvider')" input-id="storage-secret-provider" :help="$t('resource.storage.backendForm.secretHelp')">
        <input id="storage-secret-provider" v-model="common.secretProvider" maxlength="256" autocomplete="off" />
      </HNBFormField>
      <HNBFormField :label="$t('resource.storage.backendForm.secretScope')" input-id="storage-secret-scope" :help="$t('resource.storage.backendForm.scopeHelp')">
        <input id="storage-secret-scope" v-model="common.secretScope" maxlength="256" autocomplete="off" />
      </HNBFormField>
      <HNBFormField :label="$t('resource.storage.backendForm.secretName')" input-id="storage-secret-name">
        <input id="storage-secret-name" v-model="common.secretName" maxlength="256" autocomplete="off" />
      </HNBFormField>
      <HNBFormField :label="$t('resource.storage.backendForm.secretVersion')" input-id="storage-secret-version">
        <input id="storage-secret-version" v-model="common.secretVersion" maxlength="256" autocomplete="off" />
      </HNBFormField>
      <HNBFormField v-for="field in schema.fields" :key="field.name" :label="fieldLabel(field.name, field.label)" :input-id="`storage-attribute-${field.name}`" :required="field.required">
        <input
          v-if="field.type === 'boolean'"
          :id="`storage-attribute-${field.name}`"
          type="checkbox"
          @change="updateAttribute(field.name, field.type, $event)"
        />
        <HNBSelectInput
          v-else-if="field.type === 'select'"
          :model-value="String(attributes[field.name] ?? '')"
          :options="(field.options ?? []).map((option) => ({ label: optionLabel(option), value: option }))"
          @update:model-value="attributes[field.name] = $event"
        />
        <input
          v-else
          :id="`storage-attribute-${field.name}`"
          :type="field.type"
          :required="field.required"
          maxlength="512"
          @input="updateAttribute(field.name, field.type, $event)"
        />
      </HNBFormField>
    </div>
    <HNBButton type="submit" variant="primary" :loading="submitting">{{ $t('resource.storage.backendForm.create') }}</HNBButton>
  </form>
</template>

<style scoped>
.backend-form { display: flex; flex-direction: column; gap: var(--hnb-space-md); padding: var(--hnb-space-md); border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-lg); background: var(--hnb-color-bg-surface); }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--hnb-space-md); }
input:not([type='checkbox']) { width: 100%; min-height: 34px; box-sizing: border-box; padding: 0 var(--hnb-space-sm); color: var(--hnb-color-text-primary); background: var(--hnb-color-bg-surface); border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); }
input:focus { outline: 2px solid color-mix(in srgb, var(--hnb-color-primary) 30%, transparent); border-color: var(--hnb-color-primary); }
@media (max-width: 768px) { .form-grid { grid-template-columns: 1fr; } }
</style>

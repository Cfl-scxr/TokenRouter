import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const {
  listRoutersMock,
  createRouterMock,
  updateRouterMock,
  deleteRouterMock,
  listProfilesMock,
  showSuccessMock,
  showErrorMock
} = vi.hoisted(() => ({
  listRoutersMock: vi.fn(),
  createRouterMock: vi.fn(),
  updateRouterMock: vi.fn(),
  deleteRouterMock: vi.fn(),
  listProfilesMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
    showSuccess: showSuccessMock,
    showInfo: vi.fn()
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    tlsFingerprintRouters: {
      list: listRoutersMock,
      create: createRouterMock,
      update: updateRouterMock,
      delete: deleteRouterMock
    },
    tlsFingerprintProfiles: {
      list: listProfilesMock
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (params && typeof params.index !== 'undefined') return `${key}:${params.index}`
        if (params && typeof params.name !== 'undefined') return `${key}:${params.name}`
        return key
      }
    })
  }
})

import TLSFingerprintRoutersModal from '../TLSFingerprintRoutersModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const ConfirmDialogStub = defineComponent({
  name: 'ConfirmDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show"><button data-testid="confirm-delete" @click="$emit(\'confirm\')">confirm</button></div>'
})

const SelectStub = defineComponent({
  name: 'Select',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue', 'change'],
  setup(props, { emit }) {
    const onChange = (event: Event) => {
      const target = event.target as HTMLSelectElement
      const option =
        (props.options as Array<Record<string, unknown>>).find(
          item => String(item.value ?? '') === target.value
        ) ?? null
      const value = option ? option.value as string | number | boolean | null : target.value
      emit('update:modelValue', value)
      emit('change', value, option)
    }

    return {
      onChange
    }
  },
  template: `
    <select v-bind="$attrs" :value="modelValue ?? ''" @change="onChange">
      <option v-for="option in options" :key="String(option.value)" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
})

const routerRecord = {
  id: 9,
  name: 'Codex Router',
  description: 'UA based',
  enabled: true,
  rules: [
    {
      name: 'codex mac',
      enabled: true,
      match_type: 'contains',
      pattern: 'codex_cli',
      case_sensitive: false,
      tls_fingerprint_profile_id: 7,
      upstream_user_agent: 'codex_cli_rs/0.125.0',
      upstream_originator: 'codex_cli_rs'
    }
  ],
  created_at: '2026-06-01T00:00:00Z',
  updated_at: '2026-06-01T00:00:00Z'
}

function mountModal() {
  return mount(TLSFingerprintRoutersModal, {
    props: {
      show: true
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: ConfirmDialogStub,
        Select: SelectStub,
        Icon: true
      }
    }
  })
}

describe('TLSFingerprintRoutersModal', () => {
  beforeEach(() => {
    listRoutersMock.mockReset()
    createRouterMock.mockReset()
    updateRouterMock.mockReset()
    deleteRouterMock.mockReset()
    listProfilesMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()

    listRoutersMock.mockResolvedValue([routerRecord])
    createRouterMock.mockResolvedValue(routerRecord)
    updateRouterMock.mockResolvedValue(routerRecord)
    deleteRouterMock.mockResolvedValue({ message: 'ok' })
    listProfilesMock.mockResolvedValue([{ id: 7, name: 'Codex TLS' }])
  })

  it('加载列表并支持启停路由器', async () => {
    const wrapper = mountModal()
    await flushPromises()

    expect(listRoutersMock).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Codex Router')

    const toggleButton = wrapper.findAll('button').find(button => button.text().includes('common.enabled'))
    expect(toggleButton).toBeTruthy()
    await toggleButton!.trigger('click')
    await flushPromises()

    expect(updateRouterMock).toHaveBeenCalledWith(9, { enabled: false })
  })

  it('创建路由器时会提交有序规则', async () => {
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.findAll('button').find(button =>
      button.text().includes('admin.tlsFingerprintRouters.createRouter')
    )!.trigger('click')
    await flushPromises()

    await wrapper.find('input[required]').setValue('OpenCode Router')
    await wrapper.findAll('button').find(button =>
      button.text().includes('admin.tlsFingerprintRouters.form.addRule')
    )!.trigger('click')
    await flushPromises()

    const textInputs = wrapper.findAll('input[type="text"]')
    await textInputs[2].setValue('opencode')
    await textInputs[3].setValue('opencode/')
    await textInputs[4].setValue('opencode/1.0 upstream')
    await textInputs[5].setValue('opencode')
    const selects = wrapper.findAll('select')
    await selects[0].setValue('7')
    await selects[1].setValue('prefix')
    await wrapper.findAll('button').find(button => button.text().includes('common.create'))!.trigger('click')
    await flushPromises()

    expect(createRouterMock).toHaveBeenCalledWith({
      name: 'OpenCode Router',
      description: null,
      enabled: true,
      rules: [
        {
          name: 'opencode',
          enabled: true,
          match_type: 'prefix',
          pattern: 'opencode/',
          case_sensitive: false,
          tls_fingerprint_profile_id: 7,
          upstream_user_agent: 'opencode/1.0 upstream',
          upstream_originator: 'opencode'
        }
      ]
    })
  })

  it('规则 pattern 为空时阻止保存', async () => {
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.findAll('button').find(button =>
      button.text().includes('admin.tlsFingerprintRouters.createRouter')
    )!.trigger('click')
    await flushPromises()

    await wrapper.find('input[required]').setValue('Invalid Router')
    await wrapper.findAll('button').find(button =>
      button.text().includes('admin.tlsFingerprintRouters.form.addRule')
    )!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('common.create'))!.trigger('click')

    expect(createRouterMock).not.toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenCalledWith('admin.tlsFingerprintRouters.form.patternRequired')
  })

  it('删除路由器会调用删除接口', async () => {
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.attributes('title') === 'common.delete')!.trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="confirm-delete"]').trigger('click')
    await flushPromises()

    expect(deleteRouterMock).toHaveBeenCalledWith(9)
    expect(showSuccessMock).toHaveBeenCalledWith('admin.tlsFingerprintRouters.deleteSuccess')
  })
})

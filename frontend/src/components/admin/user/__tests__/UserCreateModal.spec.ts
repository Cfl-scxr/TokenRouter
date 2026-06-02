import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const { createUserMock, showErrorMock, showSuccessMock } = vi.hoisted(() => ({
  createUserMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      create: createUserMock
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
    showSuccess: showSuccessMock
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import UserCreateModal from '../UserCreateModal.vue'

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

function mountModal() {
  return mount(UserCreateModal, {
    props: {
      show: true
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Icon: true
      }
    }
  })
}

async function fillRequiredFields(wrapper: ReturnType<typeof mountModal>) {
  const inputs = wrapper.findAll('input')
  await inputs[0].setValue('new-user@example.com')
  await inputs[1].setValue('strong-pass')
}

describe('UserCreateModal', () => {
  beforeEach(() => {
    createUserMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
    createUserMock.mockResolvedValue({})
  })

  it('omits balance when the balance field is blank', async () => {
    const wrapper = mountModal()
    await fillRequiredFields(wrapper)

    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(createUserMock).toHaveBeenCalledTimes(1)
    expect(createUserMock).toHaveBeenCalledWith(expect.not.objectContaining({ balance: expect.anything() }))
  })

  it('sends explicit zero balance', async () => {
    const wrapper = mountModal()
    await fillRequiredFields(wrapper)
    await wrapper.findAll('input')[3].setValue('0')

    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(createUserMock).toHaveBeenCalledTimes(1)
    expect(createUserMock).toHaveBeenCalledWith(expect.objectContaining({ balance: 0 }))
  })
})

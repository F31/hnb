import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import HNBTable from '../HNBTable.vue'

const columns = [
  { key: 'name', title: 'Name' },
  { key: 'age', title: 'Age', width: '100px' },
]

const data = [
  { name: 'Alice', age: 30 },
  { name: 'Bob', age: 25 },
]

describe('HNBTable', () => {
  it('mounts with columns and data', () => {
    const wrapper = mount(HNBTable, { props: { columns, data } })
    expect(wrapper.props('columns')).toEqual(columns)
    expect(wrapper.props('data')).toEqual(data)
  })

  it('shows loading state', () => {
    const wrapper = mount(HNBTable, { props: { columns, data: [], loading: true } })
    expect(wrapper.exists()).toBe(true)
    expect(wrapper.props('loading')).toBe(true)
  })

  it('supports remote pagination', () => {
    const pagination = { page: 2, pageSize: 20, total: 100 }
    const wrapper = mount(HNBTable, { props: { columns, data, pagination } })
    expect(wrapper.props('pagination')).toEqual(pagination)
  })

  it('supports row selection', () => {
    const wrapper = mount(HNBTable, { props: { columns, data, selectable: true, rowKey: 'name' } })
    expect(wrapper.props('selectable')).toBe(true)
    expect(wrapper.props('rowKey')).toBe('name')
  })
})

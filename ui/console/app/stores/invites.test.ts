import { create } from '@bufbuild/protobuf'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { InviteSchema } from '../../gen/invite/v1/invite_pb'
import { deferred, setClient, setKnownKeys } from '../../test/nuxt-globals'
import { toInvite, useInvitesStore } from './invites'

const expires = new Date('2026-10-20T09:00:00Z')
const bobPB = create(InviteSchema, { id: 'i1', email: 'bob@x.io', role: 'admin', status: 'pending', invitedByName: 'Olga', expiresAt: timestampFromDate(expires) })
const dinaPB = create(InviteSchema, { id: 'i2', email: 'dina@x.io', role: 'member', status: 'draft' })

describe('toInvite', () => {
  it('converts the dates', () => {
    expect(toInvite(bobPB)).toEqual({ id: 'i1', email: 'bob@x.io', role: 'admin', status: 'pending', invitedByName: 'Olga', expiresAt: expires, createdAt: undefined })
    expect(toInvite(dinaPB).expiresAt).toBeUndefined()
  })
})

describe('useInvitesStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('loads pending invites and drafts by the org public id', async () => {
    const listInvites = vi.fn().mockResolvedValue({ invites: [bobPB], drafts: [dinaPB] })
    setClient('$inviteClient', { listInvites })
    const s = useInvitesStore()
    await s.load('orgPublic1')
    expect(listInvites).toHaveBeenCalledWith({ orgId: 'orgPublic1' })
    expect(s.pending.map(i => i.id)).toEqual(['i1'])
    expect(s.drafts.map(i => i.id)).toEqual(['i2'])
    expect(s.loading).toBe(false)
  })

  it('reads a missing list as empty', async () => {
    setClient('$inviteClient', { listInvites: vi.fn().mockResolvedValue({}) })
    const s = useInvitesStore()
    await s.load('o')
    expect(s.pending).toEqual([])
    expect(s.drafts).toEqual([])
  })

  it('shows a load error', async () => {
    setKnownKeys()
    setClient('$inviteClient', { listInvites: vi.fn().mockRejectedValue(new Error('boom')) })
    const s = useInvitesStore()
    await s.load('o')
    expect(s.error).toEqual({ message: 'error.load_invites' })
    expect(s.loading).toBe(false)
  })

  it('never shows another org\'s invites when a load fails', async () => {
    setKnownKeys()
    const listInvites = vi.fn().mockResolvedValueOnce({ invites: [bobPB], drafts: [dinaPB] }).mockRejectedValueOnce(new Error('boom'))
    setClient('$inviteClient', { listInvites })
    const s = useInvitesStore()
    await s.load('orgA')
    expect(s.pending).toHaveLength(1)
    await s.load('orgB')
    expect(s.pending).toEqual([])
    expect(s.drafts).toEqual([])
    expect(s.error).toEqual({ message: 'error.load_invites' })
    s.upsert('orgA', toInvite(bobPB))
    expect(s.pending).toEqual([])
  })

  it('keeps only the latest load', async () => {
    const first = deferred<unknown>()
    const listInvites = vi.fn().mockReturnValueOnce(first.promise).mockResolvedValueOnce({ invites: [bobPB] })
    setClient('$inviteClient', { listInvites })
    const s = useInvitesStore()
    const a = s.load('o1')
    await s.load('o2')
    first.reject(new Error('late'))
    await a
    expect(s.pending.map(i => i.id)).toEqual(['i1'])
    expect(s.error).toBeNull()
  })

  it('moves a confirmed draft to pending, replaces a resent invite and drops one', async () => {
    setClient('$inviteClient', { listInvites: vi.fn().mockResolvedValue({ invites: [bobPB], drafts: [dinaPB] }) })
    const s = useInvitesStore()
    await s.load('o1')
    s.upsert('o1', { ...toInvite(dinaPB), status: 'pending' })
    expect(s.drafts).toEqual([])
    expect(s.pending.map(i => [i.id, i.status])).toEqual([['i1', 'pending'], ['i2', 'pending']])
    s.upsert('o1', { ...toInvite(bobPB), role: 'member' })
    expect(s.pending[0]?.role).toBe('member')
    s.upsert('other', toInvite(bobPB))
    s.drop('other', 'i1')
    expect(s.pending).toHaveLength(2)
    s.drop('o1', 'i1')
    expect(s.pending.map(i => i.id)).toEqual(['i2'])
  })
})

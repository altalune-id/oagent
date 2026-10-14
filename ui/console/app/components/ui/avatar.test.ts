import { describe, expect, it } from 'vitest'
import { avatarColor, avatarColors, avatarView, initials } from './avatar'

describe('initials', () => {
  it('takes the first and last word of the name, else the first letter of the email', () => {
    expect(initials('Ana Maria', 'ana@x.io')).toBe('AM')
    expect(initials('  ana  de la maria ', '')).toBe('AM')
    expect(initials('Cy', 'cy@x.io')).toBe('C')
    expect(initials('', 'bob@x.io')).toBe('B')
    expect(initials('   ', '')).toBe('?')
    expect(initials('Élodie Ünal', '')).toBe('ÉÜ')
  })
})

describe('avatarColor', () => {
  it('is stable for one public id and spreads over the palette', () => {
    expect(avatarColor('V1StGXR8Z5jdHi6BmyT2a')).toBe(avatarColor('V1StGXR8Z5jdHi6BmyT2a'))
    const ids = ['V1StGXR8Z5jdHi6BmyT2a', 'FtGrdYskgDVDf8aoKsLdn', '5ieWO8_t8V9AC9GBMLHfx', 'izdsizt2UHJ1Cmr3uN0VP', 'adaUserPublicId000001', 'bobUserPublicId000002']
    expect(new Set(ids.map(avatarColor)).size).toBeGreaterThan(2)
    for (const id of ids) {
      expect(avatarColors).toContain(avatarColor(id))
    }
    expect(avatarColor('')).toBe(avatarColors[0])
  })
})

describe('avatarView', () => {
  it('shows the picture when there is one, with the name as its alt text', () => {
    expect(avatarView({ id: 'x', name: 'Ana Maria', email: 'ana@x.io', avatarUrl: 'https://x/a.png' }))
      .toEqual({ kind: 'image', src: 'https://x/a.png', alt: 'Ana Maria' })
    expect(avatarView({ id: 'x', name: '', email: 'ana@x.io', avatarUrl: 'https://x/a.png' }))
      .toEqual({ kind: 'image', src: 'https://x/a.png', alt: 'ana@x.io' })
  })

  it('shows initials on a color picked by the public id otherwise', () => {
    expect(avatarView({ id: 'V1StGXR8Z5jdHi6BmyT2a', name: 'Ana Maria', email: 'ana@x.io' }))
      .toEqual({ kind: 'initials', text: 'AM', label: 'Ana Maria', color: avatarColor('V1StGXR8Z5jdHi6BmyT2a') })
    expect(avatarView({ id: '', name: '', email: 'bob@x.io', avatarUrl: '' }))
      .toEqual({ kind: 'initials', text: 'B', label: 'bob@x.io', color: avatarColors[0] })
  })
})

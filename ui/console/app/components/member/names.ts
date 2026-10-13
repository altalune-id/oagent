export function initials(name: string, email: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  if (words.length > 0) {
    const first = Array.from(words[0]!)[0]!
    const last = words.length > 1 ? Array.from(words[words.length - 1]!)[0]! : ''
    return (first + last).toUpperCase()
  }
  const letter = Array.from(email.trim())[0]
  return letter ? letter.toUpperCase() : '?'
}

export function displayName(m: { name: string, email: string }): string {
  return m.name.trim() || m.email
}

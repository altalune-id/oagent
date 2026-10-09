<script setup lang="ts">
import { useAPIKeysStore } from '~/stores/apiKeys'

const { orgId } = defineProps<{ orgId: string }>()

const keys = useAPIKeysStore()
const { $apiKeyClient: client } = useNuxtApp()

await keys.ensureLoaded(orgId)

async function revoke(id: string) {
  try {
    await client.revokeAPIKey({ orgId, id })
    keys.markRevoked(id)
  }
  catch (e) {
    keys.error = e instanceof Error ? e.message : 'Failed to revoke API key'
  }
}
</script>

<template>
  <p v-if="keys.error" class="text-sm text-error">
    {{ keys.error }}
  </p>
  <p v-else-if="keys.items.length === 0" class="text-sm text-base-content/60">
    No API keys yet.
  </p>
  <div v-else class="overflow-x-auto">
    <table class="table table-sm">
      <thead>
        <tr>
          <th>Name</th>
          <th>Kind</th>
          <th>Scopes</th>
          <th>Hint</th>
          <th>Status</th>
          <th />
        </tr>
      </thead>
      <tbody>
        <tr v-for="k in keys.items" :key="k.id">
          <td>{{ k.name }}</td>
          <td>{{ k.kind }}</td>
          <td class="text-base-content/60">
            {{ k.scopes.join(', ') }}
          </td>
          <td class="text-base-content/60">
            …{{ k.secretHint }}
          </td>
          <td>
            <span class="badge badge-sm" :class="k.revokedAt ? 'badge-error' : 'badge-success'">
              {{ k.revokedAt ? 'revoked' : 'active' }}
            </span>
          </td>
          <td class="text-right">
            <button v-if="!k.revokedAt" class="btn btn-ghost btn-xs text-error" @click="revoke(k.id)">
              Revoke
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

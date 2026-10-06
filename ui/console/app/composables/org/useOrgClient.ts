import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { OrgService } from '~~/gen/org/v1/org_pb'

let client: ReturnType<typeof build> | undefined

function build() {
  const config = useRuntimeConfig()
  return createClient(OrgService, createConnectTransport({ baseUrl: config.public.apiUrl || '/api' }))
}

export function useOrgClient() {
  return (client ??= build())
}

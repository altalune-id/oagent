import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { ProjectService } from '~~/gen/project/v1/project_pb'

let client: ReturnType<typeof build> | undefined

function build() {
  const config = useRuntimeConfig()
  return createClient(ProjectService, createConnectTransport({ baseUrl: config.public.apiUrl || '/api' }))
}

export function useProjectClient() {
  return (client ??= build())
}

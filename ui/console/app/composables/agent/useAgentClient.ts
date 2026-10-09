import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { AgentService } from '~~/gen/agent/v1/agent_pb'

let client: ReturnType<typeof build> | undefined

function build() {
  const config = useRuntimeConfig()
  return createClient(AgentService, createConnectTransport({ baseUrl: config.public.apiUrl || '/api' }))
}

export function useAgentClient() {
  return (client ??= build())
}

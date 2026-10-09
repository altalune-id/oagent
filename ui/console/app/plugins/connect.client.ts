import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { AgentService } from '~~/gen/agent/v1/agent_pb'
import { APIKeyService } from '~~/gen/apikey/v1/apikey_pb'
import { OrgService } from '~~/gen/org/v1/org_pb'
import { ProjectService } from '~~/gen/project/v1/project_pb'

// One Connect transport for every client: same origin as the app and cookie-bearing (ready for
// session auth). The auth interceptor, when it lands, belongs here.
export default defineNuxtPlugin({
  name: 'connect',
  setup() {
    const config = useRuntimeConfig()
    const transport = createConnectTransport({
      baseUrl: config.public.apiUrl || '/api',
      fetch: (input, init) => globalThis.fetch(input, { ...init, credentials: 'include' }),
    })
    return {
      provide: {
        orgClient: createClient(OrgService, transport),
        projectClient: createClient(ProjectService, transport),
        agentClient: createClient(AgentService, transport),
        apiKeyClient: createClient(APIKeyService, transport),
      },
    }
  },
})

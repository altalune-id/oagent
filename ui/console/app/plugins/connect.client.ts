import type { Interceptor } from '@connectrpc/connect';
import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { AgentService } from '~~/gen/agent/v1/agent_pb';
import { APIKeyService } from '~~/gen/apikey/v1/apikey_pb';
import { AuthService } from '~~/gen/auth/v1/auth_pb';
import { InviteService } from '~~/gen/invite/v1/invite_pb';
import { MemberService } from '~~/gen/member/v1/member_pb';
import { OrgService } from '~~/gen/org/v1/org_pb';
import { ProjectService } from '~~/gen/project/v1/project_pb';
import { WebhookService } from '~~/gen/webhook/v1/webhook_pb';
import { createRedirectOnce, isUnauthenticated } from '~/lib/auth/unauthenticated';
import { loginURL } from '~/lib/auth/urls';

// One Connect transport for every client: same origin as the app and cookie-bearing. A 401 sends the
// browser to /login once and returns here afterwards. Whoami has its own transport without that
// redirect, so an anonymous console (before credentials are required) stays usable.
export default defineNuxtPlugin({
  name: 'connect',
  setup() {
    const config = useRuntimeConfig();
    const apiUrl = config.public.apiUrl;
    const redirectOnce = createRedirectOnce(url => window.location.assign(url));
    const toLogin: Interceptor = next => async (req) => {
      try {
        return await next(req);
      } catch (err) {
        if (isUnauthenticated(err)) {
          redirectOnce(loginURL(apiUrl, window.location));
        }
        throw err;
      }
    };
    const options = {
      baseUrl: apiUrl || '/api',
      fetch: (input: RequestInfo | URL, init?: RequestInit) => globalThis.fetch(input, { ...init, credentials: 'include' }),
    };
    const transport = createConnectTransport({ ...options, interceptors: [toLogin] });
    const whoamiTransport = createConnectTransport(options);
    return {
      provide: {
        orgClient: createClient(OrgService, transport),
        projectClient: createClient(ProjectService, transport),
        agentClient: createClient(AgentService, transport),
        apiKeyClient: createClient(APIKeyService, transport),
        webhookClient: createClient(WebhookService, transport),
        memberClient: createClient(MemberService, transport),
        inviteClient: createClient(InviteService, transport),
        authClient: createClient(AuthService, whoamiTransport),
      },
    };
  },
});

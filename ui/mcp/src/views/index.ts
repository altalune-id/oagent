import type { Component } from 'vue';
import type { Descriptor } from '../generic/descriptor';
import DetailView from '../generic/DetailView.vue';
import ListView from '../generic/ListView.vue';
import AuthWhoami from './AuthWhoami.vue';
import DefaultView from './DefaultView.vue';
import { descriptors } from './descriptors';
import InviteConfirm from './InviteConfirm.vue';

const registry: Record<string, Component> = {
  member_invite: InviteConfirm,
  auth_whoami: AuthWhoami,
};

export interface ResolvedView {
  component: Component;
  descriptor?: Descriptor;
}

// resolveView picks the view for the calling tool: a generic view for a described tool, else its own view, else the JSON fallback.
export function resolveView(tool: string): ResolvedView {
  const descriptor = descriptors[tool];
  if (descriptor) {
    return { component: descriptor.kind === 'list' ? ListView : DetailView, descriptor };
  }
  return { component: registry[tool] ?? DefaultView };
}

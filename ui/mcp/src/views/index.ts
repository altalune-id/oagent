import type { Component } from "vue";
import OrgCreateView from "./OrgCreateView.vue";
import OrgGetView from "./OrgGetView.vue";
import OrgListView from "./OrgListView.vue";
import ProjectCreateView from "./ProjectCreateView.vue";
import ProjectListView from "./ProjectListView.vue";
import DefaultView from "./DefaultView.vue";

const registry: Record<string, Component> = {
  org_create: OrgCreateView,
  org_get: OrgGetView,
  org_list: OrgListView,
  project_create: ProjectCreateView,
  project_list: ProjectListView,
};

// resolveView picks the view for the calling tool, falling back to a generic renderer.
export function resolveView(tool: string): Component {
  return registry[tool] ?? DefaultView;
}

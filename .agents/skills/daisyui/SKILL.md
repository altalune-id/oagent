---
name: daisyui
description: daisyUI 5 component classes, semantic colors and themes on Tailwind CSS 4, as used in oagent. Use when writing or changing markup in the console (Vue components and pages under ui/console/app) or the sign-in pages (internal/**/*.templ), choosing a daisyUI component (btn, card, modal, dropdown, drawer, menu, table, fieldset, input, toggle and others), using semantic colors such as primary, base-100 or base-content, or adding or changing a theme in ui/theme.css.
license: MIT
metadata:
  version: "5.7.x"
  source: Adapted from saadeghi/daisyui@bf3bac3 skills/daisyui (MIT), https://daisyui.com/SKILL.md
---

# daisyUI 5
daisyUI 5 is a CSS library for Tailwind CSS 4. It supplies class names for common UI components, semantic colors, and themes.

## In oagent

- daisyUI 5 is installed in two packages, pinned in the `pnpm-workspace.yaml` catalog: the console (`ui/console`, Tailwind 4 through `@tailwindcss/vite`, CSS in `app/assets/css/main.css`) and the sign-in pages (`ui/web/app.css`, which scans `internal/**/*.templ`; run `make web-css` after adding classes). The MCP view (`ui/mcp`) does not use Tailwind or daisyUI. You do not need references/install.md unless a new package needs daisyUI.
- Built-in themes are off (`@plugin "daisyui" { themes: false; }`). Every theme is a light and dark pair in `ui/theme.css` (`oagent-light`/`oagent-dark`, `bumblebee-light`/`bumblebee-dark` and so on), registered as a family in `ui/console/app/components/layout/useTheme.ts`. The console sets `data-theme` on `<html>`; the sign-in pages use `oagent-light`. To add a theme: add both modes with `@plugin "daisyui/theme"`, add the dark one to the `dark` custom variant at the top of `ui/theme.css`, and register the family. Do not change themes through the plugin options in references/config.md.
- Use semantic colors only (`bg-base-100`, `text-base-content/60`, `btn-primary`, `border-base-300`), so every theme works. `dark:` is mapped to the `*-dark` themes, but do not pair it with daisyUI colors: they already follow the theme.
- In the console, reuse `ui/console/app/components/ui` (`AppButton`, `AppModal`, `AppSelect`, `NavbarShell`, `SidebarNav`, `FieldError` and others) before writing raw daisyUI markup, and follow docs/howto/console-page.md. For templ pages, follow docs/howto/web-page.md.
- The binary embeds its assets: do not add CDN links, and do not use external placeholder images in product UI.

## When to use this skill

- Writing or changing markup with Tailwind classes in the console (`ui/console/app`) or the sign-in pages (`internal/**/*.templ`).
- Choosing a component, a semantic color, a size or a variant, or adding or changing a theme.
- The request mentions daisyUI, Tailwind, a UI component, layout, theme or color.

## References that you must read

| Task | Guide | Note |
|------|-------|------|
Install daisyUI | [references/install.md](references/install.md) | If the project does not have daisyUI, use this guide.
Use daisyUI class names | [references/usage.md](references/usage.md) | You must read this guide before you use daisyUI class names in code.
Configure daisyUI | [references/config.md](references/config.md) | Read this guide to configure themes, prefixes, logs, or other options. You do not need it for basic use.
daisyUI colors and themes | [references/colors.md](references/colors.md) | You must read this guide before you use daisyUI colors in code.
daisyUI components | [references/components/](references/components/) | You must read the applicable component guides before you use daisyUI components. Read more than one candidate guide before you select a component.

## List of components

- [accordion](references/components/accordion.md)
- [aura](references/components/aura.md)
- [alert](references/components/alert.md)
- [avatar](references/components/avatar.md)
- [badge](references/components/badge.md)
- [breadcrumbs](references/components/breadcrumbs.md)
- [button](references/components/button.md)
- [calendar](references/components/calendar.md)
- [card](references/components/card.md)
- [carousel](references/components/carousel.md)
- [chat](references/components/chat.md)
- [checkbox](references/components/checkbox.md)
- [collapse](references/components/collapse.md)
- [countdown](references/components/countdown.md)
- [diff](references/components/diff.md)
- [divider](references/components/divider.md)
- [dock (app bar)](references/components/dock.md)
- [drawer (sidebar)](references/components/drawer.md)
- [dropdown](references/components/dropdown.md)
- [FAB](references/components/fab.md)
- [fieldset](references/components/fieldset.md)
- [file-input](references/components/file-input.md)
- [filter](references/components/filter.md)
- [footer](references/components/footer.md)
- [hero](references/components/hero.md)
- [hover-3d](references/components/hover-3d.md)
- [hover-gallery](references/components/hover-gallery.md)
- [indicator](references/components/indicator.md)
- [input](references/components/input.md)
- [join (group)](references/components/join.md)
- [kbd](references/components/kbd.md)
- [label](references/components/label.md)
- [link](references/components/link.md)
- [list](references/components/list.md)
- [loading](references/components/loading.md)
- [mask](references/components/mask.md)
- [megamenu](references/components/megamenu.md)
- [menu](references/components/menu.md)
- [mockup-browser](references/components/mockup-browser.md)
- [mockup-code](references/components/mockup-code.md)
- [mockup-phone](references/components/mockup-phone.md)
- [mockup-window](references/components/mockup-window.md)
- [modal](references/components/modal.md)
- [navbar](references/components/navbar.md)
- [otp](references/components/otp.md)
- [pagination](references/components/pagination.md)
- [progress](references/components/progress.md)
- [radial-progress](references/components/radial-progress.md)
- [radio](references/components/radio.md)
- [range](references/components/range.md)
- [rating](references/components/rating.md)
- [select](references/components/select.md)
- [skeleton](references/components/skeleton.md)
- [stack](references/components/stack.md)
- [stat](references/components/stat.md)
- [status](references/components/status.md)
- [steps](references/components/steps.md)
- [swap](references/components/swap.md)
- [tab](references/components/tab.md)
- [table](references/components/table.md)
- [text-rotate](references/components/text-rotate.md)
- [textarea](references/components/textarea.md)
- [theme-controller](references/components/theme-controller.md)
- [timeline](references/components/timeline.md)
- [toast](references/components/toast.md)
- [toggle (switch)](references/components/toggle.md)
- [tooltip](references/components/tooltip.md)
- [validator](references/components/validator.md)

### Component discovery protocol

Before you write daisyUI code, do these steps in sequence:

1. Identify the intended function, behavior, and layout in the request. Do not use only the exact words.
2. Use the component list in this file to select the best candidate components.
3. If the choice is not clear, read the guides for the candidate components that can meet the request before you select one.
4. Compare the description, behavior, syntax, and rules of each candidate with the request.
5. Select the best component or component combination. Obey all its constraints.
6. Use the exact structure and constraints of the selected components.

You must match the meaning, even when the words are different from component names. A component with a different name can still be the best match. Always examine the intended function and meaning.

If the user requests a named component and a guide with that name exists, read that guide first.

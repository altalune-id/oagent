import type { DescMessage, MessageShape } from '@bufbuild/protobuf';

import type { ShownError } from '~/lib/apperror/message';
import type { FieldIssue, FieldIssues } from '~/lib/validation/violations';
import { useErrorMessage } from '~/composables/useErrorMessage';
import { errorCode } from '~/lib/apperror/message';
import { checkRequest, serverIssues } from '~/lib/validation/violations';

export interface ValidationOptions {
  // fields are the proto field names the form has an input for.
  fields: string[];
  // codes names the input a domain error code belongs under (ORG002 → slug).
  codes?: Record<string, string>;
}

// useValidation checks a request against its proto rules before it is sent, and places the server's refusals under the same inputs.
export function useValidation<Desc extends DescMessage>(schema: Desc, options: ValidationOptions) {
  const { t, te } = useI18n();
  const errorMessage = useErrorMessage();
  const issues = ref<FieldIssues>({});
  const placed = ref<Record<string, string>>({});

  const shown = new Set(options.fields);

  function describe(field: string, issue: FieldIssue): string {
    if (field === 'slug' && issue.reason === 'invalid_format') {
      return t('validation.slug_format');
    }
    if (!issue.param) {
      return t(`validation.${issue.reason}`);
    }
    return t(`validation.${issue.reason}`, issue.reason === 'too_short' ? { min: issue.param } : { max: issue.param });
  }

  function label(field: string): string {
    return te(`field.${field}`) ? t(`field.${field}`) : field;
  }

  const errors = computed<Record<string, string>>(() => {
    const out: Record<string, string> = { ...placed.value };
    for (const [field, issue] of Object.entries(issues.value)) {
      if (shown.has(field)) {
        out[field] = describe(field, issue);
      }
    }
    return out;
  });

  const formError = computed<ShownError | null>(() => {
    const hidden = Object.entries(issues.value).find(([field]) => !shown.has(field));
    return hidden ? { message: t('validation.other', { field: label(hidden[0]), message: describe(hidden[0], hidden[1]) }) } : null;
  });

  function clear() {
    issues.value = {};
    placed.value = {};
  }

  // check reports whether msg may be sent; when it may not, errors and formError say why.
  function check(msg: MessageShape<Desc>): boolean {
    placed.value = {};
    issues.value = checkRequest(schema, msg);
    return Object.keys(issues.value).length === 0;
  }

  // fromServer places a refusal under its inputs and returns null, or returns the error the form shows itself.
  function fromServer(err: unknown, fallbackKey: string): ShownError | null {
    clear();
    const server = serverIssues(err);
    if (Object.keys(server).length > 0) {
      issues.value = server;
      return formError.value;
    }
    const field = options.codes?.[errorCode(err)];
    if (field && shown.has(field)) {
      placed.value = { [field]: errorMessage(err, fallbackKey).message };
      return null;
    }
    return errorMessage(err, fallbackKey);
  }

  return { errors, formError, check, fromServer, clear };
}

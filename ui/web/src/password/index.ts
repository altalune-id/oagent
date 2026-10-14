import { enhance } from './field';

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => enhance(document));
} else {
  enhance(document);
}

import { expectedInventoryProbes } from './inventory-policy.mjs';

/** IR06/IR10: explicit artifact-bound selection before reading observations.
 * Missing selection preserves the legacy grammar, never guesses from output. */
export function stepResultPolicy({resultPolicy='',resultVariant='',...identity}) {
  if(resultPolicy===''&&resultVariant==='') return false;
  if(resultPolicy!=='same-exec-v1') throw new Error('Invalid step result policy');
  expectedInventoryProbes({...identity,inventoryPolicy:'per-process-v1',inventoryVariant:resultVariant});
  return true;
}

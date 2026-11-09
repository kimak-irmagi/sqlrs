import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

/** Public source/image pins, from independently recorded natural-run oracles.
 * PA03: a benchmark never follows a mutable default branch or latest tag.
 */
export function fixtureSpec(name) {
  if (name === 'jhipster') return {
    repository: 'https://github.com/jhipster/jhipster-sample-app', commit: '6b000b5d23a36c45e01472471b84a44fa2464044',
    directory: 'jhipster-sample-app', source: 'src/main/resources', steps: 8,
    database: 'postgres@sha256:7352e0c4d62bbac8aa69d95e40220a60967c4a19f9c4f65b4d118175f7ce9e3b', version: '5.0.3',
    image: 'liquibase/liquibase@sha256:17f98b85e6d4c33b84ee409d055f1526227c77b55dd30e3879ec5088716a32eb',
    core: '3ccd4608829836b4c0f7fb1d23a4619bac16c68ca725d623aae0838bce734156' };
  if (['mobilitydata-user', 'mobilitydata-main'].includes(name)) return {
    repository: 'https://github.com/MobilityData/mobility-feed-api', commit: 'ed866e07b9af277ae78305d17e87ea34f4e6674b',
    directory: 'mobilitydata', source: 'liquibase', steps: name.endsWith('user') ? 12 : 102,
    database: 'postgis/postgis@sha256:2a60827705ad26af212f31ca6ed4884f9fa95fffbe7eed5be516fece1bca25f8', version: '5.0.4',
    image: 'liquibase/liquibase@sha256:89efc0141afc7c7afe8df1b8fecf30a755da2e29a55e259c3dadd24adc9aea11',
    core: '00d0a18af81a12583d03524b6e685b26e3734183d965a6d79c1e9479d5e1c115' };
  throw new Error('Unknown public fixture');
}

export function validateSourceHead(head, name) {
  if (head.trim() !== fixtureSpec(name).commit) throw new Error('Public source pin differs');
  return head.trim();
}

/** PA02: Go metadata is read without executing the unverified binary. */
export function validateMetadata(text, expected) {
  const settings = Object.fromEntries([...text.matchAll(/^\s*build\s+([^=\s]+)=(.*)$/gm)].map(m => [m[1], m[2].trim()]));
  if (!/^[a-f0-9]{40}$/.test(expected) || settings['vcs.revision'] !== expected || settings['vcs.modified'] !== 'false'
    || settings.GOOS !== 'linux' || settings.GOARCH !== 'amd64') throw new Error('Artifact build provenance differs');
}

/** TA03: choose build input independently of observer tooling, before writes.
 * Explicit roots must be the hosted workspace; Git checks are read-only.
 * Contract: issue89-performance-diagnostic-design.md API preparation amendment.
 */
export function preparationCheckout({ownRoot,sourceRoot,workspace,revision}){
  let root;
  try{
    const selected=sourceRoot||ownRoot;
    if(!path.isAbsolute(selected))throw new Error();
    root=fs.realpathSync(selected);
    if(!fs.statSync(root).isDirectory())throw new Error();
    if(sourceRoot&&(!workspace||!path.isAbsolute(workspace)||fs.realpathSync(workspace)!==root))throw new Error();
  }catch{throw new Error('Invalid preparation source root');}
  try{
    const git=args=>execFileSync('git',args,{cwd:root,encoding:'utf8',stdio:['ignore','pipe','pipe'],timeout:10000}).trim();
    if(!/^[a-f0-9]{40}$/.test(revision||'')||git(['rev-parse','HEAD'])!==revision||git(['status','--porcelain','--untracked-files=no']))throw new Error();
  }catch{throw new Error('Candidate checkout is not the fixed clean source');}
  return root;
}

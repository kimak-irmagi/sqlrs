import { sanitizeSample } from './control.mjs';
import { fixtureSpec } from './inputs.mjs';

/** Failure metadata is a fixed enum, not a sanitized copy of engine output. */
export function failureKind(output){
  if(failureHTTPStatus(output)!==undefined)return 'api_http_failure';
  const value=output.match(/P01B_FAILURE_KIND ([a-z_]+)\b/)?.[1];
  return ['base_state','plan_parse','snapshot','runtime_start','clone_state','scripts_prepare','connection_refused','jdbc_missing','permission_denied','input_missing','file_missing','cli_arguments','liquibase_execute'].includes(value)?value:'unknown';
}

/** Status number only; no body, URL, DSN or arbitrary error message escapes. */
export function failureHTTPStatus(output){
  const value=output.match(/(?:prepare rejected: HTTP|released status HTTP) ([1-5][0-9]{2})\b/)?.[1];
  return value===undefined?undefined:Number(value);
}

/** PA05/PA06/PA10: output preceding a failed process/cleanup is never evidence.
 * Only typed sample records leave the private subprocess output buffer.
 */
export function parseCollectorResult(code,output,{route,fixture,revision,engineHash}){
  if(code!==0)throw new Error('Collector process failed');
  const spec=fixtureSpec(fixture);
  let samples;
  try{samples=output.split(/\r?\n/).filter(line=>line.includes('P01B_SAMPLE ')).map(line=>sanitizeSample(JSON.parse(line.slice(line.indexOf('P01B_SAMPLE ')+12))));}
  catch{throw new Error('Invalid collector evidence');}
  if(samples.length!==2)throw new Error('Incomplete collector output');
  for(let index=0;index<2;index++){
    const sample=samples[index];
    if(sample.Route!==route||sample.Fixture!==fixture||sample.Pass!==['uncached','cached'][index]||sample.EngineSHA256!==engineHash
      ||sample.SourceRevision!==revision||sample.DriverSHA256!=='6e0e4cc2d8cae902084f8a2b18728b073a6fd9d1f87c9d8bff8f298c18185b93'
      ||sample.ImageDigest!==spec.image.split('@')[1]||sample.CoreSHA256!==spec.core
      ||sample.MigrationCLIStarts!==(index===0?spec.steps+1:1)||sample.ImageInspectionContainers!==(route==='v1'?0:1)
      ||sample.LiquibaseCLIStarts!==sample.MigrationCLIStarts+sample.ImageInspectionContainers
      ||sample.JVMStartsFromPinnedLauncher!==sample.LiquibaseCLIStarts*2+sample.ImageInspectionContainers)throw new Error('Collector workload identity differs');
  }
  return samples;
}

// DG01/DG02/DG09/DG10: closed evidence and fixed experiment identity. This
// diagnostic never produces the acceptance verdict of a P01b cohort.
// Contract: docs/architecture/issue89-performance-diagnostic-design.md.
import {cleanupBoundary} from './collector-output.mjs';
export const diagnosticPins=Object.freeze({
  revision:'668d5050ad154c1967bea4c6e40b91f3a703d5ba',
  engineHash:'1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0',
  baselineHash:'b34c358d2d1e6e969f17d4e393641dd7cb53167870b8e4674db1381bf410bbca',
});
export function diagnosticSelection(name,event,repository){
  if(name!=='pull_request'||event?.action!=='labeled'||event?.label?.name!=='performance:diagnostic'||event?.pull_request?.head?.repo?.full_name!==repository)return null;
  const sha=event.pull_request.head.sha;
  if(!/^[a-f0-9]{40}$/.test(sha))throw new Error('Invalid diagnostic selection');
  return sha;
}
export function diagnosticIdentity(prepared){
  for(const key of Object.keys(diagnosticPins))if(prepared?.[key]!==diagnosticPins[key])throw new Error('Diagnostic executable identity differs');
}
export function diagnosticSchedule(){
  return [['plain','v1'],['plain','v2'],['trace','v1'],['trace','v2'],['trace','v2'],['trace','v1'],['plain','v2'],['plain','v1']]
    .map(([mode,route],index)=>({id:index+1,mode,route}));
}
// DG09: only reviewed checkpoint labels leave the private subprocess buffer.
export function diagnosticFailureKind(output){
  const value=output.match(/P01B_DIAGNOSTIC_FAILURE ([a-z0-9_]+)(?:\s|$)/)?.[1];
  return /^(?:process_tree|transport|journal|endpoint|clock|trace_(?:[0-9]|1[0-7]))$/.test(value||'')?value:'unknown';
}
// CX03/CX04: fixed failure metadata only. The stage is the last reached phase,
// not evidence that workload execution failed there; raw lines never escape.
export function diagnosticFailureContext(output){
  const context=output.match(/P01B_DIAGNOSTIC_CONTEXT (record|pending_entry|none) (superseded_exec|exec_entry|exec_resume|other|none)(?:\s|$)/);
  const stage=output.match(/P01B_FAILURE_STAGE (identity|engine_start|configuration|uncached|cached|oracle)(?:\s|$)/);
  return {diagnosticFailure:diagnosticFailureKind(output),traceLocation:context?.[1]||'none',traceShape:context?.[2]||'none',failureStage:stage?.[1]||'collector_exit',cleanupBoundary:cleanupBoundary(output)};
}
const operations=new Set(['docker.image_inspect','docker.inspect','docker.create','docker.start','docker.stop','docker.remove','docker.copy','docker.run','docker.exec','docker.step','docker.planner','host.java','host.liquibase','host.pg_control','host.pg_identity','other','exec.unconfirmed']);
const phases=new Set(['job','step','plan_task','resolve_task','instance_task','other_task','image_binding','planning','session_start','invocation']);
const number=n=>Number.isFinite(n)&&n>=0;
const invalid=()=>{throw new Error('Invalid diagnostic evidence');};
// Deliberately rebuild every nested record: unknown private fields never reach
// the artifact, even when a future collector accidentally emits them.
export function parseDiagnosticOutput(output,traced,samples){
  let rows;
  try{rows=[...output.matchAll(/P01B_DIAGNOSTIC (.*)/g)].map(m=>JSON.parse(m[1]));}catch{invalid();}
  if(rows.length!==2)throw new Error('Incomplete diagnostic output');
  return rows.map((row,index)=>{
    if(row?.Pass!==['uncached','cached'][index]||row.Traced!==traced||!number(row.ElapsedMS)||row.ElapsedMS!==samples[index]?.ElapsedMS||!Array.isArray(row.Phases)||row.Phases.length>1000)invalid();
    const Phases=row.Phases.map(p=>{
      if(!phases.has(p?.Category)||!Number.isSafeInteger(p.Ordinal)||p.Ordinal<0||typeof p.Available!=='boolean')invalid();
      if(p.Available?(!number(p.StartMS)||!number(p.EndMS)||p.EndMS<p.StartMS):(p.StartMS!==null||p.EndMS!==null))invalid();
      return {Category:p.Category,Ordinal:p.Ordinal,Available:p.Available,StartMS:p.StartMS,EndMS:p.EndMS};
    });
    let Trace=null;
    if(traced){
      const t=row.Trace;
      if(!t||!number(t.CoveredMS)||!number(t.ResidualMS)||Math.abs(t.CoveredMS+t.ResidualMS-row.ElapsedMS)>0.01||!Array.isArray(t.Intervals)||t.Intervals.length>250000||t.Commands!==t.Intervals.length)invalid();
      const Intervals=t.Intervals.map(i=>{
        if(!operations.has(i?.Category)||!number(i.StartMS)||!number(i.EndMS)||i.EndMS<i.StartMS||i.EndMS>row.ElapsedMS+0.01)invalid();
        return {Category:i.Category,StartMS:i.StartMS,EndMS:i.EndMS};
      });
      const ordered=[...Intervals].sort((a,b)=>a.StartMS-b.StartMS);let coverage=0,left=0,right=0;
      for(const i of ordered){if(i.StartMS>right){coverage+=right-left;left=i.StartMS;}right=Math.max(right,i.EndMS);}
      coverage+=right-left;
      if(Math.abs(coverage-t.CoveredMS)>0.01)invalid();
      Trace={CoveredMS:t.CoveredMS,ResidualMS:t.ResidualMS,Commands:t.Commands,Intervals};
    }else if(row.Trace!==null)invalid();
    return {Pass:row.Pass,Traced:traced,ElapsedMS:row.ElapsedMS,Phases,Trace};
  });
}

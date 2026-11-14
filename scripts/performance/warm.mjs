import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fixtureSpec, validateSourceHead } from './inputs.mjs';
import { installJdbcDriver } from '../jdbc-driver-bundle.mjs';

/** PA03: prepare public LF source trees and pinned images outside all timing. */
export function warm(repo, names) {
  const root=path.join(repo,'.tmp/performance');
  const distributions=path.join(root,'distributions');
  const seen=new Set();
  const run=(cmd,args)=>execFileSync(cmd,args,{cwd:repo,encoding:'utf8',stdio:['ignore','pipe','pipe'],timeout:20*60*1000});
  fs.mkdirSync(distributions,{recursive:true,mode:0o700});
  for(const name of names){
    const spec=fixtureSpec(name);
    if(seen.has(spec.directory))continue;
    seen.add(spec.directory);
    const checkout=path.join(root,`source-${spec.directory}`);
    if(fs.existsSync(checkout))throw new Error('Fixture checkout already exists');
    fs.mkdirSync(checkout,{recursive:true});
    run('git',['-C',checkout,'init','--quiet']);
    run('git',['-C',checkout,'config','core.autocrlf','false']);
    run('git',['-C',checkout,'fetch','--quiet','--depth=1',spec.repository,spec.commit]);
    run('git',['-C',checkout,'checkout','--quiet','--detach','FETCH_HEAD']);
    validateSourceHead(run('git',['-C',checkout,'rev-parse','HEAD']),name);
    const target=path.join(root,'inputs',spec.directory);
    if(fs.existsSync(target))throw new Error('Fixture target already exists');
    fs.mkdirSync(path.dirname(target),{recursive:true});
    fs.cpSync(path.join(checkout,spec.source),target,{recursive:true,errorOnExist:true,force:false});
    run('docker',['pull',spec.database]);
    run('docker',['pull',spec.image]);
    const id=run('docker',['create',spec.image]).trim();
    if(!/^[a-f0-9]{64}$/.test(id))throw new Error('Distribution container identity differs');
    const distribution=path.join(distributions,`liquibase-${spec.version}`);
    fs.mkdirSync(distribution,{recursive:true});
    try{run('docker',['cp',`${id}:/liquibase/.`,distribution]);}
    finally{run('docker',['rm','-f',id]);}
    installJdbcDriver(repo,distribution);
    fs.chmodSync(path.join(distribution,'liquibase'),0o755);
  }
  return distributions;
}

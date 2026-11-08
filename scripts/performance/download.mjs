import { setTimeout } from 'node:timers/promises';
import { verifyBytes } from './control.mjs';

/** PA02: retry only transient acquisition, outside timing; pins never change. */
export async function downloadPinned(url,sha,{fetchImpl=fetch,delayMS=1000}={}){
  for(let attempt=0;attempt<3;attempt++){
    let response,bytes;
    try{
      response=await fetchImpl(url,{signal:AbortSignal.timeout(120000)});
      if(response.ok)bytes=Buffer.from(await response.arrayBuffer());
    }catch{
      if(attempt===2)throw new Error('Pinned artifact transport unavailable');
      response=undefined;
    }
    if(bytes){verifyBytes(bytes,sha);return bytes;}
    if(response && response.status!==429 && response.status<500)throw new Error('Pinned artifact unavailable');
    if(attempt===2)throw new Error('Pinned artifact acquisition exhausted');
    await setTimeout(delayMS*(attempt+1));
  }
}

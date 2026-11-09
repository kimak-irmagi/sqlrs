import { setTimeout } from 'node:timers/promises';
import { verifyBytes } from './control.mjs';

/** PA02: retry only transient acquisition, outside timing; pins never change. */
export async function downloadPinned(url,sha,{fetchImpl=fetch,delayMS=1000,fallback,apiURL}={}){
  const unavailable=async()=>{
    if(fallback){
      let value,received=false;
      try{value=await fallback();received=true;}catch{/* Only acquisition failure can enable API. */}
      // Verification deliberately lives outside catch: wrong successful bytes
      // are final, never a reason to try a different transport (TA02).
      if(received){const bytes=Buffer.from(value);verifyBytes(bytes,sha);return bytes;}
    }
    if(apiURL)return downloadPinned(apiURL,sha,{delayMS,
      fetchImpl:(target,options)=>fetchImpl(target,{...options,headers:{Accept:'application/octet-stream'}})});
    throw new Error('Pinned artifact transport unavailable');
  };
  for(let attempt=0;attempt<3;attempt++){
    let response,bytes;
    try{
      response=await fetchImpl(url,{signal:AbortSignal.timeout(120000)});
      if(response.ok)bytes=Buffer.from(await response.arrayBuffer());
    }catch{
      if(attempt===2)return unavailable();
      response=undefined;
    }
    if(bytes){verifyBytes(bytes,sha);return bytes;}
    if(response && response.status!==429 && response.status<500)throw new Error('Pinned artifact unavailable');
    if(attempt===2)return unavailable();
    await setTimeout(delayMS*(attempt+1));
  }
}

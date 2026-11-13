/** IC07: choose expectations before observing counters. This is test-only;
 * selected identity is separately byte/build verified before launching. */
export function expectedInventoryProbes({inventoryPolicy='',inventoryVariant='',route,revision,engineHash}) {
  const invalid=()=>{throw new Error('Invalid inventory probe policy');};
  if(!['v1','v2'].includes(route)) invalid();
  if(inventoryPolicy==='') {
    if(inventoryVariant!=='') invalid();
    return route==='v1'?[0,0]:[1,1];
  }
  if(inventoryPolicy!=='per-process-v1'||route!=='v2'||!['new_off','new_on'].includes(inventoryVariant)||
    typeof revision!=='string'||!/^[a-f0-9]{40}$/.test(revision)||typeof engineHash!=='string'||!/^[a-f0-9]{64}$/.test(engineHash)||
    ['668d5050ad154c1967bea4c6e40b91f3a703d5ba','cab99c4c7ced5b18f4dc43a65b002d84d8fef02d'].includes(revision)||
    ['1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0','b34c358d2d1e6e969f17d4e393641dd7cb53167870b8e4674db1381bf410bbca'].includes(engineHash)) invalid();
  return [1,0];
}

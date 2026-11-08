import assert from 'node:assert/strict';
import test from 'node:test';
import crypto from 'node:crypto';
import http from 'node:http';
import { downloadPinned } from './download.mjs';

test('PA02: bounded acquisition retries never relax the artifact checksum',async()=>{
 const bytes=Buffer.from('pinned release');const sha=crypto.createHash('sha256').update(bytes).digest('hex');
 const good={ok:true,status:200,arrayBuffer:async()=>Uint8Array.from(bytes).buffer};
 let calls=0;
 const actual=await downloadPinned('https://example.invalid/release',sha,{delayMS:0,fetchImpl:async()=>++calls===1?{ok:false,status:504}:good});
 assert.deepEqual(actual,bytes);assert.equal(calls,2);
 calls=0;
 await assert.rejects(downloadPinned('https://example.invalid/release',sha,{delayMS:0,fetchImpl:async()=>{calls++;return {ok:false,status:404};}}));
 assert.equal(calls,1);
 calls=0;
 await assert.rejects(downloadPinned('https://example.invalid/release','0'.repeat(64),{delayMS:0,fetchImpl:async()=>{calls++;return good;}}));
 assert.equal(calls,1);
 calls=0;
 await assert.rejects(downloadPinned('https://example.invalid/release',sha,{delayMS:0,fetchImpl:async()=>{calls++;throw new Error('network');}}));
 assert.equal(calls,3);
});

test('PA02: retry exhaustion, throttling and interrupted bodies stay bounded',async()=>{
 const bytes=Buffer.from('verified');const sha=crypto.createHash('sha256').update(bytes).digest('hex');
 const good={ok:true,status:200,arrayBuffer:async()=>Uint8Array.from(bytes).buffer};
 let calls=0;
 await assert.rejects(downloadPinned('https://example.invalid/release',sha,{delayMS:0,fetchImpl:async()=>{calls++;return {ok:false,status:503};}}));
 assert.equal(calls,3);
 calls=0;
 assert.deepEqual(await downloadPinned('https://example.invalid/release',sha,{delayMS:0,fetchImpl:async()=>++calls===1?{ok:false,status:429}:good}),bytes);
 calls=0;
 assert.deepEqual(await downloadPinned('https://example.invalid/release',sha,{delayMS:0,fetchImpl:async()=>++calls===1?{ok:true,status:200,arrayBuffer:async()=>{throw new Error('body interrupted');}}:good}),bytes);
});

test('PA02: real fetch verifies bytes from an isolated local test server',async()=>{
 const bytes=Buffer.from('native fetch');const sha=crypto.createHash('sha256').update(bytes).digest('hex');
 const server=http.createServer((req,res)=>res.end(bytes));
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 try{assert.deepEqual(await downloadPinned(`http://127.0.0.1:${server.address().port}/`,sha),bytes);}
 finally{await new Promise(resolve=>server.close(resolve));}
});

test('PA02: alternate transport must return the identical pinned bytes, never replace a bad checksum',async()=>{
 const bytes=Buffer.from('same release');const sha=crypto.createHash('sha256').update(bytes).digest('hex');
 let alternate=0;
 const fallback=async()=>{alternate++;return bytes;};
 const down=async()=>{throw new Error('transport');};
 assert.deepEqual(await downloadPinned('https://example.invalid/release',sha,{fetchImpl:down,fallback,delayMS:0}),bytes);
 assert.equal(alternate,1);
 await assert.rejects(downloadPinned('https://example.invalid/release','0'.repeat(64),{fetchImpl:down,fallback,delayMS:0}));
 assert.equal(alternate,2);
 const bad={ok:true,status:200,arrayBuffer:async()=>new Uint8Array([0]).buffer};
 await assert.rejects(downloadPinned('https://example.invalid/release',sha,{fetchImpl:async()=>bad,fallback,delayMS:0}));
 assert.equal(alternate,2);
});

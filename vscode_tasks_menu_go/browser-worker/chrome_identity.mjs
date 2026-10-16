// Align the browser HTTP User-Agent with its REAL bundled Chromium version.
// Avoid a hard-coded version that drifts from Playwright's Chrome for Testing.
// This does not claim to defeat bot/automation detection.
const safeVersion=value=>{
 const match=/^(\d{2,3})\.(\d{1,4})\.(\d{1,5})\.(\d{1,5})$/.exec(String(value||''));
 if(!match)throw new Error('Chromium did not report a valid browser version');
 return {major:match[1],full:match[0]};
};
export function chromeIdentity(version,platform=process.platform,arch=process.arch){
 const v=safeVersion(version);
 let platformPart,clientHintsPlatform;
 if(platform==='linux'){
  platformPart=arch==='arm64'?'X11; Linux aarch64':'X11; Linux x86_64';
  clientHintsPlatform='Linux';
 }else if(platform==='darwin'){
  platformPart='Macintosh; Intel Mac OS X 10_15_7';
  clientHintsPlatform='macOS';
 }else if(platform==='win32'){
  platformPart='Windows NT 10.0; Win64; x64';
  clientHintsPlatform='Windows';
 }else{throw new Error('Unsupported Chromium host platform');}
 // Chrome's reduced version format is common since User-Agent reduction.
 const reducedVersion=v.major+'.0.0.0';
 const userAgent='Mozilla/5.0 ('+platformPart+
  ') AppleWebKit/537.36 (KHTML, like Gecko) Chrome/'+reducedVersion+' Safari/537.36';
 return {
  userAgent,
  // CDP override keeps Chrome client hints aligned with the advertised
  // version and stops HeadlessChrome brand leakage on Chromium page targets.
  userAgentMetadata:{
   brands:[{brand:'Chromium',version:v.major},{brand:'Google Chrome',version:v.major},
    {brand:'Not A(Brand',version:'99'}],
   fullVersion:v.full,
   fullVersionList:[{brand:'Chromium',version:v.full},
    {brand:'Google Chrome',version:v.full},{brand:'Not A(Brand',version:'99.0.0.0'}],
   platform:clientHintsPlatform,platformVersion:'',architecture:arch==='arm64'?'arm':'x86',
   model:'',mobile:false,bitness:'64',wow64:false,
  }
 };
}

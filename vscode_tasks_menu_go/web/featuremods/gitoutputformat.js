// Render Git/transport progress as a terminal line, not as an append-only log.
// Git often uses carriage returns, while Keybase's git remote helper rewinds
// the progress cursor with repeated backspaces. Preserve the final visible
// text, then compact only consecutive updates to the same progress stage.
function formatTaskDeckGitOutput(source){
  const input=String(source??''),rows=[];
  let cells=[],cursor=0;
  const line=()=>cells.join('');
  const newline=()=>{rows.push(line());cells=[];cursor=0;};
  function write(char){
    // Cursor motion can move past EOL; terminal cells between are spaces.
    while(cells.length<cursor)cells.push(' ');
    cells[cursor++]=char;
  }
  for(let i=0;i<input.length;i++){
    const char=input[i];
    if(char==='\r'){cursor=0;continue;}
    if(char==='\n'){newline();continue;}
    if(char==='\b'){cursor=Math.max(0,cursor-1);continue;}
    if(char==='\x1b'){
      // CSI: strip formatting and handle the cursor/line erase sequences
      // produced by progress writers (ESC[K, ESC[2K, ESC[nD).
      if(input[i+1]==='['){
        const control=/^\x1b\[([0-?]*)([ -/]*)([@-~])/.exec(input.slice(i,i+128));
        if(control){
          const params=control[1],command=control[3],num=Math.max(0,Math.min(100000,Number.parseInt(params,10)||0));
          if(command==='K'){
            if(num===2){cells=[];cursor=0;}
            else if(num===1){for(let j=0;j<=cursor&&j<cells.length;j++)cells[j]=' ';}
            else cells.length=Math.min(cursor,cells.length);
          }else if(command==='D')cursor=Math.max(0,cursor-Math.max(1,num));
          else if(command==='C')cursor=Math.min(100000,cursor+Math.max(1,num));
          else if(command==='G')cursor=Math.max(0,Math.min(100000,Math.max(1,num)-1));
          i+=control[0].length-1;
          continue;
        }
      }
      // OSC (window titles / hyperlinks): drop up to BEL or ESC backslash.
      if(input[i+1]===']'){
        let j=i+2;
        while(j<input.length&&input[j]!=='\x07'&&!(input[j]==='\x1b'&&input[j+1]==='\\'))j++;
        if(j<input.length){i=input[j]==='\x07'?j:j+1;continue;}
        break;
      }
      continue;
    }
    // Ignore non-printing C0/DEL controls, but preserve tab separation.
    if((char<' '&&char!=='\t')||char==='\x7f')continue;
    write(char);
  }
  rows.push(line());

  // A few Git transports transform cursor-redrawn progress into separate
  // newline-delimited frames. Compact only adjacent updates of the same stage
  // while retaining distinct completed transfers, warnings and final refs.
  // Keybase reports fractional percentages inside parentheses: (16.67%).
  const compact=[];
  let previousStage='',previousCompleted=false;
  for(const row of rows){
    const phase=/^((?:remote:\s*)?[^:\n]{1,100}):\s*(?:\(\s*\d{1,3}(?:\.\d+)?%\s*\)|\d{1,3}(?:\.\d+)?%)(?=\s|$)/.exec(row.trimStart());
    const stage=phase?phase[1].trim().toLowerCase():'';
    const completed=/(?:,\s*)?(?:done\.?|completed\.?)\s*$/i.test(row);
    if(stage&&stage===previousStage&&!previousCompleted)compact[compact.length-1]=row;
    else compact.push(row);
    previousStage=stage;previousCompleted=Boolean(stage&&completed);
  }
  return compact.join('\n');
}

globalThis.TaskMenuGitOutputFormat={format:formatTaskDeckGitOutput};

// Convert the same output protocol used by Git's terminal progress writer
// into stable, copyable lines for TaskDeck's non-terminal Git panel.
// Pure function: safe to use on both completed commands and live snapshots.
function formatTaskDeckGitOutput(source){
  // ANSI color/cursor control is not meaningful inside a plain-text <pre>.
  const input=String(source??'').replace(/\x1b\[[0-?]*[ -/]*[@-~]/g,'');
  const rows=[];
  const separator=/\r\n|\r|\n/g;
  let current='',cursor=0,previous=0,match;
  function write(part){
    if(!part)return;
    if(cursor===current.length)current+=part;
    else current=current.slice(0,cursor)+part+current.slice(cursor+part.length);
    cursor+=part.length;
  }
  while((match=separator.exec(input))!==null){
    write(input.slice(previous,match.index));
    if(match[0]==='\r')cursor=0;
    else{
      rows.push(current);
      current='';cursor=0;
    }
    previous=separator.lastIndex;
  }
  write(input.slice(previous));
  rows.push(current);

  // Some Git transports/tools convert carriage-return redraws into ordinary
  // newline-separated percentages. Keep only the latest adjacent update for
  // each known-shaped progress phase; preserve all diagnostics and summaries.
  const compact=[];
  let previousProgress='';
  for(const line of rows){
    const phase=/^((?:remote:\s*)?[^:\n]{1,80}):\s*\d{1,3}%\s*\([^\n)]*\)/.exec(line.trimStart());
    const key=phase?phase[1].trim().toLowerCase():'';
    if(key&&key===previousProgress)compact[compact.length-1]=line;
    else compact.push(line);
    previousProgress=key;
  }
  return compact.join('\n');
}

globalThis.TaskMenuGitOutputFormat={format:formatTaskDeckGitOutput};

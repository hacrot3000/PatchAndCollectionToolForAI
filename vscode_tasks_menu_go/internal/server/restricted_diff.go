package server

import "net/http"

// restrictedDiffOp contains only text from a quota-charged source window.
// LeftLine/RightLine use physical, 1-based source positions; zero is omitted.
type restrictedDiffOp struct {
 Kind string `json:"kind"`
 LeftLine int `json:"left_line,omitempty"`
 RightLine int `json:"right_line,omitempty"`
 Text string `json:"text"`
}

// restrictedWindowDiff computes an LCS-based, position-aware edit script.
// The HTTP boundary limits each input to MaxVisibleLines (at most 1000).
// No Git, shell, disk access or uncharged lines are used by this function.
func restrictedWindowDiff(left,right []string,leftStart,rightStart int)[]restrictedDiffOp{
 n,m:=len(left),len(right)
 dp:=make([][]int,n+1)
 for i:=range dp{dp[i]=make([]int,m+1)}
 for i:=n-1;i>=0;i--{
  for j:=m-1;j>=0;j--{
   if left[i]==right[j]{dp[i][j]=dp[i+1][j+1]+1
   }else if dp[i+1][j]>=dp[i][j+1]{dp[i][j]=dp[i+1][j]
   }else{dp[i][j]=dp[i][j+1]}
  }
 }
 ops:=make([]restrictedDiffOp,0,n+m)
 i,j:=0,0
 for i<n||j<m{
  if i<n&&j<m&&left[i]==right[j]{
   ops=append(ops,restrictedDiffOp{Kind:"equal",LeftLine:leftStart+i,RightLine:rightStart+j,Text:left[i]})
   i++;j++
  }else if i<n&&(j==m||dp[i+1][j]>=dp[i][j+1]){
   ops=append(ops,restrictedDiffOp{Kind:"remove",LeftLine:leftStart+i,Text:left[i]})
   i++
  }else{
   ops=append(ops,restrictedDiffOp{Kind:"add",RightLine:rightStart+j,Text:right[j]})
   j++
  }
 }
 return ops
}

// restrictedDiffAPI shares the guarded file loading, authenticated identity,
// quota charging and response cap path with restrictedCompareAPI.
func (s *Server) restrictedDiffAPI(w http.ResponseWriter,r *http.Request){
 s.restrictedCompareWindowAPI(w,r,true)
}

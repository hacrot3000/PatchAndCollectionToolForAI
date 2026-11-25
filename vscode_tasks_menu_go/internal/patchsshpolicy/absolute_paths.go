package patchsshpolicy

import (
	"fmt"
	"strings"
)

// Fixed operator-approved data roots. The AI cannot supply or extend this
// list in a request. Do not allow /, /root, /etc, /home, /proc, /sys, /dev,
// /run, /tmp, or broad /var and /usr prefixes.
var absoluteDataRoots = [...]string{
	"/data", "/srv", "/opt", "/var/log", "/var/www", "/var/data",
	"/mnt", "/media", "/app", "/apps", "/logs", "/storage", "/backup", "/backups",
}

// AbsoluteDataPath is a lexical admission check only. The actual remote
// target must also resolve inside these roots at execution time.
func AbsoluteDataPath(value string) bool {
	if !strings.HasPrefix(value, "/") || !safeRelativePath.MatchString(value) ||
		strings.TrimSpace(value) != value || strings.Contains(value, "//") {
		return false
	}
	for _, part := range strings.Split(value, "/")[1:] {
		if part == ".." || strings.HasPrefix(part, "-") {
			return false
		}
	}
	for _, root := range absoluteDataRoots {
		if value == root || strings.HasPrefix(value, root+"/") {
			return true
		}
	}
	return false
}

type remotePathOperands struct {
	argv []string
	absolute map[int]bool
	relative bool
}

func newRemotePathOperands(argv []string) *remotePathOperands {
	return &remotePathOperands{argv:argv, absolute:make(map[int]bool)}
}

func (p *remotePathOperands) checkArgument(index int) bool { return p.check(index+1) }

func (p *remotePathOperands) check(index int) bool {
	if index <= 0 || index >= len(p.argv) { return false }
	value := p.argv[index]
	if RelativePath(value) {
		p.relative = true
		return true
	}
	if AbsoluteDataPath(value) {
		p.absolute[index] = true
		return true
	}
	return false
}

// Only actual file/directory operands identified by the argument grammar
// are rewritten. An identical-looking grep pattern or glob remains literal.
// Relative-only commands retain their existing behavior and exact quoting.
func (p *remotePathOperands) command() (string,error) {
	quoted := make([]string,len(p.argv))
	for i, token := range p.argv { quoted[i] = quoteArg(token) }
	if len(p.absolute)==0 { return strings.Join(quoted," "),nil }
	// Do not mix the legacy login-relative lane with an absolute-root
	// command: every explicit path in this invocation must be guarded.
	if p.relative {
		return "",fmt.Errorf("remote SSH command forbidden: use only absolute whitelist paths or only login-relative paths in one command")
	}

	// This shell fragment is generated solely by trusted Go code. No model
	// shell syntax is admitted. Resolve on the remote host, not the client,
	// and pass the checked physical path as a quoted positional operand.
	patterns:=make([]string,0,len(absoluteDataRoots)*2)
	for _,root:=range absoluteDataRoots {patterns=append(patterns,root,root+"/*")}
	guard:="td_collect_path() { "+
		"resolved=$(command realpath -e -- \"$1\") || { printf '%s\\n' 'SSH COLLECT path unavailable (realpath -e required)' >&2; return 1; }; "+
		"case \"$resolved\" in "+strings.Join(patterns,"|")+") ;; *) printf '%s\\n' 'SSH COLLECT resolved path outside data whitelist' >&2; return 1 ;; esac; "+
		"if [ \"$2\" = file ]; then [ -f \"$resolved\" ] || return 1; else [ -f \"$resolved\" ] || [ -d \"$resolved\" ] || return 1; fi; "+
		"printf '%s\\n' \"$resolved\"; }; "
	kind:="any"
	switch p.argv[0] {case "cat","head","tail","wc","nl","sed":kind="file"}
	for i:=1;i<len(p.argv);i++ {
		if !p.absolute[i]{continue}
		variable:=fmt.Sprintf("td_path_%d",i)
		guard+=variable+"=$(td_collect_path "+quoted[i]+" "+kind+") || exit 1; "
		quoted[i]="\"$"+variable+"\""
	}
	// Recursive grep -r skips encountered symlinks (never allow -R).
	// Explicitly skip devices and disable rg config, which might otherwise
	// enable following symlinks or preprocessors. find/tree do not follow
	// discovered symlinks under the existing admitted flags.
	switch p.argv[0] {
	case "grep": quoted[0]="grep --devices=skip"
	case "rg": quoted[0]="rg --no-config"
	case "tree":
		hasDepth:=false
		for _,arg:=range p.argv[1:] {if arg=="-L"{hasDepth=true}}
		if !hasDepth{quoted[0]="tree -L 3"}
	}
	return guard+"unset GREP_OPTIONS RIPGREP_CONFIG_PATH; exec "+strings.Join(quoted," "),nil
}

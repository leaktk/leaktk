package git

type Scope int

const (
	ScopeWorktree Scope = iota
	ScopeLocal
	ScopeGlobal
	ScopeSystem
)

var ScopeNames = []string{
	"worktree",
	"local",
	"global",
	"system",
}

var ScopeFlags = []string{
	"--worktree",
	"--local",
	"--global",
	"--system",
}

var scopeNameMap = make(map[string]Scope, len(ScopeNames))

func init() {
	for i, name := range ScopeNames {
		scopeNameMap[name] = Scope(i)
	}
}

func (s Scope) ID() int {
	return int(s)
}

func (s Scope) String() string {
	if int(s) < 0 || int(s) > len(ScopeNames) {
		return "<invalid>"
	}
	return ScopeNames[s]
}

func (s Scope) Flag() string {
	if int(s) < 0 || int(s) > len(ScopeFlags) {
		return "<invalid>"
	}
	return ScopeFlags[s]
}

func ScopeFromName(name string) (s Scope, ok bool) {
	s, ok = scopeNameMap[name]
	return
}

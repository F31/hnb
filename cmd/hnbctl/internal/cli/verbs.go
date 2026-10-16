package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
)

// runCreate is the kk-style `create config` verb; it delegates to init.
func runCreate(ctx context.Context, env *Env, args []string) Result {
	if len(args) == 0 || args[0] != "config" {
		return fail(ReasonInvalidConfig, "create requires the config verb: hnbctl create config -f hnb.yaml [--mode existing|bootstrap]")
	}
	return runInit(ctx, env, args[1:])
}

// runAdd is the kk-style `add nodes` verb; it delegates to node add.
func runAdd(ctx context.Context, env *Env, args []string) Result {
	if len(args) == 0 || args[0] != "nodes" {
		return fail(ReasonInvalidConfig, "add requires the nodes verb: hnbctl add nodes -f nodes.yaml --target-id ID --platform-url URL")
	}
	return runNode(ctx, env, append([]string{"add"}, args[1:]...))
}

// runDelete is the kk-style removal verb. It only exposes the removal actions
// that carry real ownership semantics; node removal stays server-authoritative.
func runDelete(ctx context.Context, env *Env, args []string) Result {
	if len(args) == 0 {
		return fail(ReasonInvalidConfig, "delete requires a verb: installation|cluster")
	}
	switch args[0] {
	case "installation":
		return runUninstall(ctx, env, args[1:])
	case "cluster":
		return runPlatform(ctx, env, append([]string{"delete-management-base"}, args[1:]...))
	default:
		return fail(ReasonInvalidConfig, "unknown delete verb "+args[0]+"; use installation or cluster")
	}
}

// runUpgrade is the kk-style upgrade verb. With --to it plans a profile
// transition; otherwise it delegates to the pinned platform upgrade.
func runUpgrade(ctx context.Context, env *Env, args []string) Result {
	for _, arg := range args {
		if arg == "--to" || strings.HasPrefix(arg, "--to=") {
			return runProfileUpgrade(ctx, env, args)
		}
	}
	return runPlatform(ctx, env, append([]string{"upgrade"}, args...))
}

// runArtifact is the kk-style artifact verb; it delegates to bundle.
func runArtifact(ctx context.Context, env *Env, args []string) Result {
	return runBundle(ctx, env, args)
}

// subcommandHints maps verbs to their positional subcommands for completion.
func subcommandHints() map[string][]string {
	return map[string][]string{
		"create":    {"config"},
		"add":       {"nodes"},
		"delete":    {"installation", "cluster"},
		"artifact":  {"prepare", "verify", "import"},
		"bundle":    {"prepare", "verify", "import"},
		"cluster":   {"create", "import", "list"},
		"node":      {"add"},
		"platform":  {"uninstall-hnb", "unmanage", "delete-management-base", "upgrade", "rollback"},
		"operation": {"get"},
		"os":        {"list", "inspect"},
		"ssh":       {"enroll"},
	}
}

func completionNames() []string {
	names := make([]string, 0, len(Commands))
	for _, c := range Commands {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	return names
}

func runCompletion(_ context.Context, env *Env, args []string) Result {
	if len(args) != 1 || (args[0] != "bash" && args[0] != "zsh" && args[0] != "fish") {
		return fail(ReasonInvalidConfig, "completion requires one of: bash|zsh|fish")
	}
	var doc string
	switch args[0] {
	case "bash":
		doc = bashCompletion()
	case "zsh":
		doc = zshCompletion()
	case "fish":
		doc = fishCompletion()
	}
	if _, err := io.WriteString(env.Stdout, doc); err != nil {
		return fail(ReasonOutputFailed, "cannot write completion script")
	}
	return success("")
}

func bashCompletion() string {
	var b strings.Builder
	b.WriteString("# bash completion for hnbctl\n_hnbctl() {\n")
	b.WriteString("\tlocal cur word\n\tcur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	hints := subcommandHints()
	// Complete subcommand verbs first.
	b.WriteString("\tif (( COMP_CWORD == 2 )); then\n")
	b.WriteString("\t\tcase \"${COMP_WORDS[1]}\" in\n")
	var hintKeys []string
	for k := range hints {
		hintKeys = append(hintKeys, k)
	}
	sort.Strings(hintKeys)
	for _, k := range hintKeys {
		fmt.Fprintf(&b, "\t\t\t%s) COMPREPLY=( $(compgen -W '%s' -- \"$cur\") ); return ;;\n", k, strings.Join(hints[k], " "))
	}
	b.WriteString("\t\tesac\n\tfi\n")
	b.WriteString("\tCOMPREPLY=( $(compgen -W '")
	b.WriteString(strings.Join(completionNames(), " "))
	b.WriteString("' -- \"$cur\") )\n}\ncomplete -F _hnbctl hnbctl\n")
	return b.String()
}

func zshCompletion() string {
	var b strings.Builder
	b.WriteString("#compdef hnbctl\n_hnbctl() {\n\tlocal -a commands\n")
	b.WriteString("\tcommands=(")
	for _, n := range completionNames() {
		fmt.Fprintf(&b, "\n\t\t'%s'", n)
	}
	b.WriteString("\n\t)\n\t_arguments '1:command:->command'\n\tcase $state in\n\t\tcommand) _describe 'command' commands ;;\n\tesac\n}\ncompdef _hnbctl hnbctl\n")
	return b.String()
}

func fishCompletion() string {
	var b strings.Builder
	b.WriteString("# fish completion for hnbctl\n")
	for _, n := range completionNames() {
		fmt.Fprintf(&b, "complete -c hnbctl -n '__fish_use_subcommand' -a '%s'\n", n)
	}
	hints := subcommandHints()
	var hintKeys []string
	for k := range hints {
		hintKeys = append(hintKeys, k)
	}
	sort.Strings(hintKeys)
	for _, k := range hintKeys {
		fmt.Fprintf(&b, "complete -c hnbctl -n '__fish_seen_subcommand_from %s' -a '%s'\n", k, strings.Join(hints[k], " "))
	}
	return b.String()
}

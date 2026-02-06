package stateless

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"text/template"
	"unicode"
)

type graph struct{}

type transitionLabel struct {
	reentry       []string
	internal      []string
	transitioning []string
	ignored       []string
}

type usedTransitionTypes struct {
	hasReentry  bool
	hasInternal bool
	hasIgnored  bool
}

func (g *graph) formatStateMachine(sm *StateMachine) string {
	var sb strings.Builder
	sb.WriteString("digraph {\n\tcompound=true;\n\tnode [shape=box, style=rounded];\n\trankdir=\"LR\";\n\n")

	stateList := make([]*stateRepresentation, 0, len(sm.stateConfig))
	for _, st := range sm.stateConfig {
		stateList = append(stateList, st)
	}
	slices.SortFunc(stateList, func(a, b *stateRepresentation) int {
		return strings.Compare(fmt.Sprint(a.State), fmt.Sprint(b.State))
	})

	for _, sr := range stateList {
		if sr.Superstate == nil {
			g.formatOneState(&sb, sr, 1)
		}
	}
	for _, sr := range stateList {
		if sr.HasInitialState {
			dest := sm.stateConfig[sr.InitialTransitionTarget]
			if dest != nil {
				src := clusterStr(sr.State, true, true)
				g.formatOneLine(&sb, src, str(dest.State, true), `""`)
			}
		}
	}
	var used usedTransitionTypes
	for _, sr := range stateList {
		g.formatAllStateTransitions(&sb, sm, sr, &used)
	}
	initialState, err := sm.State(context.Background())
	if err == nil {
		sb.WriteString("\tinit [label=\"\", shape=point];\n")
		sb.WriteString(fmt.Sprintf("\tinit -> %s\n", str(initialState, true)))
	}
	g.writeLegend(used, &sb)
	sb.WriteString("}\n")
	return sb.String()
}

func (g *graph) writeLegend(used usedTransitionTypes, sb *strings.Builder) {
	var legendItems []string
	if used.hasReentry {
		legendItems = append(legendItems, "🔄 Reentry")
	}
	if used.hasInternal {
		legendItems = append(legendItems, "🔒 Internal")
	}
	if used.hasIgnored {
		legendItems = append(legendItems, "🚫 Ignored")
	}
	// Legend at bottom right (only if there are special transitions)
	if len(legendItems) > 0 {
		sb.WriteString(fmt.Sprintf("\n\tlegend [shape=none, label=\"%s\\l\"];\n", strings.Join(legendItems, "\\l")))
		sb.WriteString("\t{ rank=sink; legend; init -> legend [style=invis]; }\n")
	}
}

func (g *graph) formatActions(sr *stateRepresentation) string {
	es := make([]string, 0, len(sr.EntryActions)+len(sr.ExitActions)+len(sr.ActivateActions)+len(sr.DeactivateActions))
	for _, act := range sr.ActivateActions {
		es = append(es, fmt.Sprintf("activated / %s", esc(act.Description.String(), false)))
	}
	for _, act := range sr.DeactivateActions {
		es = append(es, fmt.Sprintf("deactivated / %s", esc(act.Description.String(), false)))
	}
	for _, act := range sr.EntryActions {
		if act.Trigger == nil {
			es = append(es, fmt.Sprintf("entry / %s", esc(act.Description.String(), false)))
		}
	}
	for _, act := range sr.ExitActions {
		es = append(es, fmt.Sprintf("exit / %s", esc(act.Description.String(), false)))
	}
	if len(es) == 0 {
		return ""
	}
	return strings.Join(es, "\\l") + "\\l"
}

func (g *graph) formatOneState(sb *strings.Builder, sr *stateRepresentation, level int) {
	indent := strings.Repeat("\t", level)
	sb.WriteString(fmt.Sprintf("%s%s [label=\"%s", indent, str(sr.State, true), str(sr.State, false)))
	act := g.formatActions(sr)
	if act != "" {
		sb.WriteString("\\n----------\\n")
		sb.WriteString(act)
	}
	sb.WriteString("\"];\n")
	if len(sr.Substates) != 0 {
		sb.WriteString(fmt.Sprintf("%ssubgraph %s {\n%s\tlabel=\"Substates of\\n%s\";\n", indent, clusterStr(sr.State, true, false), indent, str(sr.State, false)))
		sb.WriteString(fmt.Sprintf("%s\tstyle=\"dashed\";\n", indent))
		if sr.HasInitialState {
			sb.WriteString(fmt.Sprintf("%s\t\"%s\" [label=\"\", shape=point];\n", indent, clusterStr(sr.State, false, true)))
		}
		for _, substate := range sr.Substates {
			g.formatOneState(sb, substate, level+1)
		}
		sb.WriteString(indent + "}\n")
	}
}

func (g *graph) getEntryActions(ab []actionBehaviour, t Trigger) []string {
	var actions []string
	for _, ea := range ab {
		if ea.Trigger != nil && *ea.Trigger == t {
			actions = append(actions, esc(ea.Description.String(), false))
		}
	}
	return actions
}

func (g *graph) formatAllStateTransitions(sb *strings.Builder, sm *StateMachine, sr *stateRepresentation, used *usedTransitionTypes) {
	triggerList := make([]triggerBehaviour, 0, len(sr.TriggerBehaviours))
	for _, triggers := range sr.TriggerBehaviours {
		triggerList = append(triggerList, triggers...)
	}
	slices.SortFunc(triggerList, func(a, b triggerBehaviour) int {
		return strings.Compare(fmt.Sprint(a.GetTrigger()), fmt.Sprint(b.GetTrigger()))
	})

	type line struct {
		source      State
		destination State
	}

	lines := make(map[line]*transitionLabel, len(triggerList))
	order := make([]line, 0, len(triggerList))
	getLine := func(ln line) *transitionLabel {
		if lines[ln] == nil {
			order = append(order, ln)
			lines[ln] = &transitionLabel{}
		}
		return lines[ln]
	}

	for _, trigger := range triggerList {
		switch t := trigger.(type) {
		case *ignoredTriggerBehaviour:
			used.hasIgnored = true
			ln := getLine(line{sr.State, sr.State})
			ln.ignored = append(ln.ignored, g.formatOneTransition(t.Trigger, nil, t.Guard))
		case *reentryTriggerBehaviour:
			used.hasReentry = true
			actions := g.getEntryActions(sr.EntryActions, t.Trigger)
			ln := getLine(line{sr.State, t.Destination})
			ln.reentry = append(ln.reentry, g.formatOneTransition(t.Trigger, actions, t.Guard))
		case *internalTriggerBehaviour:
			used.hasInternal = true
			actions := g.getEntryActions(sr.EntryActions, t.Trigger)
			ln := getLine(line{sr.State, sr.State})
			ln.internal = append(ln.internal, g.formatOneTransition(t.Trigger, actions, t.Guard))
		case *transitioningTriggerBehaviour:
			if sm.stateConfig[sr.State] == nil {
				continue
			}
			var actions []string
			if dest := sm.stateConfig[t.Destination]; dest != nil {
				actions = g.getEntryActions(dest.EntryActions, t.Trigger)
			}
			ln := getLine(line{sr.State, t.Destination})
			ln.transitioning = append(ln.transitioning, g.formatOneTransition(t.Trigger, actions, t.Guard))
		case *dynamicTriggerBehaviour:
			// TODO: not supported yet
		}
	}

	for _, ln := range order {
		g.formatOneLine(sb, str(ln.source, true), str(ln.destination, true), g.toTransitionsLabel(*lines[ln]))
	}
}

func (g *graph) toTransitionsLabel(t transitionLabel) string {
	var sb strings.Builder
	for _, group := range []struct {
		transitions []string
		prefix      string
	}{
		{t.transitioning, ""},
		{t.reentry, "🔄 "},
		{t.internal, "🔒 "},
		{t.ignored, "🚫 "},
	} {
		for i, tr := range group.transitions {
			if i == 0 {
				sb.WriteRune('"')
			}
			sb.WriteString(group.prefix)
			sb.WriteString(tr)
			if i != len(group.transitions)-1 {
				sb.WriteString("\\l")
			} else {
				sb.WriteString("\\l\"")
			}
		}
	}
	return sb.String()
}

func (g *graph) formatOneTransition(trigger Trigger, actions []string, guards transitionGuard) string {
	var sb strings.Builder
	sb.WriteString(str(trigger, false))
	if len(actions) > 0 {
		sb.WriteString(" / ")
		sb.WriteString(strings.Join(actions, ", "))
	}
	for _, info := range guards.Guards {
		sb.WriteString(fmt.Sprintf(" [%s]", esc(info.Description.String(), false)))
	}
	return sb.String()
}

func (g *graph) formatOneLine(sb *strings.Builder, fromNodeName, toNodeName, label string) {
	sb.WriteString(fmt.Sprintf("\t%s -> %s [label=%s];\n", fromNodeName, toNodeName, label))
}

func clusterStr(state any, quote, init bool) string {
	s := fmt.Sprint(state)
	if init {
		s += "-init"
	}
	return esc("cluster_"+s, quote)
}

func str(v any, quote bool) string {
	return esc(fmt.Sprint(v), quote)
}

func isHTML(s string) bool {
	if len(s) == 0 {
		return false
	}
	ss := strings.TrimSpace(s)
	if ss[0] != '<' {
		return false
	}
	var count int
	for _, c := range ss {
		if c == '<' {
			count++
		}
		if c == '>' {
			count--
		}
	}
	return count == 0
}

func isLetter(ch rune) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_' ||
		ch >= 0x80 && unicode.IsLetter(ch) && ch != 'ε'
}

func isID(s string) bool {
	for _, c := range s {
		if !isLetter(c) {
			return false
		}
		if unicode.IsSpace(c) {
			return false
		}
		switch c {
		case '-', '/', '.', '@':
			return false
		}
	}
	return true
}

func isDigit(ch rune) bool {
	return '0' <= ch && ch <= '9' || ch >= 0x80 && unicode.IsDigit(ch)
}

func isNumber(s string) bool {
	var state int
	for _, c := range s {
		if state == 0 {
			if isDigit(c) || c == '.' {
				state = 2
			} else if c == '-' {
				state = 1
			} else {
				return false
			}
		} else if state == 1 {
			if isDigit(c) || c == '.' {
				state = 2
			}
		} else if c != '.' && !isDigit(c) {
			return false
		}
	}
	return (state == 2)
}

func isStringLit(s string) bool {
	if !strings.HasPrefix(s, `"`) || !strings.HasSuffix(s, `"`) {
		return false
	}
	var prev rune
	for _, r := range s[1 : len(s)-1] {
		if r == '"' && prev != '\\' {
			return false
		}
		prev = r
	}
	return true
}

func esc(s string, quote bool) string {
	if len(s) == 0 {
		return s
	}
	if isHTML(s) {
		return s
	}
	ss := strings.TrimSpace(s)
	if ss[0] == '<' {
		s := strings.Replace(s, "\"", "\\\"", -1)
		if quote {
			s = fmt.Sprintf("\"%s\"", s)
		}
		return s
	}
	if isID(s) {
		return s
	}
	if isNumber(s) {
		return s
	}
	if isStringLit(s) {
		return s
	}
	s = template.HTMLEscapeString(s)
	if quote {
		s = fmt.Sprintf("\"%s\"", s)
	}
	return s
}

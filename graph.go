package stateless

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

type graph struct {
}

func (g *graph) formatStateMachine(sm *StateMachine) string {
	var sb strings.Builder
	sb.WriteString("digraph {\n\tcompound=true;\n\tnode [shape=Mrecord];\n\trankdir=\"LR\";\n\n")

	stateList := make([]*stateRepresentation, 0, len(sm.stateConfig))
	for _, st := range sm.stateConfig {
		stateList = append(stateList, st)
	}
	sort.Slice(stateList, func(i, j int) bool {
		return fmt.Sprint(stateList[i].State) < fmt.Sprint(stateList[j].State)
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
				formatOneLine(&sb, src, str(dest.State, true), "")
			}
		}
	}
	for _, sr := range stateList {
		g.formatAllStateTransitions(&sb, sm, sr)
	}
	initialState, err := sm.State(context.Background())
	if err == nil {
		sb.WriteString("\tinit [label=\"\", shape=point];\n")
		sb.WriteString(fmt.Sprintf("\tinit -> %s\n", str(initialState, true)))
	}
	sb.WriteString("}\n")
	return sb.String()
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
	return strings.Join(es, "\\n")
}

func (g *graph) formatOneState(sb *strings.Builder, sr *stateRepresentation, level int) {
	var indent string
	for i := 0; i < level; i++ {
		indent += "\t"
	}
	sb.WriteString(fmt.Sprintf("%s%s [label=\"%s", indent, str(sr.State, true), str(sr.State, false)))
	act := g.formatActions(sr)
	if act != "" {
		if len(sr.Substates) == 0 {
			sb.WriteString("|")
		} else {
			sb.WriteString("\\n----------\\n")
		}
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

func (g *graph) formatAllStateTransitions(sb *strings.Builder, sm *StateMachine, sr *stateRepresentation) {
	triggerList := make([]triggerBehaviour, 0, len(sr.TriggerBehaviours))
	for _, triggers := range sr.TriggerBehaviours {
		triggerList = append(triggerList, triggers...)
	}
	sort.Slice(triggerList, func(i, j int) bool {
		ti := triggerList[i].GetTrigger()
		tj := triggerList[j].GetTrigger()
		return fmt.Sprint(ti) < fmt.Sprint(tj)
	})

	type line struct {
		source      State
		destination State
	}

	lines := make(map[line][]string, len(triggerList))
	order := make([]line, 0, len(triggerList))
	for _, trigger := range triggerList {
		switch t := trigger.(type) {
		case *ignoredTriggerBehaviour:
			ln := line{sr.State, sr.State}
			if _, ok := lines[ln]; !ok {
				order = append(order, ln)
			}
			lines[ln] = append(lines[ln], formatOneTransition(t.Trigger, nil, t.Guard))
		case *reentryTriggerBehaviour:
			actions := g.getEntryActions(sr.EntryActions, t.Trigger)
			ln := line{sr.State, t.Destination}
			if _, ok := lines[ln]; !ok {
				order = append(order, ln)
			}
			lines[ln] = append(lines[ln], formatOneTransition(t.Trigger, actions, t.Guard))
		case *internalTriggerBehaviour:
			actions := g.getEntryActions(sr.EntryActions, t.Trigger)
			ln := line{sr.State, sr.State}
			if _, ok := lines[ln]; !ok {
				order = append(order, ln)
			}
			lines[ln] = append(lines[ln], formatOneTransition(t.Trigger, actions, t.Guard))
		case *transitioningTriggerBehaviour:
			src := sm.stateConfig[sr.State]
			if src == nil {
				continue
			}
			dest := sm.stateConfig[t.Destination]
			var actions []string
			if dest != nil {
				actions = g.getEntryActions(dest.EntryActions, t.Trigger)
			}
			var destState State
			if dest == nil {
				destState = t.Destination
			} else {
				destState = dest.State
			}
			ln := line{sr.State, destState}
			if _, ok := lines[ln]; !ok {
				order = append(order, ln)
			}
			lines[ln] = append(lines[ln], formatOneTransition(t.Trigger, actions, t.Guard))
		case *dynamicTriggerBehaviour:
			// TODO: not supported yet
		}
	}

	for _, ln := range order {
		content := lines[ln]
		formatOneLine(sb, str(ln.source, true), str(ln.destination, true), strings.Join(content, "\\n"))
	}
}

func formatOneTransition(trigger Trigger, actions []string, guards transitionGuard) string {
	var sb strings.Builder
	sb.WriteString(str(trigger, false))
	if len(actions) > 0 {
		sb.WriteString(" / ")
		sb.WriteString(strings.Join(actions, ", "))
	}
	for _, info := range guards.Guards {
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(fmt.Sprintf("[%s]", esc(info.Description.String(), false)))
	}
	return sb.String()
}

func formatOneLine(sb *strings.Builder, fromNodeName, toNodeName, label string) {
	sb.WriteString(fmt.Sprintf("\t%s -> %s [label=\"%s\"", fromNodeName, toNodeName, label))
	sb.WriteString("];\n")
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

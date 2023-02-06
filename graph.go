package stateless

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

type graph[T any] struct {
}

func (g *graph[T]) formatStateMachine(sm *StateMachine[T]) string {
	var sb strings.Builder
	sb.WriteString("digraph {\n\tcompound=true;\n\tnode [shape=Mrecord];\n\trankdir=\"LR\";\n\n")

	stateList := make([]*stateRepresentation[T], 0, len(sm.stateConfig))
	for _, st := range sm.stateConfig {
		stateList = append(stateList, st)
	}
	sort.Slice(stateList, func(i, j int) bool {
		return fmt.Sprint(stateList[i].State) < fmt.Sprint(stateList[j].State)
	})

	for _, sr := range stateList {
		if sr.Superstate == nil {
			sb.WriteString(g.formatOneState(sr, 1))
		}
	}
	for _, sr := range stateList {
		if sr.HasInitialState {
			dest := sm.stateConfig[sr.InitialTransitionTarget]
			if dest != nil {
				dest, lhead := g.resolveTransition(sm, dest)
				src := clusterStr(sr.State, true, true)
				sb.WriteString(g.formatOneLine(src, str(dest.State, true), "", lhead, ""))
			}
		}
	}
	for _, sr := range stateList {
		sb.WriteString(g.formatAllStateTransitions(sm, sr))
	}
	initialState, err := sm.State(context.Background())
	if err == nil {
		sb.WriteString("\tinit [label=\"\", shape=point];\n")
		sb.WriteString(fmt.Sprintf("\tinit -> %s\n", str(initialState, true)))
	}
	sb.WriteString("}\n")
	return sb.String()
}

func (g *graph[T]) formatActions(sr *stateRepresentation[T]) string {
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
	return strings.Join(es, "\n")
}

func (g *graph[T]) formatOneState(sr *stateRepresentation[T], level int) string {
	var indent string
	for i := 0; i < level; i++ {
		indent += "\t"
	}
	var sb strings.Builder
	if len(sr.Substates) == 0 {
		sb.WriteString(fmt.Sprintf("%s%s [label=\"%s", indent, str(sr.State, true), str(sr.State, false)))
	} else {
		sb.WriteString(fmt.Sprintf("%ssubgraph %s {\n%s\tlabel=\"%s", indent, clusterStr(sr.State, true, false), indent, str(sr.State, false)))
	}
	act := g.formatActions(sr)
	if act != "" {
		if len(sr.Substates) == 0 {
			sb.WriteString("|")
		} else {
			sb.WriteString("\\n----------\\n")
		}
		sb.WriteString(act)
	}
	if len(sr.Substates) == 0 {
		sb.WriteString("\"];\n")
	} else {
		sb.WriteString("\";\n")
		if sr.HasInitialState {
			sb.WriteString(fmt.Sprintf("%s\t\"%s\" [label=\"\", shape=point];\n", indent, clusterStr(sr.State, false, true)))
		}
		for _, substate := range sr.Substates {
			sb.WriteString(g.formatOneState(substate, level+1))
		}
		sb.WriteString(indent + "}\n")
	}
	return sb.String()
}

func (g *graph[T]) getEntryActions(ab []actionBehaviour[T], t Trigger) []string {
	var actions []string
	for _, ea := range ab {
		if ea.Trigger != nil && *ea.Trigger == t {
			actions = append(actions, esc(ea.Description.String(), false))
		}
	}
	return actions
}

func getLeafState[T any](sm *StateMachine[T], sr *stateRepresentation[T]) *stateRepresentation[T] {
	if sr.HasInitialState {
		s, ok := sm.stateConfig[sr.InitialTransitionTarget]
		if !ok {
			return nil
		}
		return getLeafState(sm, s)
	}
	for _, s := range sr.Substates {
		if len(s.Substates) == 0 {
			return s
		}
		if s = getLeafState(sm, s); s != nil {
			return s
		}
	}
	return sr
}

func (g *graph[T]) resolveTransition(sm *StateMachine[T], sr *stateRepresentation[T]) (*stateRepresentation[T], string) {
	if anyLeaf := getLeafState(sm, sr); anyLeaf != nil && sr != anyLeaf {
		return anyLeaf, clusterStr(sr.State, false, false)
	}
	return sr, ""
}

func (g *graph[T]) formatAllStateTransitions(sm *StateMachine[T], sr *stateRepresentation[T]) string {
	var sb strings.Builder

	triggerList := make([]triggerBehaviour[T], 0, len(sr.TriggerBehaviours))
	for _, triggers := range sr.TriggerBehaviours {
		for _, trigger := range triggers {
			triggerList = append(triggerList, trigger)
		}
	}
	sort.Slice(triggerList, func(i, j int) bool {
		ti := triggerList[i].GetTrigger()
		tj := triggerList[j].GetTrigger()
		return fmt.Sprint(ti) < fmt.Sprint(tj)
	})

	for _, trigger := range triggerList {
		switch t := trigger.(type) {
		case *ignoredTriggerBehaviour[T]:
			sb.WriteString(g.formatOneTransition(sm, sr.State, sr.State, t.Trigger, "", "", nil, t.Guard))
		case *reentryTriggerBehaviour[T]:
			actions := g.getEntryActions(sr.EntryActions, t.Trigger)
			sb.WriteString(g.formatOneTransition(sm, sr.State, t.Destination, t.Trigger, "", "", actions, t.Guard))
		case *internalTriggerBehaviour[T]:
			actions := g.getEntryActions(sr.EntryActions, t.Trigger)
			sb.WriteString(g.formatOneTransition(sm, sr.State, sr.State, t.Trigger, "", "", actions, t.Guard))
		case *transitioningTriggerBehaviour[T]:
			src := sm.stateConfig[sr.State]
			if src == nil {
				return ""
			}
			dest := sm.stateConfig[t.Destination]
			var ltail, lhead string
			var actions []string
			src, ltail = g.resolveTransition(sm, src)
			if dest != nil {
				dest, lhead = g.resolveTransition(sm, dest)
				actions = g.getEntryActions(dest.EntryActions, t.Trigger)
			}
			var destState State
			if dest == nil {
				destState = t.Destination
			} else {
				destState = dest.State
			}
			sb.WriteString(g.formatOneTransition(sm, src.State, destState, t.Trigger, ltail, lhead, actions, t.Guard))
		case *dynamicTriggerBehaviour[T]:
			// TODO: not supported yet
		}
	}
	return sb.String()
}

func (g *graph[T]) formatOneTransition(sm *StateMachine[T], source, destination State, trigger Trigger, ltail, lhead string, actions []string, guards transitionGuard[T]) string {
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
	return g.formatOneLine(str(source, true), str(destination, true), sb.String(), lhead, ltail)
}

func (g *graph[T]) formatOneLine(fromNodeName, toNodeName, label, lhead, ltail string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\t%s -> %s [label=\"%s\"", fromNodeName, toNodeName, label))
	if lhead != "" {
		sb.WriteString(fmt.Sprintf(", lhead=\"%s\"", lhead))
	}
	if ltail != "" {
		sb.WriteString(fmt.Sprintf(", ltail=\"%s\"", ltail))
	}
	sb.WriteString("];\n")
	return sb.String()
}

func clusterStr(state interface{}, quote, init bool) string {
	s := fmt.Sprint(state)
	if init {
		s += "-init"
	}
	return esc("cluster_"+s, quote)
}

func str(v interface{}, quote bool) string {
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

func quote(s string) string {
	var quoted bool
	for _, c := range s {
		if c == '"' {
			quoted = true
			break
		}
	}
	if !quoted {
		return s
	}
	var sb strings.Builder
	for _, c := range s {
		if c == '"' {
			sb.WriteByte('\\')
		}
		sb.WriteRune(c)
	}
	return sb.String()
}

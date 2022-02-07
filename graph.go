package stateless

import (
	"context"
	"fmt"
	"strings"
	"text/template"
	"unicode"
)

type graph struct {
}

func (g *graph) FormatStateMachine(sm *StateMachine) string {
	var sb strings.Builder
	sb.WriteString("digraph {\n\tcompound=true;\n\tnode [shape=Mrecord];\n\trankdir=\"LR\";\n\n")

	for _, sr := range sm.stateConfig {
		if sr.Superstate == nil {
			sb.WriteString(g.formatOneState(sr, 1))
		}
	}
	for _, sr := range sm.stateConfig {
		if sr.HasInitialState {
			dest := sm.stateConfig[sr.InitialTransitionTarget]
			if dest != nil {
				dest, lhead := g.resolveTransition(sm, dest)
				src := fmt.Sprintf("\"cluster_%s-init\"", str(sr.State))
				sb.WriteString(g.formatOneLine(src, str(dest.State), "", lhead, ""))
			}
		}
	}
	for _, sr := range sm.stateConfig {
		sb.WriteString(g.formatAllStateTransitions(sm, sr))
	}
	initialState, err := sm.State(context.Background())
	if err == nil {
		sb.WriteString("\tinit [label=\"\", shape=point];\n")
		sb.WriteString(fmt.Sprintf("\tinit -> %s\n", str(initialState)))
	}
	sb.WriteString("}\n")
	return sb.String()
}

func (g *graph) formatActions(sr *stateRepresentation) string {
	es := make([]string, 0, len(sr.EntryActions)+len(sr.ExitActions)+len(sr.ActivateActions)+len(sr.DeactivateActions))
	for _, act := range sr.ActivateActions {
		es = append(es, fmt.Sprintf("activated / %s", esc(act.Description.String())))
	}
	for _, act := range sr.DeactivateActions {
		es = append(es, fmt.Sprintf("deactivated / %s", esc(act.Description.String())))
	}
	for _, act := range sr.EntryActions {
		if act.Trigger == nil {
			es = append(es, fmt.Sprintf("entry / %s", esc(act.Description.String())))
		}
	}
	for _, act := range sr.ExitActions {
		es = append(es, fmt.Sprintf("exit / %s", esc(act.Description.String())))
	}
	return strings.Join(es, `\n`)
}

func (g *graph) formatOneState(sr *stateRepresentation, level int) string {
	var indent string
	for i := 0; i < level; i++ {
		indent += "\t"
	}
	var sb strings.Builder
	if len(sr.Substates) == 0 {
		sb.WriteString(fmt.Sprintf("%s%s [label=\"%s", indent, str(sr.State), str(sr.State)))
	} else {
		sb.WriteString(fmt.Sprintf("%ssubgraph cluster_%s {\n%s\tlabel=\"%s", indent, str(sr.State), indent, str(sr.State)))
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
			sb.WriteString(fmt.Sprintf("%s\t\"cluster_%s-init\" [label=\"\", shape=point];\n", indent, str(sr.State)))
		}
		for _, substate := range sr.Substates {
			sb.WriteString(g.formatOneState(substate, level+1))
		}
		sb.WriteString(indent + "}\n")
	}
	return sb.String()
}

func (g *graph) getEntryActions(ab []actionBehaviour, t Trigger) []string {
	var actions []string
	for _, ea := range ab {
		if ea.Trigger == nil || *ea.Trigger == t {
			actions = append(actions, esc(ea.Description.String()))
		}
	}
	return actions
}

func getLeafState(sm *StateMachine, sr *stateRepresentation) *stateRepresentation {
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

func (g *graph) resolveTransition(sm *StateMachine, sr *stateRepresentation) (*stateRepresentation, string) {
	if anyLeaf := getLeafState(sm, sr); anyLeaf != nil && sr != anyLeaf {
		return anyLeaf, fmt.Sprintf("cluster_%s", str(sr.State))
	}
	return sr, ""
}

func (g *graph) formatAllStateTransitions(sm *StateMachine, sr *stateRepresentation) string {
	var sb strings.Builder
	for _, triggers := range sr.TriggerBehaviours {
		for _, trigger := range triggers {
			switch t := trigger.(type) {
			case *ignoredTriggerBehaviour:
				sb.WriteString(g.formatOneTransition(sm, sr.State, sr.State, t.Trigger, "", "", nil, t.Guard))
			case *reentryTriggerBehaviour:
				actions := g.getEntryActions(sr.EntryActions, t.Trigger)
				sb.WriteString(g.formatOneTransition(sm, sr.State, t.Destination, t.Trigger, "", "", actions, t.Guard))
			case *internalTriggerBehaviour:
				actions := g.getEntryActions(sr.EntryActions, t.Trigger)
				sb.WriteString(g.formatOneTransition(sm, sr.State, sr.State, t.Trigger, "", "", actions, t.Guard))
			case *transitioningTriggerBehaviour:
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
			case *dynamicTriggerBehaviour:
				// TODO: not supported yet
			}
		}
	}
	return sb.String()
}

func (g *graph) formatOneTransition(sm *StateMachine, source, destination State, trigger Trigger, ltail, lhead string, actions []string, guards transitionGuard) string {
	var sb strings.Builder
	sb.WriteString(str(trigger))
	if len(actions) > 0 {
		sb.WriteString(" / ")
		sb.WriteString(strings.Join(actions, ", "))
	}
	for _, info := range guards.Guards {
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(fmt.Sprintf("[%s]", esc(info.Description.String())))
	}
	return g.formatOneLine(str(source), str(destination), sb.String(), lhead, ltail)
}

func (g *graph) formatOneLine(fromNodeName, toNodeName, label, lhead, ltail string) string {
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

func str(v interface{}) string {
	return esc(fmt.Sprint(v))
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
	var pos bool
	for i, c := range s {
		if i == 0 {
			if !isLetter(c) {
				return false
			}
			pos = true
		}
		if unicode.IsSpace(c) {
			return false
		}
		if c == '-' {
			return false
		}
		if c == '/' {
			return false
		}
		if c == '.' {
			return false
		}
	}
	return pos
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

func esc(s string) string {
	if len(s) == 0 {
		return s
	}
	if isHTML(s) {
		return s
	}
	ss := strings.TrimSpace(s)
	if ss[0] == '<' {
		return fmt.Sprintf("\"%s\"", strings.Replace(s, "\"", "\\\"", -1))
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
	return fmt.Sprintf("\"%s\"", template.HTMLEscapeString(s))
}

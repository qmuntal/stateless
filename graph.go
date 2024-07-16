package stateless

import (
	"context"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

type graph struct {
}

// GraphConfiguration holds the configuration that is used to render
// graphs with the ToGraph method.
type GraphConfiguration struct {
	OmitIgnoredTransitions   bool // OmitIgnoredTransitions can be used to omit ignored transition from graphs.
	OmitReentrantTransitions bool // OmitReentrantTransitions can be used to omit reentrant transition from graphs.
	OmitInternalTransitions  bool // OmitInternalTransitions can be used to omit internal transition from graphs.

	IgnoredTransitionColor   color.Color // IgnoredTransitionColor is the color of ignored transitions. nil represents the default color.
	ReentrantTransitionColor color.Color // ReentrantTransitionColor is the color of reentrant transitions. nil represents the default color.
	InternalTransitionColor  color.Color // InternalTransitionColor is the color of internal transitions. nil represents the default color.
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
			sb.WriteString(g.formatOneState(sr, 1))
		}
	}
	for _, sr := range stateList {
		if sr.HasInitialState {
			dest := sm.stateConfig[sr.InitialTransitionTarget]
			if dest != nil {
				src := clusterStr(sr.State, true, true)
				sb.WriteString(g.formatOneLine(src, str(dest.State, true), "", nil))
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

func (g *graph) formatOneState(sr *stateRepresentation, level int) string {
	var indent string
	for i := 0; i < level; i++ {
		indent += "\t"
	}
	var sb strings.Builder
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
			sb.WriteString(g.formatOneState(substate, level+1))
		}
		sb.WriteString(indent + "}\n")
	}
	return sb.String()
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

func (g *graph) formatAllStateTransitions(sm *StateMachine, sr *stateRepresentation) string {
	var sb strings.Builder

	triggerList := make([]triggerBehaviour, 0, len(sr.TriggerBehaviours))
	for _, triggers := range sr.TriggerBehaviours {
		triggerList = append(triggerList, triggers...)
	}
	sort.Slice(triggerList, func(i, j int) bool {
		ti := triggerList[i].GetTrigger()
		tj := triggerList[j].GetTrigger()
		return fmt.Sprint(ti) < fmt.Sprint(tj)
	})

	for _, trigger := range triggerList {
		switch t := trigger.(type) {
		case *ignoredTriggerBehaviour:
			if !sm.graphConfig.OmitIgnoredTransitions {
				sb.WriteString(g.formatOneTransition(sm, sr.State, sr.State, t, nil, t.Guard))
			}
		case *reentryTriggerBehaviour:
			if !sm.graphConfig.OmitReentrantTransitions {
				actions := g.getEntryActions(sr.EntryActions, t.Trigger)
				sb.WriteString(g.formatOneTransition(sm, sr.State, t.Destination, t, actions, t.Guard))
			}
		case *internalTriggerBehaviour:
			if !sm.graphConfig.OmitInternalTransitions {
				actions := g.getEntryActions(sr.EntryActions, t.Trigger)
				sb.WriteString(g.formatOneTransition(sm, sr.State, sr.State, t, actions, t.Guard))
			}
		case *transitioningTriggerBehaviour:
			src := sm.stateConfig[sr.State]
			if src == nil {
				return ""
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
			sb.WriteString(g.formatOneTransition(sm, src.State, destState, t, actions, t.Guard))
		case *dynamicTriggerBehaviour:
			// TODO: not supported yet
		}
	}
	return sb.String()
}

func (g *graph) formatOneTransition(sm *StateMachine, source, destination State, tb triggerBehaviour, actions []string, guards transitionGuard) string {
	var sb strings.Builder
	sb.WriteString(str(tb.GetTrigger(), false))
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
	color := findColorForTrigger(sm, tb)
	return g.formatOneLine(str(source, true), str(destination, true), sb.String(), color)
}

func findColorForTrigger(sm *StateMachine, tb triggerBehaviour) color.Color {
	switch tb.(type) {
	case *ignoredTriggerBehaviour:
		return sm.graphConfig.IgnoredTransitionColor
	case *reentryTriggerBehaviour:
		return sm.graphConfig.ReentrantTransitionColor
	case *internalTriggerBehaviour:
		return sm.graphConfig.InternalTransitionColor
	}
	return nil
}

func (g *graph) formatOneLine(fromNodeName, toNodeName, label string, color color.Color) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\t%s -> %s [label=\"%s\"", fromNodeName, toNodeName, label))
	if color != nil {
		graphvizColor := toGraphvizColor(color)
		sb.WriteString(fmt.Sprintf(` color="%s" fontcolor="%s"`, graphvizColor, graphvizColor))
	}
	sb.WriteString("];\n")
	return sb.String()
}

func toGraphvizColor(color color.Color) string {
	r, g, b, a := color.RGBA()
	return fmt.Sprintf("#%02x%02x%02x%02x", r>>8, g>>8, b>>8, a>>8)
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

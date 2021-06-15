package stateless

import (
	"context"
	"fmt"
	"strings"

	"github.com/awalterschulze/gographviz"
)

type graph struct {
	*gographviz.Escape
}

func newGraph() *graph {
	output := gographviz.NewEscape()
	graphName := "stateless"
	if err := output.SetName(graphName); err != nil {
		panic(err)
	}
	if err := output.SetDir(true); err != nil {
		panic(err)
	}
	if err := output.AddAttr(graphName, "compound", "true"); err != nil {
		panic(err)
	}
	if err := output.AddAttr(graphName, "rankdir", "LR"); err != nil {
		panic(err)
	}

	return &graph{output}
}

func (g *graph) FormatStateMachine(sm *StateMachine) string {
	for _, sr := range sm.stateConfig {
		// process top level node only
		if sr.Superstate != nil {
			continue
		}
		if err := g.formatState(g.Name, sr); err != nil {
			panic(err)
		}
	}
	for _, sr := range sm.stateConfig {
		if err := g.formatAllStateTransitions(sm, sr); err != nil {
			panic(err)
		}
	}
	initialState, err := sm.State(context.Background())
	if err != nil {
		panic(err)
	}
	if err = g.formatInitial(g.Name, fmt.Sprint(initialState)); err != nil {
		panic(err)
	}
	return g.String()
}

func (g *graph) formatState(parent string, sr *stateRepresentation) error {
	if len(sr.Substates) == 0 {
		if err := g.AddNode(parent, fmt.Sprint(sr.State), map[string]string{
			"label": g.formatStateLabel(sr, false),
			"shape": "Mrecord",
		}); err != nil {
			return err
		}
		return nil
	}
	subGraphName := fmt.Sprintf("cluster_%s", sr.State)
	if err := g.AddSubGraph(parent, subGraphName, map[string]string{
		"label": g.formatStateLabel(sr, true),
	}); err != nil {
		return err
	}
	for _, substate := range sr.Substates {
		if err := g.formatState(subGraphName, substate); err != nil {
			return err
		}
	}
	if sr.HasInitialState {
		if err := g.formatInitial(subGraphName, fmt.Sprint(sr.InitialTransitionTarget)); err != nil {
			return err
		}
	}
	return nil
}

func (g *graph) formatInitial(parent string, initial State) error {
	initNodeName := fmt.Sprintf("%s-init", parent)
	err := g.AddNode(parent, initNodeName, map[string]string{
		"label": "init",
		"shape": "point",
	})
	if err != nil {
		return err
	}
	attrs := map[string]string{}
	initialName, err := g.getStateNameInGraph(initial, attrs, "lhead")
	err = g.AddEdge(initNodeName, initialName, true, attrs)
	if err != nil {
		return err
	}
	return nil
}

func (g *graph) formatActions(sr *stateRepresentation) string {
	es := make([]string, 0, len(sr.EntryActions)+len(sr.ExitActions)+len(sr.ActivateActions)+len(sr.DeactivateActions))
	for _, act := range sr.ActivateActions {
		es = append(es, fmt.Sprintf("activated / %s", act.Description.String()))
	}
	for _, act := range sr.DeactivateActions {
		es = append(es, fmt.Sprintf("deactivated / %s", act.Description.String()))
	}
	for _, act := range sr.EntryActions {
		if act.Trigger == nil {
			es = append(es, fmt.Sprintf("entry / %s", act.Description.String()))
		}
	}
	for _, act := range sr.ExitActions {
		es = append(es, fmt.Sprintf("exit / %s", act.Description.String()))
	}
	return strings.Join(es, `\n`)
}

func (g *graph) formatStateLabel(sr *stateRepresentation, subGraph bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprint(sr.State))
	act := g.formatActions(sr)
	if act != "" {
		if subGraph {
			sb.WriteString("\n----------\n")
		} else {
			sb.WriteString("|")
		}
		sb.WriteString(act)
	}
	return sb.String()
}

func (g *graph) getEntryActions(ab []actionBehaviour, t Trigger) []string {
	var actions []string
	for _, ea := range ab {
		if ea.Trigger == t {
			actions = append(actions, ea.Description.String())
		}
	}
	return actions
}

func (g *graph) formatAllStateTransitions(sm *StateMachine, sr *stateRepresentation) error {
	for _, triggers := range sr.TriggerBehaviours {
		for _, trigger := range triggers {
			var err error
			switch t := trigger.(type) {
			case *ignoredTriggerBehaviour:
				err = g.formatOneTransition(sr.State, sr.State, t.Trigger, nil, t.Guard)
			case *reentryTriggerBehaviour:
				actions := g.getEntryActions(sr.EntryActions, t.Trigger)
				err = g.formatOneTransition(sr.State, t.Destination, t.Trigger, actions, t.Guard)
			case *internalTriggerBehaviour:
				actions := g.getEntryActions(sr.EntryActions, t.Trigger)
				err = g.formatOneTransition(sr.State, sr.State, t.Trigger, actions, t.Guard)
			case *transitioningTriggerBehaviour:
				var actions []string
				if dest, ok := sm.stateConfig[t.Destination]; ok {
					actions = g.getEntryActions(dest.EntryActions, t.Trigger)
				}
				err = g.formatOneTransition(sr.State, t.Destination, t.Trigger, actions, t.Guard)
			case *dynamicTriggerBehaviour:
				// TODO: not supported yet
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *graph) getEdgeNodeName(node string, attrs map[string]string, subGraphLabel string) (string, error) {
	if _, ok := g.Nodes.Lookup[node]; ok {
		return node, nil
	} else if _, ok := g.SubGraphs.SubGraphs[node]; ok {
		// if state has substate, use ANY of substate as destination, and add ltail/lhead attr
		if subGraphLabel != "" {
			attrs[subGraphLabel] = node
		}
		for subNode, isChild := range g.Relations.ParentToChildren[node] {
			if !isChild {
				continue
			}
			return g.getEdgeNodeName(subNode, attrs, "")
		}
		panic(fmt.Sprintf("%s has no child", node))
	} else {
		// state may not be configured, treat it as top level simple state
		if err := g.AddNode(g.Name, node, nil); err != nil {
			return "", err
		}
		return node, nil
	}
}

func (g *graph) getStateNameInGraph(state State, attrs map[string]string, subGraphLabel string) (string, error) {
	if _, ok := g.Nodes.Lookup[fmt.Sprint(state)]; ok {
		return g.getEdgeNodeName(fmt.Sprint(state), attrs, subGraphLabel)
	} else if _, ok = g.SubGraphs.SubGraphs[fmt.Sprintf("cluster_%s", state)]; ok {
		return g.getEdgeNodeName(fmt.Sprintf("cluster_%s", state), attrs, subGraphLabel)
	}
	return g.getEdgeNodeName(fmt.Sprint(state), attrs, subGraphLabel)
}

func (g *graph) formatOneTransition(source, destination State, trigger Trigger, actions []string, guards transitionGuard) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprint(trigger))
	if len(actions) > 0 {
		sb.WriteString(" / ")
		sb.WriteString(strings.Join(actions, ", "))
	}
	for _, info := range guards.Guards {
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(fmt.Sprintf("[%s]", info.Description.String()))
	}
	attrs := map[string]string{"label": sb.String()}
	sourceName, err := g.getStateNameInGraph(source, attrs, "ltail")
	if err != nil {
		return err
	}
	destinationName, err := g.getStateNameInGraph(destination, attrs, "lhead")
	if err != nil {
		return err
	}
	if source == destination && source != sourceName {
		// TODO internal transition on subgraph is not supported by graphviz
		return nil
	}
	return g.AddEdge(sourceName, destinationName, true, attrs)
}

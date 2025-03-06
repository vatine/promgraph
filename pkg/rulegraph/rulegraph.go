// Copyright 2021 Ingvar Mattsson
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rulegraph

import (
	"fmt"
	"io"
	"strings"

	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
)

type ruleType uint8

const (
	recorded ruleType = 0
	alert    ruleType = 1
	unknown  ruleType = 2
)

// Custom error type
type compoundError struct {
	errors []error
}

// Error string function
func (ce *compoundError) Error() string {
	var out strings.Builder

	for _, e := range ce.errors {
		fmt.Fprintf(&out, "%v\n", e)
	}

	return out.String()
}

func (ce *compoundError) hasError() bool {
	return len(ce.errors) > 0
}

// Accumulate errors into our custom error type.
func (ce *compoundError) acc(e ...error) {
	if len(e) == 1 {
		if ce2, ok := e[0].(*compoundError); ok {
			ce.errors = append(ce.errors, ce2.errors...)
		}
	}
	ce.errors = append(ce.errors, e...)
}

// Nodes and edges for a graph. This also has some internal data
// structures to facilitate walking the expression tree(s) of rules.
type Graph struct {
	nodes map[string]ruleType
	edges map[string]bool
	nexts []string
}

// Create a new, emplty, graph that is ready to use.
func newGraph() *Graph {
	rv := new(Graph)
	rv.nodes = make(map[string]ruleType)
	rv.edges = make(map[string]bool)
	return rv
}

// Method implementing the promql parser Visitor interface.
func (g *Graph) Visit(node parser.Node, path []parser.Node) (parser.Visitor, error) {
	if node == nil && path == nil {
		return nil, nil
	}

	if vs, ok := node.(*parser.VectorSelector); ok {
		// We have something that can be used
		switch {
		case vs.Name == "ALERTS":
			// We need to parse out possible alerts...
			for _, m := range vs.LabelMatchers {
				if m.Name == "alertname" {
					for name, rt := range g.nodes {
						if rt == alert && m.Matches(name) {
							g.nexts = append(g.nexts, name)
						}
					}

				}
			}
		case vs.Name != "":
			g.nexts = append(g.nexts, vs.Name)
		}
	}

	return g, nil
}

// Contruct a string representing the edge from one node to another.
func buildEdge(from, to string) string {
	if strings.Contains(from, ":") {
		from = fmt.Sprintf("\"%s\"", from)
	}

	if strings.Contains(to, ":") {
		to = fmt.Sprintf("\"%s\"", to)
	}

	return fmt.Sprintf("%s -> %s", from, to)
}

// Get all edges from one node to successors. This basically means
// "parse the expression, then traverse the expression AST, looking
// for metrics".
func (g *Graph) getEdges(r rulefmt.RuleNode) {
	g.nexts = []string{}

	expr, _ := parser.ParseExpr(r.Expr.Value)

	_ = parser.Walk(g, expr, nil)

	from := ruleName(r)
	for _, next := range g.nexts {
		edge := buildEdge(from, next)
		g.edges[edge] = true
		if _, ok := g.nodes[next]; !ok {
			g.nodes[next] = unknown
		}
	}
}

// Return the ruleType (basically, "recorded" or "alert") of a rule.
func getType(r rulefmt.RuleNode) ruleType {
	if r.Record.Value == "" {
		return alert
	}

	return recorded
}

// Return the name of a rule.
func ruleName(r rulefmt.RuleNode) string {
	name := r.Record.Value
	if name == "" {
		name = r.Alert.Value
	}

	if i := strings.Index(name, "{"); i >= 0 {
		name = name[0:i]
	}

	return name
}

// Build a diagram of the interdependency of all rule files passed
// in. We expect that these have already been checked for errors and
// passed that check.
//
// When we have processed these, simply serialise the graph as a DOT
// graph to w.
func BuildRuleDiagram(groups []rulefmt.RuleGroup) *Graph {
	g := newGraph()

	for _, group := range groups {
		for _, rule := range group.Rules {
			g.nodes[ruleName(rule)] = getType(rule)
		}
	}

	for _, group := range groups {
		for _, rule := range group.Rules {
			g.getEdges(rule)
		}
	}

	return g
}

func EmitGraph(g *Graph, w io.Writer) {
	fmt.Fprintf(w, "digraph {\n")
	for name, t := range g.nodes {
		if strings.Contains(name, ":") {
			name = fmt.Sprintf("\"%s\"", name)
		}
		switch {
		case t == recorded:
			fmt.Fprintf(w, "  %s [shape=oval]\n", name)
		case t == alert:
			fmt.Fprintf(w, "  %s [shape=doubleoctagon]\n", name)
		case t == unknown:
			fmt.Fprintf(w, "  %s [shape=rect]\n", name)
		default:
			fmt.Fprintf(w, "  /* Unknown node type %v for %s */\n", t, name)
		}
	}

	fmt.Fprintf(w, "\n")

	for edge := range g.edges {
		fmt.Fprintf(w, "  %s\n", edge)
	}

	fmt.Fprintf(w, "}\n")
}

func LoadRulefile(filename string) ([]rulefmt.RuleGroup, error) {
	var ce compoundError

	rgs, errs := rulefmt.ParseFile(filename)
	if errs != nil {
		ce.acc(errs...)
		return []rulefmt.RuleGroup{}, &ce
	}

	return rgs.Groups, nil
}

func LoadRuleFiles(names ...string) ([]rulefmt.RuleGroup, error) {
	var acc []rulefmt.RuleGroup
	var ce compoundError

	for _, name := range names {
		rgs, err := LoadRulefile(name)
		if err != nil {
			ce.acc(err)
		} else {
			acc = append(acc, rgs...)
		}
	}

	if ce.hasError() {
		return []rulefmt.RuleGroup{}, &ce
	}

	return acc, nil
}

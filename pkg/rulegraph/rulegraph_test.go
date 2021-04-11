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
	"sort"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
)

func graphEq(g1, g2 *Graph, t *testing.T) {
	var errSeen bool

	nodes := make(map[string]int)

	for key, _ := range g1.nodes {
		nodes[key] = 1
	}

	for key, _ := range g2.nodes {
		nodes[key] |= 2
	}

	for key, val := range nodes {
		switch {
		case val == 1:
			t.Errorf("Node %s seen, not expected", key)
			errSeen = true
		case val == 2:
			t.Errorf("Node %s not seen, was expected", key)
			errSeen = true
		}
	}

	edges := make(map[string]int)

	for key, _ := range g1.edges {
		edges[key] = 1
	}

	for key, _ := range g2.edges {
		edges[key] |= 2
	}

	for key, val := range edges {
		switch {
		case val == 1:
			t.Errorf("Edge %s seen, not expected", key)
			errSeen = true
		case val == 2:
			t.Errorf("Edge %s not seen, was expected", key)
			errSeen = true
		}
	}

	if errSeen {
		return
	}

	for key, _ := range nodes {
		if g1.nodes[key] != g2.nodes[key] {
			t.Errorf("Node %s, saw type %d, expected %d", key, g1.nodes[key], g2.nodes[key])
		}
	}
}

func TestGraphFinder(t *testing.T) {
	cases := []struct {
		expr string
		want []string
	}{
		{"a+b", []string{"a", "b"}},
		{"max_over_time(a[1h]) > 3", []string{"a"}},
	}

	for ix, c := range cases {
		var mf Graph
		expr, err := parser.ParseExpr(c.expr)
		if err != nil {
			t.Errorf("Case #%d, unexpected error: %v", ix, err)
			continue
		}

		parser.Walk(&mf, expr, nil)
		want := strings.Join(c.want, ",")
		sort.Strings(mf.nexts)
		saw := strings.Join(mf.nexts, ",")
		if saw != want {
			t.Errorf("Case #%d, saw %s want %s", ix, saw, want)
		}
	}
}

func TestCompoundError(t *testing.T) {
	var ce compoundError

	ce.acc(fmt.Errorf("error1"))
	ce.acc(fmt.Errorf("error2"))

	saw := ce.Error()
	want := "error1\nerror2\n"
	if saw != want {
		t.Errorf("Saw «%s», want «%s».", saw, want)
	}
}

func TestLoadRulefile(t *testing.T) {
	cases := []struct {
		filename string
		err      bool
	}{
		{"testdata/ok.rule", false},
		{"testdata/bad.rule", true},
	}

	for ix, c := range cases {
		_, err := LoadRulefile(c.filename)

		if err != nil && !c.err {
			t.Errorf("Case #%d, unexpected error: %v", ix, err)
		}

		if err == nil && c.err {
			t.Errorf("Case #%d, no error, expected one.", ix)
		}
	}
}

func buildGraph(rules, alerts, unknowns []string, edges []string) *Graph {
	g := newGraph()

	for _, name := range rules {
		g.nodes[name] = recorded
	}

	for _, name := range alerts {
		g.nodes[name] = alert
	}

	for _, name := range unknowns {
		g.nodes[name] = unknown
	}

	for _, edge := range edges {
		g.edges[edge] = true
	}

	return g
}

func TestBuildRuleDiagram(t *testing.T) {
	cases := []struct {
		files   []string
		rules   []string
		alerts  []string
		unknown []string
		edges   []string
	}{
		{
			[]string{"testdata/ok.rule"},
			[]string{"test:rule:sum"},
			[]string{},
			[]string{"up"},
			[]string{"test:rule:sum -> up"},
		},
		{
			[]string{"testdata/ok.rule", "testdata/alerts.rule"},
			[]string{"test:rule:sum"},
			[]string{"TestAlert"},
			[]string{"up"},
			[]string{"test:rule:sum -> up", "TestAlert -> test:rule:sum"},
		},
	}

	for ix, c := range cases {
		groups, err := LoadRuleFiles(c.files...)
		if err != nil {
			t.Errorf("Case #%d, error(s) loading: %v", ix, err)
			continue
		}
		want := buildGraph(c.rules, c.alerts, c.unknown, c.edges)
		got := BuildRuleDiagram(groups)
		graphEq(got, want, t)
	}
}

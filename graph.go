package main

import (
	"fmt"
	"strings"

	"github.com/emicklei/dot"
)

func newGraph() *dot.Graph {
	g := dot.NewGraph(dot.Directed)
	g.Attr("newrank", "true") // global rank instead of per-subgraph (ensures access rules are always in the same place (at bottom))
	return g
}

func newNamespaceSubgraph(g *dot.Graph, ns string) *dot.Graph {
	if ns == "" {
		return g
	}
	gns := g.Subgraph(ns, dot.ClusterOption{})
	gns.Attr("style", "dashed")
	return gns
}

func newSubjectNode0(g *dot.Graph, kind, name string, exists, highlight bool) dot.Node {
	return g.Node(kind+"-"+name).
		Box().
		Attr("label", formatLabel(fmt.Sprintf("%s\n(%s)", name, kind), highlight)).
		Attr("style", iff(exists, "filled", "dotted")).
		Attr("color", iff(exists, "black", "red")).
		Attr("penwidth", iff(highlight || !exists, "2.0", "1.0")).
		Attr("fillcolor", "#2f6de1").
		Attr("fontcolor", iff(exists, "#f0f0f0", "#030303"))
}

func newRoleBindingNode(g *dot.Graph, name string, highlight bool) dot.Node {
	return g.Node("rb-"+name).
		Attr("label", formatLabel(name, highlight)).
		Attr("shape", "octagon").
		Attr("style", "filled").
		Attr("penwidth", iff(highlight, "2.0", "1.0")).
		Attr("fillcolor", "#ffcc00").
		Attr("fontcolor", "#030303")
}

func newClusterRoleBindingNode(g *dot.Graph, name string, highlight bool) dot.Node {
	return g.Node("crb-"+name).
		Attr("label", formatLabel(name, highlight)).
		Attr("shape", "doubleoctagon").
		Attr("style", "filled").
		Attr("penwidth", iff(highlight, "2.0", "1.0")).
		Attr("fillcolor", "#ffcc00").
		Attr("fontcolor", "#030303")
}

func newRoleNode(g *dot.Graph, namespace, name string, exists, highlight bool) dot.Node {
	node := g.Node("r-"+namespace+"/"+name).
		Attr("label", formatLabel(name, highlight)).
		Attr("shape", "octagon").
		Attr("style", iff(exists, "filled", "dotted")).
		Attr("color", iff(exists, "black", "red")).
		Attr("penwidth", iff(highlight || !exists, "2.0", "1.0")).
		Attr("fillcolor", "#ff9900").
		Attr("fontcolor", "#030303")
	g.Root().AddToSameRank("Roles", node)
	return node
}

func newClusterRoleNode(g *dot.Graph, bindingNamespace, roleName string, exists, highlight bool) dot.Node {
	node := g.Node("cr-"+bindingNamespace+"/"+roleName).
		Attr("label", formatLabel(roleName, highlight)).
		Attr("shape", "doubleoctagon").
		Attr("style", iff(exists, iff(bindingNamespace == "", "filled", "filled,dashed"), "dotted")).
		Attr("color", iff(exists, "black", "red")).
		Attr("penwidth", iff(highlight || !exists, "2.0", "1.0")).
		Attr("fillcolor", "#ff9900").
		Attr("fontcolor", "#030303")
	g.Root().AddToSameRank("Roles", node)
	return node
}

func newRulesNode0(g *dot.Graph, namespace, roleName, rulesHTML string, highlight bool) dot.Node {
	return g.Node("rules-"+namespace+"/"+roleName).
		Attr("label", dot.HTML(rulesHTML)).
		Attr("shape", "note").
		Attr("penwidth", iff(highlight, "2.0", "1.0"))
}

func regularLine(str string) string {
	return escapeHTML(str) + `<br align="left"/>`
}

func boldLine(str string) string {
	return "<b>" + escapeHTML(str) + "</b>" + `<br align="left"/>`
}

func formatLabel(label string, highlight bool) interface{} {
	if highlight {
		return dot.HTML("<b>" + escapeHTML(label) + "</b>")
	} else {
		return label
	}
}

func escapeHTML(str string) string {
	str = strings.ReplaceAll(str, `<`, `&lt;`)
	str = strings.ReplaceAll(str, `>`, `&gt;`)
	str = strings.ReplaceAll(str, ` `, `&nbsp;`)
	str = strings.ReplaceAll(str, "\n", `<br/>`)
	return str
}

func newSubjectToBindingEdge(subjectNode dot.Node, bindingNode dot.Node) dot.Edge {
	return edge(subjectNode, bindingNode).Attr("dir", "back")
}

func newBindingToRoleEdge(bindingNode dot.Node, roleNode dot.Node) dot.Edge {
	return edge(bindingNode, roleNode)
}

func newRoleToRulesEdge(roleNode dot.Node, rulesNode dot.Node) dot.Edge {
	return edge(roleNode, rulesNode)
}

// edge creates a new edge between two nodes, but only if the edge doesn't exist yet
func edge(from dot.Node, to dot.Node) dot.Edge {
	existingEdges := from.EdgesTo(to)
	if len(existingEdges) == 0 {
		return from.Edge(to)
	} else {
		return existingEdges[0]
	}
}

func iff(condition bool, string1, string2 string) string {
	if condition {
		return string1
	} else {
		return string2
	}
}

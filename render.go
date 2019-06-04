package main

import (
	"fmt"
	"strings"

	"github.com/emicklei/dot"
)

func (r *Rback) genGraph() *dot.Graph {
	g := newGraph()
	r.renderLegend(g)

	for _, bindings := range r.permissions.RoleBindings {
		for _, binding := range bindings {
			if !r.shouldRenderBinding(binding) {
				continue
			}

			gns := newNamespaceSubgraph(g, binding.namespace)

			bindingNode := r.newBindingNode(gns, binding)
			roleNode := r.newRoleAndRulesNodePair(gns, binding.namespace, binding.role)

			newBindingToRoleEdge(bindingNode, roleNode)

			saNodes := []dot.Node{}
			for _, subject := range binding.subjects {
				renderSubject := (r.config.resourceKind != kindServiceAccount) ||
					(r.namespaceSelected(subject.namespace) && r.resourceNameSelected(subject.name))

				if renderSubject {
					gns := newNamespaceSubgraph(g, subject.namespace)
					subjectNode := r.newSubjectNode(gns, subject.kind, subject.namespace, subject.name)
					saNodes = append(saNodes, subjectNode)
				}
			}

			for _, saNode := range saNodes {
				newSubjectToBindingEdge(saNode, bindingNode)
			}
		}
	}

	// draw any additional ServiceAccounts that weren't referenced by bindings (and thus drawn in the code above)
	if r.config.resourceKind == "" || r.config.resourceKind == kindServiceAccount {
		for ns, sas := range r.permissions.ServiceAccounts {
			if !r.namespaceSelected(ns) {
				continue
			}
			gns := newNamespaceSubgraph(g, ns)

			for sa, _ := range sas {
				renderSA := r.config.resourceKind == "" || (r.namespaceSelected(ns) && r.resourceNameSelected(sa))
				if renderSA {
					r.newSubjectNode(gns, "ServiceAccount", ns, sa)
				}
			}
		}
	}

	// draw any additional Roles that weren't referenced by bindings (and thus already drawn)
	for ns, roles := range r.permissions.Roles {
		var renderRoles bool

		areClusterRoles := ns == ""
		if areClusterRoles {
			renderRoles = (r.config.resourceKind == "" || r.config.resourceKind == kindClusterRole) && r.allNamespaces()
		} else {
			renderRoles = (r.config.resourceKind == "" || r.config.resourceKind == kindRole) && r.namespaceSelected(ns)
		}

		if !renderRoles {
			continue
		}

		gns := newNamespaceSubgraph(g, ns)
		for roleName, _ := range roles {
			renderRole := r.namespaceSelected(ns) && r.resourceNameSelected(roleName)
			if renderRole {
				r.newRoleAndRulesNodePair(gns, "", NamespacedName{ns, roleName})
			}
		}
	}

	return g
}

func (r *Rback) renderLegend(g *dot.Graph) {
	if !r.config.showLegend {
		return
	}

	legend := g.Subgraph("LEGEND", dot.ClusterOption{})

	namespace := newNamespaceSubgraph(legend, "Namespace")

	sa := newSubjectNode0(namespace, "Kind", "Subject", true, false)
	missingSa := newSubjectNode0(namespace, "Kind", "Missing Subject", false, false)

	role := newRoleNode(namespace, "ns", "Role", true, false)
	clusterRoleBoundLocally := newClusterRoleNode(namespace, "ns", "ClusterRole", true, false) // bound by (namespaced!) RoleBinding
	clusterrole := newClusterRoleNode(legend, "", "ClusterRole", true, false)

	roleBinding := newRoleBindingNode(namespace, "RoleBinding", false)
	newSubjectToBindingEdge(sa, roleBinding)
	newSubjectToBindingEdge(missingSa, roleBinding)
	newBindingToRoleEdge(roleBinding, role)

	roleBinding2 := newRoleBindingNode(namespace, "RoleBinding-to-ClusterRole", false)
	roleBinding2.Attr("label", "RoleBinding")
	newSubjectToBindingEdge(sa, roleBinding2)
	newBindingToRoleEdge(roleBinding2, clusterRoleBoundLocally)

	clusterRoleBinding := newClusterRoleBindingNode(legend, "ClusterRoleBinding", false)
	newSubjectToBindingEdge(sa, clusterRoleBinding)
	newBindingToRoleEdge(clusterRoleBinding, clusterrole)

	if r.config.showRules {
		nsrules := newRulesNode0(namespace, "ns", "Role", "Namespace-scoped\naccess rules", false)
		newRoleToRulesEdge(role, nsrules)

		nsrules2 := newRulesNode0(namespace, "ns", "ClusterRole", "Namespace-scoped access rules From ClusterRole", false)
		nsrules2.Attr("label", "Namespace-scoped\naccess rules")
		newRoleToRulesEdge(clusterRoleBoundLocally, nsrules2)

		clusterrules := newRulesNode0(legend, "", "ClusterRole", "Cluster-scoped\naccess rules", false)
		newRoleToRulesEdge(clusterrole, clusterrules)
	}
}

func (r *Rback) shouldRenderBinding(binding Binding) bool {
	switch r.config.resourceKind {
	case "":
		return r.namespaceSelected(binding.namespace)
	case kindRoleBinding:
		return r.namespaceSelected(binding.namespace) && r.resourceNameSelected(binding.name)
	case kindClusterRoleBinding:
		return binding.namespace == "" && r.resourceNameSelected(binding.name)
	case kindServiceAccount:
		for _, subject := range binding.subjects {
			if subject.kind == "ServiceAccount" &&
				r.namespaceSelected(subject.namespace) &&
				r.resourceNameSelected(subject.name) &&
				r.subjectExists("ServiceAccount", subject.namespace, subject.name) {
				return true
			}
		}
	case kindUser:
		for _, subject := range binding.subjects {
			if subject.kind == "User" && r.resourceNameSelected(subject.name) {
				return true
			}
		}
	case kindGroup:
		for _, subject := range binding.subjects {
			if subject.kind == "Group" && r.resourceNameSelected(subject.name) {
				return true
			}
		}
	case kindRole:
		bindingPointsToClusterRole := binding.role.namespace == ""
		return !bindingPointsToClusterRole &&
			r.namespaceSelected(binding.role.namespace) &&
			r.resourceNameSelected(binding.role.name) &&
			r.roleExists(binding.role)
	case kindClusterRole:
		bindingPointsToClusterRole := binding.role.namespace == ""
		return bindingPointsToClusterRole &&
			r.resourceNameSelected(binding.role.name) &&
			r.roleExists(binding.role)
	case kindRule:
		bindingPointsToClusterRole := binding.role.namespace == ""
		return r.ruleMatchesSelection(binding.role) && (bindingPointsToClusterRole || r.namespaceSelected(binding.role.namespace))
	}
	return false
}

func (r *Rback) newBindingNode(gns *dot.Graph, binding Binding) dot.Node {
	if binding.namespace == "" {
		return newClusterRoleBindingNode(gns, binding.name, r.isFocused(kindClusterRoleBinding, "", binding.name))
	} else {
		return newRoleBindingNode(gns, binding.name, r.isFocused(kindRoleBinding, binding.namespace, binding.name))
	}
}

func (r *Rback) newRoleAndRulesNodePair(gns *dot.Graph, bindingNamespace string, role NamespacedName) dot.Node {
	var roleNode dot.Node
	if role.namespace == "" {
		roleNode = newClusterRoleNode(gns, bindingNamespace, role.name, r.roleExists(role), r.isFocused(kindClusterRole, role.namespace, role.name))
	} else {
		roleNode = newRoleNode(gns, role.namespace, role.name, r.roleExists(role), r.isFocused(kindRole, role.namespace, role.name))
	}
	if r.config.showRules {
		rulesNode := r.newRulesNode(gns, role.namespace, role.name, r.isFocused(kindRule, role.namespace, role.name))
		if rulesNode != nil {
			newRoleToRulesEdge(roleNode, *rulesNode)
		}
	}
	return roleNode
}

func (r *Rback) roleExists(role NamespacedName) bool {
	if roles, nsExists := r.permissions.Roles[role.namespace]; nsExists {
		if _, roleExists := roles[role.name]; roleExists {
			return true
		}
	}
	return false
}

func (r *Rback) newSubjectNode(gns *dot.Graph, kind string, ns string, name string) dot.Node {
	return newSubjectNode0(gns, kind, name, r.subjectExists(kind, ns, name), r.isFocused(strings.ToLower(kind), ns, name))
}

func (r *Rback) subjectExists(kind string, ns string, name string) bool {
	if strings.ToLower(kind) != kindServiceAccount {
		return true // assume users and groups exist
	}

	if sas, nsExists := r.permissions.ServiceAccounts[ns]; nsExists {
		if _, saExists := sas[name]; saExists {
			return true
		}
	}
	return false
}

func (r *Rback) isFocused(kind string, ns string, name string) bool {
	if kind == kindRule {
		return r.ruleMatchesSelection(NamespacedName{ns, name})
	} else {
		return r.config.resourceKind == kind && r.namespaceSelected(ns) && r.resourceNameSelected(name)
	}
}

func (r *Rback) ruleMatchesSelection(roleRef NamespacedName) bool {
	if r.config.resourceKind == kindRule {
		if roles, found := r.permissions.Roles[roleRef.namespace]; found {
			if role, found := roles[roleRef.name]; found {
				return r.config.whoCan.matchesAnyRuleIn(role)
			}
		}
	}
	return false
}

func (w *WhoCan) matchesAnyRuleIn(role Role) bool {
	for _, rule := range role.rules {
		if w.matches(rule) {
			return true
		}
	}
	return false
}

func (w *WhoCan) matches(rule Rule) bool {
	return (contains(rule.verbs, "*") || contains(rule.verbs, w.verb)) &&
		(contains(rule.resources, "*") || contains(rule.resources, w.resourceKind)) &&
		(w.resourceName == "" || len(rule.resourceNames) == 0 || contains(rule.resourceNames, w.resourceName)) // TODO: also check API group!
}

func (r *Rback) newRulesNode(g *dot.Graph, namespace, roleName string, highlight bool) *dot.Node {
	var rulesText string
	if roles, found := r.permissions.Roles[namespace]; found {
		if role, found := roles[roleName]; found {
			ellipsis := regularLine("...")
			for _, rule := range role.rules {
				ruleMatches := r.config.resourceKind == kindRule && highlight && r.config.whoCan.matches(rule)
				if ruleMatches {
					rulesText += boldLine(rule.toHumanReadableString())
				} else {
					if r.config.whoCan.showMatchedOnly {
						if !strings.HasSuffix(rulesText, ellipsis) {
							rulesText += ellipsis
						}
					} else {
						rulesText += regularLine(rule.toHumanReadableString())
					}
				}
			}
		}
	}
	if rulesText == "" {
		return nil
	} else {
		node := newRulesNode0(g, namespace, roleName, rulesText, highlight)
		return &node
	}
}

func (r *Rule) toHumanReadableString() string {
	result := strings.Join(r.verbs, ",")
	if len(r.resources) > 0 {
		result += fmt.Sprintf(` %v`, strings.Join(r.resources, ","))
	}
	if len(r.resourceNames) > 0 {
		result += fmt.Sprintf(` "%v"`, strings.Join(r.resourceNames, ","))
	}
	if len(r.nonResourceURLs) > 0 {
		result += fmt.Sprintf(` %v`, strings.Join(r.nonResourceURLs, ","))
	}
	if len(r.apiGroups) > 1 || (len(r.apiGroups) == 1 && r.apiGroups[0] != "") {
		result += fmt.Sprintf(` (%v)`, strings.Join(r.apiGroups, ","))
	}
	return result
}

func (r *Rback) resourceNameSelected(name string) bool {
	return r.allResourceNames() || contains(r.config.resourceNames, name)
}

func (r *Rback) allResourceNames() bool {
	return len(r.config.resourceNames) == 0
}

func (r *Rback) namespaceSelected(ns string) bool {
	return r.allNamespaces() || contains(r.config.namespaces, ns)
}

func (r *Rback) allNamespaces() bool {
	return len(r.config.namespaces) == 1 && r.config.namespaces[0] == ""
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if value == v {
			return true
		}
	}
	return false
}

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/emicklei/dot"
	"github.com/mhausenblas/kubecuddler"
)

type Rback struct {
	config      Config
	permissions Permissions
}

type Config struct {
	showRules       bool
	showLegend      bool
	namespaces      []string
	ignoredPrefixes []string
	resourceKind    string
	resourceNames   []string
}

type Permissions struct {
	ServiceAccounts     map[string]map[string]string // map[namespace]map[name]json
	Roles               map[string]map[string]string
	ClusterRoles        map[string]string
	RoleBindings        map[string]map[string]string
	ClusterRoleBindings map[string]string
}

func main() {

	config := Config{}
	flag.BoolVar(&config.showLegend, "show-legend", true, "Whether to show the legend or not")
	flag.BoolVar(&config.showRules, "show-rules", true, "Whether to render RBAC access rules (e.g. \"get pods\") or not")

	var namespaces string
	flag.StringVar(&namespaces, "n", "", "The namespace to render (also supports multiple, comma-delimited namespaces)")

	var ignoredPrefixes string
	flag.StringVar(&ignoredPrefixes, "ignore-prefixes", "system:", "Comma-delimited list of (Cluster)Role(Binding) prefixes to ignore ('none' to not ignore anything)")
	flag.Parse()

	if flag.NArg() > 0 {
		config.resourceKind = normalizeKind(flag.Arg(0))
	}
	if flag.NArg() > 1 {
		config.resourceNames = flag.Args()[1:]
	}

	config.namespaces = strings.Split(namespaces, ",")

	if ignoredPrefixes != "none" {
		config.ignoredPrefixes = strings.Split(ignoredPrefixes, ",")
	}

	rback := Rback{config: config}

	err := rback.fetchPermissions()
	if err != nil {
		fmt.Printf("Can't query permissions due to :%v", err)
		os.Exit(-1)
	}
	g := rback.genGraph()
	fmt.Println(g.String())
}

var kindMap = map[string]string{
	"sa":                  "serviceaccount",
	"serviceaccounts":     "serviceaccount",
	"rb":                  "rolebinding",
	"rolebindings":        "rolebinding",
	"crb":                 "clusterrolebinding",
	"clusterrolebindings": "clusterrolebinding",
	"r":                   "role",
	"roles":               "role",
	"cr":                  "clusterrole",
	"clusterroles":        "clusterrole",
}

func normalizeKind(kind string) string {
	kind = strings.ToLower(kind)
	entry, exists := kindMap[kind]
	if exists {
		return entry
	}
	return kind
}

func (r *Rback) shouldIgnore(name string) bool {
	for _, prefix := range r.config.ignoredPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// getServiceAccounts retrieves data about service accounts across all namespaces
func (r *Rback) getServiceAccounts() (result map[string]map[string]string, err error) {
	return r.getNamespacedResources("sa", []string{""}, []string{})
}

func getNamespacedName(obj map[string]interface{}) NamespacedName {
	metadata := obj["metadata"].(map[string]interface{})
	ns := metadata["namespace"]
	name := metadata["name"]
	return NamespacedName{ns.(string), name.(string)}
}

// getRoles retrieves data about roles across all namespaces
func (r *Rback) getRoles() (result map[string]map[string]string, err error) {
	return r.getNamespacedResources("roles", []string{""}, []string{})
}

// getRoleBindings retrieves data about roles across all namespaces
func (r *Rback) getRoleBindings() (result map[string]map[string]string, err error) {
	return r.getNamespacedResources("rolebindings", []string{""}, []string{})
}

func (r *Rback) getNamespacedResources(kind string, namespaces, names []string) (result map[string]map[string]string, err error) {
	result = make(map[string]map[string]string)
	for _, namespace := range namespaces {
		var args []string
		if namespace == "" {
			args = []string{kind, "--all-namespaces", "--output", "json"}
		} else if len(names) == 0 {
			args = []string{kind, "-n", namespace, "--output", "json"}
		} else {
			args = append([]string{kind, "-n", namespace, "--output", "json"}, names...)
		}
		res, err := kubecuddler.Kubectl(true, true, "", "get", args...)
		if err != nil {
			return result, err
		}
		var d map[string]interface{}
		b := []byte(res)
		err = json.Unmarshal(b, &d)
		if err != nil {
			return result, err
		}

		if d["kind"] == "List" {
			items := d["items"].([]interface{})
			for _, i := range items {
				item := i.(map[string]interface{})
				nn := getNamespacedName(item)
				if !r.shouldIgnore(nn.name) {
					itemJson, _ := struct2json(item)
					result[nn.namespace] = ensureMap(result[nn.namespace])
					result[nn.namespace][nn.name] = itemJson
				}
			}
		} else {
			nn := getNamespacedName(d)
			itemJson, _ := struct2json(d)
			result[nn.namespace] = ensureMap(result[nn.namespace])
			result[nn.namespace][nn.name] = itemJson
		}
	}
	return result, nil
}

// ensureMap is similar to append(), but for maps - it creates a new map if necessary
func ensureMap(m map[string]string) map[string]string {
	if m != nil {
		return m
	}
	return make(map[string]string)
}

// getClusterRoles retrieves data about cluster roles
func (r *Rback) getClusterRoles() (result map[string]string, err error) {
	return r.getClusterScopedResources("clusterroles")
}

// getClusterRoleBindings retrieves data about cluster role bindings
func (r *Rback) getClusterRoleBindings() (result map[string]string, err error) {
	return r.getClusterScopedResources("clusterrolebindings")

}

func (r *Rback) getClusterScopedResources(kind string) (result map[string]string, err error) {
	result = map[string]string{}
	res, err := kubecuddler.Kubectl(true, true, "", "get", kind, "--output", "json")
	if err != nil {
		return result, err
	}
	var d map[string]interface{}
	b := []byte(res)
	err = json.Unmarshal(b, &d)
	if err != nil {
		return result, err
	}
	items := d["items"].([]interface{})
	for _, i := range items {
		item := i.(map[string]interface{})
		metadata := item["metadata"].(map[string]interface{})
		name := metadata["name"].(string)
		if !r.shouldIgnore(name) {
			itemJson, _ := struct2json(item)
			result[name] = itemJson
		}
	}
	return result, nil
}

// fetchPermissions retrieves data about all access control related data
// from service accounts to roles and bindings, both namespaced and the
// cluster level.
func (r *Rback) fetchPermissions() error {
	sa, err := r.getServiceAccounts()
	if err != nil {
		return err
	}
	r.permissions.ServiceAccounts = sa

	roles, err := r.getRoles()
	if err != nil {
		return err
	}
	r.permissions.Roles = roles

	rb, err := r.getRoleBindings()
	if err != nil {
		return err
	}
	r.permissions.RoleBindings = rb

	cr, err := r.getClusterRoles()
	if err != nil {
		return err
	}
	r.permissions.ClusterRoles = cr

	crb, err := r.getClusterRoleBindings()
	if err != nil {
		return err
	}
	r.permissions.ClusterRoleBindings = crb
	return nil
}

type Binding struct {
	NamespacedName
	role     NamespacedName
	subjects []KindNamespacedName
}

type NamespacedName struct {
	namespace string
	name      string
}

type KindNamespacedName struct {
	kind string
	NamespacedName
}

// lookupBindings lists bindings & roles for a given service account
func (r *Rback) lookupBindings(bindings map[string]string, saName, saNamespace string) (results []Binding, err error) {
	for _, rb := range bindings {
		var binding map[string]interface{}
		b := []byte(rb)
		err = json.Unmarshal(b, &binding)
		if err != nil {
			return results, err
		}

		metadata := binding["metadata"].(map[string]interface{})
		bindingName := metadata["name"].(string)
		bindingNs := ""
		if metadata["namespace"] != nil {
			bindingNs = metadata["namespace"].(string)
		}

		roleRef := binding["roleRef"].(map[string]interface{})
		roleName := roleRef["name"].(string)
		roleNs := ""
		if roleRef["namespace"] != nil {
			roleNs = roleRef["namespace"].(string)
		}

		if binding["subjects"] != nil {
			subjects := binding["subjects"].([]interface{})

			includeBinding := false
			if saName == "" {
				includeBinding = true
			} else {
				for _, subject := range subjects {
					s := subject.(map[string]interface{})
					if s["name"] == saName && s["namespace"] == saNamespace {
						includeBinding = true
						break
					}
				}
			}

			if includeBinding {
				subs := []KindNamespacedName{}
				for _, subject := range subjects {
					s := subject.(map[string]interface{})
					subs = append(subs, KindNamespacedName{
						kind: s["kind"].(string),
						NamespacedName: NamespacedName{
							namespace: stringOrEmpty(s["namespace"]),
							name:      s["name"].(string),
						},
					})
				}

				results = append(results, Binding{
					NamespacedName: NamespacedName{bindingNs, bindingName},
					role:           NamespacedName{roleNs, roleName},
					subjects:       subs,
				})
			}
		}
	}
	return results, nil
}

func stringOrEmpty(i interface{}) string {
	if i == nil {
		return ""
	}
	return i.(string)
}

// lookupResources lists resources referenced in a role.
// if namespace is empty then the scope is cluster-wide.
func (r *Rback) lookupResources(namespace, role string) (rules string, err error) {
	if namespace != "" { // look up in roles
		rules, err = findAccessRules(r.permissions.Roles[namespace], role)
		if err != nil {
			return "", err
		}
	}
	// ... otherwise, look up in cluster roles:
	clusterRules, err := findAccessRules(r.permissions.ClusterRoles, role)
	if err != nil {
		return "", err
	}
	return clusterRules + rules, nil
}

func findAccessRules(roles map[string]string, roleName string) (resources string, err error) {
	roleJson, found := roles[roleName]
	if found {
		var role map[string]interface{}
		b := []byte(roleJson)
		err = json.Unmarshal(b, &role)
		if err != nil {
			return "", err
		}
		rules := role["rules"].([]interface{})
		for _, rule := range rules {
			r := rule.(map[string]interface{})
			resources += toHumanReadableRule(r) + "\n"
		}
	}
	return resources, nil
}

func toHumanReadableRule(rule map[string]interface{}) string {
	line := toString(rule["verbs"])
	resourceKinds := toString(rule["resources"])
	if resourceKinds != "" {
		line += fmt.Sprintf(` %v`, resourceKinds)
	}
	resourceNames := toString(rule["resourceNames"])
	if resourceNames != "" {
		line += fmt.Sprintf(` "%v"`, resourceNames)
	}
	nonResourceURLs := toString(rule["nonResourceURLs"])
	if nonResourceURLs != "" {
		line += fmt.Sprintf(` %v`, nonResourceURLs)
	}
	apiGroups := toString(rule["apiGroups"])
	if apiGroups != "" {
		line += fmt.Sprintf(` (%v)`, apiGroups)
	}
	return line
}

func toString(values interface{}) string {
	if values == nil {
		return ""
	}
	var strs []string
	for _, v := range values.([]interface{}) {
		strs = append(strs, v.(string))
	}
	return strings.Join(strs, ",")
}

func (r *Rback) genGraph() *dot.Graph {
	g := dot.NewGraph(dot.Directed)
	g.Attr("newrank", "true") // global rank instead of per-subgraph (ensures access rules are always in the same place (at bottom))
	r.renderLegend(g)

	if r.config.resourceKind == "" || r.config.resourceKind == "serviceaccount" {
		for _, ns := range r.determineNamespacesToShow(r.permissions) {
			gns := r.newNamespaceSubgraph(g, ns)

			for sa, _ := range r.permissions.ServiceAccounts[ns] {
				renderSA := (r.config.resourceKind == "") ||
					((r.allNamespaces() || contains(r.config.namespaces, ns)) &&
						(r.allResourceNames() || contains(r.config.resourceNames, sa)))
				if renderSA {
					r.newSubjectNode(gns, "ServiceAccount", ns, sa)
				}
			}
		}
	}

	r.permissions.RoleBindings[""] = r.permissions.ClusterRoleBindings

	// roles:
	for _, roleBindings := range r.permissions.RoleBindings {
		bindings, err := r.lookupBindings(roleBindings, "", "")
		if err != nil {
			fmt.Printf("Can't look up roles due to: %v", err)
			os.Exit(-2)
		}
		for _, binding := range bindings {
			if !r.shouldRenderBinding(binding) {
				continue
			}

			gns := r.newNamespaceSubgraph(g, binding.namespace)

			bindingNode := r.newBindingNode(gns, binding)
			roleNode := r.newRoleAndRulesNodePair(gns, binding.namespace, binding.role)

			edge(bindingNode, roleNode)

			saNodes := []dot.Node{}
			for _, subject := range binding.subjects {
				if r.shouldIgnore(subject.name) {
					continue
				}
				renderSubject := (r.config.resourceKind != "serviceaccount") ||
					((r.allNamespaces() || contains(r.config.namespaces, subject.namespace)) &&
						(r.allResourceNames() || contains(r.config.resourceNames, subject.name)))

				if renderSubject {
					gns := r.newNamespaceSubgraph(g, subject.namespace)
					subjectNode := r.newSubjectNode(gns, subject.kind, subject.namespace, subject.name)
					saNodes = append(saNodes, subjectNode)
				}
			}

			for _, saNode := range saNodes {
				edge(saNode, bindingNode).Attr("dir", "back")
			}
		}
	}
	return g
}

func (r *Rback) shouldRenderBinding(binding Binding) bool {
	switch r.config.resourceKind {
	case "":
		return r.allNamespaces() || contains(r.config.namespaces, binding.namespace)
	case "rolebinding":
		return (r.allNamespaces() || contains(r.config.namespaces, binding.namespace)) && (r.allResourceNames() || contains(r.config.resourceNames, binding.name))
	case "clusterrolebinding":
		return binding.namespace == "" && (r.allResourceNames() || contains(r.config.resourceNames, binding.name))
	case "serviceaccount":
		for _, subject := range binding.subjects {
			if subject.kind == "ServiceAccount" &&
				(r.allNamespaces() || contains(r.config.namespaces, subject.namespace)) &&
				(r.allResourceNames() || contains(r.config.resourceNames, subject.name)) &&
				r.subjectExists("ServiceAccount", subject.namespace, subject.name) {
				return true
			}
		}
	case "role":
		return (r.allNamespaces() || contains(r.config.namespaces, binding.role.namespace)) && (r.allResourceNames() || contains(r.config.resourceNames, binding.role.name))
	case "clusterrole":
		return (binding.role.namespace == "" || r.allNamespaces() || contains(r.config.namespaces, binding.role.namespace)) && (r.allResourceNames() || contains(r.config.resourceNames, binding.role.name))
	}
	return false
}

func (r *Rback) allResourceNames() bool {
	return len(r.config.resourceNames) == 0
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

func (r *Rback) determineNamespacesToShow(p Permissions) (namespaces []string) {
	if r.allNamespaces() {
		type void struct{}
		var present void
		namespaceSet := make(map[string]void)
		for ns, _ := range p.ServiceAccounts {
			namespaceSet[ns] = present
		}
		for ns, _ := range p.RoleBindings {
			namespaceSet[ns] = present
		}
		for ns, _ := range p.Roles {
			namespaceSet[ns] = present
		}

		for ns, _ := range namespaceSet {
			namespaces = append(namespaces, ns)
		}
		return namespaces
	} else {
		return r.config.namespaces
	}
}

func (r *Rback) newBindingNode(gns *dot.Graph, binding Binding) dot.Node {
	isClusterRoleBinding := binding.namespace == ""
	if isClusterRoleBinding {
		return r.newClusterRoleBindingNode(gns, binding.name, r.isFocused("clusterrolebinding", "", binding.name))
	} else {
		return r.newRoleBindingNode(gns, binding.name, r.isFocused("rolebinding", binding.namespace, binding.name))
	}
}

func (r *Rback) newRoleAndRulesNodePair(gns *dot.Graph, bindingNamespace string, role NamespacedName) dot.Node {
	var roleNode dot.Node
	isClusterRole := role.namespace == ""
	if isClusterRole {
		roleNode = r.newClusterRoleNode(gns, bindingNamespace, role.name, r.clusterRoleExists(role), r.isFocused("clusterrole", role.namespace, role.name))
	} else {
		roleNode = r.newRoleNode(gns, role.namespace, role.name, r.roleExists(role), r.isFocused("role", role.namespace, role.name))
	}
	if r.config.showRules {
		rules, err := r.lookupResources(role.namespace, role.name)
		if err != nil {
			fmt.Printf("Can't look up entities and resources due to: %v", err)
			os.Exit(-3)
		}
		if rules != "" {
			rulesNode := newRulesNode(gns, role.namespace, role.name, rules)
			edge(roleNode, rulesNode)
		}
	}
	return roleNode
}

func (r *Rback) renderLegend(g *dot.Graph) {
	if !r.config.showLegend {
		return
	}

	legend := g.Subgraph("LEGEND", dot.ClusterOption{})

	namespace := legend.Subgraph("Namespace", dot.ClusterOption{})
	namespace.Attr("style", "dashed")

	sa := r.newSubjectNode0(namespace, "Kind", "Subject", true, false)
	missingSa := r.newSubjectNode0(namespace, "Kind", "Missing Subject", false, false)

	role := r.newRoleNode(namespace, "ns", "Role", true, false)
	clusterRoleBoundLocally := r.newClusterRoleNode(namespace, "ns", "ClusterRole", true, false) // bound by (namespaced!) RoleBinding
	clusterrole := r.newClusterRoleNode(legend, "", "ClusterRole", true, false)

	roleBinding := r.newRoleBindingNode(namespace, "RoleBinding", false)
	edge(sa, roleBinding).Attr("dir", "back")
	edge(missingSa, roleBinding).Attr("dir", "back")
	edge(roleBinding, role)

	roleBinding2 := r.newRoleBindingNode(namespace, "RoleBinding-to-ClusterRole", false)
	roleBinding2.Attr("label", "RoleBinding")
	edge(sa, roleBinding2).Attr("dir", "back")
	edge(roleBinding2, clusterRoleBoundLocally)

	clusterRoleBinding := r.newClusterRoleBindingNode(legend, "ClusterRoleBinding", false)
	edge(sa, clusterRoleBinding).Attr("dir", "back")
	edge(clusterRoleBinding, clusterrole)

	if r.config.showRules {
		nsrules := newRulesNode(namespace, "ns", "Role", "Namespace-scoped\naccess rules")
		edge(role, nsrules)

		nsrules2 := newRulesNode(namespace, "ns", "ClusterRole", "Namespace-scoped access rules From ClusterRole")
		nsrules2.Attr("label", "Namespace-scoped\naccess rules")
		edge(clusterRoleBoundLocally, nsrules2)

		clusterrules := newRulesNode(legend, "", "ClusterRole", "Cluster-scoped\naccess rules")
		edge(clusterrole, clusterrules)
	}
}

// struct2json turns a map into a JSON string
func struct2json(s map[string]interface{}) (string, error) {
	str, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(str), nil
}

func (r *Rback) newNamespaceSubgraph(g *dot.Graph, ns string) *dot.Graph {
	if ns == "" {
		return g
	}
	gns := g.Subgraph(ns, dot.ClusterOption{})
	gns.Attr("style", "dashed")
	return gns
}

func (r *Rback) newSubjectNode(gns *dot.Graph, kind string, ns string, name string) dot.Node {
	return r.newSubjectNode0(gns, kind, name, r.subjectExists(kind, ns, name), r.isFocused(strings.ToLower(kind), ns, name))
}

func (r *Rback) newSubjectNode0(g *dot.Graph, kind, name string, exists, highlight bool) dot.Node {
	return g.Node(kind+"-"+name).
		Box().
		Attr("label", fmt.Sprintf("%s\n(%s)", name, kind)).
		Attr("style", iff(exists, "filled", "dotted")).
		Attr("color", iff(exists, "black", "red")).
		Attr("penwidth", iff(highlight || !exists, "2.0", "1.0")).
		Attr("fillcolor", "#2f6de1").
		Attr("fontcolor", iff(exists, "#f0f0f0", "#030303"))
}

func (r *Rback) newRoleBindingNode(g *dot.Graph, name string, highlight bool) dot.Node {
	return g.Node("rb-"+name).
		Attr("label", name).
		Attr("shape", "octagon").
		Attr("style", "filled").
		Attr("penwidth", iff(highlight, "2.0", "1.0")).
		Attr("fillcolor", "#ffcc00").
		Attr("fontcolor", "#030303")
}

func (r *Rback) newClusterRoleBindingNode(g *dot.Graph, name string, highlight bool) dot.Node {
	return g.Node("crb-"+name).
		Attr("label", name).
		Attr("shape", "doubleoctagon").
		Attr("style", "filled").
		Attr("penwidth", iff(highlight, "2.0", "1.0")).
		Attr("fillcolor", "#ffcc00").
		Attr("fontcolor", "#030303")
}

func (r *Rback) newRoleNode(g *dot.Graph, namespace, name string, exists, highlight bool) dot.Node {
	return g.Node("r-"+namespace+"/"+name).
		Attr("label", name).
		Attr("shape", "octagon").
		Attr("style", iff(exists, "filled", "dotted")).
		Attr("color", iff(exists, "black", "red")).
		Attr("penwidth", iff(highlight || !exists, "2.0", "1.0")).
		Attr("fillcolor", "#ff9900").
		Attr("fontcolor", "#030303")
}

func (r *Rback) newClusterRoleNode(g *dot.Graph, bindingNamespace, roleName string, exists, highlight bool) dot.Node {
	return g.Node("cr-"+bindingNamespace+"/"+roleName).
		Attr("label", roleName).
		Attr("shape", "doubleoctagon").
		Attr("style", iff(exists, iff(bindingNamespace == "", "filled", "filled,dashed"), "dotted")).
		Attr("color", iff(exists, "black", "red")).
		Attr("penwidth", iff(highlight || !exists, "2.0", "1.0")).
		Attr("fillcolor", "#ff9900").
		Attr("fontcolor", "#030303")
}

func (r *Rback) isFocused(kind string, ns string, name string) bool {
	return r.config.resourceKind == kind &&
		(r.allNamespaces() || contains(r.config.namespaces, ns)) &&
		contains(r.config.resourceNames, name)
}

func (r *Rback) subjectExists(kind string, ns string, name string) bool {
	if strings.ToLower(kind) != "serviceaccount" {
		return true // assume users and groups exist
	}

	if sas, nsExists := r.permissions.ServiceAccounts[ns]; nsExists {
		if _, saExists := sas[name]; saExists {
			return true
		}
	}
	return false
}

func (r *Rback) roleExists(role NamespacedName) bool {
	if roles, nsExists := r.permissions.Roles[role.namespace]; nsExists {
		if _, roleExists := roles[role.name]; roleExists {
			return true
		}
	}
	return false
}

func (r *Rback) clusterRoleExists(role NamespacedName) bool {
	if _, roleExists := r.permissions.ClusterRoles[role.name]; roleExists {
		return true
	}
	return false
}

func newRulesNode(g *dot.Graph, namespace, roleName, rules string) dot.Node {
	rules = strings.ReplaceAll(rules, `\`, `\\`)
	rules = strings.ReplaceAll(rules, "\n", `\l`) // left-justify text
	rules = strings.ReplaceAll(rules, `"`, `\"`)  // using Literal, so we need to escape quotes
	return g.Node("rules-"+namespace+"/"+roleName).
		Attr("label", dot.Literal(`"`+rules+`"`)).
		Attr("shape", "note")
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

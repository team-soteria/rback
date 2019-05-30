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
	config Config
}

type Config struct {
	renderRules     bool
	showLegend      bool
	namespaces      []string
	ignoredPrefixes []string
	resourceKind    string
	resourceNames   []string
}

type Permissions struct {
	ServiceAccounts     map[string][]string
	Roles               map[string][]string
	ClusterRoles        []string
	RoleBindings        map[string][]string
	ClusterRoleBindings []string
}

func main() {

	config := Config{}
	flag.BoolVar(&config.showLegend, "show-legend", true, "Whether to show the legend or not")
	flag.BoolVar(&config.renderRules, "render-rules", true, "Whether to render RBAC rules (e.g. \"get pods\") or not")

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

	p, err := rback.getPermissions()
	if err != nil {
		fmt.Printf("Can't query permissions due to :%v", err)
		os.Exit(-1)
	}
	g := rback.genGraph(p)
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

func item2Name(name, namespace string, item map[string]interface{}) string {
	return name
}

// getServiceAccounts retrieves data about service accounts across all namespaces
func (r *Rback) getServiceAccounts(namespaces, names []string) (result map[string][]string, err error) {
	return r.getNamespacedResources("sa", namespaces, names, item2Name)
}

func getNamespacedName(obj map[string]interface{}) NamespacedName {
	metadata := obj["metadata"].(map[string]interface{})
	ns := metadata["namespace"]
	name := metadata["name"]
	return NamespacedName{ns.(string), name.(string)}
}

func item2json(name, namespace string, item map[string]interface{}) string {
	itemJson, _ := struct2json(item)
	return itemJson
}

// getRoles retrieves data about roles across all namespaces
func (r *Rback) getRoles() (result map[string][]string, err error) {
	return r.getNamespacedResources("roles", []string{""}, []string{}, item2json)
}

// getRoleBindings retrieves data about roles across all namespaces
func (r *Rback) getRoleBindings(namespaces, names []string) (result map[string][]string, err error) {
	return r.getNamespacedResources("rolebindings", namespaces, names, item2json)
}

func (r *Rback) getNamespacedResources(kind string, namespaces, names []string, mapFunc func(name, namespace string, item map[string]interface{}) string) (result map[string][]string, err error) {
	result = make(map[string][]string)
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
					result[nn.namespace] = append(result[nn.namespace], mapFunc(nn.name, nn.namespace, item))
				}
			}
		} else {
			nn := getNamespacedName(d)
			result[nn.namespace] = append(result[nn.namespace], mapFunc(nn.name, nn.namespace, d))
		}
	}
	return result, nil
}

// getClusterRoles retrieves data about cluster roles
func (r *Rback) getClusterRoles() (result []string, err error) {
	return r.getClusterScopedResources("clusterroles")
}

// getClusterRoleBindings retrieves data about cluster role bindings
func (r *Rback) getClusterRoleBindings() (result []string, err error) {
	return r.getClusterScopedResources("clusterrolebindings")

}

func (r *Rback) getClusterScopedResources(kind string) (result []string, err error) {
	result = []string{}
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
		name := metadata["name"]
		if !r.shouldIgnore(name.(string)) {
			itemJson, _ := struct2json(item)
			result = append(result, itemJson)
		}
	}
	return result, nil
}

// getPermissions retrieves data about all access control related data
// from service accounts to roles and bindings, both namespaced and the
// cluster level.
func (r *Rback) getPermissions() (Permissions, error) {
	p := Permissions{}
	saNames := []string{}
	if r.config.resourceKind == "serviceaccount" {
		saNames = r.config.resourceNames
	}
	sa, err := r.getServiceAccounts(r.config.namespaces, saNames)
	if err != nil {
		return p, err
	}
	p.ServiceAccounts = sa

	roles, err := r.getRoles()
	if err != nil {
		return p, err
	}
	p.Roles = roles

	rb, err := r.getRoleBindings([]string{""}, []string{})
	if err != nil {
		return p, err
	}
	p.RoleBindings = rb

	cr, err := r.getClusterRoles()
	if err != nil {
		return p, err
	}
	p.ClusterRoles = cr

	crb, err := r.getClusterRoleBindings()
	if err != nil {
		return p, err
	}
	p.ClusterRoleBindings = crb
	return p, nil
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
func (r *Rback) lookupBindings(bindings []string, saName, saNamespace string) (results []Binding, err error) {
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
func (r *Rback) lookupResources(namespace, role string, p Permissions) (rules string, err error) {
	if namespace != "" { // look up in roles
		rules, err = findAccessRules(p.Roles[namespace], role)
		if err != nil {
			return "", err
		}
	}
	// ... otherwise, look up in cluster roles:
	clusterRules, err := findAccessRules(p.ClusterRoles, role)
	if err != nil {
		return "", err
	}
	return clusterRules + rules, nil
}

func findAccessRules(roles []string, roleName string) (resources string, err error) {
	for _, roleJson := range roles {
		var role map[string]interface{}
		b := []byte(roleJson)
		err = json.Unmarshal(b, &role)
		if err != nil {
			return "", err
		}
		metadata := role["metadata"].(map[string]interface{})
		name := metadata["name"]
		if name == roleName {
			rules := role["rules"].([]interface{})
			for _, rule := range rules {
				r := rule.(map[string]interface{})
				resources += toHumanReadableRule(r) + "\n"
			}
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

func (r *Rback) genGraph(p Permissions) *dot.Graph {
	g := dot.NewGraph(dot.Directed)
	g.Attr("newrank", "true") // global rank instead of per-subgraph (ensures access rules are always in the same place (at bottom))
	r.renderLegend(g)

	subjectNodes := map[KindNamespacedName]dot.Node{}
	nsSubgraphs := map[string]*dot.Graph{}
	nsSubgraphs[""] = g

	allNamespaces := len(r.config.namespaces) == 1 && r.config.namespaces[0] == ""
	allResourceNames := len(r.config.resourceNames) == 0

	if r.config.resourceKind == "" || r.config.resourceKind == "serviceaccount" {
		for _, ns := range r.determineNamespacesToShow(p) {
			gns := r.existingOrNewNamespaceSubgraph(g, nsSubgraphs, ns)

			for _, sa := range p.ServiceAccounts[ns] {
				r.existingOrNewSubjectNode(gns, subjectNodes, "ServiceAccount", ns, sa)
			}
		}
	}

	allBindings := [][]string{
		p.ClusterRoleBindings,
	}
	for _, roleBindings := range p.RoleBindings {
		allBindings = append(allBindings, roleBindings)
	}

	// roles:
	for _, roleBindings := range allBindings {
		bindings, err := r.lookupBindings(roleBindings, "", "")
		if err != nil {
			fmt.Printf("Can't look up roles due to: %v", err)
			os.Exit(-2)
		}
		for _, binding := range bindings {
			renderBinding := false
			if r.config.resourceKind == "" {
				renderBinding = allNamespaces || contains(r.config.namespaces, binding.namespace)
			} else if r.config.resourceKind == "rolebinding" {
				renderBinding = (allNamespaces || contains(r.config.namespaces, binding.namespace)) && (allResourceNames || contains(r.config.resourceNames, binding.name))
			} else if r.config.resourceKind == "clusterrolebinding" {
				renderBinding = binding.namespace == "" && (allResourceNames || contains(r.config.resourceNames, binding.name))
			} else if r.config.resourceKind == "serviceaccount" {
				for _, subject := range binding.subjects {
					if subject.kind == "ServiceAccount" && (allNamespaces || contains(r.config.namespaces, subject.namespace)) && (allResourceNames || contains(r.config.resourceNames, subject.name)) {
						renderBinding = true
						break
					}
				}
			} else if r.config.resourceKind == "role" {
				renderBinding = (allNamespaces || contains(r.config.namespaces, binding.role.namespace)) && (allResourceNames || contains(r.config.resourceNames, binding.role.name))
			}

			if !renderBinding {
				continue
			}

			gns := r.existingOrNewNamespaceSubgraph(g, nsSubgraphs, binding.namespace)
			bindingNode := r.renderBindingAndRole(gns, binding.NamespacedName, binding.role, p)

			saNodes := []dot.Node{}
			for _, subject := range binding.subjects {
				if !r.shouldIgnore(subject.name) {
					renderSubject := false
					if r.config.resourceKind == "" {
						renderSubject = true
					} else if r.config.resourceKind == "rolebinding" {
						renderSubject = true
					} else if r.config.resourceKind == "clusterrolebinding" {
						renderSubject = true
					} else if r.config.resourceKind == "serviceaccount" {
						renderSubject = (allNamespaces || contains(r.config.namespaces, subject.namespace)) && (allResourceNames || contains(r.config.resourceNames, subject.name))
					} else if r.config.resourceKind == "role" {
						renderSubject = true
					}

					if renderSubject {
						gns := r.existingOrNewNamespaceSubgraph(g, nsSubgraphs, subject.namespace)
						subjectNode := r.existingOrNewSubjectNode(gns, subjectNodes, subject.kind, subject.namespace, subject.name)
						saNodes = append(saNodes, subjectNode)
					}
				}
			}

			for _, saNode := range saNodes {
				saNode.Edge(bindingNode).Attr("dir", "back")
			}
		}
	}
	return g
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
	if len(r.config.namespaces) == 1 && r.config.namespaces[0] == "" {
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

func (r *Rback) existingOrNewSubjectNode(gns *dot.Graph, subjectNodes map[KindNamespacedName]dot.Node, kind string, ns string, name string) dot.Node {
	knn := KindNamespacedName{kind, NamespacedName{ns, name}}
	node, found := subjectNodes[knn]
	if !found {
		node = newSubjectNode(gns, kind, name)
		subjectNodes[knn] = node
	}
	return node
}

func (r *Rback) existingOrNewNamespaceSubgraph(g *dot.Graph, nsSubgraphs map[string]*dot.Graph, ns string) *dot.Graph {
	gns := nsSubgraphs[ns]
	if gns == nil {
		gns = r.newNamespaceSubgraph(g, ns)
		nsSubgraphs[ns] = gns
	}
	return gns
}

func (r *Rback) renderLegend(g *dot.Graph) {
	if !r.config.showLegend {
		return
	}

	legend := g.Subgraph("LEGEND", dot.ClusterOption{})

	namespace := legend.Subgraph("Namespace", dot.ClusterOption{})
	namespace.Attr("style", "dashed")

	sa := newSubjectNode(namespace, "Kind", "Subject")

	role := newRoleNode(namespace, "ns", "Role")
	clusterRoleBoundLocally := newClusterRoleNode(namespace, "ns", "ClusterRole") // bound by (namespaced!) RoleBinding
	clusterrole := newClusterRoleNode(legend, "", "ClusterRole")

	roleBinding := newRoleBindingNode(namespace, "RoleBinding")
	sa.Edge(roleBinding).Attr("dir", "back")
	roleBinding.Edge(role)

	roleBinding2 := newRoleBindingNode(namespace, "RoleBinding-to-ClusterRole")
	roleBinding2.Attr("label", "RoleBinding")
	sa.Edge(roleBinding2).Attr("dir", "back")
	roleBinding2.Edge(clusterRoleBoundLocally)

	clusterRoleBinding := newClusterRoleBindingNode(legend, "ClusterRoleBinding")
	sa.Edge(clusterRoleBinding).Attr("dir", "back")
	clusterRoleBinding.Edge(clusterrole)

	if r.config.renderRules {
		nsrules := newRulesNode(namespace, "ns", "Role", "Namespace-scoped\naccess rules")
		legend.Edge(role, nsrules)

		nsrules2 := newRulesNode(namespace, "ns", "ClusterRole", "Namespace-scoped access rules From ClusterRole")
		nsrules2.Attr("label", "Namespace-scoped\naccess rules")
		legend.Edge(clusterRoleBoundLocally, nsrules2)

		clusterrules := newRulesNode(legend, "", "ClusterRole", "Cluster-scoped\naccess rules")
		legend.Edge(clusterrole, clusterrules)
	}
}

func (r *Rback) renderBindingAndRole(g *dot.Graph, binding, role NamespacedName, p Permissions) dot.Node {
	var roleNode dot.Node

	isClusterRole := role.namespace == ""
	if isClusterRole {
		roleNode = newClusterRoleNode(g, binding.namespace, role.name)
	} else {
		roleNode = newRoleNode(g, binding.namespace, role.name)
	}

	var roleBindingNode dot.Node
	isClusterRoleBinding := binding.namespace == ""
	if isClusterRoleBinding {
		roleBindingNode = newClusterRoleBindingNode(g, binding.name)
	} else {
		roleBindingNode = newRoleBindingNode(g, binding.name)
	}
	roleBindingNode.Edge(roleNode)

	if r.config.renderRules {
		rules, err := r.lookupResources(binding.namespace, role.name, p)
		if err != nil {
			fmt.Printf("Can't look up entities and resources due to: %v", err)
			os.Exit(-3)
		}
		if rules != "" {
			resnode := newRulesNode(g, binding.namespace, role.name, rules)
			g.Edge(roleNode, resnode)
		}
	}

	return roleBindingNode
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
	gns := g.Subgraph(ns, dot.ClusterOption{})
	gns.Attr("style", "dashed")
	return gns
}

func newSubjectNode(g *dot.Graph, kind, name string) dot.Node {
	return g.Node(kind+"-"+name).
		Box().
		Attr("label", fmt.Sprintf("%s\n(%s)", name, kind)).
		Attr("style", "filled").
		Attr("fillcolor", "#2f6de1").
		Attr("fontcolor", "#f0f0f0")
}

func newRoleBindingNode(g *dot.Graph, name string) dot.Node {
	return g.Node("rb-"+name).
		Attr("label", name).
		Attr("shape", "octagon").
		Attr("style", "filled").
		Attr("fillcolor", "#ffcc00").
		Attr("fontcolor", "#030303")
}

func newClusterRoleBindingNode(g *dot.Graph, name string) dot.Node {
	return g.Node("crb-"+name).
		Attr("label", name).
		Attr("shape", "doubleoctagon").
		Attr("style", "filled").
		Attr("fillcolor", "#ffcc00").
		Attr("fontcolor", "#030303")
}

func newRoleNode(g *dot.Graph, namespace, name string) dot.Node {
	return g.Node("r-"+namespace+"/"+name).
		Attr("label", name).
		Attr("shape", "octagon").
		Attr("style", "filled").
		Attr("fillcolor", "#ff9900").
		Attr("fontcolor", "#030303")
}

func newClusterRoleNode(g *dot.Graph, namespace, name string) dot.Node {
	return g.Node("cr-"+namespace+"/"+name).
		Attr("label", name).
		Attr("shape", "doubleoctagon").
		Attr("style", "filled").
		Attr("fillcolor", "#ff9900").
		Attr("fontcolor", "#030303")
}

func newRulesNode(g *dot.Graph, namespace, roleName, rules string) dot.Node {
	rules = strings.ReplaceAll(rules, `\`, `\\`)
	rules = strings.ReplaceAll(rules, "\n", `\l`) // left-justify text
	rules = strings.ReplaceAll(rules, `"`, `\"`)  // using Literal, so we need to escape quotes
	return g.Node("rules-"+namespace+"/"+roleName).
		Attr("label", dot.Literal(`"`+rules+`"`)).
		Attr("shape", "note")
}

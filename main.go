package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/emicklei/dot"
)

type Rback struct {
	config      Config
	permissions Permissions
}

type Config struct {
	inputFile       string
	showRules       bool
	showLegend      bool
	namespaces      []string
	ignoredPrefixes []string
	resourceKind    string
	resourceNames   []string
	whoCan          WhoCan
}

type WhoCan struct {
	verb, resourceKind, resourceName string
	showMatchedOnly                  bool
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
		(w.resourceName == "" || contains(rule.resourceNames, w.resourceName)) // TODO: also check API group!
}

type Permissions struct {
	ServiceAccounts map[string]map[string]string // map[namespace]map[name]json
	Roles           map[string]map[string]Role   // ClusterRoles are stored in Roles[""]
	RoleBindings    map[string]map[string]string // ClusterRoleBindings are stored in RoleBindings[""]
}

func main() {

	config := Config{}
	flag.StringVar(&config.inputFile, "f", "", "The name of the file to use as input (otherwise stdin is used)")
	flag.BoolVar(&config.showLegend, "show-legend", true, "Whether to show the legend or not")
	flag.BoolVar(&config.showRules, "show-rules", true, "Whether to render RBAC access rules (e.g. \"get pods\") or not")
	flag.BoolVar(&config.whoCan.showMatchedOnly, "show-matched-rules-only", false, "When running who-can, only show the matched rule instead of all rules specified in the role")

	var namespaces string
	flag.StringVar(&namespaces, "n", "", "The namespace to render (also supports multiple, comma-delimited namespaces)")

	var ignoredPrefixes string
	flag.StringVar(&ignoredPrefixes, "ignore-prefixes", "system:", "Comma-delimited list of (Cluster)Role(Binding) prefixes to ignore ('none' to not ignore anything)")
	flag.Parse()

	if flag.NArg() > 0 {
		if flag.Arg(0) == "who-can" {
			if flag.NArg() < 3 {
				fmt.Println("Usage: rback who-can VERB RESOURCE [NAME]")
				os.Exit(-4)
			}
			config.resourceKind = "rule"
			config.whoCan.verb = flag.Arg(1)
			config.whoCan.resourceKind = flag.Arg(2)
			if flag.NArg() > 3 {
				config.whoCan.resourceName = flag.Arg(3)
			}
		} else {
			config.resourceKind = normalizeKind(flag.Arg(0))
			if flag.NArg() > 1 {
				config.resourceNames = flag.Args()[1:]
			}
		}
	}

	config.namespaces = strings.Split(namespaces, ",")

	if ignoredPrefixes != "none" {
		config.ignoredPrefixes = strings.Split(ignoredPrefixes, ",")
	}

	rback := Rback{config: config}

	var err error
	reader := os.Stdin
	if config.inputFile != "" {
		reader, err = os.Open(config.inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't open file %s: %v\n", config.inputFile, err)
			os.Exit(-1)
		}
	}

	err = rback.parseRBAC(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't parse RBAC resources from stdin: %v\n", err)
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

func getNamespacedName(obj map[string]interface{}) NamespacedName {
	metadata := obj["metadata"].(map[string]interface{})
	ns := ""
	rawNs := metadata["namespace"]
	if rawNs != nil {
		ns = rawNs.(string)
	}

	name := metadata["name"]
	return NamespacedName{ns, name.(string)}
}

func toRole(rawRole map[string]interface{}) Role {
	rules := []Rule{}
	rawRules := rawRole["rules"].([]interface{})
	for _, r := range rawRules {
		rules = append(rules, toRule(r))
	}

	return Role{
		getNamespacedName(rawRole),
		rules,
	}
}

// parseRBAC parses RBAC resources from the given reader and stores them in maps under r.permissions
func (r *Rback) parseRBAC(reader io.Reader) (err error) {
	var input map[string]interface{}

	decoder := json.NewDecoder(reader)
	err = decoder.Decode(&input)
	if err != nil {
		return err
	}

	if input["kind"] != "List" {
		return fmt.Errorf("Expected kind=List, but found %v", input["kind"])
	}

	r.permissions.ServiceAccounts = make(map[string]map[string]string)
	r.permissions.Roles = make(map[string]map[string]Role)
	r.permissions.RoleBindings = make(map[string]map[string]string)

	items := input["items"].([]interface{})
	for _, i := range items {
		item := i.(map[string]interface{})
		nn := getNamespacedName(item)

		if r.shouldIgnore(nn.name) {
			continue
		}

		kind := item["kind"].(string)

		switch kind {
		case "ServiceAccount":
			if r.permissions.ServiceAccounts[nn.namespace] == nil {
				r.permissions.ServiceAccounts[nn.namespace] = make(map[string]string)
			}
			json, _ := struct2json(item)
			r.permissions.ServiceAccounts[nn.namespace][nn.name] = json
		case "RoleBinding", "ClusterRoleBinding":
			if r.permissions.RoleBindings[nn.namespace] == nil {
				r.permissions.RoleBindings[nn.namespace] = make(map[string]string)
			}
			json, _ := struct2json(item)
			r.permissions.RoleBindings[nn.namespace][nn.name] = json
		case "Role", "ClusterRole":
			if r.permissions.Roles[nn.namespace] == nil {
				r.permissions.Roles[nn.namespace] = make(map[string]Role)
			}
			r.permissions.Roles[nn.namespace][nn.name] = toRole(item)
		default:
			log.Printf("Ignoring resource kind %s", kind)
		}
	}
	return nil
}

type Binding struct {
	NamespacedName
	role     NamespacedName
	subjects []KindNamespacedName
}

type Role struct {
	NamespacedName
	rules []Rule
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

func toRule(rule interface{}) Rule {
	r := rule.(map[string]interface{})
	return Rule{
		verbs:           toStringArray(r["verbs"]),
		resources:       toStringArray(r["resources"]),
		resourceNames:   toStringArray(r["resourceNames"]),
		nonResourceURLs: toStringArray(r["nonResourceURLs"]),
		apiGroups:       toStringArray(r["apiGroups"]),
	}
}

type Rule struct {
	verbs           []string
	resources       []string
	resourceNames   []string
	nonResourceURLs []string
	apiGroups       []string
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

func toStringArray(values interface{}) []string {
	if values == nil {
		return []string{}
	}
	var strs []string
	for _, v := range values.([]interface{}) {
		strs = append(strs, v.(string))
	}
	return strs
}

func (r *Rback) genGraph() *dot.Graph {
	g := dot.NewGraph(dot.Directed)
	g.Attr("newrank", "true") // global rank instead of per-subgraph (ensures access rules are always in the same place (at bottom))
	r.renderLegend(g)

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
					(r.namespaceSelected(subject.namespace) && r.resourceNameSelected(subject.name))

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

	// draw any additional ServiceAccounts that weren't referenced by bindings (and thus drawn in the code above)
	if r.config.resourceKind == "" || r.config.resourceKind == "serviceaccount" {
		for ns, sas := range r.permissions.ServiceAccounts {
			if !r.namespaceSelected(ns) {
				continue
			}
			gns := r.newNamespaceSubgraph(g, ns)

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
			renderRoles = (r.config.resourceKind == "" || r.config.resourceKind == "clusterrole") && r.allNamespaces()
		} else {
			renderRoles = (r.config.resourceKind == "" || r.config.resourceKind == "role") && r.namespaceSelected(ns)
		}

		if !renderRoles {
			continue
		}

		gns := r.newNamespaceSubgraph(g, ns)
		for roleName, _ := range roles {
			renderRole := r.namespaceSelected(ns) && r.resourceNameSelected(roleName)
			if renderRole {
				r.newRoleAndRulesNodePair(gns, "", NamespacedName{ns, roleName})
			}
		}
	}

	return g
}

func (r *Rback) shouldRenderBinding(binding Binding) bool {
	switch r.config.resourceKind {
	case "":
		return r.namespaceSelected(binding.namespace)
	case "rolebinding":
		return r.namespaceSelected(binding.namespace) && r.resourceNameSelected(binding.name)
	case "clusterrolebinding":
		return binding.namespace == "" && r.resourceNameSelected(binding.name)
	case "serviceaccount":
		for _, subject := range binding.subjects {
			if subject.kind == "ServiceAccount" &&
				r.namespaceSelected(subject.namespace) &&
				r.resourceNameSelected(subject.name) &&
				r.subjectExists("ServiceAccount", subject.namespace, subject.name) {
				return true
			}
		}
	case "role":
		bindingPointsToClusterRole := binding.role.namespace == ""
		return !bindingPointsToClusterRole &&
			r.namespaceSelected(binding.role.namespace) &&
			r.resourceNameSelected(binding.role.name) &&
			r.roleExists(binding.role)
	case "clusterrole":
		bindingPointsToClusterRole := binding.role.namespace == ""
		return bindingPointsToClusterRole &&
			r.resourceNameSelected(binding.role.name) &&
			r.roleExists(binding.role)
	case "rule":
		bindingPointsToClusterRole := binding.role.namespace == ""
		return r.ruleMatchesSelection(binding.role) && (bindingPointsToClusterRole || r.namespaceSelected(binding.role.namespace))
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
		roleNode = r.newClusterRoleNode(gns, bindingNamespace, role.name, r.roleExists(role), r.isFocused("clusterrole", role.namespace, role.name))
	} else {
		roleNode = r.newRoleNode(gns, role.namespace, role.name, r.roleExists(role), r.isFocused("role", role.namespace, role.name))
	}
	if r.config.showRules {
		rulesNode := r.newRulesNode(gns, role.namespace, role.name, r.isFocused("rule", role.namespace, role.name))
		if rulesNode != nil {
			edge(roleNode, *rulesNode)
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
		nsrules := r.newRulesNode0(namespace, "ns", "Role", "Namespace-scoped\naccess rules", false)
		edge(role, nsrules)

		nsrules2 := r.newRulesNode0(namespace, "ns", "ClusterRole", "Namespace-scoped access rules From ClusterRole", false)
		nsrules2.Attr("label", "Namespace-scoped\naccess rules")
		edge(clusterRoleBoundLocally, nsrules2)

		clusterrules := r.newRulesNode0(legend, "", "ClusterRole", "Cluster-scoped\naccess rules", false)
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
		Attr("label", formatLabel(fmt.Sprintf("%s\n(%s)", name, kind), highlight)).
		Attr("style", iff(exists, "filled", "dotted")).
		Attr("color", iff(exists, "black", "red")).
		Attr("penwidth", iff(highlight || !exists, "2.0", "1.0")).
		Attr("fillcolor", "#2f6de1").
		Attr("fontcolor", iff(exists, "#f0f0f0", "#030303"))
}

func (r *Rback) newRoleBindingNode(g *dot.Graph, name string, highlight bool) dot.Node {
	return g.Node("rb-"+name).
		Attr("label", formatLabel(name, highlight)).
		Attr("shape", "octagon").
		Attr("style", "filled").
		Attr("penwidth", iff(highlight, "2.0", "1.0")).
		Attr("fillcolor", "#ffcc00").
		Attr("fontcolor", "#030303")
}

func (r *Rback) newClusterRoleBindingNode(g *dot.Graph, name string, highlight bool) dot.Node {
	return g.Node("crb-"+name).
		Attr("label", formatLabel(name, highlight)).
		Attr("shape", "doubleoctagon").
		Attr("style", "filled").
		Attr("penwidth", iff(highlight, "2.0", "1.0")).
		Attr("fillcolor", "#ffcc00").
		Attr("fontcolor", "#030303")
}

func (r *Rback) newRoleNode(g *dot.Graph, namespace, name string, exists, highlight bool) dot.Node {
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

func (r *Rback) newClusterRoleNode(g *dot.Graph, bindingNamespace, roleName string, exists, highlight bool) dot.Node {
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

func formatLabel(label string, highlight bool) interface{} {
	if highlight {
		return dot.HTML("<b>" + escapeHTML(label) + "</b>")
	} else {
		return label
	}
}

func (r *Rback) isFocused(kind string, ns string, name string) bool {
	if kind == "rule" {
		return r.ruleMatchesSelection(NamespacedName{ns, name})
	} else {
		return r.config.resourceKind == kind && r.namespaceSelected(ns) && r.resourceNameSelected(name)
	}
}

func (r *Rback) resourceNameSelected(name string) bool {
	return r.allResourceNames() || contains(r.config.resourceNames, name)
}

func (r *Rback) namespaceSelected(ns string) bool {
	return r.allNamespaces() || contains(r.config.namespaces, ns)
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

func (r *Rback) ruleMatchesSelection(roleRef NamespacedName) bool {
	if r.config.resourceKind == "rule" {
		if roles, found := r.permissions.Roles[roleRef.namespace]; found {
			if role, found := roles[roleRef.name]; found {
				return r.config.whoCan.matchesAnyRuleIn(role)
			}
		}
	}
	return false
}

func (r *Rback) newRulesNode(g *dot.Graph, namespace, roleName string, highlight bool) *dot.Node {
	var rules string
	if roles, found := r.permissions.Roles[namespace]; found {
		if role, found := roles[roleName]; found {
			for _, rule := range role.rules {
				ruleMatches := r.config.resourceKind == "rule" && highlight && r.config.whoCan.matches(rule)
				if ruleMatches {
					rules += "<b>" + escapeHTML(rule.toHumanReadableString()) + "</b>" + `<br align="left"/>`
				} else {
					if r.config.whoCan.showMatchedOnly {
						if !strings.HasSuffix(rules, `...<br align="left"/>`) {
							rules += `...<br align="left"/>`
						}
					} else {
						rules += escapeHTML(rule.toHumanReadableString()) + `<br align="left"/>`
					}
				}
			}
		}
	}
	if rules == "" {
		return nil
	} else {
		node := r.newRulesNode0(g, namespace, roleName, rules, highlight)
		return &node
	}
}

func escapeHTML(str string) string {
	str = strings.ReplaceAll(str, `<`, `&lt;`)
	str = strings.ReplaceAll(str, `>`, `&gt;`)
	str = strings.ReplaceAll(str, ` `, `&nbsp;`)
	str = strings.ReplaceAll(str, "\n", `<br/>`)
	return str
}

func (r *Rback) newRulesNode0(g *dot.Graph, namespace, roleName, rulesHTML string, highlight bool) dot.Node {
	return g.Node("rules-"+namespace+"/"+roleName).
		Attr("label", dot.HTML(rulesHTML)).
		Attr("shape", "note").
		Attr("penwidth", iff(highlight, "2.0", "1.0"))
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

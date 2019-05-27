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
	renderRules bool
	namespace   string
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
	flag.BoolVar(&config.renderRules, "render-rules", true, "Whether to render RBAC rules (e.g. \"get pods\") or not")
	flag.StringVar(&config.namespace, "n", "", "The namespace to render")
	flag.Parse()

	rback := Rback{config: config}

	p, err := rback.getPermissions()
	if err != nil {
		fmt.Printf("Can't query permissions due to :%v", err)
		os.Exit(-1)
	}
	g := rback.genGraph(p)
	fmt.Println(g.String())
}

// getServiceAccounts retrieves data about service accounts across all namespaces
func (r *Rback) getServiceAccounts(namespace string) (serviceAccounts map[string][]string, err error) {
	serviceAccounts = make(map[string][]string)
	var res string
	if namespace == "" {
		res, err = kubecuddler.Kubectl(true, true, "", "get", "sa", "--all-namespaces", "--output", "json")
	} else {
		res, err = kubecuddler.Kubectl(true, true, "", "get", "sa", "-n", namespace, "--output", "json")
	}
	if err != nil {
		return serviceAccounts, err
	}
	var d map[string]interface{}
	b := []byte(res)
	err = json.Unmarshal(b, &d)
	if err != nil {
		return serviceAccounts, err
	}
	saitems := d["items"].([]interface{})
	for _, sa := range saitems {
		serviceaccount := sa.(map[string]interface{})
		metadata := serviceaccount["metadata"].(map[string]interface{})
		ns := metadata["namespace"]
		name := metadata["name"]
		serviceAccounts[ns.(string)] = append(serviceAccounts[ns.(string)], name.(string))
	}
	return serviceAccounts, nil
}

// getRoles retrieves data about roles across all namespaces
func (r *Rback) getRoles() (roles map[string][]string, err error) {
	roles = make(map[string][]string)
	res, err := kubecuddler.Kubectl(true, true, "", "get", "roles", "--all-namespaces", "--output", "json")
	if err != nil {
		return roles, err
	}
	var d map[string]interface{}
	b := []byte(res)
	err = json.Unmarshal(b, &d)
	if err != nil {
		return roles, err
	}
	roleitems := d["items"].([]interface{})
	for _, ri := range roleitems {
		role := ri.(map[string]interface{})
		metadata := role["metadata"].(map[string]interface{})
		ns := metadata["namespace"]
		rj, _ := struct2json(role)
		roles[ns.(string)] = append(roles[ns.(string)], rj)
	}
	return roles, nil
}

// getRoleBindings retrieves data about roles across all namespaces
func (r *Rback) getRoleBindings() (rolebindings map[string][]string, err error) {
	rolebindings = make(map[string][]string)
	res, err := kubecuddler.Kubectl(true, true, "", "get", "rolebindings", "--all-namespaces", "--output", "json")
	if err != nil {
		return rolebindings, err
	}
	var d map[string]interface{}
	b := []byte(res)
	err = json.Unmarshal(b, &d)
	if err != nil {
		return rolebindings, err
	}
	rbitems := d["items"].([]interface{})
	for _, rbi := range rbitems {
		rolebinding := rbi.(map[string]interface{})
		metadata := rolebinding["metadata"].(map[string]interface{})
		ns := metadata["namespace"]
		rbj, _ := struct2json(rolebinding)
		rolebindings[ns.(string)] = append(rolebindings[ns.(string)], rbj)
	}
	return rolebindings, nil
}

// getClusterRoles retrieves data about cluster roles
func (r *Rback) getClusterRoles() (croles []string, err error) {
	croles = []string{}
	res, err := kubecuddler.Kubectl(true, true, "", "get", "clusterroles", "--output", "json")
	if err != nil {
		return croles, err
	}
	var d map[string]interface{}
	b := []byte(res)
	err = json.Unmarshal(b, &d)
	if err != nil {
		return croles, err
	}
	croleitems := d["items"].([]interface{})
	for _, cri := range croleitems {
		crole := cri.(map[string]interface{})
		metadata := crole["metadata"].(map[string]interface{})
		name := metadata["name"]
		if !strings.HasPrefix(name.(string), "system:") {
			crj, _ := struct2json(crole)
			croles = append(croles, crj)
		}
	}
	return croles, nil
}

// getClusterRoleBindings retrieves data about cluster role bindings
func (r *Rback) getClusterRoleBindings() (crolebindings []string, err error) {
	crolebindings = []string{}
	res, err := kubecuddler.Kubectl(true, true, "", "get", "clusterrolebindings", "--output", "json")
	if err != nil {
		return crolebindings, err
	}
	var d map[string]interface{}
	b := []byte(res)
	err = json.Unmarshal(b, &d)
	if err != nil {
		return crolebindings, err
	}
	crolebindingitems := d["items"].([]interface{})
	for _, cri := range crolebindingitems {
		crolebinding := cri.(map[string]interface{})
		metadata := crolebinding["metadata"].(map[string]interface{})
		name := metadata["name"]
		if !strings.HasPrefix(name.(string), "system:") {
			crbj, _ := struct2json(crolebinding)
			crolebindings = append(crolebindings, crbj)
		}
	}
	return crolebindings, nil
}

// getPermissions retrieves data about all access control related data
// from service accounts to roles and bindings, both namespaced and the
// cluster level.
func (r *Rback) getPermissions() (Permissions, error) {
	p := Permissions{}
	sa, err := r.getServiceAccounts(r.config.namespace)
	if err != nil {
		return p, err
	}
	p.ServiceAccounts = sa
	roles, err := r.getRoles()
	if err != nil {
		return p, err
	}
	p.Roles = roles
	rb, err := r.getRoleBindings()
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

// lookupRoles lists roles for a given service account
func (r *Rback) lookupRoles(bindings []string, sa string) (roles []string, err error) {
	for _, rb := range bindings {
		var d map[string]interface{}
		b := []byte(rb)
		err = json.Unmarshal(b, &d)
		if err != nil {
			return roles, err
		}
		roleRef := d["roleRef"].(map[string]interface{})
		r := roleRef["name"].(string)
		if d["subjects"] != nil {
			subjects := d["subjects"].([]interface{})
			for _, subject := range subjects {
				s := subject.(map[string]interface{})
				if s["name"] == sa {
					roles = append(roles, r)
				}
			}
		}
	}
	return roles, nil
}

// lookupResources lists resources referenced in a role.
// if namespace is empty then the scope is cluster-wide.
func (r *Rback) lookupResources(namespace, role string, p Permissions) (resources string, err error) {
	if namespace != "" { // look up in roles
		for _, roles := range p.Roles[namespace] {
			var d map[string]interface{}
			b := []byte(roles)
			err = json.Unmarshal(b, &d)
			if err != nil {
				return "", err
			}
			metadata := d["metadata"].(map[string]interface{})
			rname := metadata["name"]
			if rname == role {
				rules := d["rules"].([]interface{})
				for _, rule := range rules {
					r := rule.(map[string]interface{})
					resources += toHumanReadableRule(r) + "\n"
				}
			}
		}
	}
	// ... otherwise, look up in cluster roles:
	for _, cr := range p.ClusterRoles {
		var d map[string]interface{}
		b := []byte(cr)
		err = json.Unmarshal(b, &d)
		if err != nil {
			return "", err
		}
		metadata := d["metadata"].(map[string]interface{})
		crname := metadata["name"]
		if crname == role {
			rules := d["rules"].([]interface{})
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
	r.renderLegend(g)

	for ns, serviceaccounts := range p.ServiceAccounts {
		gns := g.Subgraph(ns, dot.ClusterOption{})
		gns.Attr("style", "dashed")

		for _, sa := range serviceaccounts {
			sanode := newServiceAccountNode(gns, sa)
			// cluster roles:
			croles, err := r.lookupRoles(p.ClusterRoleBindings, sa)
			if err != nil {
				fmt.Printf("Can't look up cluster roles due to: %v", err)
				os.Exit(-2)
			}
			for _, crole := range croles {
				r.renderRole(g, crole, sanode, p, "")
			}
			// roles:
			roles, err := r.lookupRoles(p.RoleBindings[ns], sa)
			if err != nil {
				fmt.Printf("Can't look up roles due to: %v", err)
				os.Exit(-2)
			}
			for _, role := range roles {
				r.renderRole(gns, role, sanode, p, ns)
			}

		}
	}
	return g
}

func (r *Rback) renderLegend(g *dot.Graph) {
	legend := g.Subgraph("LEGEND", dot.ClusterOption{})

	namespace := legend.Subgraph("Namespace", dot.ClusterOption{})
	namespace.Attr("style", "dashed")

	las := newServiceAccountNode(namespace, "ServiceAccount")
	lr := newRoleNode(namespace, "(Cluster)Role")
	legend.Edge(las, lr)
	if r.config.renderRules {
		lac := newRulesNode(namespace, "Access rules")
		legend.Edge(lr, lac)
	}
}

func (r *Rback) renderRole(g *dot.Graph, roleName string, saNode dot.Node, p Permissions, ns string) {
	roleNode := newRoleNode(g, roleName)
	g.Edge(saNode, roleNode)

	if r.config.renderRules {
		res, err := r.lookupResources(ns, roleName, p)
		if err != nil {
			fmt.Printf("Can't look up entities and resources due to: %v", err)
			os.Exit(-3)
		}
		if res != "" {
			resnode := newRulesNode(g, res)
			g.Edge(roleNode, resnode)
		}
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

func newServiceAccountNode(g *dot.Graph, id string) dot.Node {
	return g.Node(id).
		Box().
		Attr("style", "filled").
		Attr("fillcolor", "#2f6de1").
		Attr("fontcolor", "#f0f0f0")
}

func newRoleNode(g *dot.Graph, id string) dot.Node {
	return g.Node(id).
		Attr("style", "filled").
		Attr("fillcolor", "#ff9900").
		Attr("fontcolor", "#030303")
}

func newRulesNode(g *dot.Graph, id string) dot.Node {
	return g.Node(id)
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/emicklei/dot"
	"github.com/mhausenblas/kubecuddler"
)

type Permissions struct {
	ServiceAccounts     map[string][]string
	Roles               map[string][]string
	ClusterRoles        []string
	RoleBindings        map[string][]string
	ClusterRoleBindings []string
}

func main() {
	p, err := getPermissions()
	if err != nil {
		fmt.Printf("Can't query permissions due to :%v", err)
		os.Exit(-1)
	}
	g := genGraph(p)
	fmt.Println(g.String())
}

// getServiceAccounts retrieves data about service accounts across all namespaces
func getServiceAccounts() (serviceAccounts map[string][]string, err error) {
	serviceAccounts = make(map[string][]string)
	res, err := kubecuddler.Kubectl(true, true, "", "get", "sa", "--all-namespaces", "--output", "json")
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
func getRoles() (roles map[string][]string, err error) {
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
func getRoleBindings() (rolebindings map[string][]string, err error) {
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
func getClusterRoles() (croles []string, err error) {
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
func getClusterRoleBindings() (crolebindings []string, err error) {
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
func getPermissions() (Permissions, error) {
	p := Permissions{}
	sa, err := getServiceAccounts()
	if err != nil {
		return p, err
	}
	p.ServiceAccounts = sa
	roles, err := getRoles()
	if err != nil {
		return p, err
	}
	p.Roles = roles
	rb, err := getRoleBindings()
	if err != nil {
		return p, err
	}
	p.RoleBindings = rb
	cr, err := getClusterRoles()
	if err != nil {
		return p, err
	}
	p.ClusterRoles = cr
	crb, err := getClusterRoleBindings()
	if err != nil {
		return p, err
	}
	p.ClusterRoleBindings = crb
	return p, nil
}

// lookupRoles lists roles in a namespace for a given service account
func lookupRoles(namespace, sa string, p Permissions) (roles []string, err error) {
	for _, rb := range p.RoleBindings[namespace] {
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

// lookupClusterRoles lists cluster roles for a given service account
func lookupClusterRoles(sa string, p Permissions) (clusterroles []string, err error) {
	for _, crb := range p.ClusterRoleBindings {
		var d map[string]interface{}
		b := []byte(crb)
		err = json.Unmarshal(b, &d)
		if err != nil {
			return clusterroles, err
		}
		roleRef := d["roleRef"].(map[string]interface{})
		r := roleRef["name"].(string)
		if d["subjects"] != nil {
			subjects := d["subjects"].([]interface{})
			for _, subject := range subjects {
				s := subject.(map[string]interface{})
				if s["name"] == sa {
					clusterroles = append(clusterroles, r)
				}
			}
		}

	}
	return clusterroles, nil
}

// lookupResources lists resources referenced in a role.
// if namespace is empty then the scope is cluster-wide.
func lookupResources(namespace, role string, p Permissions) (resources string, err error) {
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
					rj, _ := struct2json(r)
					resources += fmt.Sprintf("%v\n", rj)
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
				rj, _ := struct2json(r)
				resources += fmt.Sprintf("%v\n", rj)
			}
		}
	}
	return resources, nil
}

func genGraph(p Permissions) *dot.Graph {
	g := dot.NewGraph(dot.Directed)
	// legend:
	las := g.Node("SERVICE ACCOUNT").Attr("style", "filled").Attr("fillcolor", "#2f6de1").Attr("fontcolor", "#f0f0f0")
	lr := g.Node("(CLUSTER) ROLE").Attr("style", "filled").Attr("fillcolor", "#ff9900").Attr("fontcolor", "#030303")
	lac := g.Node("ACCESS RULES")
	g.Edge(las, lr)
	g.Edge(lr, lac)

	for ns, serviceaccounts := range p.ServiceAccounts {
		gns := g.Subgraph(ns, dot.ClusterOption{})
		for _, sa := range serviceaccounts {
			sanode := gns.Node(sa).Attr("style", "filled").Attr("fillcolor", "#2f6de1").Attr("fontcolor", "#f0f0f0")
			// cluster roles:
			croles, err := lookupClusterRoles(sa, p)
			if err != nil {
				fmt.Printf("Can't look up cluster roles due to: %v", err)
				os.Exit(-2)
			}
			for _, crole := range croles {
				crnode := g.Node(crole).Attr("style", "filled").Attr("fillcolor", "#ff9900").Attr("fontcolor", "#030303")
				g.Edge(sanode, crnode)
				res, err := lookupResources("", crole, p)
				if err != nil {
					fmt.Printf("Can't look up entities and resources due to: %v", err)
					os.Exit(-3)
				}
				if res != "" {
					resnode := g.Node(res)
					g.Edge(crnode, resnode)
				}
			}
			// roles:
			roles, err := lookupRoles(ns, sa, p)
			if err != nil {
				fmt.Printf("Can't look up roles due to: %v", err)
				os.Exit(-2)
			}
			for _, role := range roles {
				crnode := gns.Node(role).Attr("style", "filled").Attr("fillcolor", "#ff9900").Attr("fontcolor", "#030303")
				gns.Edge(sanode, crnode)
				res, err := lookupResources(ns, role, p)
				if err != nil {
					fmt.Printf("Can't look up entities and resources due to: %v", err)
					os.Exit(-3)
				}
				if res != "" {
					resnode := gns.Node(res)
					gns.Edge(crnode, resnode)
				}
			}

		}
	}
	return g
}

// struct2json turns a map into a JSON string
func struct2json(s map[string]interface{}) (string, error) {
	str, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(str), nil
}

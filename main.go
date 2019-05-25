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

func getPermissions() (Permissions, error) {
	p := Permissions{
		ServiceAccounts:     make(map[string][]string),
		Roles:               make(map[string][]string),
		ClusterRoles:        []string{},
		RoleBindings:        make(map[string][]string),
		ClusterRoleBindings: []string{},
	}
	// get all service accounts (namespaced):
	res, err := kubecuddler.Kubectl(true, true, "", "get", "sa", "--all-namespaces", "--no-headers")
	if err != nil {
		return p, err
	}
	for _, r := range strings.Split(res, "\n") {
		f := strings.Fields(r)
		p.ServiceAccounts[f[0]] = append(p.ServiceAccounts[f[0]], f[1])
	}
	// get all roles (namespaced):
	res, err = kubecuddler.Kubectl(true, true, "", "get", "roles", "--all-namespaces", "--no-headers")
	if err != nil {
		return p, err
	}
	for _, r := range strings.Split(res, "\n") {
		f := strings.Fields(r)
		p.Roles[f[0]] = append(p.Roles[f[0]], f[1])
	}
	// get all cluster roles:
	res, err = kubecuddler.Kubectl(true, true, "", "get", "clusterroles", "--no-headers")
	if err != nil {
		return p, err
	}
	for _, r := range strings.Split(res, "\n") {
		f := strings.Fields(r)
		if !strings.HasPrefix(f[0], "system:") {
			p.ClusterRoles = append(p.ClusterRoles, f[0])
		}
	}
	// get all rolebindings (namespaced):
	res, err = kubecuddler.Kubectl(true, true, "", "get", "rolebindings", "--all-namespaces", "--no-headers")
	if err != nil {
		return p, err
	}
	for _, r := range strings.Split(res, "\n") {
		f := strings.Fields(r)
		p.RoleBindings[f[0]] = append(p.RoleBindings[f[0]], f[1])
	}
	// get all cluster role bindings:
	res, err = kubecuddler.Kubectl(true, true, "", "get", "clusterrolebindings", "--no-headers")
	if err != nil {
		return p, err
	}
	for _, r := range strings.Split(res, "\n") {
		f := strings.Fields(r)
		if !strings.HasPrefix(f[0], "system:") {
			p.ClusterRoleBindings = append(p.ClusterRoleBindings, f[0])
		}
	}
	return p, nil
}

func lookupNamespacedResources(namespace, sa string, p Permissions) (roles []string, err error) {
	for _, rb := range p.RoleBindings[namespace] {
		res, err := kubecuddler.Kubectl(true, true, "", "--namespace", namespace, "get", "rolebinding", rb, "--output", "json")
		if err != nil {
			return roles, err
		}
		var d map[string]interface{}
		b := []byte(res)
		err = json.Unmarshal(b, &d)
		if err != nil {
			return roles, err
		}
		roleRef := d["roleRef"].(map[string]interface{})
		r := roleRef["name"].(string)
		// fmt.Fprintf(os.Stderr, "checking role %v", role)
		subjects := d["subjects"].([]interface{})
		// fmt.Println(subjects)
		for _, subject := range subjects {
			s := subject.(map[string]interface{})
			// fmt.Fprintf(os.Stderr, "subject: %v", s)
			if s["name"] == sa {
				roles = append(roles, r)
			}
		}
	}
	return roles, nil
}

func lookupResources(sa string, p Permissions) (clusterroles []string, err error) {
	for _, crb := range p.ClusterRoleBindings {
		res, err := kubecuddler.Kubectl(true, true, "", "get", "clusterrolebinding", crb, "--output", "json")
		if err != nil {
			return clusterroles, err
		}
		var d map[string]interface{}
		b := []byte(res)
		err = json.Unmarshal(b, &d)
		if err != nil {
			return clusterroles, err
		}
		roleRef := d["roleRef"].(map[string]interface{})
		r := roleRef["name"].(string)
		// fmt.Fprintf(os.Stderr, "checking role %v", role)
		subjects := d["subjects"].([]interface{})
		// fmt.Println(subjects)
		for _, subject := range subjects {
			s := subject.(map[string]interface{})
			// fmt.Fprintf(os.Stderr, "subject: %v", s)
			if s["name"] == sa {
				clusterroles = append(clusterroles, r)
			}
		}
	}
	return clusterroles, nil
}

func genGraph(p Permissions) *dot.Graph {
	g := dot.NewGraph(dot.Directed)
	for ns, serviceaccounts := range p.ServiceAccounts {
		gns := g.Subgraph(ns, dot.ClusterOption{})
		for _, sa := range serviceaccounts {
			gns.Node(sa).Attr("style", "filled").Attr("fillcolor", "#2f6de1").Attr("fontcolor", "#f0f0f0")
			// fmt.Fprintf(os.Stderr, "in namespace [%v], looking up resources for service account [%v]\n", ns, sa)
			roles, err := lookupResources(sa, p)
			if err != nil {
				fmt.Printf("Can't look up entities and resources due to: %v", err)
				os.Exit(-2)
			}
			for _, role := range roles {
				gns.Node(role).Attr("style", "filled").Attr("fillcolor", "#ff9900").Attr("fontcolor", "#030303")
			}
		}
	}
	return g
}

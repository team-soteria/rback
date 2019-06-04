package main

type Permissions struct {
	ServiceAccounts map[string]map[string]string  // map[namespace]map[name]json
	Roles           map[string]map[string]Role    // ClusterRoles are stored in Roles[""]
	RoleBindings    map[string]map[string]Binding // ClusterRoleBindings are stored in RoleBindings[""]
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

type Rule struct {
	verbs           []string
	resources       []string
	resourceNames   []string
	nonResourceURLs []string
	apiGroups       []string
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
)

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
	r.permissions.RoleBindings = make(map[string]map[string]Binding)

	items := input["items"].([]interface{})
	for _, i := range items {
		item := i.(map[string]interface{})
		nn := getNamespacedName(getMetadata(item))

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
				r.permissions.RoleBindings[nn.namespace] = make(map[string]Binding)
			}
			r.permissions.RoleBindings[nn.namespace][nn.name] = r.toBinding(item)
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

func (r *Rback) shouldIgnore(name string) bool {
	for _, prefix := range r.config.ignoredPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func toKindNamespacedName(obj interface{}) KindNamespacedName {
	o := obj.(map[string]interface{})
	return KindNamespacedName{
		kind:           o["kind"].(string),
		NamespacedName: getNamespacedName(o),
	}
}

func getNamespacedName(metadataOrRef map[string]interface{}) NamespacedName {
	return NamespacedName{
		stringOrEmpty(metadataOrRef["namespace"]),
		metadataOrRef["name"].(string),
	}
}

func getMetadata(obj map[string]interface{}) map[string]interface{} {
	metadata := obj["metadata"].(map[string]interface{})
	return metadata
}

func toRole(rawRole map[string]interface{}) Role {
	rules := []Rule{}
	rawRules := rawRole["rules"].([]interface{})
	for _, r := range rawRules {
		rules = append(rules, toRule(r))
	}

	return Role{
		getNamespacedName(getMetadata(rawRole)),
		rules,
	}
}

func (r *Rback) toBinding(rawBinding map[string]interface{}) Binding {
	subjects := []KindNamespacedName{}
	if rawBinding["subjects"] != nil {
		rawSubjects := rawBinding["subjects"].([]interface{})
		for _, s := range rawSubjects {
			subject := toKindNamespacedName(s)
			if !r.shouldIgnore(subject.name) {
				subjects = append(subjects, subject)
			}
		}
	}
	return Binding{
		NamespacedName: getNamespacedName(getMetadata(rawBinding)),
		role:           getNamespacedName(rawBinding["roleRef"].(map[string]interface{})),
		subjects:       subjects,
	}
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

// struct2json turns a map into a JSON string
func struct2json(s map[string]interface{}) (string, error) {
	str, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(str), nil
}

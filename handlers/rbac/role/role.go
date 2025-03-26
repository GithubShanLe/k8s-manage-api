package role

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s-manage-api/handlers"
	"k8s-manage-api/k8s"
	"net/http"

	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ListRoleRequest struct {
	RoleName  string `json:"roleName"`
	NameSpace string `json:"namespace"`
}

type ListRoleResponse struct {
	handlers.ErrorResponse
	Roles []Role `json:"roles"`
}

type Role struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels"`
	Pods       string            `json:"pods"`
	CreateTime string            `json:"createTime"`
	Rules      []RuleInfo        `json:"rules"`
}
type RuleInfo struct {
	Verbs    []string `json:"verbs"`
	ApiGroup []string `json:"apiGroup"`
	Resource []string `json:"resource"`
}

func ListRole(w http.ResponseWriter, r *http.Request) {
	clientset := k8s.GetClient()
	var resp ListRoleResponse
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}()

	// 解析请求参数
	var req ListRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("解析请求失败: %v", err)
		return
	}

	listOptions := metav1.ListOptions{}
	if req.RoleName != "" {
		listOptions.FieldSelector = fmt.Sprintf("metadata.name=%s", req.RoleName)
	}
	roles, err := clientset.RbacV1().Roles(req.NameSpace).List(context.Background(), listOptions)
	if err != nil {
		resp.ErrorCode = "500"
		resp.ErrorMessage = fmt.Sprintf("获取svc列表失败: %v", err)
		return
	}
	for _, role := range roles.Items {
		resp.Roles = append(resp.Roles, Role{
			Name:       role.Name,
			Namespace:  role.Namespace,
			Labels:     role.Labels,
			CreateTime: role.CreationTimestamp.Format("2006-01-02 15:04:05"),
			Rules:      returnRules(role),
		})
	}
	return
}

func returnRules(role v1.Role) []RuleInfo {
	var rules []RuleInfo
	for _, rule := range role.Rules {
		rules = append(rules, RuleInfo{
			Verbs:    rule.Verbs,
			ApiGroup: rule.APIGroups,
			Resource: rule.Resources,
		})
	}
	return rules
}

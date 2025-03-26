package clusterrole

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

type ListClusterRoleRequest struct {
	ClusterRoleName string `json:"clusterRoleName"`
}

type ListClusterRoleResponse struct {
	handlers.ErrorResponse
	ClusterRoles []ClusterRole `json:"clusterRoles"`
}

type ClusterRole struct {
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

func ListClusterRole(w http.ResponseWriter, r *http.Request) {
	clientset := k8s.GetClient()
	var resp ListClusterRoleResponse
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}()

	// 解析请求参数
	var req ListClusterRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("解析请求失败: %v", err)
		return
	}

	listOptions := metav1.ListOptions{}
	if req.ClusterRoleName != "" {
		listOptions.FieldSelector = fmt.Sprintf("metadata.name=%s", req.ClusterRoleName)
	}
	clusterRoles, err := clientset.RbacV1().ClusterRoles().List(context.TODO(), listOptions)
	if err != nil {
		resp.ErrorCode = "500"
		resp.ErrorMessage = fmt.Sprintf("获取svc列表失败: %v", err)
		return
	}
	for _, clusterRole := range clusterRoles.Items {
		resp.ClusterRoles = append(resp.ClusterRoles, ClusterRole{
			Name:       clusterRole.Name,
			Namespace:  clusterRole.Namespace,
			Labels:     clusterRole.Labels,
			CreateTime: clusterRole.CreationTimestamp.Format("2006-01-02 15:04:05"),
			Rules:      returnRules(clusterRole),
		})
	}
	return
}

func returnRules(clusterRole v1.ClusterRole) []RuleInfo {
	var rules []RuleInfo
	for _, rule := range clusterRole.Rules {
		rules = append(rules, RuleInfo{
			Verbs:    rule.Verbs,
			ApiGroup: rule.APIGroups,
			Resource: rule.Resources,
		})
	}
	return rules
}

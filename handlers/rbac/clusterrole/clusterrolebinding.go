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

type ListClusterRoleBindingRequest struct {
	ClusterRoleBindingName string `json:"clusterRoleBindingName"`
}

type ListClusterRoleBindingResponse struct {
	handlers.ErrorResponse
	ClusterRoleBindings []ClusterRoleBinding `json:"clusterRoleBindings"`
}

type ClusterRoleBinding struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels"`
	Pods       string            `json:"pods"`
	CreateTime string            `json:"createTime"`
	RoleRef    v1.RoleRef        `json:"roleRef"`
	Subjects   []v1.Subject      `json:"subjects"`
	Yaml       string            `json:"yaml"`
}

func ListClusterRoleBinding(w http.ResponseWriter, r *http.Request) {
	clientset := k8s.GetClient()
	var resp ListClusterRoleBindingResponse
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}()

	// 解析请求参数
	var req ListClusterRoleBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("解析请求失败: %v", err)
		return
	}

	listOptions := metav1.ListOptions{}
	if req.ClusterRoleBindingName != "" {
		listOptions.FieldSelector = fmt.Sprintf("metadata.name=%s", req.ClusterRoleBindingName)
	}
	clusterRoleBindings, err := clientset.RbacV1().ClusterRoleBindings().List(context.TODO(), listOptions)
	if err != nil {
		resp.ErrorCode = "500"
		resp.ErrorMessage = fmt.Sprintf("获取svc列表失败: %v", err)
		return
	}
	for _, clusterRoleBinding := range clusterRoleBindings.Items {
		// 转换为 YAML
		yamlData, err := k8s.ResourceToYAML(&clusterRoleBinding)
		if err != nil {
			resp.ErrorCode = "500"
			resp.ErrorMessage = fmt.Sprintf("转换YAML失败: %v", err)
			return
		}

		resp.ClusterRoleBindings = append(resp.ClusterRoleBindings, ClusterRoleBinding{
			Name:       clusterRoleBinding.Name,
			Namespace:  clusterRoleBinding.Namespace,
			Labels:     clusterRoleBinding.Labels,
			CreateTime: clusterRoleBinding.CreationTimestamp.Format("2006-01-02 15:04:05"),
			RoleRef:    clusterRoleBinding.RoleRef,
			Subjects:   clusterRoleBinding.Subjects,
			Yaml:       string(yamlData),
		})
	}
	return
}



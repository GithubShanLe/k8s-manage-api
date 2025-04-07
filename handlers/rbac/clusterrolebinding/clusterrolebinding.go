package clusterrolebinding

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
	ClusterRolebindingName  string `json:"clusterRolebindingName"`
}

type ListRoleResponse struct {
	handlers.ErrorResponse
	ClusterRolebindings []ClusterRoleBinding `json:"clusterRolebindings"`
}

type ClusterRoleBinding struct {
	Name       string            `json:"name"`
	Labels     map[string]string `json:"labels"`
	CreateTime string            `json:"createTime"`
	Subjects []v1.Subject      `json:"subjects"`
}

func ListClusterRoleBinding(w http.ResponseWriter, r *http.Request) {
	clientset := k8s.GetClient()
	var resp ListRoleResponse
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
	if req.ClusterRolebindingName != "" {
		listOptions.FieldSelector = fmt.Sprintf("metadata.name=%s", req.ClusterRolebindingName)
	}
	rolebindings, err := clientset.RbacV1().ClusterRoleBindings().List(context.Background(), listOptions)
	if err != nil {
		resp.ErrorCode = "500"
		resp.ErrorMessage = fmt.Sprintf("获取svc列表失败: %v", err)
		return
	}
	for _, clusterRolebinding := range rolebindings.Items {
		resp.ClusterRolebindings = append(resp.ClusterRolebindings, ClusterRoleBinding{
			Name:       clusterRolebinding.Name,
			Labels:     clusterRolebinding.Labels,
			CreateTime: clusterRolebinding.CreationTimestamp.Format("2006-01-02 15:04:05"),
			Subjects:   clusterRolebinding.Subjects,
		})
	}
	return
}
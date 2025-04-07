package rolebinding

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

type ListRoleBindingRequest struct {
	RoleBindingName  string `json:"roleBindingName"`
	NameSpace string `json:"namespace"`
}

type ListRoleResponse struct {
	handlers.ErrorResponse
	RoleBindings []RoleBinding `json:"roleBindings"`
}

type RoleBinding struct {
	Name       string            `json:"name"`
	NameSpace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels"`
	CreateTime string            `json:"createTime"`
	Subjects []v1.Subject      `json:"subjects"`
}

func ListRoleBinding(w http.ResponseWriter, r *http.Request) {
	clientset := k8s.GetClient()
	var resp ListRoleResponse
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}()

	// 解析请求参数
	var req ListRoleBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("解析请求失败: %v", err)
		return
	}

	listOptions := metav1.ListOptions{}
	if req.RoleBindingName != "" {
		listOptions.FieldSelector = fmt.Sprintf("metadata.name=%s", req.RoleBindingName)
	}
	rolebindings, err := clientset.RbacV1().RoleBindings(req.NameSpace).List(context.Background(), listOptions)
	if err != nil {
		resp.ErrorCode = "500"
		resp.ErrorMessage = fmt.Sprintf("获取svc列表失败: %v", err)
		return
	}
	for _, roleBinding := range rolebindings.Items {
		resp.RoleBindings = append(resp.RoleBindings, RoleBinding{
			Name:       roleBinding.Name,
			NameSpace:  roleBinding.Namespace,
			Labels:     roleBinding.Labels,
			CreateTime: roleBinding.CreationTimestamp.Format("2006-01-02 15:04:05"),
			Subjects:   roleBinding.Subjects,
		})
	}
	return
}
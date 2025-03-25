package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s-manage-api/handlers"
	"k8s-manage-api/k8s"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ListstatefulsetRequest struct {
	StatefulsetName string `json:"statefulsetName"`
	NameSpace       string `json:"namespace"`
}

type ListstatefulsetResponse struct {
	handlers.ErrorResponse
	Statefulsets []statefulset `json:"statefulsets"`
}

type statefulset struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels"`
	Images     []string          `json:"images"`
	Pods       string            `json:"pods"`
	CreateTime string            `json:"createTime"`
}

func Liststatefulset(w http.ResponseWriter, r *http.Request) {
	clientset := k8s.GetClient()
	var resp ListstatefulsetResponse
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}()

	// 解析请求参数
	var req ListstatefulsetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("解析请求失败: %v", err)
		return
	}

	listOptions := metav1.ListOptions{}
	if req.StatefulsetName != "" {
		listOptions.FieldSelector = fmt.Sprintf("metadata.name=%s", req.StatefulsetName)
	}
	svcs, err := clientset.AppsV1().StatefulSets(req.NameSpace).List(context.Background(), listOptions)
	if err != nil {
		resp.ErrorCode = "500"
		resp.ErrorMessage = fmt.Sprintf("获取svc列表失败: %v", err)
		return
	}
	for _, svc := range svcs.Items {
		resp.Statefulsets = append(resp.Statefulsets, statefulset{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Labels:    svc.Labels,
			Images: func() []string {
				var images []string
				for _, container := range svc.Spec.Template.Spec.Containers {
					images = append(images, container.Image)
				}
				return images
			}(),
			Pods:       fmt.Sprintf("%d/%d", svc.Status.ReadyReplicas, svc.Status.Replicas),
			CreateTime: svc.CreationTimestamp.Format("2006-01-02 15:04:05"),
		})
	}
	return
}

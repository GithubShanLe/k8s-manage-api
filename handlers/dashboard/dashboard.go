package dashboard

import (
	"encoding/json"
	"k8s-manage-api/k8s"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ClusterResourceStats struct {
	Nodes               int `json:"nodes"`                 // 节点总数
	ReadyNodes          int `json:"readyNodes"`            // 就绪节点数
	Namespaces          int `json:"namespaces"`            // 命名空间数
	Pods                int `json:"pods"`                 // Pod总数
	RunningPods         int `json:"runningPods"`           // 运行中Pod数
	ErrorPods          int `json:"errorPods"`             // 异常Pod数
	Deployments        int `json:"deployments"`           // 部署总数
	AvailableDeployments int `json:"availableDeployments"` // 可用部署数
	Services           int `json:"services"`              // 服务总数
	LBServices         int `json:"lbServices"`            // 负载均衡服务数
	Ingresses          int `json:"ingresses"`             // Ingress数
	PVCs               int `json:"pvcs"`                  // 持久卷声明数
}

// GetClusterResourceStats 获取集群资源统计信息
func GetClusterResourceStats(w http.ResponseWriter, r *http.Request) {
	ctx :=r.Context()
	stats := &ClusterResourceStats{}
	clientset := k8s.GetClient()
	// 获取节点状态
	if nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{}); err == nil {
		stats.Nodes = len(nodes.Items)
		for _, node := range nodes.Items {
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
					stats.ReadyNodes++
				}
			}
		}
	}

	// 获取Pod状态
	if pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{}); err == nil {
		stats.Pods = len(pods.Items)
		for _, pod := range pods.Items {
			switch pod.Status.Phase {
			case corev1.PodRunning:
				stats.RunningPods++
			case corev1.PodFailed, corev1.PodUnknown:
				stats.ErrorPods++
			}
		}
	}

	// 获取部署状态
	if deployments, err := clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{}); err == nil {
		stats.Deployments = len(deployments.Items)
		for _, deploy := range deployments.Items {
			if deploy.Status.AvailableReplicas == *deploy.Spec.Replicas {
				stats.AvailableDeployments++
			}
		}
	}

	// 获取服务状态
	if services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{}); err == nil {
		stats.Services = len(services.Items)
		for _, svc := range services.Items {
			if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				stats.LBServices++
			}
		}
	}

	// 获取命名空间数
	if namespaces, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{}); err == nil {
		stats.Namespaces = len(namespaces.Items)
	}

	// 获取所有命名空间的Pod数
	if pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{}); err == nil {
		stats.Pods = len(pods.Items)
	}

	// 获取所有命名空间的Service数
	if services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{}); err == nil {
		stats.Services = len(services.Items)
	}

	// 获取所有命名空间的Ingress数
	if ingresses, err := clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{}); err == nil {
		stats.Ingresses = len(ingresses.Items)
	}

	// 获取所有命名空间的PVC数
	if pvcs, err := clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{}); err == nil {
		stats.PVCs = len(pvcs.Items)
	}

// 返回 JSON 响应
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(stats)
}

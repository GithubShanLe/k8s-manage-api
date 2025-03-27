package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog"
	sigyaml "sigs.k8s.io/yaml"
)

var (
	YamlOperationClient *YamlOperation
	YamlOperationDryRunClient *YamlOperation
	err            error
)


func ResourceToYAML(obj interface{}) (string, error) {
	yamlBytes, err := sigyaml.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func Apply(
	ctx context.Context,
	t string, // action
	yamlData string,
	dryRun bool,
	labelSelector string,
	fieldSelector string) (string, error) {

	restConfig := GetRestConfig()
	// 1. Prepare a RESTMapper to find GVR
	dc, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		klog.Error(ctx, "discovery.NewDiscoveryClientForConfig error", zap.Error(err))
		return "", err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	// 2. Prepare the dynamic client
	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		klog.Error(ctx, "dynamic.NewForConfig error", zap.Error(err))
		return "", err
	}

	decoder := yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(yamlData), len(yamlData))
	for {
		var rawObj runtime.RawExtension
		if err = decoder.Decode(&rawObj); err != nil {
			fmt.Println("err", err)
			return "",err
		}
		obj, gvk, err := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode(rawObj.Raw, nil, nil)
		if err != nil {
			return "", err
		}
		unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return "", err
		}

		unstructuredObj := &unstructured.Unstructured{Object: unstructuredMap}

		// 4. Find GVR
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return "", err
		}

		// 5. Obtain REST interface for the GVR
		var dri dynamic.ResourceInterface
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			// namespaced resources should specify the namespace
			dri = dyn.Resource(mapping.Resource).Namespace(unstructuredObj.GetNamespace())
		} else {
			// for cluster-wide resources
			dri = dyn.Resource(mapping.Resource)
		}

		switch t {
		case "create":
			{
				opt := metav1.CreateOptions{}
				if dryRun {
					opt.DryRun = []string{"All"}
				}
				// objByte, _ := unstructuredObj.MarshalJSON()
				// klog.Info(ctx, "create resource "+unstructuredObj.GetName(), zap.Any("unstructuredObj", string(objByte)))
				_, err := dri.Create(ctx, unstructuredObj, opt)
				if err != nil {
					klog.Error(ctx, "dri create failed", zap.Error(err))
					return "", err
				}

				
			}
		case "apply":
			{
				opt := metav1.ApplyOptions{}
				if dryRun {
					opt.DryRun = []string{"All"}
				}
				fmt.Println("unstructuredObj.GetName()", unstructuredObj.GetName())
				_, err := dri.Apply(ctx, unstructuredObj.GetName(), unstructuredObj, opt)
				if err != nil {
					klog.Error(ctx, "dri apply failed", zap.Error(err))
					return "", err
				}
			}
		case "patch":
			{
				data, err := json.Marshal(obj)
				if err != nil {
					return "", err
				}
				opt := metav1.PatchOptions{}
				if dryRun {
					opt.DryRun = []string{"All"}
				}
				_, err = dri.Patch(ctx, unstructuredObj.GetName(), types.MergePatchType, data, opt)
				if err != nil {
					klog.Error(ctx, "dri patch failed", zap.Error(err))
					return "", err
				}
			}
		case "delete":
			{
				opt := metav1.DeleteOptions{}
				if dryRun {
					opt.DryRun = []string{"All"}
				}
				err := dri.Delete(ctx, unstructuredObj.GetName(), opt)
				if err != nil {
					klog.Error(ctx, "dri delete failed", zap.Error(err))
					return "", err
				}
			}
		case "update":
			{
				opt := metav1.UpdateOptions{}
				if dryRun {
					opt.DryRun = []string{"All"}
				}
				_, err := dri.Update(ctx, unstructuredObj, opt)
				if err != nil {
					klog.Error(ctx, "dri update failed", zap.Error(err))
					return "", err
				}
			}
		case "list":
			{
				opt := metav1.ListOptions{}
				if labelSelector != "" {
					opt.LabelSelector = labelSelector
				}
				if fieldSelector != "" {
					opt.FieldSelector = fieldSelector
				}
				dataList, err := dri.List(ctx, opt)
				if err != nil {
					klog.Error(ctx, fmt.Sprintf("dri.List error:%s", err.Error()))
					return "", err
				}
				data, err := dataList.MarshalJSON()
				if err != nil {
					klog.Error(ctx, "dri list failed", zap.Error(err))
					return "", err
				}
				return string(data), nil
			}
		case "get":
			{
				dataGet, err := dri.Get(ctx, unstructuredObj.GetName(), metav1.GetOptions{})
				if err != nil {
					return "", err
				}
				data, err := dataGet.MarshalJSON()
				if err != nil {
					return "", err
				}
				return string(data), nil
			}
		}
	}
	return "", nil
}


// YamlOperation 定义了对 K8s 资源的基本操作
type YamlOperation struct {
    ctx        context.Context
    dyn        dynamic.Interface
    mapper     *restmapper.DeferredDiscoveryRESTMapper
    dryRun     bool
}

func init(){
	YamlOperationClient,err=NewYamlOperation(context.TODO(),false)
	if err!= nil {
		panic(err)
	}
	YamlOperationDryRunClient,err=NewYamlOperation(context.TODO(),true)
	if err!= nil {
		panic(err)
	}
}

// NewYamlOperation 创建一个新的 YamlOperation 实例
func NewYamlOperation(ctx context.Context, dryRun bool) (*YamlOperation, error) {
    restConfig := GetRestConfig()
    
    // 初始化 Discovery Client
    dc, err := discovery.NewDiscoveryClientForConfig(restConfig)
    if err != nil {
        return nil, fmt.Errorf("创建 Discovery Client 失败: %v", err)
    }
    
    // 初始化 REST Mapper
    mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))
    
    // 初始化 Dynamic Client
    dyn, err := dynamic.NewForConfig(restConfig)
    if err != nil {
        return nil, fmt.Errorf("创建 Dynamic Client 失败: %v", err)
    }
    
    return &YamlOperation{
        ctx:    ctx,
        dyn:    dyn,
        mapper: mapper,
        dryRun: dryRun,
    }, nil
}

// Apply 应用 YAML 配置到集群
func (y *YamlOperation) Apply(yamlData string) error {
    return y.processYaml(yamlData, func(dri dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
        opt := metav1.ApplyOptions{FieldManager: "k8s-manager-api"}
        if y.dryRun {
            opt.DryRun = []string{"All"}
        }
        _, err := dri.Apply(y.ctx, obj.GetName(), obj,opt)
        return err
    })
}

// Create 创建新的资源
func (y *YamlOperation) Create(yamlData string) error {
    return y.processYaml(yamlData, func(dri dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
        opt := metav1.CreateOptions{}
        if y.dryRun {
            opt.DryRun = []string{"All"}
        }
        _, err := dri.Create(y.ctx, obj, opt)
        return err
    })
}

// Delete 删除资源
func (y *YamlOperation) Delete(yamlData string) error {
    return y.processYaml(yamlData, func(dri dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
        opt := metav1.DeleteOptions{}
        if y.dryRun {
            opt.DryRun = []string{"All"}
        }
        return dri.Delete(y.ctx, obj.GetName(), opt)
    })
}

// Patch 更新资源的部分内容
func (y *YamlOperation) Patch(yamlData string) error {
    return y.processYaml(yamlData, func(dri dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
        data, err := json.Marshal(obj)
        if err != nil {
            return err
        }
        opt := metav1.PatchOptions{}
        if y.dryRun {
            opt.DryRun = []string{"All"}
        }
        _, err = dri.Patch(y.ctx, obj.GetName(), types.MergePatchType, data, opt)
        return err
    })
}

func (y *YamlOperation) Get(yamlData string) error {
    return y.processYaml(yamlData, func(dri dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
        _, err := dri.Get(y.ctx, obj.GetName(), metav1.GetOptions{})
        return err
    })
}

// processYaml 处理 YAML 文件并执行操作
func (y *YamlOperation) processYaml(yamlData string, operation func(dynamic.ResourceInterface, *unstructured.Unstructured) error) error {
    decoder := yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(yamlData), len(yamlData))
    
    for {
        // 解码 YAML 文档
        var rawObj runtime.RawExtension
        if err := decoder.Decode(&rawObj); err != nil {
            if err.Error() == "EOF" {
                break
            }
            return fmt.Errorf("解码 YAML 失败: %v", err)
        }
        
        // 转换为 unstructured 对象
        obj, gvk, err := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode(rawObj.Raw, nil, nil)
        if err != nil {
            return fmt.Errorf("解析资源失败: %v", err)
        }
        
        unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
        if err != nil {
            return fmt.Errorf("转换为 unstructured 失败: %v", err)
        }
        
        unstructuredObj := &unstructured.Unstructured{Object: unstructuredMap}
        
        // 获取资源映射
        mapping, err := y.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
        if err != nil {
            return fmt.Errorf("获取资源映射失败: %v", err)
        }
        
        // 获取资源接口
        var dri dynamic.ResourceInterface
        if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
            dri = y.dyn.Resource(mapping.Resource).Namespace(unstructuredObj.GetNamespace())
        } else {
            dri = y.dyn.Resource(mapping.Resource)
        }
        
        // 执行操作
        if err := operation(dri, unstructuredObj); err != nil {
            return fmt.Errorf("执行操作失败: %v", err)
        }
    }
    
    return nil
}

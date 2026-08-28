package informer

import (
	"context"
	"sync"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type RoleBindingInfo struct {
	Name        string
	Namespace   string
	RoleRefName string
	Subjects    []rbacv1.Subject
}

type RoleBindingInformer struct {
	clientset     kubernetes.Interface
	mu            sync.RWMutex
	roleBindings  map[string]*RoleBindingInfo
	lastSyncTime  time.Time
	store         cache.Store
	controller    cache.Controller
}

func NewRoleBindingInformer(clientset kubernetes.Interface) *RoleBindingInformer {
	return &RoleBindingInformer{
		clientset:    clientset,
		roleBindings: make(map[string]*RoleBindingInfo),
	}
}

func (i *RoleBindingInformer) Start(ctx context.Context) {
	watchlist := cache.NewListWatchFromClient(
		i.clientset.RbacV1().RESTClient(),
		"rolebindings",
		"",
		fields.Everything(),
	)

	store, controller := cache.NewInformer(
		watchlist,
		&rbacv1.RoleBinding{},
		time.Minute,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				rb := obj.(*rbacv1.RoleBinding)
				i.addOrUpdate(rb)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				rb := newObj.(*rbacv1.RoleBinding)
				i.addOrUpdate(rb)
			},
			DeleteFunc: func(obj interface{}) {
				rb, ok := obj.(*rbacv1.RoleBinding)
				if !ok {
					tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
					if !ok {
						return
					}
					rb, ok = tombstone.Obj.(*rbacv1.RoleBinding)
					if !ok {
						return
					}
				}
				i.remove(rb)
			},
		},
	)

	i.store = store
	i.controller = controller

	go controller.Run(ctx.Done())
}

func (i *RoleBindingInformer) addOrUpdate(rb *rbacv1.RoleBinding) {
	i.mu.Lock()
	defer i.mu.Unlock()
	key := rb.Namespace + "/" + rb.Name
	i.roleBindings[key] = &RoleBindingInfo{
		Name:        rb.Name,
		Namespace:   rb.Namespace,
		RoleRefName: rb.RoleRef.Name,
		Subjects:    rb.Subjects,
	}
	klog.V(4).Infof("RoleBinding cached: %s/%s -> %s", rb.Namespace, rb.Name, rb.RoleRef.Name)
}

func (i *RoleBindingInformer) remove(rb *rbacv1.RoleBinding) {
	i.mu.Lock()
	defer i.mu.Unlock()
	key := rb.Namespace + "/" + rb.Name
	delete(i.roleBindings, key)
	klog.V(4).Infof("RoleBinding removed from cache: %s/%s", rb.Namespace, rb.Name)
}

func (i *RoleBindingInformer) GetRoleBindings() map[string]*RoleBindingInfo {
	i.mu.RLock()
	defer i.mu.RUnlock()
	result := make(map[string]*RoleBindingInfo, len(i.roleBindings))
	for k, v := range i.roleBindings {
		result[k] = v
	}
	return result
}

func (i *RoleBindingInformer) ListRoleBindings() []*RoleBindingInfo {
	i.mu.RLock()
	defer i.mu.RUnlock()
	result := make([]*RoleBindingInfo, 0, len(i.roleBindings))
	for _, v := range i.roleBindings {
		result = append(result, v)
	}
	return result
}

func (i *RoleBindingInformer) HasSynced() bool {
	if i.controller == nil {
		return false
	}
	return i.controller.HasSynced()
}

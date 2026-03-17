package rules

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"drift-sentinel/pkg/metrics"
)

type Controller struct {
	logger            *slog.Logger
	metrics           *metrics.Registry
	store             *Store
	factory           informers.SharedInformerFactory
	configMapInformer cache.SharedIndexInformer
	namespaceInformer cache.SharedIndexInformer
	configMapLister   corev1listers.ConfigMapLister
	namespaceLister   corev1listers.NamespaceLister
	ready             atomic.Bool
}

type NamespaceModeResolver struct {
	lister corev1listers.NamespaceLister
}

func NewController(client kubernetes.Interface, store *Store, logger *slog.Logger, registry *metrics.Registry, resyncPeriod time.Duration) *Controller {
	if logger == nil {
		logger = slog.Default()
	}
	if registry == nil {
		registry = metrics.NewRegistry()
	}
	if store == nil {
		store = NewStore()
	}

	factory := informers.NewSharedInformerFactory(client, resyncPeriod)
	configMapInformer := factory.Core().V1().ConfigMaps()
	namespaceInformer := factory.Core().V1().Namespaces()

	controller := &Controller{
		logger:            logger,
		metrics:           registry,
		store:             store,
		factory:           factory,
		configMapInformer: configMapInformer.Informer(),
		namespaceInformer: namespaceInformer.Informer(),
		configMapLister:   configMapInformer.Lister(),
		namespaceLister:   namespaceInformer.Lister(),
	}

	controller.registerHandlers(configMapInformer)
	return controller
}

func (c *Controller) Start(ctx context.Context, syncTimeout time.Duration) error {
	c.factory.Start(ctx.Done())

	syncCtx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()

	if !cache.WaitForCacheSync(syncCtx.Done(), c.configMapInformer.HasSynced, c.namespaceInformer.HasSynced) {
		return fmt.Errorf("timed out waiting for informer caches to sync")
	}

	if err := c.rebuildRules(); err != nil {
		return err
	}

	c.ready.Store(true)
	return nil
}

func (c *Controller) NamespaceModeResolver() *NamespaceModeResolver {
	return &NamespaceModeResolver{lister: c.namespaceLister}
}

func (r *NamespaceModeResolver) ResolveMode(_ context.Context, namespace string) (Mode, bool, error) {
	ns, err := r.lister.Get(namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}

	value := ns.Annotations[NamespaceModeAnnotation]
	if value == "" {
		return "", false, nil
	}

	mode := Mode(value)
	if !isValidMode(mode) {
		return "", false, fmt.Errorf("invalid namespace mode %q on namespace %s", value, namespace)
	}

	return mode, true, nil
}

func (c *Controller) registerHandlers(configMapInformer corev1informers.ConfigMapInformer) {
	_, _ = configMapInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ any) {
			c.metrics.RecordConfigEvent("add")
			c.triggerRebuild()
		},
		UpdateFunc: func(oldObj, newObj any) {
			oldConfigMap, oldOK := oldObj.(*corev1.ConfigMap)
			newConfigMap, newOK := newObj.(*corev1.ConfigMap)
			if oldOK && newOK && oldConfigMap.ResourceVersion == newConfigMap.ResourceVersion {
				return
			}

			c.metrics.RecordConfigEvent("update")
			c.triggerRebuild()
		},
		DeleteFunc: func(_ any) {
			c.metrics.RecordConfigEvent("delete")
			c.triggerRebuild()
		},
	})
}

func (c *Controller) triggerRebuild() {
	if !c.ready.Load() {
		return
	}

	if err := c.rebuildRules(); err != nil {
		c.logger.Error("failed to rebuild rule cache", "error", err)
	}
}

func (c *Controller) rebuildRules() error {
	configMaps, err := c.configMapLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list configmaps: %w", err)
	}

	validRules := make([]Rule, 0)
	invalidCount := 0

	for _, configMap := range configMaps {
		internal := FromCoreConfigMap(configMap)
		if !IsRuleConfigMap(internal) {
			continue
		}

		rule, parseErr := ParseConfigMap(internal)
		if parseErr != nil {
			invalidCount++
			c.logger.Error(
				"failed to parse rule configmap",
				"namespace", configMap.Namespace,
				"name", configMap.Name,
				"error", parseErr,
			)
			continue
		}

		validRules = append(validRules, rule)
	}

	c.store.Replace(validRules)
	c.metrics.SetRulesLoaded(len(validRules))
	c.logger.Info("rule cache rebuilt", "rules_loaded", len(validRules), "invalid_rules", invalidCount)
	return nil
}

func FromCoreConfigMap(configMap *corev1.ConfigMap) ConfigMap {
	annotations := make(map[string]string, len(configMap.Annotations))
	for key, value := range configMap.Annotations {
		annotations[key] = value
	}

	data := make(map[string]string, len(configMap.Data))
	for key, value := range configMap.Data {
		data[key] = value
	}

	return ConfigMap{
		Name:        configMap.Name,
		Namespace:   configMap.Namespace,
		Annotations: annotations,
		Data:        data,
	}
}

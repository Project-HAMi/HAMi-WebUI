package data

import (
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"vgpu/internal/biz"
	"vgpu/internal/data/prom"
)

const informerResyncPeriod = time.Hour

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(
	NewData,
	NewDeviceCatalog,
	NewNodeRepo,
	NewPodRepo,
	wire.Bind(new(biz.PodRepo), new(*podRepo)),
	wire.Bind(new(biz.SchedulingRepo), new(*podRepo)),
)

// Data .
type Data struct {
	k8sCl    kubernetes.Interface
	eventsCl kubernetes.Interface
	promCl   *prom.Client
	// Shared by the repositories, so each resource has one cache.
	informers informers.SharedInformerFactory
	stopCh    chan struct{}
}

// NewData .
func NewData(logger log.Logger, promCl *prom.Client) (*Data, func(), error) {
	log := log.NewHelper(log.With(logger, "module", "vgpu-service/data"))
	cfg := config.GetConfigOrDie()
	k8sCl, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	eventsCl, err := newBoundedSchedulingEventClient(cfg)
	if err != nil {
		return nil, nil, err
	}
	stopCh := make(chan struct{})
	return &Data{
			k8sCl:     k8sCl,
			eventsCl:  eventsCl,
			promCl:    promCl,
			informers: informers.NewSharedInformerFactory(k8sCl, informerResyncPeriod),
			stopCh:    stopCh,
		}, func() {
			close(stopCh)
			log.Info("message", "closing the data resources")
		}, nil
}

// startInformers starts the informers requested so far and waits for their caches.
func (d *Data) startInformers() {
	d.informers.Start(d.stopCh)
	d.informers.WaitForCacheSync(d.stopCh)
}

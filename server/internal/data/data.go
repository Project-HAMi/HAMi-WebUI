package data

import (
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"vgpu/internal/data/prom"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(
	NewData,
	NewNodeRepo,
	NewPodRepo,
)

// Data .
type Data struct {
	k8sCl  kubernetes.Interface
	promCl *prom.Client
}

// NewData .
func NewData(logger log.Logger, promCl *prom.Client) (*Data, func(), error) {
	log := log.NewHelper(log.With(logger, "module", "vgpu-service/data"))
	cfg := config.GetConfigOrDie()
	k8sCl, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}

	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}
	return &Data{
			k8sCl:  k8sCl,
			promCl: promCl,
		}, func() {
			log.Info("message", "closing the data resources")
		}, nil
}

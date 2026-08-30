package cloudserver

import (
	"context"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
)

type cachedCloudSkillPackageStore struct {
	cache   *skillintel.CachedPackageStore
	durable cloud.SkillPackageObjectStore
}

func (s *cachedCloudSkillPackageStore) Put(ctx context.Context, pkg skillintel.Package) error {
	return s.cache.Put(ctx, pkg)
}

func (s *cachedCloudSkillPackageStore) Get(ctx context.Context, packageHash string) (skillintel.Package, bool, error) {
	return s.cache.Get(ctx, packageHash)
}

func (s *cachedCloudSkillPackageStore) ObjectURI(packageHash string) (string, error) {
	return s.durable.ObjectURI(packageHash)
}

type cloudSkillServices struct {
	Runtime    *cloud.SkillRuntime
	Packages   cloud.SkillPackageObjectStore
	Imports    *cloud.SkillImportService
	Configured bool
	StorageMode string
	Err        error
}

var cloudSkillServicesByServer sync.Map

func skillServicesForServer(s *Server) *cloudSkillServices {
	if s == nil || s.Store == nil {
		return &cloudSkillServices{Runtime: cloud.NewSkillRuntime(nil, nil)}
	}
	if cached, ok := cloudSkillServicesByServer.Load(s); ok {
		return cached.(*cloudSkillServices)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s3Store, externalConfigured, err := cloud.SkillPackageStoreFromEnv(ctx)
	services := &cloudSkillServices{Configured: false, StorageMode: "unavailable", Err: err}
	if err != nil {
		services.Runtime = cloud.NewSkillRuntime(s.Store, nil)
		actual, _ := cloudSkillServicesByServer.LoadOrStore(s, services)
		return actual.(*cloudSkillServices)
	}

	var durable cloud.SkillPackageObjectStore
	if externalConfigured {
		durable = s3Store
		services.StorageMode = "object"
	} else if s.Store.DB != nil {
		postgresStore, postgresErr := cloud.NewPostgresSkillPackageStore(s.Store.DB)
		if postgresErr != nil {
			services.Err = postgresErr
			services.Runtime = cloud.NewSkillRuntime(s.Store, nil)
			actual, _ := cloudSkillServicesByServer.LoadOrStore(s, services)
			return actual.(*cloudSkillServices)
		}
		durable = postgresStore
		services.StorageMode = "postgres"
	} else {
		services.Runtime = cloud.NewSkillRuntime(s.Store, nil)
		actual, _ := cloudSkillServicesByServer.LoadOrStore(s, services)
		return actual.(*cloudSkillServices)
	}

	cache, cacheErr := skillintel.NewCachedPackageStore(durable, skillintel.DefaultPackageMemoryCacheBytes)
	if cacheErr != nil {
		services.Err = cacheErr
		services.Runtime = cloud.NewSkillRuntime(s.Store, nil)
		actual, _ := cloudSkillServicesByServer.LoadOrStore(s, services)
		return actual.(*cloudSkillServices)
	}
	packages := &cachedCloudSkillPackageStore{cache: cache, durable: durable}
	services.Packages = packages
	services.Runtime = cloud.NewSkillRuntime(s.Store, packages)
	services.Imports, services.Err = cloud.NewSkillImportService(packages, s.Store)
	services.Configured = services.Err == nil
	actual, _ := cloudSkillServicesByServer.LoadOrStore(s, services)
	return actual.(*cloudSkillServices)
}

func clearSkillServicesForServer(s *Server) {
	if s != nil {
		cloudSkillServicesByServer.Delete(s)
	}
}

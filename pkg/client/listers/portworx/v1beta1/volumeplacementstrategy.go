/*
Copyright 2017 The Portworx Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by lister-gen. DO NOT EDIT.

package v1beta1

import (
	v1beta1 "github.com/portworx/talisman/pkg/apis/portworx/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// VolumePlacementStrategyLister helps list VolumePlacementStrategies.
type VolumePlacementStrategyLister interface {
	// List lists all VolumePlacementStrategies in the indexer.
	List(selector labels.Selector) (ret []*v1beta1.VolumePlacementStrategy, err error)
	// Get retrieves the VolumePlacementStrategy from the index for a given name.
	Get(name string) (*v1beta1.VolumePlacementStrategy, error)
	VolumePlacementStrategyListerExpansion
}

// volumePlacementStrategyLister implements the VolumePlacementStrategyLister interface.
type volumePlacementStrategyLister struct {
	indexer cache.Indexer
}

// NewVolumePlacementStrategyLister returns a new VolumePlacementStrategyLister.
func NewVolumePlacementStrategyLister(indexer cache.Indexer) VolumePlacementStrategyLister {
	return &volumePlacementStrategyLister{indexer: indexer}
}

// List lists all VolumePlacementStrategies in the indexer.
func (s *volumePlacementStrategyLister) List(selector labels.Selector) (ret []*v1beta1.VolumePlacementStrategy, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1beta1.VolumePlacementStrategy))
	})
	return ret, err
}

// Get retrieves the VolumePlacementStrategy from the index for a given name.
func (s *volumePlacementStrategyLister) Get(name string) (*v1beta1.VolumePlacementStrategy, error) {
	obj, exists, err := s.indexer.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1beta1.Resource("volumeplacementstrategy"), name)
	}
	return obj.(*v1beta1.VolumePlacementStrategy), nil
}

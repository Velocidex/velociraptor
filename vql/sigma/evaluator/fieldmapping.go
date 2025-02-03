package evaluator

import (
	"sync"

	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type FieldMappingRecord struct {
	Name   string
	Lambda *vfilter.Lambda
}

type FieldMappingResolver struct {
	mappings map[string]FieldMappingRecord
	mu       sync.Mutex
}

func (self *FieldMappingResolver) Get(name string) (*vfilter.Lambda, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, pres := self.mappings[name]
	if !pres {
		return nil, utils.NotFoundError
	}
	return res.Lambda, nil
}

func (self *FieldMappingResolver) Len() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return len(self.mappings)
}

func (self *FieldMappingResolver) Set(name string, lambda *vfilter.Lambda) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.mappings[name] = FieldMappingRecord{Name: name, Lambda: lambda}
}

func NewFieldMappingResolver() *FieldMappingResolver {
	return &FieldMappingResolver{
		mappings: make(map[string]FieldMappingRecord),
	}
}

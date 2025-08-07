package controller

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const (
	// typeAvailableHub represents the status of the Deployment reconciliation
	typeAvailableHub = "Available"
	// typeDegradedHub represents the status used when the custom resource is deleted and the finalizer operations are must occur.
	typeDegradedHub = "Degraded"

	hubFinalizer = "mydecisive.ai/finalizer"

	ObjectModified          ObjectState = true
	ObjectUnchanged         ObjectState = false
	LabelManagedByMdaiKey               = "app.kubernetes.io/managed-by"
	LabelManagedByMdaiValue             = "mdai-operator"
)

type ObjectState bool

type ConfigBlock map[string]any

func getAs[T any](m map[string]any, key string) (T, bool) {
	v, ok := m[key]
	if !ok {
		var zero T
		return zero, false
	}
	val, ok := v.(T)
	if !ok {
		var zero T
		return zero, false
	}
	return val, true
}

func mustGetAs[T any](m map[string]any, key string) T {
	v, ok := getAs[T](m, key)
	if !ok {
		panic(fmt.Sprintf("expected %T at key %q, got %T (%#v)", v, key, m[key], m[key]))
	}
	return v
}

func (c ConfigBlock) GetMap(key string) (ConfigBlock, bool) {
	m, ok := getAs[map[string]any](c, key)
	return m, ok
}

func (c ConfigBlock) MustMap(key string) ConfigBlock {
	return mustGetAs[map[string]any](c, key)
}

func (c ConfigBlock) GetSlice(key string) ([]any, bool) {
	return getAs[[]any](c, key)
}

func (c ConfigBlock) MustSlice(key string) []any {
	return mustGetAs[[]any](c, key)
}

func (c ConfigBlock) GetString(key string) (string, bool) {
	return getAs[string](c, key)
}

func (c ConfigBlock) MustString(key string) string {
	return mustGetAs[string](c, key)
}

func (c ConfigBlock) GetInt(key string) (int, bool) {
	return getAs[int](c, key)
}

func (c ConfigBlock) MustInt(key string) int {
	return mustGetAs[int](c, key)
}

func (c ConfigBlock) GetFloat(key string) (float64, bool) {
	return getAs[float64](c, key)
}

func (c ConfigBlock) MustFloat(key string) float64 {
	return mustGetAs[float64](c, key)
}

func (c ConfigBlock) Set(key string, value any) {
	c[key] = value
}

func (c ConfigBlock) YAMLBytes() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c ConfigBlock) YAML() (string, error) {
	b, err := c.YAMLBytes()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

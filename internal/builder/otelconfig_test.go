package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConfigBlock_MustVsDirect_Equivalence_SetDeepValues(t *testing.T) {
	config1 := sampleConfig()

	config1["receivers"].(map[string]any)["otlp"].(map[string]any)["protocols"].(map[string]any)["grpc"].(map[string]any)["max_recv_msg_size_mib"] = 64 //nolint:forcetypeassert

	groupByKey := "groupbyattrs/obs"
	config1["processors"].(map[string]any)[groupByKey] = map[string]any{ //nolint:forcetypeassert
		"keys": []string{"resource.attributes.service.name", "resource.attributes.k8s.pod.uid"},
	}

	observerName := "observer"
	tracesPipelineName := "traces/" + observerName
	pipelineProcessors := []any{"batch", groupByKey}
	config1["service"].(map[string]any)["pipelines"].(map[string]any)[tracesPipelineName] = map[string]any{ //nolint:forcetypeassert
		"receivers":  []any{"otlp"},
		"processors": pipelineProcessors,
		"exporters":  []any{"datavendor"},
	}

	config2 := sampleConfig()

	config2.MustMap("receivers").
		MustMap("otlp").
		MustMap("protocols").
		MustMap("grpc").
		Set("max_recv_msg_size_mib", 64)

	config2.MustMap("processors").Set(groupByKey, map[string]any{
		"keys": []string{"resource.attributes.service.name", "resource.attributes.k8s.pod.uid"},
	})

	config2.MustMap("service").
		MustMap("pipelines").
		Set(tracesPipelineName, map[string]any{
			"receivers":  []any{"otlp"},
			"processors": pipelineProcessors,
			"exporters":  []any{"datavendor"},
		})

	require.Equal(t, config1, config2)
}

func TestConfigBlock_GetAndMust_Behavior(t *testing.T) {
	cfg := sampleConfig()

	m, ok := cfg.GetMap("receivers")
	require.True(t, ok)
	require.NotNil(t, m)

	_, ok = cfg.GetMap("does_not_exist")
	require.False(t, ok)
	require.Panics(t, func() { cfg.MustMap("does_not_exist") })

	cfg.MustMap("service").MustMap("pipelines").Set("names", []any{"traces/default"})
	s, ok := cfg.MustMap("service").MustMap("pipelines").GetSlice("names")
	require.True(t, ok)
	require.Equal(t, []any{"traces/default"}, s)

	cfg.Set("s", "hello")
	cfg.Set("i", 123)
	cfg.Set("f", 1.5)

	str, ok := cfg.GetString("s")
	require.True(t, ok)
	require.Equal(t, "hello", str)
	require.Equal(t, "hello", cfg.MustString("s"))

	i, ok := cfg.GetInt("i")
	require.True(t, ok)
	require.Equal(t, 123, i)
	require.Equal(t, 123, cfg.MustInt("i"))

	f, ok := cfg.GetFloat("f")
	require.True(t, ok)
	require.InDelta(t, 1.5, f, 0.00001)
	require.InDelta(t, 1.5, cfg.MustFloat("f"), 0.00001)

	_, ok = cfg.GetInt("s")
	require.False(t, ok)
	require.Panics(t, func() { _ = cfg.MustInt("s") })
}

func TestConfigBlock_YAML_RoundTrip(t *testing.T) {
	cfg := sampleConfig()
	cfg.MustMap("service").MustMap("pipelines").Set("metrics/default", map[string]any{
		"receivers":  []any{"otlp"},
		"processors": []any{"batch"},
		"exporters":  []any{"datavendor"},
	})

	y, err := cfg.YAMLBytes()
	require.NoError(t, err)

	var back map[string]any
	require.NoError(t, yaml.Unmarshal(y, &back))
	require.Equal(t, map[string]any(cfg), back)
}

func TestConfigBlock_GetMap_SafeFalseOnMissingOrWrongType(t *testing.T) {
	cfg := ConfigBlock{
		"a": map[string]any{"nested": 1},
		"b": 42,
	}
	_, ok := cfg.GetMap("a")
	assert.True(t, ok)

	_, ok = cfg.GetMap("nope")
	assert.False(t, ok)

	_, ok = cfg.GetMap("b")
	assert.False(t, ok)
}

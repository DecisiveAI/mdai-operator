package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valkey-io/valkey-go"
	"k8s.io/utils/ptr"

	v1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"github.com/go-logr/logr"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/valkey-io/valkey-go/mock"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	v1core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestMdaiCR() *v1.MdaiHub {
	return &v1.MdaiHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hub",
			Namespace: "default",
		},
		Spec:   v1.MdaiHubSpec{},
		Status: v1.MdaiHubStatus{},
	}
}

func newFakeClientForCR(cr *v1.MdaiHub, scheme *runtime.Scheme) client.Client {
	collector := &v1beta1.OpenTelemetryCollector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "collector1",
			Namespace: "default",
			Labels: map[string]string{
				LabelMdaiHubName: "test-hub",
			},
		},
	}
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr, collector).
		WithStatusSubresource(cr).
		Build()
}

func createTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = v1core.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = prometheusv1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)
	_ = v1beta1.AddToScheme(scheme)
	return scheme
}

func TestFinalizeHub_Success(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()

	mdaiCR := &v1.MdaiHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-hub",
			Namespace:  "default",
			Finalizers: []string{hubFinalizer},
		},
		Spec:   v1.MdaiHubSpec{},
		Status: v1.MdaiHubStatus{Conditions: []metav1.Condition{}},
	}
	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	fakeValkey := mock.NewClient(ctrl)
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Scan().Cursor(0).Match(VariableKeyPrefix+mdaiCR.Name+"/"+"*").Count(100).Build()).
		Return(mock.Result(mock.ValkeyArray(mock.ValkeyInt64(0), mock.ValkeyArray(mock.ValkeyString(VariableKeyPrefix+mdaiCR.Name+"/"+"key")))))
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Del().Key(VariableKeyPrefix+mdaiCR.Name+"/"+"key").Build()).
		Return(mock.Result(mock.ValkeyInt64(1)))

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, fakeValkey, time.Duration(30))

	state, err := adapter.finalizeHub(ctx)
	if err != nil {
		t.Fatalf("finalizeHub returned error: %v", err)
	}
	if state != ObjectModified {
		t.Errorf("expected state ObjectModified, got %v", state)
	}

	updatedCR := &v1.MdaiHub{}
	if err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-hub", Namespace: "default"}, updatedCR); err != nil {
		t.Fatalf("failed to get updated CR: %v", err)
	}
	for _, f := range updatedCR.Finalizers {
		if f == hubFinalizer {
			t.Errorf("expected finalizer %q to be removed, got: %v", hubFinalizer, updatedCR.Finalizers)
		}
	}

	cond := meta.FindStatusCondition(updatedCR.Status.Conditions, typeDegradedHub)
	if cond == nil {
		t.Errorf("expected condition %q to be set", typeDegradedHub)
	} else if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected condition %q to be True, got %v", typeDegradedHub, cond.Status)
	}
}

func TestEnsureFinalizerInitialized_AddsFinalizer(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()

	mdaiCR := &v1.MdaiHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-hub",
			Namespace:  "default",
			Finalizers: []string{},
		},
		Spec:   v1.MdaiHubSpec{},
		Status: v1.MdaiHubStatus{},
	}

	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, nil, time.Duration(30))
	_, err := adapter.ensureFinalizerInitialized(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	updatedCR := &v1.MdaiHub{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-hub", Namespace: "default"}, updatedCR)
	if err != nil {
		t.Fatalf("Failed to get updated CR: %v", err)
	}
	if !slices.Contains(updatedCR.Finalizers, hubFinalizer) {
		t.Errorf("Expected finalizer %q to be added", hubFinalizer)
	}
}

func TestEnsureFinalizerInitialized_AlreadyPresent(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()
	mdaiCR := &v1.MdaiHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-hub",
			Namespace:  "default",
			Finalizers: []string{hubFinalizer},
		},
		Spec:   v1.MdaiHubSpec{},
		Status: v1.MdaiHubStatus{},
	}

	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, nil, time.Duration(30))
	_, err := adapter.ensureFinalizerInitialized(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	updatedCR := &v1.MdaiHub{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-hub", Namespace: "default"}, updatedCR)
	if err != nil {
		t.Fatalf("Failed to get updated CR: %v", err)
	}
	if len(updatedCR.Finalizers) != 1 || !slices.Contains(updatedCR.Finalizers, hubFinalizer) {
		t.Errorf("Expected finalizers to contain only %q, got %v", hubFinalizer, updatedCR.Finalizers)
	}
}

func TestEnsureStatusInitialized_SetsInitialStatus(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()
	mdaiCR := newTestMdaiCR()
	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, nil, time.Duration(30))
	_, err := adapter.ensureStatusInitialized(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	updatedCR := &v1.MdaiHub{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-hub", Namespace: "default"}, updatedCR)
	if err != nil {
		t.Fatalf("Failed to get updated CR: %v", err)
	}
	if len(updatedCR.Status.Conditions) == 0 {
		t.Errorf("Expected at least one status condition to be set")
	} else {
		cond := meta.FindStatusCondition(updatedCR.Status.Conditions, typeAvailableHub)
		if cond == nil || cond.Status != metav1.ConditionUnknown {
			t.Errorf("Expected %q condition with status %q, got: %+v", typeAvailableHub, metav1.ConditionUnknown, cond)
		}
	}
}

func TestGetConfigMapSHA(t *testing.T) {
	cm := v1core.ConfigMap{
		Data: map[string]string{"key": "value"},
	}
	sha, err := getConfigMapSHA(cm)
	if err != nil {
		t.Fatalf("getConfigMapSHA returned error: %v", err)
	}

	data, err := json.Marshal(cm)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	sum := sha256.Sum256(data)
	expected := hex.EncodeToString(sum[:])
	if sha != expected {
		t.Errorf("Expected SHA %q, got %q", expected, sha)
	}
}

func TestDeleteFinalizer(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()

	mdaiCR := &v1.MdaiHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-hub",
			Namespace:  "default",
			Finalizers: []string{hubFinalizer, "other"},
		},
	}

	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, nil, time.Duration(30))
	if err := adapter.deleteFinalizer(ctx, mdaiCR, hubFinalizer); err != nil {
		t.Fatalf("deleteFinalizer returned error: %v", err)
	}

	if slices.Contains(mdaiCR.Finalizers, hubFinalizer) {
		t.Errorf("Expected finalizer %q to be removed", hubFinalizer)
	}
	if !slices.Contains(mdaiCR.Finalizers, "other") {
		t.Errorf("Expected finalizer %q to remain", "other")
	}
}

func TestCreateOrUpdateEnvConfigMap(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()
	mdaiCR := newTestMdaiCR()
	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, nil, time.Duration(30))
	envMap := map[string]string{"VAR": "value"}
	if _, err := adapter.createOrUpdateEnvConfigMap(ctx, envMap, envConfigMapNamePostfix, "default"); err != nil {
		t.Fatalf("createOrUpdateEnvConfigMap returned error: %v", err)
	}

	cm := &v1core.ConfigMap{}
	cmName := mdaiCR.Name + envConfigMapNamePostfix
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: "default"}, cm); err != nil {
		t.Fatalf("Failed to get ConfigMap %q: %v", cmName, err)
	}
	if cm.Data["VAR"] != "value" {
		t.Errorf("Expected env var value %q, got %q", "value", cm.Data["VAR"])
	}
}

func TestCreateOrUpdateManualEnvConfigMap(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()
	mdaiCR := newTestMdaiCR()
	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, nil, time.Duration(30))
	envMap := map[string]string{"VAR": "string"}
	if _, err := adapter.createOrUpdateEnvConfigMap(ctx, envMap, manualEnvConfigMapNamePostfix, "default"); err != nil {
		t.Fatalf("createOrUpdateEnvConfigMap returned error: %v", err)
	}

	cm := &v1core.ConfigMap{}
	cmName := mdaiCR.Name + manualEnvConfigMapNamePostfix
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: "default"}, cm); err != nil {
		t.Fatalf("Failed to get ConfigMap %q: %v", cmName, err)
	}
	if cm.Data["VAR"] != "string" {
		t.Errorf("Expected env var value %q, got %q", "value", cm.Data["VAR"])
	}
}

func TestBuildCollectorConfig(t *testing.T) {
	mdaiCR := newTestMdaiCR()
	observers := []v1.Observer{
		{
			Name:                    "obs1",
			LabelResourceAttributes: []string{"label1", "label2"},
		},
	}
	mdaiCR.Spec.Observers = observers

	scheme := createTestScheme()
	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, nil, time.Duration(30))
	config, err := adapter.getObserverCollectorConfig(observers, nil)
	if err != nil {
		t.Fatalf("getObserverCollectorConfig returned error: %v", err)
	}
	if !strings.Contains(config, "obs1") {
		t.Errorf("Expected collector config to contain observer name %q, got: %s", "obs1", config)
	}
}

func TestEnsureVariableSynced(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()
	storageType := v1.VariableSourceTypeBuiltInValkey

	variableSet := v1.Variable{
		StorageType: storageType,
		Type:        v1.VariableTypeComputed,
		DataType:    v1.VariableDataTypeSet,
		Key:         "mykey_set",
		SerializeAs: []v1.Serializer{
			{
				Name: "MY_ENV_SET",
				Transformers: []v1.VariableTransformer{
					{Type: v1.TransformerTypeJoin,
						Join: &v1.JoinTransformer{
							Delimiter: ",",
						},
					},
				},
			},
		},
	}
	variableString := v1.Variable{
		StorageType: storageType,
		Type:        v1.VariableTypeComputed,
		DataType:    v1.VariableDataTypeString,
		Key:         "mykey_string",
		SerializeAs: []v1.Serializer{
			{
				Name: "MY_ENV_STR",
			},
		},
	}
	variableBoolean := v1.Variable{
		StorageType: storageType,
		Type:        v1.VariableTypeComputed,
		DataType:    v1.VariableDataTypeString,
		Key:         "mykey_bool",
		SerializeAs: []v1.Serializer{
			{
				Name: "MY_ENV_BOOL",
			},
		},
	}

	variableInt := v1.Variable{
		StorageType: storageType,
		Type:        v1.VariableTypeComputed,
		DataType:    v1.VariableDataTypeInt,
		Key:         "mykey_int",
		SerializeAs: []v1.Serializer{
			{
				Name: "MY_ENV_INT",
			},
		},
	}

	variableMap := v1.Variable{
		StorageType: storageType,
		Type:        v1.VariableTypeComputed,
		DataType:    v1.VariableDataTypeMap,
		Key:         "mykey_map",
		SerializeAs: []v1.Serializer{
			{
				Name: "MY_ENV_MAP",
			},
		},
	}

	variablePl := v1.Variable{
		StorageType: storageType,
		Type:        v1.VariableTypeMeta,
		DataType:    v1.MetaVariableDataTypePriorityList,
		Key:         "mykey_pl",
		VariableRefs: []string{
			"some_key",
			"mykey_set",
		},
		SerializeAs: []v1.Serializer{
			{
				Name: "MY_ENV_PL",
				Transformers: []v1.VariableTransformer{
					{Type: v1.TransformerTypeJoin,
						Join: &v1.JoinTransformer{
							Delimiter: ",",
						},
					},
				},
			},
		},
	}

	variableHs := v1.Variable{
		StorageType: storageType,
		Type:        v1.VariableTypeMeta,
		DataType:    v1.MetaVariableDataTypeHashSet,
		Key:         "mykey_hs",
		VariableRefs: []string{
			"some_key",
			"mykey_set",
		},
		SerializeAs: []v1.Serializer{
			{
				Name: "MY_ENV_HS",
			},
		},
	}

	mdaiCR := newTestMdaiCR()
	mdaiCR.Spec.Variables = []v1.Variable{
		variableSet,
		variableString,
		variableBoolean,
		variableInt,
		variableMap,
		variablePl,
		variableHs,
	}

	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	fakeValkey := mock.NewClient(ctrl)

	// audit
	fakeValkey.EXPECT().Do(ctx, XaddMatcher{Type: "collector_restart"}).Return(mock.Result(mock.ValkeyString(""))).Times(1)

	// getting variables
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Smembers().Key(VariableKeyPrefix+mdaiCR.Name+"/"+variableSet.Key).Build()).
		Return(mock.Result(mock.ValkeyArray(mock.ValkeyString("service1"), mock.ValkeyString("service2"))))
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Get().Key(VariableKeyPrefix+mdaiCR.Name+"/"+variableString.Key).Build()).
		Return(mock.Result(mock.ValkeyString("serviceA")))
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Get().Key(VariableKeyPrefix+mdaiCR.Name+"/"+variableBoolean.Key).Build()).
		Return(mock.Result(mock.ValkeyString("true")))
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Get().Key(VariableKeyPrefix+mdaiCR.Name+"/"+variableInt.Key).Build()).
		Return(mock.Result(mock.ValkeyString("10")))

	expectedMap := map[string]valkey.ValkeyMessage{
		"field1": mock.ValkeyString("value1"),
		"field2": mock.ValkeyString("value1"),
	}
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Hgetall().Key(VariableKeyPrefix+mdaiCR.Name+"/"+variableMap.Key).Build()).
		Return(mock.Result(mock.ValkeyMap(expectedMap)))

	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Arbitrary("PRIORITYLIST.GETORCREATE").
		Keys(VariableKeyPrefix+mdaiCR.Name+"/"+variablePl.Key).
		Args(VariableKeyPrefix+mdaiCR.Name+"/"+"some_key", VariableKeyPrefix+mdaiCR.Name+"/"+"mykey_set").Build()).
		Return(mock.Result(mock.ValkeyArray(mock.ValkeyString("service1"), mock.ValkeyString("service2"))))

	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Arbitrary("HASHSET.GETORCREATE").
		Keys(VariableKeyPrefix+mdaiCR.Name+"/"+variableHs.Key).
		Args(VariableKeyPrefix+mdaiCR.Name+"/"+"some_key", VariableKeyPrefix+mdaiCR.Name+"/"+"mykey_set").Build()).
		Return(mock.Result(mock.ValkeyString("INFO|WARNING")))

	// scan for delete & actual delete of non defined variable
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Scan().Cursor(0).Match(VariableKeyPrefix+mdaiCR.Name+"/"+"*").Count(100).Build()).
		Return(mock.Result(mock.ValkeyArray(mock.ValkeyInt64(0), mock.ValkeyArray(mock.ValkeyString(VariableKeyPrefix+mdaiCR.Name+"/"+"key")))))
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Del().Key(VariableKeyPrefix+mdaiCR.Name+"/"+"key").Build()).
		Return(mock.Result(mock.ValkeyInt64(1)))

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, fakeValkey, time.Duration(30))

	opResult, err := adapter.ensureVariableSynced(ctx)
	if err != nil {
		t.Fatalf("ensureVariableSynced returned error: %v", err)
	}
	if opResult != ContinueOperationResult() {
		t.Errorf("expected ContinueProcessing, got: %v", opResult)
	}

	envCMName := mdaiCR.Name + envConfigMapNamePostfix
	envCM := &v1core.ConfigMap{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: envCMName, Namespace: "default"}, envCM); err != nil {
		t.Fatalf("failed to get env ConfigMap %q: %v", envCMName, err)
	}
	assert.Equal(t, len(envCM.Data), 7)
	assert.Equal(t, envCM.Data["MY_ENV_SET"], "service1,service2")
	assert.Equal(t, envCM.Data["MY_ENV_STR"], "serviceA")
	assert.Equal(t, envCM.Data["MY_ENV_BOOL"], "true")
	assert.Equal(t, envCM.Data["MY_ENV_INT"], "10")
	assert.Equal(t, envCM.Data["MY_ENV_MAP"], "field1: value1\nfield2: value1\n")
	assert.Equal(t, envCM.Data["MY_ENV_PL"], "service1,service2")
	assert.Equal(t, envCM.Data["MY_ENV_HS"], "INFO|WARNING")

	updatedCollector := &v1beta1.OpenTelemetryCollector{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "collector1", Namespace: "default"}, updatedCollector); err != nil {
		t.Fatalf("failed to get updated collector: %v", err)
	}
	restartAnnotation := "kubectl.kubernetes.io/restartedAt"
	if ann, ok := updatedCollector.Annotations[restartAnnotation]; !ok || strings.TrimSpace(ann) == "" {
		t.Errorf("expected collector to have restart annotation %q set, got: %v", restartAnnotation, updatedCollector.Annotations)
	}
}
func TestEnsureManualAndComputedVariableSynced(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()
	storageType := v1.VariableSourceTypeBuiltInValkey
	variableType := v1.VariableDataTypeSet
	varWith := v1.Serializer{
		Name: "MY_ENV",
		Transformers: []v1.VariableTransformer{
			{Type: v1.TransformerTypeJoin,
				Join: &v1.JoinTransformer{
					Delimiter: ",",
				},
			},
		},
	}
	computedVariable := v1.Variable{
		StorageType: storageType,
		Type:        v1.VariableTypeComputed,
		DataType:    variableType,
		Key:         "mykey",
		SerializeAs: []v1.Serializer{varWith},
	}
	manualVariable := v1.Variable{
		StorageType: storageType,
		Type:        v1.VariableTypeManual,
		DataType:    variableType,
		Key:         "mymanualkey",
		SerializeAs: []v1.Serializer{varWith},
	}
	mdaiCR := newTestMdaiCR()
	mdaiCR.Spec.Variables = []v1.Variable{computedVariable, manualVariable}

	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	fakeValkey := mock.NewClient(ctrl)
	expectedComputedKey := VariableKeyPrefix + mdaiCR.Name + "/" + computedVariable.Key
	expectedManualKey := VariableKeyPrefix + mdaiCR.Name + "/" + manualVariable.Key

	fakeValkey.EXPECT().Do(ctx, XaddMatcher{Type: "collector_restart"}).Return(mock.Result(mock.ValkeyString(""))).Times(1)

	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Smembers().Key(expectedComputedKey).Build()).
		Return(mock.Result(mock.ValkeyArray(mock.ValkeyString("default"))))
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Smembers().Key(expectedManualKey).Build()).
		Return(mock.Result(mock.ValkeyArray(mock.ValkeyString("default"))))

	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Scan().Cursor(0).Match(VariableKeyPrefix+mdaiCR.Name+"/"+"*").Count(100).Build()).
		Return(mock.Result(mock.ValkeyArray(mock.ValkeyInt64(0), mock.ValkeyArray(mock.ValkeyString(VariableKeyPrefix+mdaiCR.Name+"/"+"key")))))
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Del().Key(VariableKeyPrefix+mdaiCR.Name+"/"+"key").Build()).
		Return(mock.Result(mock.ValkeyInt64(1)))

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, fakeValkey, time.Duration(30))

	opResult, err := adapter.ensureVariableSynced(ctx)
	if err != nil {
		t.Fatalf("ensureVariableSynced returned error: %v", err)
	}
	if opResult != ContinueOperationResult() {
		t.Errorf("expected ContinueProcessing, got: %v", opResult)
	}

	envCMName := mdaiCR.Name + envConfigMapNamePostfix
	envCM := &v1core.ConfigMap{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: envCMName, Namespace: "default"}, envCM); err != nil {
		t.Fatalf("failed to get env ConfigMap %q: %v", envCMName, err)
	}
	if v, ok := envCM.Data["MY_ENV"]; !ok || v != "default" {
		t.Errorf("expected env var MY_ENV to be 'set', got %q", v)
	}

	envManualCMName := mdaiCR.Name + manualEnvConfigMapNamePostfix
	envManualCM := &v1core.ConfigMap{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: envManualCMName, Namespace: "default"}, envManualCM); err != nil {
		t.Fatalf("failed to get env ConfigMap %q: %v", envManualCMName, err)
	}
	if v, ok := envManualCM.Data["mymanualkey"]; !ok || v != "set" {
		t.Errorf("expected env var MY_ENV to be 'default', got %q", v)
	}

	updatedCollector := &v1beta1.OpenTelemetryCollector{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "collector1", Namespace: "default"}, updatedCollector); err != nil {
		t.Fatalf("failed to get updated collector: %v", err)
	}
	restartAnnotation := "kubectl.kubernetes.io/restartedAt"
	if ann, ok := updatedCollector.Annotations[restartAnnotation]; !ok || strings.TrimSpace(ann) == "" {
		t.Errorf("expected collector to have restart annotation %q set, got: %v", restartAnnotation, updatedCollector.Annotations)
	}
}

type XaddMatcher struct {
	Type string
}

func (xadd XaddMatcher) Matches(x any) bool {
	if cmd, ok := x.(valkey.Completed); ok {
		commands := cmd.Commands()
		return slices.Contains(commands, "XADD") && slices.Contains(commands, "mdai_hub_event_history") && slices.Contains(commands, xadd.Type)
	}
	return false
}

func (xadd XaddMatcher) String() string {
	return "Wanted XADD to mdai_hub_event_history command with " + xadd.Type
}

func TestEnsureEvaluationsSynchronized_WithEvaluations(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()

	alertName := "alert1"
	var expr = intstr.FromString("up == 0")
	var duration1 prometheusv1.Duration = "5m"
	eval := v1.PrometheusAlert{
		Name:     alertName,
		Expr:     expr,
		For:      &duration1,
		Severity: "critical",
	}

	evals := []v1.PrometheusAlert{eval}
	var interval prometheusv1.Duration = "10m"
	mdaiCR := &v1.MdaiHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hub",
			Namespace: "default",
		},
		Spec: v1.MdaiHubSpec{
			PrometheusAlert: evals,
			Config: &v1.Config{
				EvaluationInterval: &interval,
			},
		},
		Status: v1.MdaiHubStatus{},
	}

	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)
	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, nil, time.Duration(30))

	opResult, err := adapter.ensurePrometheusAlertsSynchronized(ctx)
	if err != nil {
		t.Fatalf("ensurePrometheusAlertsSynchronized returned error: %v", err)
	}
	if opResult != ContinueOperationResult() {
		t.Errorf("expected ContinueOperationResult, got: %v", opResult)
	}

	ruleName := "mdai-" + mdaiCR.Name + "-alert-rules"
	promRule := &prometheusv1.PrometheusRule{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: ruleName, Namespace: mdaiCR.Namespace}, promRule)
	if err != nil {
		t.Fatalf("failed to get PrometheusRule: %v", err)
	}

	group := promRule.Spec.Groups[0]
	if group.Interval == nil || *group.Interval != interval {
		t.Errorf("expected EvaluationInterval %q, got %v", interval, group.Interval)
	}

	if len(group.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(group.Rules))
	}
	rule := group.Rules[0]
	if rule.Alert != "alert1" {
		t.Errorf("expected alert name 'alert1', got %q", rule.Alert)
	}
	if rule.Expr != intstr.FromString("up == 0") {
		t.Errorf("expected expr 'up == 0', got %q", rule.Expr)
	}
	if *rule.For != duration1 {
		t.Errorf("expected For '5m', got %q", *rule.For)
	}
}

func TestEnsureEvaluationsSynchronized_NoEvaluations(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()
	mdaiCR := newTestMdaiCR()
	ruleName := "mdai-" + mdaiCR.Name + "-alert-rules"
	promRule := &prometheusv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ruleName,
			Namespace: mdaiCR.Namespace,
		},
		Spec: prometheusv1.PrometheusRuleSpec{
			Groups: []prometheusv1.RuleGroup{
				{
					Name:  "mdai",
					Rules: []prometheusv1.Rule{{Alert: "old-alert", Expr: intstr.FromString("1==1")}},
				},
			},
		},
	}

	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)
	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, nil, time.Duration(30))

	opResult, err := adapter.ensurePrometheusAlertsSynchronized(ctx)
	if err != nil {
		t.Fatalf("ensurePrometheusAlertsSynchronized returned error: %v", err)
	}
	if opResult != ContinueOperationResult() {
		t.Errorf("expected ContinueOperationResult, got: %v", opResult)
	}

	err = fakeClient.Get(ctx, types.NamespacedName{Name: ruleName, Namespace: mdaiCR.Namespace}, promRule)
	if err == nil {
		t.Errorf("expected PrometheusRule %q to be deleted, but it still exists", ruleName)
	} else if !apierrors.IsNotFound(err) {
		t.Errorf("unexpected error getting PrometheusRule: %v", err)
	}
}

func TestEnsureHubDeletionProcessed_WithDeletion(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()

	mdaiCR := &v1.MdaiHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-hub",
			Namespace:         "default",
			Finalizers:        []string{hubFinalizer},
			DeletionTimestamp: &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
		},
		Spec:   v1.MdaiHubSpec{},
		Status: v1.MdaiHubStatus{Conditions: []metav1.Condition{}},
	}

	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	fakeValkey := mock.NewClient(ctrl)
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Scan().Cursor(0).Match(VariableKeyPrefix+mdaiCR.Name+"/"+"*").Count(100).Build()).
		Return(mock.Result(mock.ValkeyArray(mock.ValkeyInt64(0), mock.ValkeyArray(mock.ValkeyString(VariableKeyPrefix+mdaiCR.Name+"/"+"key")))))
	fakeValkey.EXPECT().Do(ctx, fakeValkey.B().Del().Key(VariableKeyPrefix+mdaiCR.Name+"/"+"key").Build()).
		Return(mock.Result(mock.ValkeyInt64(1)))

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, fakeValkey, time.Duration(30))

	opResult, err := adapter.ensureHubDeletionProcessed(ctx)
	if err != nil {
		t.Fatalf("ensureHubDeletionProcessed returned error: %v", err)
	}

	if opResult != StopOperationResult() {
		t.Errorf("expected StopOperationResult, got: %v", opResult)
	}

	updatedCR := &v1.MdaiHub{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: mdaiCR.Name, Namespace: mdaiCR.Namespace}, updatedCR)
	if err != nil {
		if apierrors.IsNotFound(err) {
			t.Logf("CR not found after finalization, which is acceptable")
		} else {
			t.Fatalf("failed to get updated CR: %v", err)
		}
	}
}

func TestEnsureObserversSynchronized_WithObservers(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()

	observer := v1.Observer{
		Name:                    "watcher4",
		ResourceRef:             "watcher-collector",
		LabelResourceAttributes: []string{"service.name", "team", "region"},
		CountMetricName:         ptr.To("mdai_watcher_four_count_total"),
		BytesMetricName:         ptr.To("mdai_watcher_four_bytes_total"),
		Filter: &v1.ObserverFilter{
			ErrorMode: ptr.To("ignore"),
			Logs: &v1.ObserverLogsFilter{
				LogRecord: []string{`attributes["log_level"] == "INFO"`},
			},
		},
	}
	observers := []v1.Observer{observer}
	observerResource := v1.ObserverResource{
		Name:  "watcher-collector",
		Image: ptr.To("public.ecr.aws/p3k6k6h3/watcher-observer"),
	}
	observerResources := []v1.ObserverResource{observerResource}

	mdaiCR := &v1.MdaiHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hub",
			Namespace: "default",
		},
		Spec: v1.MdaiHubSpec{
			Observers:         observers,
			ObserverResources: observerResources,
		},
		Status: v1.MdaiHubStatus{},
	}

	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)
	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, nil, time.Duration(30))

	// Call ensureObserversSynchronized.
	opResult, err := adapter.ensureObserversSynchronized(ctx)
	if err != nil {
		t.Fatalf("ensureObserversSynchronized returned error: %v", err)
	}
	if opResult != ContinueOperationResult() {
		t.Errorf("expected ContinueOperationResult, got: %v", opResult)
	}

	configMapName := adapter.getScopedObserverResourceName(observerResource, "config")
	cm := &v1core.ConfigMap{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: mdaiCR.Namespace}, cm); err != nil {
		t.Fatalf("failed to get ConfigMap %q: %v", configMapName, err)
	}
	if _, ok := cm.Data["collector.yaml"]; !ok {
		t.Errorf("expected collector.yaml key in ConfigMap data, got: %v", cm.Data)
	}

	deploymentName := mdaiCR.Name + "-watcher-collector"
	deploy := &appsv1.Deployment{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: mdaiCR.Namespace}, deploy); err != nil {
		t.Fatalf("failed to get Deployment %q: %v", deploymentName, err)
	}
	hash, ok := deploy.Spec.Template.Annotations["mdai-collector-config/sha256"]
	if !ok || hash == "" {
		t.Errorf("expected mdai-collector-config/sha256 annotation to be set in Deployment, got: %v", deploy.Spec.Template.Annotations)
	}

	serviceName := mdaiCR.Name + "-watcher-collector-service"
	svc := &v1core.Service{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: mdaiCR.Namespace}, svc); err != nil {
		t.Fatalf("failed to get Service %q: %v", serviceName, err)
	}
	expectedAppLabel := mdaiCR.Name + "-watcher-collector"
	if svc.Spec.Selector["app"] != expectedAppLabel {
		t.Errorf("expected service selector app to be %q, got: %q", expectedAppLabel, svc.Spec.Selector["app"])
	}
	if len(svc.Spec.Ports) != 2 {
		t.Errorf("expected service to have two ports, got %d", len(svc.Spec.Ports))
	} else {
		port := svc.Spec.Ports[0]
		if port.Name != "otlp-grpc" || port.Port != 4317 || port.TargetPort.String() != "otlp-grpc" {
			t.Errorf("unexpected service port configuration: %+v", port)
		}
	}
}

func TestEnsureStatusSetToDone(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()
	mdaiCR := newTestMdaiCR()
	fakeClient := newFakeClientForCR(mdaiCR, scheme)
	recorder := record.NewFakeRecorder(10)

	adapter := NewHubAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme, nil, time.Duration(30))

	opResult, err := adapter.ensureStatusSetToDone(ctx)
	if err != nil {
		t.Fatalf("ensureStatusSetToDone returned error: %v", err)
	}
	if opResult != ContinueOperationResult() {
		t.Errorf("expected ContinueOperationResult, got: %v", opResult)
	}

	updatedCR := &v1.MdaiHub{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-hub", Namespace: "default"}, updatedCR)
	if err != nil {
		t.Fatalf("failed to re-fetch mdaiCR: %v", err)
	}

	cond := meta.FindStatusCondition(updatedCR.Status.Conditions, typeAvailableHub)
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected condition %q to be True, got: %v", typeAvailableHub, cond.Status)
	}
	if cond.Reason != "Reconciling" {
		t.Errorf("expected reason 'Reconciling', got: %q", cond.Reason)
	}
	if cond.Message != "reconciled successfully" {
		t.Errorf("expected message 'reconciled successfully', got: %q", cond.Message)
	}
}

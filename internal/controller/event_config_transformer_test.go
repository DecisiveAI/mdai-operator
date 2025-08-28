package controller

import (
	"testing"

	"github.com/decisiveai/mdai-data-core/events/triggers"
	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestTransformWhenToTrigger_AlertTrigger(t *testing.T) {
	when := &mdaiv1.When{
		AlertName: ptr.To("some-alert-name"),
		Status:    ptr.To("firing"),
	}

	trigger, err := transformWhenToTrigger(when)
	require.NoError(t, err)
	require.NotNil(t, trigger)

	alertTrigger, ok := trigger.(*triggers.AlertTrigger)
	require.True(t, ok)

	assert.Equal(t, "some-alert-name", alertTrigger.Name)
	assert.Equal(t, "firing", alertTrigger.Status)
}

func TestTransformWhenToTrigger_AlertTrigger_EmptyStatus(t *testing.T) {
	when := &mdaiv1.When{
		AlertName: ptr.To("some-alert-name"),
		Status:    ptr.To(""), // Empty string status
	}

	trigger, err := transformWhenToTrigger(when)
	require.NoError(t, err)
	require.NotNil(t, trigger)

	alertTrigger, ok := trigger.(*triggers.AlertTrigger)
	require.True(t, ok)

	assert.Equal(t, "some-alert-name", alertTrigger.Name)
	assert.Empty(t, alertTrigger.Status)
}

func TestTransformWhenToTrigger_VariableTrigger(t *testing.T) {
	when := &mdaiv1.When{
		VariableUpdated: ptr.To("some-variable-name"),
		UpdateType:      ptr.To("any"),
	}

	trigger, err := transformWhenToTrigger(when)
	require.NoError(t, err)
	require.NotNil(t, trigger)

	variableTrigger, ok := trigger.(*triggers.VariableTrigger)
	require.True(t, ok)

	assert.Equal(t, "some-variable-name", variableTrigger.Name)
	assert.Equal(t, "any", variableTrigger.UpdateType)
}

func TestTransformWhenToTrigger_VariableTrigger_EmptyUpdateType(t *testing.T) {
	testCases := []struct {
		name       string
		updateType *string
	}{
		{
			name: "nil update type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			when := &mdaiv1.When{
				VariableUpdated: ptr.To("some-variable-name"),
				UpdateType:      tc.updateType,
			}

			trigger, err := transformWhenToTrigger(when)
			require.NoError(t, err)
			require.NotNil(t, trigger)

			variableTrigger, ok := trigger.(*triggers.VariableTrigger)
			require.True(t, ok)

			assert.Equal(t, "some-variable-name", variableTrigger.Name)
			assert.Empty(t, variableTrigger.UpdateType)
		})
	}
}

func TestTransformWhenToTrigger_NilInput(t *testing.T) {
	trigger, err := transformWhenToTrigger(nil)
	require.Error(t, err)
	assert.Nil(t, trigger)
	assert.EqualError(t, err, "when is required")
}

func TestTransformWhenToTrigger_BothAlertAndVariableSpecified(t *testing.T) {
	when := &mdaiv1.When{
		AlertName:       ptr.To("some-alert"),
		VariableUpdated: ptr.To("some-variable"),
	}

	trigger, err := transformWhenToTrigger(when)
	require.Error(t, err)
	assert.Nil(t, trigger)
	assert.EqualError(t, err, "when: specify exactly one of alertName or variableUpdated")
}

func TestTransformWhenToTrigger_NeitherAlertNorVariableSpecified(t *testing.T) {
	when := &mdaiv1.When{}

	trigger, err := transformWhenToTrigger(when)
	require.Error(t, err)
	assert.Nil(t, trigger)
	assert.EqualError(t, err, "when: specify either alertName or variableUpdated")
}

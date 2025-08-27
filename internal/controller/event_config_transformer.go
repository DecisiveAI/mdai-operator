package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/decisiveai/mdai-data-core/events"
	"github.com/decisiveai/mdai-data-core/events/triggers"
	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"k8s.io/utils/ptr"
)

func transformWhenToTrigger(when *mdaiv1.When) (triggers.Trigger, error) {
	if when == nil {
		return nil, errors.New("when is required")
	}

	alertName := strings.TrimSpace(ptr.Deref(when.AlertName, ""))
	status := strings.TrimSpace(ptr.Deref(when.Status, ""))
	varUpdated := strings.TrimSpace(ptr.Deref(when.VariableUpdated, ""))
	updateType := strings.TrimSpace(ptr.Deref(when.UpdateType, ""))

	hasAlert := alertName != ""
	hasVar := varUpdated != ""

	switch {
	case hasAlert && hasVar:
		return nil, errors.New("when: specify exactly one of alertName or variableUpdated")
	case hasAlert:
		return &triggers.AlertTrigger{
			Name:   alertName,
			Status: status,
		}, nil
	case hasVar:
		return &triggers.VariableTrigger{
			Name:       varUpdated,
			UpdateType: updateType,
		}, nil
	default:
		return nil, errors.New("when: specify either alertName or variableUpdated")
	}
}

func transformThenToCommands(actions []mdaiv1.Action) ([]events.Command, error) {
	cmds := make([]events.Command, 0, len(actions))
	for i, action := range actions {
		start := len(cmds)

		if action.AddToSet != nil {
			bytes, err := json.Marshal(action.AddToSet)
			if err != nil {
				return nil, fmt.Errorf("encode variable.set.add: %w", err)
			}
			cmds = append(cmds, events.Command{
				Type:   "variable.set.add",
				Inputs: bytes,
			})
		}
		if action.RemoveFromSet != nil {
			bytes, err := json.Marshal(action.RemoveFromSet)
			if err != nil {
				return nil, fmt.Errorf("encode variable.set.remove: %w", err)
			}
			cmds = append(cmds, events.Command{
				Type:   "variable.set.remove",
				Inputs: bytes,
			})
		}
		if action.CallWebhook != nil {
			bytes, err := json.Marshal(action.CallWebhook)
			if err != nil {
				return nil, fmt.Errorf("encode webhook.call: %w", err)
			}
			cmds = append(cmds, events.Command{
				Type:   "webhook.call",
				Inputs: bytes,
			})
		}

		if len(cmds) == start {
			return nil, fmt.Errorf("then[%d]: at least one action must be specified", i)
		}
	}
	return cmds, nil
}

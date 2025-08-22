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
			cmds = append(cmds, events.Command{
				Type: "variable.set.add",
				Inputs: map[string]any{
					"variableRef": strings.TrimSpace(action.AddToSet.Set),
					"valueFrom":   strings.TrimSpace(action.AddToSet.Value),
				},
			})
		}
		if action.RemoveFromSet != nil {
			cmds = append(cmds, events.Command{
				Type: "variable.set.remove",
				Inputs: map[string]any{
					"variableRef": strings.TrimSpace(action.RemoveFromSet.Set),
					"valueFrom":   strings.TrimSpace(action.RemoveFromSet.Value),
				},
			})
		}
		if action.CallWebhook != nil {
			method := strings.TrimSpace(action.CallWebhook.Method)
			if method == "" {
				method = "POST"
			}
			inputs := map[string]any{
				"url":    strings.TrimSpace(action.CallWebhook.URL),
				"method": method,
			}
			if body := decodeWebhookBody(action); body != nil {
				inputs["body"] = body
			}
			if len(action.CallWebhook.Headers) > 0 {
				inputs["headers"] = action.CallWebhook.Headers
			}
			cmds = append(cmds, events.Command{
				Type:   "webhook.call",
				Inputs: inputs,
			})
		}

		if len(cmds) == start {
			return nil, fmt.Errorf("then[%d]: at least one action must be specified", i)
		}
	}
	return cmds, nil
}

func decodeWebhookBody(action mdaiv1.Action) map[string]any {
	if action.CallWebhook == nil || action.CallWebhook.Body == nil || len(action.CallWebhook.Body.Raw) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(action.CallWebhook.Body.Raw, &obj); err == nil {
		return obj
	}
	return map[string]any{"raw": string(action.CallWebhook.Body.Raw)}
}

package controller

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/decisiveai/mdai-data-core/events"
	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"k8s.io/utils/ptr"
)

func transformWhenToTrigger(when *mdaiv1.When) (events.Trigger, error) {
	if when == nil {
		return events.Trigger{}, fmt.Errorf("when is required")
	}

	alertName := strings.TrimSpace(ptr.Deref(when.AlertName, ""))
	status := strings.TrimSpace(ptr.Deref(when.Status, ""))
	varUpdated := strings.TrimSpace(ptr.Deref(when.VariableUpdated, ""))
	cond := strings.TrimSpace(ptr.Deref(when.Condition, ""))

	hasAlert := alertName != ""
	hasVar := varUpdated != ""

	switch {
	case hasAlert && hasVar:
		return events.Trigger{}, fmt.Errorf("when: specify exactly one of alertName or variableUpdated")
	case hasAlert:
		subject := mapSubjectForWhen("alert") // e.g., "alert.*.*.alertmanager.*"
		parts := []string{fmt.Sprintf("trigger.data.alertname == %q", alertName)}
		if status != "" {
			parts = append(parts, fmt.Sprintf("trigger.data.status == %q", status))
		}
		if cond != "" {
			parts = append(parts, cond)
		}
		return events.Trigger{
			Subject:   subject,
			Condition: strings.Join(parts, " && "),
		}, nil
	case hasVar:
		subject := mapSubjectForWhen("variable") // e.g., "variable.*.*.mdaihub.*"
		parts := []string{fmt.Sprintf("trigger.data.variable == %q", varUpdated)}
		if cond != "" {
			parts = append(parts, cond)
		}
		return events.Trigger{
			Subject:   subject,
			Condition: strings.Join(parts, " && "),
		}, nil
	default:
		// Neither set: broad subject, pass-through condition (if any).
		return events.Trigger{
			Subject:   mapSubjectForWhen("any"),
			Condition: cond,
		}, nil
	}
}

func mapSubjectForWhen(kind string) string {
	switch kind {
	case "alert":
		return "alert.*.*.alertmanager.*" // FIXME this is a placeholder and needs to be updated based on actual subject mapping
	case "variable":
		return "variable.*.*.mdaihub.*"
	default:
		return ">"
	}
}

func transformThenToCommands(actions []mdaiv1.Action) ([]events.Command, error) {
	cmds := make([]events.Command, 0, len(actions))
	for i, action := range actions {
		start := len(cmds)

		if action.AddToSet != nil {
			cmds = append(cmds, events.Command{
				Type: "variable.set.add",
				Inputs: map[string]interface{}{
					"variableRef": strings.TrimSpace(action.AddToSet.Set),
					"valueFrom":   strings.TrimSpace(action.AddToSet.Value),
				},
			})
		}
		if action.RemoveFromSet != nil {
			cmds = append(cmds, events.Command{
				Type: "variable.set.remove",
				Inputs: map[string]interface{}{
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
			inputs := map[string]interface{}{
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

func decodeWebhookBody(action mdaiv1.Action) map[string]interface{} {
	if action.CallWebhook == nil || action.CallWebhook.Body == nil || len(action.CallWebhook.Body.Raw) == 0 {
		return nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(action.CallWebhook.Body.Raw, &obj); err == nil {
		return obj
	}
	return map[string]interface{}{"raw": string(action.CallWebhook.Body.Raw)}
}

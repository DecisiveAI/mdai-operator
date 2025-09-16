# Automation Rule ConfigMap

The MDAI Operator processes the `rules` defined in the `MdaiHub` custom resource and transforms them into a machine-readable format stored in a dedicated `ConfigMap`. This `ConfigMap` is then mounted and consumed by the eventing engine, which executes the automation logic.

## ConfigMap Naming and Structure

-   **Name**: The `ConfigMap` is automatically named using the pattern `<mdaihub-name>-automation`.
-   **Labels**: It is labeled with `mydecisive.ai/hub-name` and `mydecisive.ai/configmap-type: automation` for easy discovery.
-   **Data**: The `data` section of the `ConfigMap` contains a key-value pair for each rule defined in the `MdaiHub` spec.
    -   **Key**: The `name` of the `AutomationRule`.
    -   **Value**: A JSON string representing the compiled rule, including its trigger and commands.

## Rule JSON Structure

Each value in the `ConfigMap` is a JSON object with the following structure:

```json
{
  "name": "string",
  "trigger": {},
  "commands": []
}
```
`name`: The unique name of the rule.  
`trigger`: An object defining the condition that activates the rule. Its structure depends on the trigger type.  
`commands`: An array of action objects to be executed when the rule is triggered.  
### Trigger Object
The trigger object will have one of two forms:
#### 1. Alert Trigger (`triggers.AlertTrigger`)  
   Triggered by a Prometheus alert status change.  
#### 2. Variable Trigger (`triggers.VariableTrigger`)
   Triggered by a change to a managed variable.

### Commands Array
The commands array contains a list of actions to perform. Each action is an object with a type and an inputs payload.  
`type`: A string identifying the command type (e.g., var_set_add, webhook_call).  
`inputs`: A JSON object containing the parameters for the command. The structure of inputs corresponds to the action definition in the MdaiHub API (e.g., SetAction, CallWebhookAction).
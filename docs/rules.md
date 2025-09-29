# MDAI Automation Rules

MDAI Automation Rules define the logic for the MDAI operator to react to events and perform actions. These rules create a powerful automation loop, allowing you to dynamically manage your system's configuration based on its state. Rules are defined in the `rules` array within the `MdaiHub` spec.

Each rule is composed of two main parts:
1.  `when`: The trigger condition that activates the rule.
2.  `then`: A sequence of actions to execute when the rule is triggered.

```yaml
# In MdaiHub spec
rules:
  - when:
      # ... trigger definition
    then:
      # ... list of actions
```
## `when` - Trigger Conditions
The when block specifies the event that will trigger the rule's actions. A rule can be triggered by one of two types of events: a Prometheus alert or a variable update. You must specify exactly one of these.  
### Triggering on a Prometheus Alert
You can trigger a rule when a Prometheus alert changes its state.  
`alertName` (required): The name of the Prometheus alert to monitor.  
`status` (optional): The status of the alert that will trigger the rule. Common values are `firing` or `resolved`. If omitted, the rule will trigger for any status change.  
Example:
```yaml
when:
  alertName: HighRequestLatency
  status: firing
```
This rule will be triggered when the HighRequestLatency alert starts firing.

### Triggering on a Variable Update
You can trigger a rule when a variable's value is modified.  
`variableUpdated` (required): The key of the variable to monitor for changes.  
`updateType` (optional): The specific type of change to listen for. This is particularly useful for collection types like set or map. If omitted, any change will trigger the rule.  
Example:  
```yaml
when:
  variableUpdated: ACTIVE_USERS
  updateType: add # e.g., for a 'set' variable
```
This rule will be triggered whenever a new item is added to the ACTIVE_USERS set.
## `then` - Actions
The then block is an array of actions to be executed sequentially when the `when` condition is met. These actions are used to modify computed variables or call external webhooks.   
On failure of any action in the sequence, all following actions are skipped.
### Modifying Variables
These actions update the values of computed variables stored in Valkey.
#### setVariable
Sets or updates the value of a scalar variable (`string`, `int`, `boolean`).  
`key`: The name of the variable to update.  
`value`: The new value for the variable. Value could use interpolation. 
```yaml
then:
  - setVariable:
      key: SAMPLING_RATE
      value: "100"
```
#### addToSet
Adds a new item to a set variable.  
`set`: The name of the set variable.  
`value`: The string value to add to the set. Value could use interpolation.
```yaml
then:
  - addToSet:
      set: DENY_LIST_IPS
      value: "192.168.1.100"
```
#### removeFromSet
Removes an item from a set variable.  
`set`: The name of the set variable.  
`value`: The string value to remove from the set.  
```yaml
then:
  - removeFromSet:
      set: DENY_LIST_IPS
      value: "192.168.1.100"
```
#### addToMap
Adds or updates a key-value pair in a map variable.  
`map`: The name of the map variable.  
`key`: The key within the map to add or update.  
`value`: The value to set for the key. Value could use interpolation.
```yaml
then:
  - addToMap:
      map: SEVERITY_LEVEL
      key: my-service
      value: "warn"
```
#### removeFromMap
Removes a key-value pair from a map variable.  
`map`: The name of the map variable.  
`key`: The key within the map to remove.  
```yaml
then:
  - removeFromMap:
      map: SEVERITY_LEVEL
      key: my-service
```
### Calling Webhooks
#### callWebhook
Makes an HTTP request to an external endpoint. This is useful for sending notifications or triggering external workflows.    
`url` or `urlFrom`: The webhook URL. Can be provided as a direct string (url) or sourced from a Secret/ConfigMap (urlFrom).  
`method`: The HTTP method. Defaults to POST. Allowed values: POST, PUT, PATCH.  
`headers`: A map of custom HTTP headers to include in the request.  
`templateRef`: A reference to a built-in payload template. Rght now limited to `slackAlertTemplate`.  
`templateValues`: A map of key-value pairs to populate the specified template.  
`timeout`: The request timeout. Defaults to 0s (no timeout).  
```yaml
      then:
        - callWebhook:
            url:
              valueFrom:
                secretKeyRef: { name: slack-webhook-secret, key: url }
            templateRef: slackAlertTemplate
            templateValues:
              labels_val_ref_primary: mdai_service
              message: Service was >2x expected error rate for five minutes compared to the last hour!
              link_text: See alert in Prometheus
              link_url: http://localhost:9090/alerts
```

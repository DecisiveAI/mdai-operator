# Testing Mdai Operator
✅ - automated
## Startup
### Use case 1: valkey connection
On a startup operator manager will wait till Valkey connection is available.

## Finalization
### Use case 1: hub deleted
When hub is deleted:  
✅ config maps created for OTEL collector's left untouched  
✅ prometheus operator rules are deleted
- Valkey keys `variable/hub_name/*` deleted
- all observers (watchers) related to the hub are uninstalled

## Hub Status
hub status and conditions should reflect successful reconciliation or error state.

## Variables
### Use case 1: new OTEL collector
When new OTEL collector created and has a reference to the hub:  
✅ a new reconcile loop is triggered.   

If hub has a variable defined:  
✅ a config map with variable is created,  
✅ deployed to the same namespace as OTEL collector and 
- a collector restart is not triggered since the config map hash didn't change.

### Use case 2: variable updated in Valkey
When variable defined at hub's CR is updated in Valkey:  
✅ a valkey notification channel triggers a new reconcile loop    
✅ all relevant config maps with variables updated
✅  all relevant OTEL collectors restarted
### Use case 3: variable deleted
When variable is deleted from CR:
- a key `variable/hub_name/key_name`  deleted from Valkey
## Evaluations
TODO
## Observers
TODO
## Audit
TODO

## Variables
```
SET variable/mdaihub-second/any_service_alerted true
SADD variable/mdaihub-second/service_list serviceA
HSET variable/mdaihub-second/attribute_map send_batch_size 100 timeout 15s

SET variable/mdaihub-second/default "default"
SET  variable/mdaihub-second/filter "- severity_number < SEVERITY_NUMBER_WARN \n- IsMatch(resource.attributes[\"service.name\"], \"${env:SERVICE_LIST}\")"

SET variable/mdaihub-second/severity_number 1
HSET variable/mdaihub-second/severity_filters_by_level 1 "INFO|WARNING" 2 "INFO"
```
- check otel/mdaihub-second-variables config map

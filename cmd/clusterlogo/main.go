/*
	Logic to add

1. Check API server logs to see if I can figure out when actions were last done by different entities (potentially even cloud provider logs?)
2. Add DB logic

Start with just SAs - very beta

- I only care about permissions. Therefore, I need to go through all the CRB and RBs and extract the entities and all of their permissions. Go through based on subjects.kind (in yaml/json)
- Need to check re cloud providers - will this get all the entities or is there more mapping with things like annotations?

- Flattened - verbs that only work for specific resources need to be documented
- Flatten everything to subresources (maybe during the "over-permissive" check actions over every subresource and the parent resoource) - e.g. given get on pods when only need get on pods/status
- Add the rolebindings check - if type clusterrole.... if type role..... - everything is to be namespaced



- Talk name:
Ready? Set... ClusterLoGo! Exploring Least Privileged Through Kubernetes Logs

*/

package main

import (
	"fmt"
)

func main() {
	//AuthMain()
	kube_issues := ConnectAndCollect()
	for key, value := range kube_issues {
		fmt.Printf("The following issues were found:\n%s: %s\n", key, value)
	}
}

/* Self-Managed
Can only currently be run on the api server node (kind of)
Check the --audit-policy-file and --audit-log-path from the api server manifest
--audit-policy-file for log events and levels, --audit-log-path for the log file location


if webhook configured to ingest logs:
Check --audit-webhook-config-file for the webhook configuration (basically specialized kubeconfig)

potentially use last-applied for write permissions? -- https://kubernetes.io/docs/reference/kubectl/generated/kubectl_apply/kubectl_apply_view-last-applied/

Log structure:
https://kubernetes.io/docs/reference/config-api/apiserver-audit.v1/#audit-k8s-io-v1-Event
https://github.com/falcosecurity/plugins/tree/main/plugins/k8saudit
*/

/* GKE
GKE Audit logs - stored in a dedicated datastore in cloud logging
https://cloud.google.com/kubernetes-engine/docs/how-to/audit-logging
https://cloud.google.com/kubernetes-engine/docs/how-to/view-logs


https://cloud.google.com/logging/docs/reference/v2/rest/v2/entries/list
https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry


Query example
log_id("cloudaudit.googleapis.com/activity")
resource.type="k8s_cluster"
resource.labels.cluster_name="CLUSTER_NAME"
resource.labels.project_id="PROJECT_ID"
resource.labels.location="REGION"
protoPayload.request.metadata.name="WORKLOAD_NAME"

*/

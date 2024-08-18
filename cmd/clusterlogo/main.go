/*
- Need to check re cloud providers - will this get all the entities or is there more mapping with things like annotations?

- Flattened - the verbs that only work for specific resources needs to be documented
- Flatten everything to subresources (maybe during the "over-permissive" check actions over every subresource and the parent resource - heirarchichally - if get on pods, check last pod get, if not then check subresources) - e.g. given get on pods when only need get on pods/status
- Docs - write what it accounts for - stuff like resourceNames, all permissions are individually handled (some may appear twice for the same source if given by different granters). Permissions given specifically to subresources are shown as such in the db
- Docs - inclusterconfig as default, fallback to kubeconfig


- Check how to deploy - can be as a container? - maybe have a "mode" for kube-collection (can send to db outside cluster) and "mode" for log collection/parsing (so can do separately) - CHECK WHAT HAPPENS IF CREDS FILE IS NOT ~/.aws/credentials
- Check re aggregated roles - maybe add a boolean column if yay/nay? (https://pkg.go.dev/k8s.io/api/rbac/v1#ClusterRole)
- Add check for stale resource? roles/clusterroles with no bindings?
- Remove groups (?)

- Talk name:
Ready? Set... ClusterLoGo! Exploring Least Privileged Through Kubernetes Logs

*/

package main

func main() {
	KubeCollect()
	AuthMain()
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
